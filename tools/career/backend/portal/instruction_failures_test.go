package portal

import (
	"testing"

	"career-tool-backend/db"
)

const (
	listingYAML = "container: .job\nfields:\n  - label: title\n    selector: h2\n  - label: url\n    selector: a\n    attribute: href\npagination:\n  nextSelector: .next\n  maxPages: 2\n"
	detailYAML  = "fields:\n  - label: description\n    selector: .desc\n"
)

func sx(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := db.JobsDB.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func failedAttempts(t *testing.T, link string) int {
	return count(t, `SELECT COUNT(*) FROM instruction_attempts WHERE portal_link_id = ? AND status = 'failed'`, link)
}

func errorsStored(t *testing.T, link string) int {
	return count(t, `SELECT COUNT(*) FROM portal_links WHERE id = ? AND (instructions_ai_error IS NOT NULL OR instructions_ai_error_at IS NOT NULL OR instructions_ai_error_dismissed_at IS NOT NULL
		OR job_detail_instructions_ai_error IS NOT NULL OR job_detail_instructions_ai_error_at IS NOT NULL OR job_detail_instructions_ai_error_dismissed_at IS NOT NULL)`, link)
}

// giveFailures stores 2 failed + 1 succeeded attempt (one of each kind failed) and an error of each kind on the link.
var giveSeq int

func giveFailures(t *testing.T, link string) {
	t.Helper()
	giveSeq++
	link0 := link
	id := func(s string) string {
		return link0 + "-" + s + "-" + string(rune('0'+giveSeq%10)) + string(rune('a'+giveSeq/10))
	}
	sx(t, `INSERT INTO instruction_attempts (id, portal_link_id, kind, triggered_by, status, failure_reason) VALUES (?, ?, 'job_detail', 'manual', 'failed', 'error')`, id("f1"), link)
	sx(t, `INSERT INTO instruction_attempts (id, portal_link_id, kind, triggered_by, status, failure_reason) VALUES (?, ?, 'listing', 'manual', 'failed', 'not_saved')`, id("f2"), link)
	sx(t, `INSERT INTO instruction_attempts (id, portal_link_id, kind, triggered_by, status) VALUES (?, ?, 'listing', 'manual', 'succeeded')`, id("s1"), link)
	sx(t, `UPDATE portal_links SET instructions_ai_error = 'e1', instructions_ai_error_at = '2026-11-08 20:00:00', instructions_ai_error_dismissed_at = '2026-11-08 20:01:00',
		job_detail_instructions_ai_error = 'e2', job_detail_instructions_ai_error_at = '2026-11-08 20:14:36', job_detail_instructions_ai_error_dismissed_at = '2026-11-08 20:12:51',
		instructions_ai_conversation_id = 'conv-l', job_detail_instructions_ai_conversation_id = 'conv-d' WHERE id = ?`, link)
}

const ruleJSON = `{"host":"x.test","include":["/job/"]}`

func TestFullyInstructedDecisionTable(t *testing.T) {
	cases := []struct {
		name string
		s    instructedSnapshot
		want bool
	}{
		{"nothing", instructedSnapshot{}, false},
		{"AI listing only", instructedSnapshot{HasAI: true}, false},
		{"details only", instructedSnapshot{HasDetail: true}, false},
		{"AI listing and details", instructedSnapshot{HasAI: true, HasDetail: true}, true},
		{"rule and details", instructedSnapshot{HasRule: true, HasDetail: true}, true},
		{"rule only", instructedSnapshot{HasRule: true}, false},
		{"rod link: a rule does not count", instructedSnapshot{Engine: LinkEngineRod, HasRule: true, HasDetail: true}, false},
		{"rod link: AI listing counts", instructedSnapshot{Engine: LinkEngineRod, HasAI: true, HasDetail: true}, true},
		{"login link: a rule does not count", instructedSnapshot{RequiresLogin: true, HasRule: true, HasDetail: true}, false},
		{"login link: AI listing counts", instructedSnapshot{RequiresLogin: true, HasAI: true, HasDetail: true}, true},
	}
	for _, c := range cases {
		if got := fullyInstructed(c.s); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestIsFullyInstructedReadsTheLink(t *testing.T) {
	setupAttemptsDB(t, "none", "ai", "rule", "blank", "rod", "login")
	sx(t, `UPDATE portal_links SET crawl_instructions = ?, job_detail_crawl_instructions = ? WHERE id = 'ai'`, listingYAML, detailYAML)
	sx(t, `UPDATE portal_links SET link_rules = ?, job_detail_crawl_instructions = ? WHERE id = 'rule'`, ruleJSON, detailYAML)
	sx(t, `UPDATE portal_links SET crawl_instructions = '  ', job_detail_crawl_instructions = ? WHERE id = 'blank'`, detailYAML)
	sx(t, `UPDATE portal_links SET link_rules = ?, job_detail_crawl_instructions = ?, crawl_engine = 'rod' WHERE id = 'rod'`, ruleJSON, detailYAML)
	sx(t, `UPDATE portal_links SET link_rules = ?, job_detail_crawl_instructions = ?, requires_login = 1 WHERE id = 'login'`, ruleJSON, detailYAML)
	want := map[string]bool{"none": false, "ai": true, "rule": true, "blank": false, "rod": false, "login": false, "missing": false}
	for id, w := range want {
		got, err := IsFullyInstructed(id)
		if err != nil || got != w {
			t.Errorf("%s: got %v err %v, want %v", id, got, err, w)
		}
	}
}

func TestClearInstructionFailuresRemovesOnlyThisLinksFailuresAndErrors(t *testing.T) {
	setupAttemptsDB(t, "a", "b")
	giveFailures(t, "a")
	giveFailures(t, "b")
	sx(t, `INSERT INTO instruction_attempts (id, portal_link_id, kind, triggered_by, status) VALUES ('a-run', 'a', 'job_detail', 'manual', 'running')`)
	n, err := ClearInstructionFailures("a")
	if err != nil || n != 2 {
		t.Fatalf("removed %d err %v", n, err)
	}
	if failedAttempts(t, "a") != 0 || errorsStored(t, "a") != 0 {
		t.Error("link a must have no failed attempt and no error")
	}
	if count(t, `SELECT COUNT(*) FROM instruction_attempts WHERE portal_link_id = 'a' AND status = 'succeeded'`) != 1 {
		t.Error("the succeeded attempt stays")
	}
	if count(t, `SELECT COUNT(*) FROM instruction_attempts WHERE id = 'a-run' AND status = 'running'`) != 1 {
		t.Error("a running attempt stays")
	}
	if count(t, `SELECT COUNT(*) FROM portal_links WHERE id = 'a' AND instructions_ai_conversation_id = 'conv-l' AND job_detail_instructions_ai_conversation_id = 'conv-d'`) != 1 {
		t.Error("the conversation ids stay")
	}
	if failedAttempts(t, "b") != 2 || errorsStored(t, "b") != 1 {
		t.Error("link b is untouched")
	}
}

func TestClearFailuresIfInstructedDoesNothingForALinkThatIsNotInstructed(t *testing.T) {
	setupAttemptsDB(t, "a")
	giveFailures(t, "a")
	sx(t, `UPDATE portal_links SET crawl_instructions = ? WHERE id = 'a'`, listingYAML) // the details are missing
	if err := ClearFailuresIfInstructed("a"); err != nil {
		t.Fatal(err)
	}
	if failedAttempts(t, "a") != 2 || errorsStored(t, "a") != 1 {
		t.Error("the counters are the evidence a page cannot be crawled: they stay")
	}
}

func TestSavingTheInstructionThatCompletesThePairClearsTheFailures(t *testing.T) {
	setupAttemptsDB(t, "a", "b")
	giveFailures(t, "a")
	giveFailures(t, "b")
	// listing first: the pair is not complete yet
	if err := UpdatePortalLinkCrawlInstructions("a", listingYAML); err != nil {
		t.Fatal(err)
	}
	if failedAttempts(t, "a") != 2 || errorsStored(t, "a") != 1 {
		t.Fatal("half instructed keeps the failures")
	}
	if err := UpdatePortalLinkJobDetailCrawlInstructions("a", detailYAML); err != nil {
		t.Fatal(err)
	}
	if failedAttempts(t, "a") != 0 || errorsStored(t, "a") != 0 {
		t.Error("the second save completes the pair and clears")
	}
	// the other order, through the origin-aware save
	if err := UpdatePortalLinkJobDetailCrawlInstructionsFrom("b", detailYAML, JobDetailOriginAI); err != nil {
		t.Fatal(err)
	}
	if failedAttempts(t, "b") != 2 {
		t.Fatal("details only keeps the failures")
	}
	if err := UpdatePortalLinkCrawlInstructions("b", listingYAML); err != nil {
		t.Fatal(err)
	}
	if failedAttempts(t, "b") != 0 || errorsStored(t, "b") != 0 {
		t.Error("the listing save completes the pair and clears")
	}
}

func TestAFailedSaveClearsNothing(t *testing.T) {
	setupAttemptsDB(t, "a")
	giveFailures(t, "a")
	sx(t, `UPDATE portal_links SET job_detail_crawl_instructions = ? WHERE id = 'a'`, detailYAML)
	if err := UpdatePortalLinkCrawlInstructions("a", "not: [valid"); err == nil {
		t.Fatal("must be refused")
	}
	if failedAttempts(t, "a") != 2 || errorsStored(t, "a") != 1 {
		t.Error("a refused save changes nothing")
	}
}

func TestSavingARuleThatCompletesThePairClears(t *testing.T) {
	setupAttemptsDB(t, "a")
	giveFailures(t, "a")
	sx(t, `UPDATE portal_links SET job_detail_crawl_instructions = ?, link_rules = ? WHERE id = 'a'`, detailYAML, ruleJSON)
	// switching the link to the AI method is a save of the crawl settings: the link is instructed (rule or AI counts), so the failures go
	if err := SaveLinkCrawlSettings("a", CrawlMethodInstructions, nil, false); err != nil {
		t.Fatal(err)
	}
	if failedAttempts(t, "a") != 0 || errorsStored(t, "a") != 0 {
		t.Error("saving the crawl settings of an instructed link clears the failures")
	}
	giveFailures(t, "a")
	if err := SaveLinkCrawlSettings("a", CrawlMethodInstructions, nil, true); err == nil {
		// clearing the rule leaves only the details: not instructed any more, the failures given above stay
		if failedAttempts(t, "a") != 2 {
			t.Error("a link that lost its only listing instructions keeps the failures")
		}
	} else {
		t.Fatal(err)
	}
}

func TestInheritedInstructionsThatCompleteThePairClear(t *testing.T) {
	setupAttemptsDB(t, "a")
	giveFailures(t, "a")
	if _, _, err := SaveInheritedInstructions("a", listingYAML, detailYAML); err != nil {
		t.Fatal(err)
	}
	if failedAttempts(t, "a") != 0 || errorsStored(t, "a") != 0 {
		t.Error("inherited instructions complete the pair and clear")
	}
}

// The user's case: the failure arrives after the link is instructed.
func TestAFailureThatEndsAfterTheLinkIsInstructedLeavesNoTrace(t *testing.T) {
	setupAttemptsDB(t, "a", "b")
	sx(t, `UPDATE portal_links SET crawl_instructions = ?, job_detail_crawl_instructions = ? WHERE id = 'a'`, listingYAML, detailYAML)
	sx(t, `UPDATE portal_links SET crawl_instructions = ? WHERE id = 'b'`, listingYAML) // not instructed
	mustStart(t, "a", InstructionKindJobDetail)
	mustStart(t, "b", InstructionKindJobDetail)
	status, err := FinishInstructionAttempt("a", InstructionKindJobDetail, "conversation: model kept calling tools without a final reply")
	if err != nil || status != InstructionAttemptFailed {
		t.Fatalf("the client still hears 'failed': %q %v", status, err)
	}
	if failedAttempts(t, "a") != 0 {
		t.Error("an instructed link keeps no failed attempt")
	}
	if _, err := FinishInstructionAttempt("b", InstructionKindJobDetail, "boom"); err != nil {
		t.Fatal(err)
	}
	if failedAttempts(t, "b") != 1 {
		t.Error("a link that is not instructed records its failure as before")
	}
}

func TestErrorSettersStoreNothingForAnInstructedLink(t *testing.T) {
	setupAttemptsDB(t, "a", "b")
	sx(t, `UPDATE portal_links SET crawl_instructions = ?, job_detail_crawl_instructions = ? WHERE id = 'a'`, listingYAML, detailYAML)
	msg := "ran out of tool calls"
	if err := UpdatePortalLinkJobDetailInstructionsAIStatus("a", &msg); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePortalLinkInstructionsAIStatus("a", &msg); err != nil {
		t.Fatal(err)
	}
	if errorsStored(t, "a") != 0 {
		t.Error("an instructed link stores no AI error")
	}
	// a link that is not instructed stores them as before
	if err := UpdatePortalLinkJobDetailInstructionsAIStatus("b", &msg); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePortalLinkInstructionsAIStatus("b", &msg); err != nil {
		t.Fatal(err)
	}
	if count(t, `SELECT COUNT(*) FROM portal_links WHERE id = 'b' AND instructions_ai_error = ? AND job_detail_instructions_ai_error = ?`, msg, msg) != 1 {
		t.Error("a link that is not instructed stores its errors")
	}
	// clearing an error on request still works, on any link
	empty := ""
	if err := UpdatePortalLinkJobDetailInstructionsAIStatus("b", &empty); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePortalLinkInstructionsAIStatus("b", nil); err != nil {
		t.Fatal(err)
	}
	if count(t, `SELECT COUNT(*) FROM portal_links WHERE id = 'b' AND instructions_ai_error IS NULL AND job_detail_instructions_ai_error IS NULL`) != 1 {
		t.Error("an empty error clears")
	}
}

func TestStartUpSweepCleansInstructedLinksOnce(t *testing.T) {
	setupAttemptsDB(t, "ai", "rule", "half", "clean", "rod")
	for _, id := range []string{"ai", "rule", "half", "clean", "rod"} {
		giveFailures(t, id)
	}
	sx(t, `UPDATE portal_links SET crawl_instructions = ?, job_detail_crawl_instructions = ? WHERE id = 'ai'`, listingYAML, detailYAML)
	sx(t, `UPDATE portal_links SET link_rules = ?, job_detail_crawl_instructions = ? WHERE id = 'rule'`, ruleJSON, detailYAML)
	sx(t, `UPDATE portal_links SET crawl_instructions = ? WHERE id = 'half'`, listingYAML)
	sx(t, `UPDATE portal_links SET link_rules = ?, job_detail_crawl_instructions = ?, crawl_engine = 'rod' WHERE id = 'rod'`, ruleJSON, detailYAML)
	// an instructed link with nothing to clear is not counted
	sx(t, `INSERT INTO portal_links (id, portal_id, url, crawl_instructions, job_detail_crawl_instructions) VALUES ('tidy', 'p1', 'https://x/tidy', ?, ?)`, listingYAML, detailYAML)
	n, err := ClearFailuresForInstructedLinks()
	if err != nil || n != 2 {
		t.Fatalf("cleaned %d err %v, want 2 (ai and rule)", n, err)
	}
	for _, id := range []string{"ai", "rule"} {
		if failedAttempts(t, id) != 0 || errorsStored(t, id) != 0 {
			t.Errorf("%s must be clean", id)
		}
	}
	for _, id := range []string{"half", "clean", "rod"} {
		if failedAttempts(t, id) != 2 || errorsStored(t, id) != 1 {
			t.Errorf("%s is not instructed and keeps its failures", id)
		}
	}
	if n, err := ClearFailuresForInstructedLinks(); err != nil || n != 0 {
		t.Errorf("a second sweep finds nothing: %d %v", n, err)
	}
}

// What the user's data looks like: a job-list rule, job detail instructions, four failed job-detail attempts and the late error.
func TestTheIndeedLinkOfTheUser(t *testing.T) {
	setupAttemptsDB(t, "indeed")
	sx(t, `UPDATE portal_links SET link_rules = ?, job_detail_crawl_instructions = ?, job_detail_instructions_ai_error = 'conversation: model kept calling tools without a final reply: stopped after 8 rounds',
		job_detail_instructions_ai_error_at = '2026-11-08 20:14:36', job_detail_instructions_ai_error_dismissed_at = '2026-11-08 20:12:51' WHERE id = 'indeed'`, ruleJSON, detailYAML)
	sx(t, `INSERT INTO instruction_attempts (id, portal_link_id, kind, triggered_by, status) VALUES ('s', 'indeed', 'listing', 'manual', 'succeeded')`)
	for i, reason := range []string{"not_saved", "error", "not_saved", "error"} {
		sx(t, `INSERT INTO instruction_attempts (id, portal_link_id, kind, triggered_by, status, failure_reason) VALUES (?, 'indeed', 'job_detail', 'manual', 'failed', ?)`, string(rune('a'+i)), reason)
	}
	if n, err := ClearFailuresForInstructedLinks(); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	stats, err := InstructionStatsByLink()
	if err != nil {
		t.Fatal(err)
	}
	if s := stats["indeed"]; s.JobDetail.Failed != 0 || s.JobDetail.ConsecutiveFailures != 0 || s.Listing.Succeeded != 1 {
		t.Errorf("stats after the sweep: %+v", s)
	}
	if errorsStored(t, "indeed") != 0 {
		t.Error("the late error is gone")
	}
}

// An instructed link with only an error text (no failed attempt row) is cleaned too, and one with only failed attempts (no error text).
func TestStartUpSweepCleansAnErrorWithoutAttemptsAndAttemptsWithoutAnError(t *testing.T) {
	setupAttemptsDB(t, "onlyerr", "onlyfail")
	sx(t, `UPDATE portal_links SET crawl_instructions = ?, job_detail_crawl_instructions = ? WHERE id IN ('onlyerr', 'onlyfail')`, listingYAML, detailYAML)
	sx(t, `UPDATE portal_links SET job_detail_instructions_ai_error = 'x' WHERE id = 'onlyerr'`)
	sx(t, `INSERT INTO instruction_attempts (id, portal_link_id, kind, triggered_by, status, failure_reason) VALUES ('f', 'onlyfail', 'listing', 'manual', 'failed', 'error')`)
	if n, err := ClearFailuresForInstructedLinks(); err != nil || n != 2 {
		t.Fatalf("cleaned %d err %v, want 2", n, err)
	}
	if errorsStored(t, "onlyerr") != 0 || failedAttempts(t, "onlyfail") != 0 {
		t.Error("both must be clean")
	}
}
