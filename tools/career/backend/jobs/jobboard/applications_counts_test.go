package jobboard

import "testing"

func st(kind string) *Status { return &Status{ID: kind, Name: kind, Kind: kind} }

// Plan 265: Applied are the applications on the board in an Applied status, Waiting the ones the other side owes a reply to; one can be both.
func TestCountsOfAppliedAndWaiting(t *testing.T) {
	apps := []Application{
		{JobID: "a", Status: st(KindApplied), WaitingForReply: true},
		{JobID: "b", Status: st(KindApplied)},
		{JobID: "c", Status: st(KindDeclined), WaitingForReply: true},
		{JobID: "d", Status: st(KindDeclined)},
		{JobID: "e", WaitingForReply: true}, // not on the board: only a protocol entry
		{JobID: "f", Status: st(KindCustom)},
		{JobID: "g", Status: st(KindStopped)},
		{JobID: "h", Status: st(KindNotReviewed)},
	}
	got := CountsOf("p1", apps)
	if got != (ProfileCounts{ProfileID: "p1", Total: 8, Applied: 2, Waiting: 3}) {
		t.Errorf("%+v", got)
	}
}

func TestCountsOfNothingIsZeros(t *testing.T) {
	for _, apps := range [][]Application{nil, {}} {
		if got := CountsOf("p", apps); got != (ProfileCounts{ProfileID: "p"}) {
			t.Errorf("%+v", got)
		}
	}
}
