package portal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
)

func setupAttemptsDB(t *testing.T, linkIDs ...string) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatalf("InitDatabases: %v", err)
	}
	t.Cleanup(func() {
		db.JobsDB.Close()
		db.CareerDB.Close()
	})
	if _, err := db.JobsDB.Exec(`INSERT INTO portals (id, name) VALUES ('p1', 'P')`); err != nil {
		t.Fatal(err)
	}
	for _, id := range linkIDs {
		if _, err := db.JobsDB.Exec(`INSERT INTO portal_links (id, portal_id, url) VALUES (?, 'p1', ?)`, id, "https://x/"+id); err != nil {
			t.Fatal(err)
		}
	}
}

func mustStart(t *testing.T, link, kind string) string {
	t.Helper()
	id, started, err := StartInstructionAttempt(link, kind, InstructionTriggerManual, "conv")
	if err != nil || !started {
		t.Fatalf("start: id=%q started=%v err=%v", id, started, err)
	}
	return id
}

func TestStartIsIdempotentAndValidates(t *testing.T) {
	setupAttemptsDB(t, "l1")

	id := mustStart(t, "l1", InstructionKindListing)
	again, started, err := StartInstructionAttempt("l1", InstructionKindListing, InstructionTriggerAuto, "")
	if err != nil || started || again != id {
		t.Fatalf("second start: id=%q started=%v err=%v, want %q false nil", again, started, err, id)
	}
	// The other kind is independent.
	mustStart(t, "l1", InstructionKindJobDetail)

	if _, _, err := StartInstructionAttempt("l1", "bogus", InstructionTriggerManual, ""); !errors.Is(err, ErrInvalidInstructionKind) {
		t.Errorf("bad kind: %v", err)
	}
	if _, _, err := StartInstructionAttempt("l1", InstructionKindListing, "robot", ""); !errors.Is(err, ErrInvalidInstructionTrigger) {
		t.Errorf("bad trigger: %v", err)
	}
	if _, _, err := StartInstructionAttempt("nope", InstructionKindListing, InstructionTriggerManual, ""); !errors.Is(err, ErrUnknownPortalLink) {
		t.Errorf("unknown link: %v", err)
	}
}

func TestFinishWithoutSaveFails(t *testing.T) {
	setupAttemptsDB(t, "l1")
	mustStart(t, "l1", InstructionKindListing)

	status, err := FinishInstructionAttempt("l1", InstructionKindListing, "")
	if err != nil || status != InstructionAttemptFailed {
		t.Fatalf("got %q, %v", status, err)
	}
	var msg string
	if err := db.JobsDB.QueryRow(`SELECT error_message FROM instruction_attempts`).Scan(&msg); err != nil || msg != defaultInstructionAttemptError {
		t.Fatalf("default message: %q, %v", msg, err)
	}

	// Nothing running any more: a second finish is a no-op.
	if status, err := FinishInstructionAttempt("l1", InstructionKindListing, "x"); err != nil || status != InstructionAttemptNone {
		t.Fatalf("second finish: %q, %v", status, err)
	}
}

func TestFinishKeepsClientErrorAndCapsIt(t *testing.T) {
	setupAttemptsDB(t, "l1")
	mustStart(t, "l1", InstructionKindListing)
	if _, err := FinishInstructionAttempt("l1", InstructionKindListing, strings.Repeat("é", 5000)); err != nil {
		t.Fatal(err)
	}
	var msg string
	if err := db.JobsDB.QueryRow(`SELECT error_message FROM instruction_attempts`).Scan(&msg); err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(msg)); n != maxInstructionAttemptTextLength {
		t.Fatalf("error not capped: %d runes", n)
	}
}

func TestFinishAfterSaveSucceeds(t *testing.T) {
	setupAttemptsDB(t, "l1")
	mustStart(t, "l1", InstructionKindListing)
	if err := MarkInstructionSaved("l1", InstructionKindListing); err != nil {
		t.Fatal(err)
	}
	// A client-reported error cannot turn a real save into a failure.
	status, err := FinishInstructionAttempt("l1", InstructionKindListing, "turn error")
	if err != nil || status != InstructionAttemptSucceeded {
		t.Fatalf("got %q, %v", status, err)
	}
	var errMsg *string
	if err := db.JobsDB.QueryRow(`SELECT error_message FROM instruction_attempts`).Scan(&errMsg); err != nil || errMsg != nil {
		t.Fatalf("succeeded attempt must have no error: %v %v", errMsg, err)
	}
}

