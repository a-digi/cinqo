package applicationlog

import (
	"reflect"
	"testing"
)

func TestSummariesCountPlannedAndLast(t *testing.T) {
	setup(t) // the clock stands at 2026-01-01 00:00:xx UTC
	mustAdd(t, profA, job1, in("sent", "email", "2025-12-01T09:00:00Z"))
	mustAdd(t, profA, job1, in("received", "phone", "2025-12-20"))       // a day: the newest that has happened
	mustAdd(t, profA, job1, in("note", "video", "2030-01-01T10:00:00Z")) // planned
	mustAdd(t, profA, job1, in("note", "in_person", "2031-05-05"))       // planned, a day
	mustAdd(t, profA, job2, in("sent", "email", "2025-06-01T08:15:00Z"))
	mustAdd(t, profB, job1, in("received", "sms", "2025-12-30")) // another profile's protocol

	got, err := Summaries(profA)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Summary{
		job1: {Count: 4, Planned: 2, LastAt: "2025-12-20", LastDirection: "received"},
		job2: {Count: 1, Planned: 0, LastAt: "2025-06-01T08:15:00Z", LastDirection: "sent"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	other, err := Summaries(profB)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(other, map[string]Summary{job1: {Count: 1, LastAt: "2025-12-30", LastDirection: "received"}}) {
		t.Errorf("profile B: %+v", other)
	}
}

func TestSummariesOnlyPlannedHasNoLast(t *testing.T) {
	setup(t)
	mustAdd(t, profA, job1, in("note", "video", "2030-01-01T10:00:00Z"))
	got, err := Summaries(profA)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Summary{Count: 1, Planned: 1}); got[job1] != want {
		t.Errorf("got %+v want %+v", got[job1], want)
	}
}

func TestSummariesLastIsByDateNotByWritingOrder(t *testing.T) {
	setup(t)
	mustAdd(t, profA, job1, in("received", "email", "2025-12-10")) // written first, happened last
	mustAdd(t, profA, job1, in("sent", "email", "2025-12-01"))
	got, _ := Summaries(profA)
	if got[job1].LastAt != "2025-12-10" || got[job1].LastDirection != "received" {
		t.Errorf("got %+v", got[job1])
	}
}

func TestSummariesTodayHasHappenedAndNothingMeansEmptyMap(t *testing.T) {
	setup(t)
	got, err := Summaries(profA)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty: %v %v", got, err)
	}
	mustAdd(t, profA, job1, in("sent", "email", "2026-01-01")) // today (the test clock), a day: stored at midnight
	got, _ = Summaries(profA)
	if got[job1].Planned != 0 || got[job1].LastAt != "2026-01-01" {
		t.Errorf("today: %+v", got[job1])
	}
}
