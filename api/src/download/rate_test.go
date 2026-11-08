package download

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

func at(sec float64) time.Time { return t0.Add(time.Duration(sec * float64(time.Second))) }

func TestNoSpeedIsReportedBeforeTheSampleSpanIsLongEnough(t *testing.T) {
	r := newRate()
	if r.perSecond(at(0)) != 0 {
		t.Error("no samples: speed must be 0")
	}
	r.add(at(0), 0)
	if r.perSecond(at(0)) != 0 {
		t.Error("one sample: speed must be 0")
	}
	r.add(at(0.5), 500)
	if got := r.perSecond(at(0.5)); got != 0 {
		t.Errorf("under a second of data: speed = %v, want 0 (not a noisy first guess)", got)
	}
	r.add(at(1), 1000)
	if got := r.perSecond(at(1)); got != 1000 {
		t.Errorf("after a second: speed = %v, want 1000", got)
	}
}

func TestSpeedFollowsTheLastFewSecondsNotTheWholeTransfer(t *testing.T) {
	r := newRate()
	// 1000 B/s for 6 seconds, then 100 B/s for 6 seconds.
	cum := int64(0)
	for s := 0; s <= 6; s++ {
		r.add(at(float64(s)), cum)
		cum += 1000
	}
	cum -= 1000
	for s := 7; s <= 12; s++ {
		cum += 100
		r.add(at(float64(s)), cum)
	}
	got := r.perSecond(at(12))
	if got < 90 || got > 250 {
		t.Errorf("speed = %v, want about 100-200 (the recent rate), not the 550 average", got)
	}
}

func TestAStallBringsTheSpeedBackToZero(t *testing.T) {
	r := newRate()
	for s := 0; s <= 3; s++ {
		r.add(at(float64(s)), int64(s)*1000)
	}
	if r.perSecond(at(3)) == 0 {
		t.Fatal("speed must be known while data flows")
	}
	if got := r.perSecond(at(9)); got != 0 {
		t.Errorf("no byte for longer than the window: speed = %v, want 0", got)
	}
}

func TestSamplesCloserThanTheStepAreMergedAndOldOnesDropped(t *testing.T) {
	r := newRate()
	for i := 0; i < 1000; i++ {
		r.add(at(float64(i)*0.001), int64(i)*10) // a thousand updates in one second
	}
	if len(r.samples) > 12 {
		t.Errorf("%d samples kept for one second of updates, want about 10", len(r.samples))
	}
	for s := 2; s < 100; s++ {
		r.add(at(float64(s)), int64(s)*1000)
	}
	if len(r.samples) > 8 {
		t.Errorf("%d samples kept, want only the last window", len(r.samples))
	}
}

func TestNoSpeedWhenNothingMoved(t *testing.T) {
	r := newRate()
	r.add(at(0), 500)
	r.add(at(2), 500)
	if got := r.perSecond(at(2)); got != 0 {
		t.Errorf("speed = %v, want 0 when no byte arrived", got)
	}
}

func TestParseContentRange(t *testing.T) {
	good := map[string][2]int64{"bytes 5-9/10": {5, 10}, "bytes 0-99/100": {0, 100}, " bytes 7-9/*": {7, -1}}
	for in, want := range good {
		start, total, ok := parseContentRange(in)
		if !ok || start != want[0] || total != want[1] {
			t.Errorf("%q: %d %d %v, want %v", in, start, total, ok, want)
		}
	}
	for _, in := range []string{"", "5-9/10", "bytes 5-9", "bytes x-9/10", "bytes -5/10", "bytes 5-9/0", "bytes 5-9/x", "items 5-9/10"} {
		if _, _, ok := parseContentRange(in); ok {
			t.Errorf("%q must not parse", in)
		}
	}
}
