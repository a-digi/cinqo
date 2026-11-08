package portal

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
)

func linkQuestion(t *testing.T, id string) *JobsQuestion {
	t.Helper()
	portals, err := ListPortals()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range portals {
		for _, l := range p.Links {
			if l.ID == id {
				return l.JobsQuestion
			}
		}
	}
	t.Fatalf("link %s not listed", id)
	return nil
}

func TestAskRecordsOneQuestionAndRejectsASecond(t *testing.T) {
	setupAttemptsDB(t, "l1")
	if q := linkQuestion(t, "l1"); q != nil {
		t.Fatalf("a fresh link has a question: %+v", q)
	}
	if err := AskJobsQuestion("l1", "  2 attempts, container matched nothing  "); err != nil {
		t.Fatal(err)
	}
	q := linkQuestion(t, "l1")
	if q == nil || q.Note != "2 attempts, container matched nothing" || q.AskedAt == "" {
		t.Fatalf("question not listed as expected: %+v", q)
	}
	if err := AskJobsQuestion("l1", "again"); !errors.Is(err, ErrJobsQuestionAlreadyAsked) {
		t.Errorf("second ask: %v", err)
	}
	if q := linkQuestion(t, "l1"); q.Note != "2 attempts, container matched nothing" {
		t.Errorf("a rejected ask must not overwrite the pending one: %+v", q)
	}
}

func TestAskValidation(t *testing.T) {
	setupAttemptsDB(t, "l1")
	if err := AskJobsQuestion("nope", "x"); !errors.Is(err, ErrUnknownPortalLink) {
		t.Errorf("unknown link: %v", err)
	}
	if err := AskJobsQuestion("l1", "   "); !errors.Is(err, ErrJobsQuestionNoteRequired) {
		t.Errorf("empty note: %v", err)
	}
	if err := AskJobsQuestion("l1", strings.Repeat("é", 500)); err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(linkQuestion(t, "l1").Note)); n != maxJobsQuestionNoteLen {
		t.Errorf("note length = %d runes, want %d", n, maxJobsQuestionNoteLen)
	}
}

func TestAnswerYesClearsWithoutAcknowledging(t *testing.T) {
	setupAttemptsDB(t, "l1")
	addListingRun(t, "l1", "completed", 0, 0, 0, stamp(5))
	if err := AskJobsQuestion("l1", "x"); err != nil {
		t.Fatal(err)
	}
	if err := AnswerJobsQuestion("l1", "yes"); err != nil {
		t.Fatal(err)
	}
	if linkQuestion(t, "l1") != nil {
		t.Error("answer yes must clear the question")
	}
	if c, _ := lastCrawl(t, "l1"); c.AcknowledgedAt != "" {
		t.Errorf("answer yes must not acknowledge: %q", c.AcknowledgedAt)
	}
}

func TestAnswerNoClearsAndAcknowledges(t *testing.T) {
	setupAttemptsDB(t, "l1")
	addListingRun(t, "l1", "completed", 0, 0, 0, stamp(5))
	if err := AskJobsQuestion("l1", "x"); err != nil {
		t.Fatal(err)
	}
	if err := AnswerJobsQuestion("l1", "no"); err != nil {
		t.Fatal(err)
	}
	if linkQuestion(t, "l1") != nil {
		t.Error("answer no must clear the question")
	}
	if c, _ := lastCrawl(t, "l1"); c.AcknowledgedAt == "" {
		t.Error("answer no must acknowledge the zero-job streak")
	}
}

func TestAnswerValidation(t *testing.T) {
	setupAttemptsDB(t, "l1")
	if err := AnswerJobsQuestion("l1", "maybe"); !errors.Is(err, ErrInvalidJobsQuestionAnswer) {
		t.Errorf("invalid answer: %v", err)
	}
	if err := AnswerJobsQuestion("nope", "yes"); !errors.Is(err, ErrUnknownPortalLink) {
		t.Errorf("unknown link: %v", err)
	}
	if err := AnswerJobsQuestion("l1", "no"); !errors.Is(err, ErrNoPendingJobsQuestion) {
		t.Errorf("nothing pending: %v", err)
	}
	if err := AskJobsQuestion("l1", "x"); err != nil {
		t.Fatal(err)
	}
	if err := AnswerJobsQuestion("l1", "maybe"); !errors.Is(err, ErrInvalidJobsQuestionAnswer) {
		t.Fatal(err)
	}
	if linkQuestion(t, "l1") == nil {
		t.Error("an invalid answer must leave the question pending")
	}
}

func TestSavingInstructionsClearsThePendingQuestion(t *testing.T) {
	setupAttemptsDB(t, "l1")
	if err := AskJobsQuestion("l1", "x"); err != nil {
		t.Fatal(err)
	}
	yaml := "container: .job\nfields:\n  - label: title\n    selector: h2\n  - label: url\n    selector: a\n    attribute: href\npagination:\n  nextSelector: .next\n  maxPages: 2\n"
	if err := UpdatePortalLinkCrawlInstructions("l1", yaml); err != nil {
		t.Fatalf("save: %v", err)
	}
	if linkQuestion(t, "l1") != nil {
		t.Error("saving instructions must clear the pending question")
	}
}

func TestDeletingTheLinkTakesItsQuestionAlong(t *testing.T) {
	setupAttemptsDB(t, "l1")
	if err := AskJobsQuestion("l1", "x"); err != nil {
		t.Fatal(err)
	}
	if err := RemovePortalLink("l1"); err != nil {
		t.Fatal(err)
	}
	if err := AnswerJobsQuestion("l1", "yes"); !errors.Is(err, ErrUnknownPortalLink) {
		t.Errorf("after delete: %v", err)
	}
}