func TestMarkSavedScopedToKindAndRunning(t *testing.T) {
	setupAttemptsDB(t, "l1")

	// No running attempt: harmless no-op.
	if err := MarkInstructionSaved("l1", InstructionKindListing); err != nil {
		t.Fatalf("no-op mark: %v", err)
	}

	mustStart(t, "l1", InstructionKindListing)
	// Saving the OTHER kind must not mark the listing attempt.
	if err := MarkInstructionSaved("l1", InstructionKindJobDetail); err != nil {
		t.Fatal(err)
	}
	if status, _ := FinishInstructionAttempt("l1", InstructionKindListing, ""); status != InstructionAttemptFailed {
		t.Fatalf("other kind's save must not count, got %q", status)
	}
	// A finished attempt is never marked retroactively.
	if err := MarkInstructionSaved("l1", InstructionKindListing); err != nil {
		t.Fatal(err)
	}
	var saved *string
	if err := db.JobsDB.QueryRow(`SELECT saved_at FROM instruction_attempts`).Scan(&saved); err != nil || saved != nil {
		t.Fatalf("finished attempt must stay unmarked: %v %v", saved, err)
	}
	if err := MarkInstructionSaved("l1", "bogus"); !errors.Is(err, ErrInvalidInstructionKind) {
		t.Fatalf("bad kind: %v", err)
	}
}

func TestInstructionStatsByLink(t *testing.T) {
	setupAttemptsDB(t, "l1", "l2", "l3")

	// l1 listing: fail, fail (latest error wins), succeed; job_detail: running.
	mustStart(t, "l1", InstructionKindListing)
	FinishInstructionAttempt("l1", InstructionKindListing, "first")
	mustStart(t, "l1", InstructionKindListing)
	FinishInstructionAttempt("l1", InstructionKindListing, "second")
	mustStart(t, "l1", InstructionKindListing)
	MarkInstructionSaved("l1", InstructionKindListing)
	FinishInstructionAttempt("l1", InstructionKindListing, "")
	mustStart(t, "l1", InstructionKindJobDetail)

	// l2: one failure only. l3: no attempts at all.
	mustStart(t, "l2", InstructionKindListing)
	FinishInstructionAttempt("l2", InstructionKindListing, "boom")

	stats, err := InstructionStatsByLink()
	if err != nil {
		t.Fatal(err)
	}

	l1 := stats["l1"]
	if l1.Listing.Attempts != 3 || l1.Listing.Succeeded != 1 || l1.Listing.Failed != 2 || l1.Listing.Running {
		t.Errorf("l1 listing: %+v", l1.Listing)
	}
	if l1.Listing.LastError != "second" || l1.Listing.LastFailureAt == "" {
		t.Errorf("l1 last failure: %+v", l1.Listing)
	}
	if l1.JobDetail.Attempts != 0 || !l1.JobDetail.Running {
		t.Errorf("a running attempt is reported but not counted: %+v", l1.JobDetail)
	}
	if l2 := stats["l2"]; l2.Listing.Failed != 1 || l2.Listing.LastError != "boom" || l2.JobDetail.Attempts != 0 {
		t.Errorf("l2: %+v", l2)
	}
	if _, present := stats["l3"]; present {
		t.Errorf("link with no attempts must be absent")
	}
}

const (
	validListingYAML = "fields:\n  - label: title\n    selector: h2\n  - label: url\n    selector: a\n    attribute: href\npagination:\n  nextSelector: a.next\n  maxPages: 3\n"
	validDetailYAML  = "fields:\n  - label: description\n    selector: .body\n"
)

