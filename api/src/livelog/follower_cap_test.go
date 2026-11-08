package livelog

import (
	"fmt"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func TestTheCapStopsTheSixthLogAndNeverCountsTheSameLogTwice(t *testing.T) {
	c := NewFollowerCap(5, 15*time.Second)
	for i := 0; i < 5; i++ {
		if !c.Acquire("ada", fmt.Sprintf("log-%d", i), t0) {
			t.Fatalf("log %d refused", i)
		}
	}
	if c.Acquire("ada", "log-5", t0) {
		t.Fatal("the sixth log was accepted")
	}
	for i := 0; i < 20; i++ { // the same five, polled again and again, are never refused
		if !c.Acquire("ada", fmt.Sprintf("log-%d", i%5), t0.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("a log already followed was refused on poll %d", i)
		}
	}
	if got := c.Active("ada", t0.Add(20*time.Second)); got != 5 {
		t.Errorf("active = %d", got)
	}
}

func TestAPlaceIsFreeAgainWhenTheLogIsNotPolledForTheWindow(t *testing.T) {
	c := NewFollowerCap(2, 10*time.Second)
	c.Acquire("ada", "a", t0)
	c.Acquire("ada", "b", t0.Add(9*time.Second))
	if c.Acquire("ada", "c", t0.Add(9*time.Second)) {
		t.Fatal("full")
	}
	// "a" was last polled 10 s ago: it no longer counts (a poll exactly at the window's end is free)
	if !c.Acquire("ada", "c", t0.Add(10*time.Second)) {
		t.Fatal("a place did not free up after the window")
	}
	if got := c.Active("ada", t0.Add(10*time.Second)); got != 2 {
		t.Errorf("active = %d", got)
	}
	if c.Acquire("ada", "a", t0.Add(10*time.Second)) { // "a" is no longer followed: it needs a place again, and none is free
		t.Error("a expired log must need a place again")
	}
}

func TestARefusalRecordsNothingAndPeopleAreIndependent(t *testing.T) {
	c := NewFollowerCap(1, time.Minute)
	c.Acquire("ada", "a", t0)
	for i := 0; i < 5; i++ {
		if c.Acquire("ada", "b", t0) {
			t.Fatal("accepted")
		}
	}
	if got := c.Active("ada", t0); got != 1 {
		t.Errorf("a refused log was recorded: %d", got)
	}
	if !c.Acquire("grace", "a", t0) || !c.Acquire("", "a", t0) {
		t.Error("another person (or an empty user id) has a cap of its own")
	}
}

func TestDefaultsApplyForZeroValues(t *testing.T) {
	c := NewFollowerCap(0, 0)
	for i := 0; i < DefaultMaxFollowers; i++ {
		if !c.Acquire("u", fmt.Sprint(i), t0) {
			t.Fatalf("refused at %d", i)
		}
	}
	if c.Acquire("u", "x", t0) || c.Acquire("u", "x", t0.Add(DefaultFollowWindow-time.Second)) {
		t.Error("the default cap is five for the default window")
	}
	if !c.Acquire("u", "x", t0.Add(DefaultFollowWindow)) {
		t.Error("and a place frees up after it")
	}
}

func TestPeopleWhoStoppedDoNotStayInMemory(t *testing.T) {
	c := NewFollowerCap(5, time.Second)
	for i := 0; i < 3*sweepAbove; i++ {
		c.Acquire(fmt.Sprintf("user-%d", i), "log", t0)
	}
	c.Acquire("late", "log", t0.Add(time.Hour)) // everyone else stopped long ago
	c.mu.Lock()
	n := len(c.users)
	c.mu.Unlock()
	if n > 2 {
		t.Errorf("%d people kept in memory after they all stopped", n)
	}
}

func TestACapIsSafeForConcurrentUse(t *testing.T) {
	c := NewFollowerCap(5, time.Minute)
	done := make(chan struct{})
	for g := 0; g < 16; g++ {
		go func(g int) {
			for i := 0; i < 500; i++ {
				c.Acquire(fmt.Sprint(g%4), fmt.Sprint(i%7), t0.Add(time.Duration(i)*time.Millisecond))
				c.Active(fmt.Sprint(g%4), t0)
			}
			done <- struct{}{}
		}(g)
	}
	for g := 0; g < 16; g++ {
		<-done
	}
	for u := 0; u < 4; u++ {
		if n := c.Active(fmt.Sprint(u), t0.Add(time.Second)); n > 5 {
			t.Errorf("user %d follows %d logs with a cap of 5", u, n)
		}
	}
}