// The MCP tool is the only way the agent can reach this; it must only be
// able to ask, and must tell the agent to stop.
func TestAskToolIsRegisteredAndOnlyAsks(t *testing.T) {
	setupAttemptsDB(t, "l1")
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	RegisterAskUserIfPageHasJobs(server)

	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil)
	session, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) != 1 || tools.Tools[0].Name != "ask_user_if_page_has_jobs" {
		t.Fatalf("tools: %v %v", tools, err)
	}
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "ask_user_if_page_has_jobs",
		Arguments: map[string]any{"portalLinkId": "l1", "note": "2 attempts"},
	})
	if err != nil || res.IsError {
		t.Fatalf("call: %v %+v", err, res)
	}
	if text := res.Content[0].(*mcp.TextContent).Text; !strings.Contains(text, "End your turn") {
		t.Errorf("the tool must tell the agent to end its turn: %q", text)
	}
	if linkQuestion(t, "l1") == nil {
		t.Error("the call must record the question")
	}
	// A second call is an error result, not a second question.
	res, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "ask_user_if_page_has_jobs",
		Arguments: map[string]any{"portalLinkId": "l1", "note": "again"},
	})
	if err != nil || !res.IsError {
		t.Errorf("second call must be an error result: %v %+v", err, res)
	}
	_ = db.JobsDB
}

func TestZeroItemsMessageSendsTheAgentToAskTheUser(t *testing.T) {
	msg := classifyCrawlResultPages([]CrawlResultPage{{}}).Message
	for _, want := range []string{"ask_user_if_page_has_jobs", "ONE fix attempt", "never a result to save"} {
		if !strings.Contains(msg, want) {
			t.Errorf("0-items message lacks %q: %s", want, msg)
		}
	}
}

func TestAnswerNoDropsTheRunningAttemptAndYesKeepsIt(t *testing.T) {
	setupAttemptsDB(t, "l1", "l2")
	for _, id := range []string{"l1", "l2"} {
		mustStart(t, id, InstructionKindListing)
		if err := AskJobsQuestion(id, "x"); err != nil {
			t.Fatal(err)
		}
	}
	if err := AnswerJobsQuestion("l1", "no"); err != nil {
		t.Fatal(err)
	}
	if err := AnswerJobsQuestion("l2", "yes"); err != nil {
		t.Fatal(err)
	}
	count := func(id string) (total, running int) {
		if err := db.JobsDB.QueryRow(`SELECT COUNT(*), COALESCE(SUM(status = 'running'), 0) FROM instruction_attempts WHERE portal_link_id = ?`, id).Scan(&total, &running); err != nil {
			t.Fatal(err)
		}
		return
	}
	if total, _ := count("l1"); total != 0 {
		t.Errorf("answer no must drop the attempt, %d left", total)
	}
	if total, running := count("l2"); total != 1 || running != 1 {
		t.Errorf("answer yes must keep the running attempt, got total=%d running=%d", total, running)
	}
	// And a "no" never becomes a failure in the stats.
	stats, err := InstructionStatsByLink()
	if err != nil {
		t.Fatal(err)
	}
	if s := stats["l1"].Listing; s.Failed != 0 || s.Attempts != 0 {
		t.Errorf("answer no must not count as an attempt: %+v", s)
	}
}

func itemsPage(n int) []CrawlResultPage {
	items := make([]map[string]any, n)
	for i := range items {
		items[i] = map[string]any{"title": "Job", "url": "https://x/" + string(rune('a'+i))}
	}
	return []CrawlResultPage{{Items: items}}
}

func TestLowItemCountWarnsButNeverChangesOK(t *testing.T) {
	for n, wantWarning := range map[int]bool{1: true, 2: true, 3: false, 25: false} {
		r := classifyCrawlResultPages(itemsPage(n))
		if (r.Warning != "") != wantWarning {
			t.Errorf("%d items: warning = %q, want a warning: %v", n, r.Warning, wantWarning)
		}
		if !r.OK {
			t.Errorf("%d usable items: ok must stay true", n)
		}
		if wantWarning && !strings.Contains(r.Warning, "container selector") {
			t.Errorf("%d items: warning must point at the container selector: %q", n, r.Warning)
		}
	}
}

func TestLowItemCountIsTheTotalAcrossPages(t *testing.T) {
	pages := append(itemsPage(2), itemsPage(2)...)
	if r := classifyCrawlResultPages(pages); r.Warning != "" || r.ItemsFound != 4 {
		t.Errorf("two pages of two items are 4 in total, no warning: %+v", r)
	}
}

func TestZeroItemsKeepTheirOwnMessageAndNoLowCountWarning(t *testing.T) {
	r := classifyCrawlResultPages([]CrawlResultPage{{}})
	if r.Warning != "" || r.OK || !strings.Contains(r.Message, "No items were extracted") {
		t.Errorf("0 items: %+v", r)
	}
}

func TestWarningIsOmittedFromTheJSONWhenThereIsNone(t *testing.T) {
	b, err := json.Marshal(classifyCrawlResultPages(itemsPage(5)))
	if err != nil || strings.Contains(string(b), "warning") {
		t.Errorf("no warning expected in %s (%v)", b, err)
	}
	b, _ = json.Marshal(classifyCrawlResultPages(itemsPage(1)))
	if !strings.Contains(string(b), `"warning"`) {
		t.Errorf("warning expected in %s", b)
	}
}