// callSaveTool runs one of the real AI-facing save tools through an
// in-memory MCP session, the same path an AI call takes.
func callSaveTool(t *testing.T, register func(*mcp.Server), tool string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	register(server)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func savedAtSet(t *testing.T, link, kind string) bool {
	t.Helper()
	var saved *string
	if err := db.JobsDB.QueryRow(`SELECT saved_at FROM instruction_attempts WHERE portal_link_id = ? AND kind = ?`, link, kind).Scan(&saved); err != nil {
		t.Fatal(err)
	}
	return saved != nil
}

func TestSaveToolsMarkTheRunningAttempt(t *testing.T) {
	setupAttemptsDB(t, "l1")
	mustStart(t, "l1", InstructionKindListing)
	mustStart(t, "l1", InstructionKindJobDetail)

	res := callSaveTool(t, RegisterSetPortalLinkCrawlInstructions, "set_portal_link_crawl_instructions",
		map[string]any{"portalLinkId": "l1", "instructions": validListingYAML})
	if res.IsError {
		t.Fatalf("listing save failed: %+v", res.Content)
	}
	if !savedAtSet(t, "l1", InstructionKindListing) {
		t.Error("listing save must mark the running listing attempt")
	}
	if savedAtSet(t, "l1", InstructionKindJobDetail) {
		t.Error("listing save must not mark the job-detail attempt")
	}

	res = callSaveTool(t, RegisterSetPortalLinkJobDetailCrawlInstructions, "set_portal_link_job_detail_crawl_instructions",
		map[string]any{"portalLinkId": "l1", "instructions": validDetailYAML})
	if res.IsError {
		t.Fatalf("job-detail save failed: %+v", res.Content)
	}
	if !savedAtSet(t, "l1", InstructionKindJobDetail) {
		t.Error("job-detail save must mark the running job-detail attempt")
	}
}

func TestRejectedSaveDoesNotMark(t *testing.T) {
	setupAttemptsDB(t, "l1")
	mustStart(t, "l1", InstructionKindListing)

	res := callSaveTool(t, RegisterSetPortalLinkCrawlInstructions, "set_portal_link_crawl_instructions",
		map[string]any{"portalLinkId": "l1", "instructions": "not: [valid"})
	if !res.IsError {
		t.Fatal("invalid instructions must be rejected")
	}
	if savedAtSet(t, "l1", InstructionKindListing) {
		t.Error("a rejected save must not mark the attempt")
	}
}

func TestManualEditDoesNotMark(t *testing.T) {
	setupAttemptsDB(t, "l1")
	mustStart(t, "l1", InstructionKindListing)

	// The shared function the human PUT /portal-links path uses.
	if err := UpdatePortalLinkCrawlInstructions("l1", validListingYAML); err != nil {
		t.Fatal(err)
	}
	if savedAtSet(t, "l1", InstructionKindListing) {
		t.Error("a manual edit is not an AI attempt and must not mark it")
	}
}

func TestSaveWithoutRunningAttemptIsHarmless(t *testing.T) {
	setupAttemptsDB(t, "l1")
	res := callSaveTool(t, RegisterSetPortalLinkCrawlInstructions, "set_portal_link_crawl_instructions",
		map[string]any{"portalLinkId": "l1", "instructions": validListingYAML})
	if res.IsError {
		t.Fatalf("save without a tracked attempt must still succeed: %+v", res.Content)
	}
}

func TestListPortalsIncludesInstructionStats(t *testing.T) {
	setupAttemptsDB(t, "l1", "l2")

	mustStart(t, "l1", InstructionKindListing)
	FinishInstructionAttempt("l1", InstructionKindListing, "boom")
	mustStart(t, "l1", InstructionKindListing)
	MarkInstructionSaved("l1", InstructionKindListing)
	FinishInstructionAttempt("l1", InstructionKindListing, "")
	mustStart(t, "l1", InstructionKindJobDetail) // still running

	portals, err := ListPortals()
	if err != nil {
		t.Fatal(err)
	}
	if len(portals) != 1 || len(portals[0].Links) != 2 {
		t.Fatalf("unexpected portals: %+v", portals)
	}
	byID := map[string]portalLink{}
	for _, l := range portals[0].Links {
		byID[l.ID] = l
	}

	l1 := byID["l1"].InstructionStats
	if l1.Listing.Attempts != 2 || l1.Listing.Succeeded != 1 || l1.Listing.Failed != 1 || l1.Listing.LastError != "boom" {
		t.Errorf("l1 listing: %+v", l1.Listing)
	}
	if !l1.JobDetail.Running || l1.JobDetail.Attempts != 0 {
		t.Errorf("l1 job_detail: %+v", l1.JobDetail)
	}
	if byID["l2"].InstructionStats != (InstructionStats{}) {
		t.Errorf("a link with no attempts must be all zeros: %+v", byID["l2"].InstructionStats)
	}

	// The JSON the frontend reads: always present, camelCase keys.
	raw, err := json.Marshal(byID["l2"])
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	_ = json.Unmarshal(raw, &decoded)
	stats, ok := decoded["instructionStats"].(map[string]any)
	if !ok {
		t.Fatalf("instructionStats missing from link JSON: %s", raw)
	}
	for _, kind := range []string{"listing", "jobDetail"} {
		k, ok := stats[kind].(map[string]any)
		if !ok || k["attempts"] != float64(0) || k["running"] != false {
			t.Errorf("%s: %v", kind, stats[kind])
		}
	}
}

func backdateAttempt(t *testing.T, id string, minutes int) {
	t.Helper()
	if _, err := db.JobsDB.Exec(`UPDATE instruction_attempts SET started_at = datetime('now', ?) WHERE id = ?`, fmt.Sprintf("-%d minutes", minutes), id); err != nil {
		t.Fatal(err)
	}
}

func attemptRow(t *testing.T, id string) (status string, finished bool, errMsg string) {
	t.Helper()
	var fin, msg *string
	if err := db.JobsDB.QueryRow(`SELECT status, finished_at, error_message FROM instruction_attempts WHERE id = ?`, id).Scan(&status, &fin, &msg); err != nil {
		t.Fatal(err)
	}
	if msg != nil {
		errMsg = *msg
	}
	return status, fin != nil, errMsg
}

func TestReconcileStaleInstructionAttempts(t *testing.T) {
	setupAttemptsDB(t, "l1", "l2", "l3")

	// All started first, then aged: starting reaps stale rows itself,
	// so backdating between starts would end earlier ones early.
	stale := mustStart(t, "l1", InstructionKindListing)
	savedButUnfinished := mustStart(t, "l2", InstructionKindListing)
	MarkInstructionSaved("l2", InstructionKindListing)
	fresh := mustStart(t, "l3", InstructionKindListing)
	backdateAttempt(t, stale, 31)
	backdateAttempt(t, savedButUnfinished, 45)
	backdateAttempt(t, fresh, 29)

	n, err := ReconcileStaleInstructionAttempts()
	if err != nil || n != 2 {
		t.Fatalf("ended %d (err %v), want 2", n, err)
	}

	if status, fin, msg := attemptRow(t, stale); status != "failed" || !fin || msg != interruptedInstructionAttemptError {
		t.Errorf("stale: %q finished=%v msg=%q", status, fin, msg)
	}
	// A save that happened but whose finish call was lost is a success.
	if status, fin, msg := attemptRow(t, savedButUnfinished); status != "succeeded" || !fin || msg != "" {
		t.Errorf("saved-but-unfinished: %q finished=%v msg=%q", status, fin, msg)
	}
	if status, fin, _ := attemptRow(t, fresh); status != "running" || fin {
		t.Errorf("a run inside the window must be left alone: %q finished=%v", status, fin)
	}

	// Idempotent.
	if n, err := ReconcileStaleInstructionAttempts(); err != nil || n != 0 {
		t.Errorf("second pass ended %d (err %v), want 0", n, err)
	}
}

func TestStaleAttemptDoesNotBlockStartOrHideAlert(t *testing.T) {
	setupAttemptsDB(t, "l1")

	stale := mustStart(t, "l1", InstructionKindListing)
	backdateAttempt(t, stale, 40)

	// Stats heal it first: the pill must not stay suppressed by a ghost run.
	stats, err := InstructionStatsByLink()
	if err != nil {
		t.Fatal(err)
	}
	if k := stats["l1"].Listing; k.Running || k.Failed != 1 || k.Attempts != 1 {
		t.Fatalf("stale run must be counted failed, not running: %+v", k)
	}

	// And a fresh start is a real new attempt, not the dead one.
	again := mustStart(t, "l1", InstructionKindListing)
	if again == stale {
		t.Fatal("start must create a new attempt once the stale one is reaped")
	}
}

func TestStartReapsStaleRowBeforeCheckingForRunning(t *testing.T) {
	setupAttemptsDB(t, "l1")
	stale := mustStart(t, "l1", InstructionKindJobDetail)
	backdateAttempt(t, stale, 60)

	id, started, err := StartInstructionAttempt("l1", InstructionKindJobDetail, InstructionTriggerAuto, "")
	if err != nil || !started || id == stale {
		t.Fatalf("id=%q started=%v err=%v (stale=%q)", id, started, err, stale)
	}
	if status, _, _ := attemptRow(t, stale); status != "failed" {
		t.Errorf("stale row status = %q, want failed", status)
	}
}

func failureReason(t *testing.T, id string) *string {
	t.Helper()
	var reason *string
	if err := db.JobsDB.QueryRow(`SELECT failure_reason FROM instruction_attempts WHERE id = ?`, id).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	return reason
}

func TestFailureReasonIsRecordedOnEveryFinishPath(t *testing.T) {
	setupAttemptsDB(t, "l1")

	// Ended cleanly, nothing saved: the AI gave up.
	gaveUp := mustStart(t, "l1", InstructionKindListing)
	FinishInstructionAttempt("l1", InstructionKindListing, "")
	if r := failureReason(t, gaveUp); r == nil || *r != InstructionFailureNotSaved {
		t.Errorf("clean end without a save = %v, want not_saved", r)
	}

	// The turn itself errored.
	errored := mustStart(t, "l1", InstructionKindListing)
	FinishInstructionAttempt("l1", InstructionKindListing, "provider returned 500")
	if r := failureReason(t, errored); r == nil || *r != InstructionFailureError {
		t.Errorf("turn error = %v, want error", r)
	}

	// Saved: a success has no failure reason, even if the client also sent an error.
	ok := mustStart(t, "l1", InstructionKindListing)
	MarkInstructionSaved("l1", InstructionKindListing)
	FinishInstructionAttempt("l1", InstructionKindListing, "late error")
	if r := failureReason(t, ok); r != nil {
		t.Errorf("a success must have no failure reason, got %v", *r)
	}

	// Left running until stale.
	stale := mustStart(t, "l1", InstructionKindListing)
	backdateAttempt(t, stale, 40)
	if _, err := ReconcileStaleInstructionAttempts(); err != nil {
		t.Fatal(err)
	}
	if r := failureReason(t, stale); r == nil || *r != InstructionFailureInterrupted {
		t.Errorf("a reaped attempt = %v, want interrupted", r)
	}

	// A stale one that had already saved is a success, with no reason.
	savedStale := mustStart(t, "l1", InstructionKindListing)
	MarkInstructionSaved("l1", InstructionKindListing)
	backdateAttempt(t, savedStale, 50)
	if _, err := ReconcileStaleInstructionAttempts(); err != nil {
		t.Fatal(err)
	}
	if r := failureReason(t, savedStale); r != nil {
		t.Errorf("a stale attempt that had saved must stay reasonless, got %v", *r)
	}
}

// finishAs runs one full attempt for a link and kind and ends it the
// given way: "gave-up" (clean end, nothing saved), "error", "interrupted"
// (left running past the stale window) or "success".
func finishAs(t *testing.T, link, kind, how string) {
	t.Helper()
	id := mustStart(t, link, kind)
	switch how {
	case "gave-up":
		FinishInstructionAttempt(link, kind, "")
	case "error":
		FinishInstructionAttempt(link, kind, "provider returned 500")
	case "success":
		MarkInstructionSaved(link, kind)
		FinishInstructionAttempt(link, kind, "")
	case "interrupted":
		backdateAttempt(t, id, 40)
		if _, err := ReconcileStaleInstructionAttempts(); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown outcome %q", how)
	}
}

func streak(t *testing.T, link, kind string) int {
	t.Helper()
	stats, err := InstructionStatsByLink()
	if err != nil {
		t.Fatal(err)
	}
	s := stats[link]
	return s.forKind(kind).ConsecutiveFailures
}

func TestConsecutiveFailuresCountOnlyGaveUpRunsSinceTheLastSuccess(t *testing.T) {
	setupAttemptsDB(t, "l1")
	const kind = InstructionKindListing

	finishAs(t, "l1", kind, "gave-up")
	finishAs(t, "l1", kind, "gave-up")
	if got := streak(t, "l1", kind); got != 2 {
		t.Fatalf("two gave-up runs = %d, want 2", got)
	}

	// A success ends the streak; a failure after it starts a new one.
	finishAs(t, "l1", kind, "success")
	if got := streak(t, "l1", kind); got != 0 {
		t.Fatalf("after a success = %d, want 0", got)
	}
	finishAs(t, "l1", kind, "gave-up")
	if got := streak(t, "l1", kind); got != 1 {
		t.Fatalf("one failure after the success = %d, want 1", got)
	}
}

func TestErrorsAndInterruptionsAreNeutral(t *testing.T) {
	setupAttemptsDB(t, "l1")
	const kind = InstructionKindListing

	finishAs(t, "l1", kind, "gave-up")
	finishAs(t, "l1", kind, "error")       // neither adds…
	finishAs(t, "l1", kind, "interrupted") // …nor resets
	finishAs(t, "l1", kind, "gave-up")
	if got := streak(t, "l1", kind); got != 2 {
		t.Fatalf("gave-up, error, interrupted, gave-up = %d, want 2", got)
	}

	// Only errors, never a gave-up run: no streak at all.
	setupAttemptsDBMore(t, "l2")
	finishAs(t, "l2", kind, "error")
	finishAs(t, "l2", kind, "error")
	finishAs(t, "l2", kind, "error")
	if got := streak(t, "l2", kind); got != 0 {
		t.Fatalf("three provider errors must not look like an uncrawlable page, got %d", got)
	}
}

func TestStreaksAreIndependentPerKindAndLink(t *testing.T) {
	setupAttemptsDB(t, "l1", "l2")

	finishAs(t, "l1", InstructionKindListing, "gave-up")
	finishAs(t, "l1", InstructionKindListing, "gave-up")
	finishAs(t, "l1", InstructionKindJobDetail, "gave-up")
	finishAs(t, "l2", InstructionKindListing, "success")

	if got := streak(t, "l1", InstructionKindListing); got != 2 {
		t.Errorf("l1 listing = %d, want 2", got)
	}
	if got := streak(t, "l1", InstructionKindJobDetail); got != 1 {
		t.Errorf("l1 job_detail = %d, want 1", got)
	}
	if got := streak(t, "l2", InstructionKindListing); got != 0 {
		t.Errorf("l2 listing = %d, want 0", got)
	}
	// A success of one kind never resets the other kind's streak.
	finishAs(t, "l1", InstructionKindJobDetail, "success")
	if got := streak(t, "l1", InstructionKindListing); got != 2 {
		t.Errorf("a job_detail success must not reset the listing streak, got %d", got)
	}
}

func TestARunningAttemptDoesNotChangeTheStreak(t *testing.T) {
	setupAttemptsDB(t, "l1")
	finishAs(t, "l1", InstructionKindListing, "gave-up")
	finishAs(t, "l1", InstructionKindListing, "gave-up")
	mustStart(t, "l1", InstructionKindListing) // still running

	stats, err := InstructionStatsByLink()
	if err != nil {
		t.Fatal(err)
	}
	if k := stats["l1"].Listing; k.ConsecutiveFailures != 2 || !k.Running {
		t.Fatalf("a run in flight must not count or reset: %+v", k)
	}
}

func TestListPortalsExposesConsecutiveFailures(t *testing.T) {
	setupAttemptsDB(t, "l1", "l2")
	finishAs(t, "l1", InstructionKindListing, "gave-up")
	finishAs(t, "l1", InstructionKindListing, "gave-up")
	finishAs(t, "l1", InstructionKindListing, "gave-up")

	portals, err := ListPortals()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]portalLink{}
	for _, l := range portals[0].Links {
		byID[l.ID] = l
	}
	if got := byID["l1"].InstructionStats.Listing.ConsecutiveFailures; got != 3 {
		t.Errorf("l1 = %d, want 3", got)
	}

	for id, want := range map[string]float64{"l1": 3, "l2": 0} {
		raw, _ := json.Marshal(byID[id])
		var decoded map[string]any
		_ = json.Unmarshal(raw, &decoded)
		listing := decoded["instructionStats"].(map[string]any)["listing"].(map[string]any)
		if listing["consecutiveFailures"] != want {
			t.Errorf("%s JSON consecutiveFailures = %v, want %v", id, listing["consecutiveFailures"], want)
		}
		if _, ok := decoded["instructionStats"].(map[string]any)["jobDetail"].(map[string]any)["consecutiveFailures"]; !ok {
			t.Errorf("%s: jobDetail must also carry consecutiveFailures", id)
		}
	}
}

// setupAttemptsDBMore adds another link to an already-initialised test DB.
func setupAttemptsDBMore(t *testing.T, linkID string) {
	t.Helper()
	if _, err := db.JobsDB.Exec(`INSERT INTO portal_links (id, portal_id, url) VALUES (?, 'p1', ?)`, linkID, "https://x/"+linkID); err != nil {
		t.Fatal(err)
	}
}
