package crawlhandlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/handlertest"
	"career-tool-backend/portal"
)

// Step 161: "Start fresh" wipes one link's AI history and returns the conversation ids for the frontend to delete.

type attemptSeed struct {
	id, link, kind, status, conv, started, reason string
}

func seedFreshDB(t *testing.T) {
	t.Helper()
	handlertest.SetupHandlerDB(t) // portal p1 and link l1
	if _, err := db.JobsDB.Exec(`INSERT INTO portal_links (id, portal_id, url) VALUES ('l2', 'p1', 'https://y')`); err != nil {
		t.Fatal(err)
	}
	for _, a := range []attemptSeed{
		{"a1", "l1", "listing", "failed", "conv-1", "2026-11-01 10:00:00", "not_saved"},
		{"a2", "l1", "listing", "failed", "conv-2", "2026-11-01 10:05:00", "not_saved"},
		{"a3", "l1", "job_detail", "failed", "conv-3", "2026-11-01 10:10:00", "error"},
		{"a4", "l1", "listing", "failed", "conv-1", "2026-11-01 10:15:00", "error"}, // the same conversation twice: one id
		{"a5", "l1", "listing", "failed", "", "2026-11-01 10:20:00", "interrupted"}, // an attempt that never got a conversation
		{"b1", "l2", "listing", "failed", "other-conv-1", "2026-11-01 11:00:00", "not_saved"},
		{"b2", "l2", "job_detail", "succeeded", "other-conv-2", "2026-11-01 11:05:00", ""},
	} {
		var reason any
		if a.reason != "" {
			reason = a.reason
		}
		var conv any
		if a.conv != "" {
			conv = a.conv
		}
		if _, err := db.JobsDB.Exec(`INSERT INTO instruction_attempts (id, portal_link_id, kind, status, conversation_id, started_at, finished_at, failure_reason) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			a.id, a.link, a.kind, a.status, conv, a.started, a.started, reason); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"l1", "l2"} {
		other := ""
		if id == "l2" {
			other = "-l2"
		}
		if _, err := db.JobsDB.Exec(`UPDATE portal_links SET
			crawl_instructions = ?, job_detail_crawl_instructions = ?,
			instructions_ai_error = 'listing failed', instructions_ai_error_at = '2026-11-01 10:00:00', instructions_ai_conversation_id = ?, instructions_ai_error_dismissed_at = '2026-11-01 10:01:00',
			job_detail_instructions_ai_error = 'detail failed', job_detail_instructions_ai_error_at = '2026-11-01 10:00:00', job_detail_instructions_ai_conversation_id = ?, job_detail_instructions_ai_error_dismissed_at = '2026-11-01 10:01:00',
			jobs_question_asked_at = '2026-11-01 10:02:00', jobs_question_note = 'asked'
			WHERE id = ?`,
			"container: x"+other, "fields: y"+other, "cur-listing"+other, "cur-detail"+other, id); err != nil {
			t.Fatal(err)
		}
	}
}

type linkState struct {
	listingInstr, detailInstr, err1, errAt1, conv1, dis1, err2, errAt2, conv2, dis2, jq, jqNote *string
}

func stateOf(t *testing.T, id string) linkState {
	t.Helper()
	var s linkState
	if err := db.JobsDB.QueryRow(`SELECT crawl_instructions, job_detail_crawl_instructions,
		instructions_ai_error, instructions_ai_error_at, instructions_ai_conversation_id, instructions_ai_error_dismissed_at,
		job_detail_instructions_ai_error, job_detail_instructions_ai_error_at, job_detail_instructions_ai_conversation_id, job_detail_instructions_ai_error_dismissed_at,
		jobs_question_asked_at, jobs_question_note FROM portal_links WHERE id = ?`, id).Scan(
		&s.listingInstr, &s.detailInstr, &s.err1, &s.errAt1, &s.conv1, &s.dis1, &s.err2, &s.errAt2, &s.conv2, &s.dis2, &s.jq, &s.jqNote); err != nil {
		t.Fatal(err)
	}
	return s
}

func attemptsOf(t *testing.T, link string) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM instruction_attempts WHERE portal_link_id = ?`, link).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func idsOf(t *testing.T, m map[string]any) []string {
	t.Helper()
	raw, ok := m["conversationIds"].([]any)
	if !ok {
		t.Fatalf("conversationIds must be a JSON array, got %#v", m["conversationIds"])
	}
	out := []string{}
	for _, v := range raw {
		out = append(out, v.(string))
	}
	return out
}

func allAIFieldsCleared(s linkState) bool {
	for _, p := range []*string{s.err1, s.errAt1, s.conv1, s.dis1, s.err2, s.errAt2, s.conv2, s.dis2, s.jq, s.jqNote} {
		if p != nil {
			return false
		}
	}
	return true
}

func TestStartFreshClearsOnlyThisLinksHistoryAndKeepsTheInstructions(t *testing.T) {
	seedFreshDB(t)
	before2 := stateOf(t, "l2")

	rec := handlertest.PostJSON(StartFreshHandler, `{"portalLinkId":"l1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	got := handlertest.Decode(t, rec)
	// oldest first, one entry per conversation, then the link's current ids; never another link's
	want := []string{"conv-1", "conv-2", "conv-3", "cur-listing", "cur-detail"}
	if ids := idsOf(t, got); !reflect.DeepEqual(ids, want) {
		t.Errorf("conversation ids = %v, want %v", ids, want)
	}
	if got["attemptsRemoved"] != float64(5) {
		t.Errorf("attemptsRemoved = %v, want 5", got["attemptsRemoved"])
	}

	if n := attemptsOf(t, "l1"); n != 0 {
		t.Errorf("l1 still has %d attempts", n)
	}
	s := stateOf(t, "l1")
	if !allAIFieldsCleared(s) {
		t.Errorf("l1's AI errors, conversation ids, dismissals and jobs question must all be cleared: %+v", s)
	}
	if s.listingInstr == nil || *s.listingInstr != "container: x" || s.detailInstr == nil || *s.detailInstr != "fields: y" {
		t.Errorf("the stored instructions must be kept: %v / %v", s.listingInstr, s.detailInstr)
	}
	// the pills' inputs: no attempts and no failure streak any more
	stats, err := portal.InstructionStatsByLink()
	if err != nil {
		t.Fatal(err)
	}
	if st := stats["l1"]; st.Listing.Attempts != 0 || st.Listing.ConsecutiveFailures != 0 || st.JobDetail.Attempts != 0 {
		t.Errorf("l1's stats must be empty: %+v", st)
	}

	// the other link is untouched
	if n := attemptsOf(t, "l2"); n != 2 {
		t.Errorf("l2 must keep its 2 attempts, has %d", n)
	}
	if after2 := stateOf(t, "l2"); !reflect.DeepEqual(after2, before2) {
		t.Errorf("l2's row changed")
	}
	if st := stats["l2"]; st.Listing.Attempts != 1 || st.JobDetail.Succeeded != 1 {
		t.Errorf("l2's stats must be intact: %+v", st)
	}
}

func TestStartFreshCanAlsoClearTheStoredInstructions(t *testing.T) {
	seedFreshDB(t)
	if rec := handlertest.PostJSON(StartFreshHandler, `{"portalLinkId":"l1","clearInstructions":true}`); rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	s := stateOf(t, "l1")
	if s.listingInstr != nil || s.detailInstr != nil || !allAIFieldsCleared(s) {
		t.Errorf("both instructions and the AI leftovers must be cleared: %+v", s)
	}
	if o := stateOf(t, "l2"); o.listingInstr == nil || *o.listingInstr != "container: x-l2" || o.detailInstr == nil {
		t.Errorf("l2's instructions must stay: %+v", o)
	}
}

func TestStartFreshRefusesWhileAGenerationIsRunning(t *testing.T) {
	for _, kind := range []string{"listing", "job_detail"} {
		seedFreshDB(t)
		if _, err := db.JobsDB.Exec(`INSERT INTO instruction_attempts (id, portal_link_id, kind, status, conversation_id) VALUES ('run', 'l1', ?, 'running', 'conv-run')`, kind); err != nil {
			t.Fatal(err)
		}
		rec := handlertest.PostJSON(StartFreshHandler, `{"portalLinkId":"l1","clearInstructions":true}`)
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "still running") {
			t.Fatalf("%s running: got %d %s, want 409", kind, rec.Code, rec.Body.String())
		}
		if n := attemptsOf(t, "l1"); n != 6 {
			t.Errorf("%s running: nothing may be deleted, %d attempts left", kind, n)
		}
		if s := stateOf(t, "l1"); s.listingInstr == nil || s.conv1 == nil || s.jq == nil {
			t.Errorf("%s running: the link must be untouched: %+v", kind, s)
		}
		db.JobsDB.Close() // the next round seeds a fresh database
		db.CareerDB.Close()
	}
	seedFreshDB(t) // a run of ANOTHER link does not block this one
	if _, err := db.JobsDB.Exec(`INSERT INTO instruction_attempts (id, portal_link_id, kind, status) VALUES ('run2', 'l2', 'listing', 'running')`); err != nil {
		t.Fatal(err)
	}
	if rec := handlertest.PostJSON(StartFreshHandler, `{"portalLinkId":"l1"}`); rec.Code != http.StatusOK {
		t.Errorf("a run of another link must not block: %d %s", rec.Code, rec.Body.String())
	}
}

func TestStartFreshIgnoresAStaleRunningRow(t *testing.T) {
	seedFreshDB(t)
	if _, err := db.JobsDB.Exec(`INSERT INTO instruction_attempts (id, portal_link_id, kind, status, conversation_id, started_at) VALUES ('stale', 'l1', 'listing', 'running', 'conv-stale', datetime('now', '-3 hours'))`); err != nil {
		t.Fatal(err)
	}
	rec := handlertest.PostJSON(StartFreshHandler, `{"portalLinkId":"l1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("a crashed run (running for hours) must not block: %d %s", rec.Code, rec.Body.String())
	}
	if ids := idsOf(t, handlertest.Decode(t, rec)); !contains(ids, "conv-stale") {
		t.Errorf("the stale run's conversation must be returned too: %v", ids)
	}
	if n := attemptsOf(t, "l1"); n != 0 {
		t.Errorf("%d attempts left", n)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestStartFreshOnALinkWithNothingToClearIsHarmlessAndRepeatable(t *testing.T) {
	handlertest.SetupHandlerDB(t)
	for i := 0; i < 2; i++ {
		rec := handlertest.PostJSON(StartFreshHandler, `{"portalLinkId":"l1"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("call %d: %d %s", i, rec.Code, rec.Body.String())
		}
		got := handlertest.Decode(t, rec)
		if ids := idsOf(t, got); len(ids) != 0 || got["attemptsRemoved"] != float64(0) {
			t.Errorf("call %d: nothing to clear: %v", i, got)
		}
		if !strings.Contains(rec.Body.String(), `"conversationIds":[]`) {
			t.Errorf("call %d: the ids must be an empty JSON array, not null: %s", i, rec.Body.String())
		}
	}
}

func TestStartFreshIsRepeatableAfterWork(t *testing.T) {
	seedFreshDB(t)
	handlertest.PostJSON(StartFreshHandler, `{"portalLinkId":"l1"}`)
	got := handlertest.Decode(t, handlertest.PostJSON(StartFreshHandler, `{"portalLinkId":"l1"}`))
	if len(idsOf(t, got)) != 0 || got["attemptsRemoved"] != float64(0) {
		t.Errorf("a second start fresh finds nothing left: %v", got)
	}
}

func TestStartFreshRejectsBadInput(t *testing.T) {
	handlertest.SetupHandlerDB(t)
	for name, c := range map[string]struct {
		body string
		want int
	}{
		"no link id":   {`{}`, http.StatusBadRequest},
		"not JSON":     {`nope`, http.StatusBadRequest},
		"unknown link": {`{"portalLinkId":"zzz"}`, http.StatusBadRequest},
		"oversized":    {`{"portalLinkId":"` + strings.Repeat("a", 70<<10) + `"}`, http.StatusBadRequest},
	} {
		if rec := handlertest.PostJSON(StartFreshHandler, c.body); rec.Code != c.want {
			t.Errorf("%s: got %d (%s), want %d", name, rec.Code, strings.TrimSpace(rec.Body.String()), c.want)
		}
	}
	rec := httptest.NewRecorder()
	StartFreshHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: got %d, want 405", rec.Code)
	}
}

func TestStartFreshDoesNotTouchOtherTables(t *testing.T) {
	seedFreshDB(t)
	if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, portal_link_id) VALUES ('j1', 'https://x/j1', 'A job', 'l1')`); err != nil {
		t.Fatal(err)
	}
	handlertest.PostJSON(StartFreshHandler, `{"portalLinkId":"l1","clearInstructions":true}`)
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs WHERE portal_link_id = 'l1'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("the link's saved jobs must stay: %d (%v)", n, err)
	}
	var links int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM portal_links`).Scan(&links); err != nil || links != 2 {
		t.Fatalf("both links must stay: %d (%v)", links, err)
	}
}

func TestManifestDeclaresStartFresh(t *testing.T) {
	raw, err := os.ReadFile("../../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Routes []map[string]string `json:"routes"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, r := range parsed.Routes {
		if r["path_suffix"] == "portal-links/start-fresh" {
			found++
			if r["method"] != "POST" || r["required_scope"] != "tool:career:portals" {
				t.Errorf("method=%q scope=%q", r["method"], r["required_scope"])
			}
		}
	}
	if found != 1 {
		t.Errorf("manifest.json must declare portal-links/start-fresh exactly once, found %d", found)
	}
}

// The handler is called directly by the tests above; this pins that main.go really serves it.
func TestMainServesStartFresh(t *testing.T) {
	raw, err := os.ReadFile("../../main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `http.HandleFunc("/portal-links/start-fresh", crawlhandlers.StartFreshHandler)`) {
		t.Error(`main.go must register http.HandleFunc("/portal-links/start-fresh", crawlhandlers.StartFreshHandler)`)
	}
}
