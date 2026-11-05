package main

import (
	"strings"
	"testing"

	"career-tool-backend/applicationlog"
	"career-tool-backend/jobboard"
)

func TestTheBoardCardsShowTheProtocolSummaryOfThisProfileOnly(t *testing.T) {
	p1, p2 := boardFixture(t)
	mustSet(t, p1, "j1", jobboard.AppliedID)
	mustSet(t, p1, "j2", jobboard.NotReviewedID)
	mustSet(t, p2, "j1", jobboard.AppliedID)
	for _, in := range []applicationlog.Input{
		{Direction: "sent", Channel: "email", OccurredAt: "2025-01-10T09:00:00Z"},
		{Direction: "received", Channel: "phone", OccurredAt: "2025-01-12"},
		{Direction: "note", Channel: "video", OccurredAt: "2099-01-01T10:00:00Z"},
	} {
		if _, err := applicationlog.Add(p1, "j1", in); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := applicationlog.Add(p2, "j1", applicationlog.Input{Direction: "sent", Channel: "sms", OccurredAt: "2025-02-02"}); err != nil {
		t.Fatal(err)
	}

	got := cardOf(t, boardOf(t, p1, ""), "j1").Protocol
	want := applicationlog.Summary{Count: 3, Planned: 1, LastAt: "2025-01-12", LastDirection: "received"}
	if got == nil || *got != want {
		t.Errorf("profile 1 j1: got %+v want %+v", got, want)
	}
	if c := cardOf(t, boardOf(t, p1, ""), "j2"); c.Protocol != nil {
		t.Errorf("a job without entries has a protocol: %+v", c.Protocol)
	}
	got2 := cardOf(t, boardOf(t, p2, ""), "j1").Protocol
	if got2 == nil || got2.Count != 1 || got2.LastDirection != "sent" {
		t.Errorf("profile 2 j1: %+v", got2)
	}
}

func TestACardWithoutAProtocolHasNoProtocolKeyAtAll(t *testing.T) {
	p1, _ := boardFixture(t)
	mustSet(t, p1, "j1", jobboard.NotReviewedID)
	rec := boardCall(t, jobBoardHandler, "GET", "/job-board?profileId="+p1, "")
	wantStatus(t, rec, 200)
	if strings.Contains(rec.Body.String(), `"protocol"`) {
		t.Errorf("the board JSON mentions a protocol: %s", rec.Body.String())
	}
}

func TestTheBoardSortsAndKeepsEverythingElseWithAProtocol(t *testing.T) {
	p1, _ := boardFixture(t)
	mustSet(t, p1, "j1", jobboard.NotReviewedID)
	if _, err := applicationlog.Add(p1, "j1", applicationlog.Input{Direction: "sent", Channel: "email", OccurredAt: "2025-01-10"}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"&sort=match", "&sort=moved"} {
		if c := cardOf(t, boardOf(t, p1, q), "j1"); c.Protocol == nil || c.Protocol.Count != 1 {
			t.Errorf("%s: %+v", q, c.Protocol)
		}
	}
}
