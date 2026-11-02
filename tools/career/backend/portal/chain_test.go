package portal

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"career-tool-backend/db"
)

// Plan 163, step 7: the whole chain from the listing instructions to complete job details, driven with fakes (no AI, no browser). The
// work the Platforms page or the run follower does (start an AI run, start a crawl, clear a flag) is played by the test with the same
// rows the real code writes; the events are delivered to the real handlers, or skipped and repaired by the real sweeper.
type chain struct {
	t      *testing.T
	stages []string
}

func (c *chain) exec(q string, args ...any) {
	c.t.Helper()
	if _, err := db.JobsDB.Exec(q, args...); err != nil {
		c.t.Fatalf("%s: %v", q, err)
	}
}

func (c *chain) stage(want string) {
	c.t.Helper()
	got, err := PipelineForLinks()
	if err != nil || len(got) != 1 {
		c.t.Fatalf("pipeline: %v %v", got, err)
	}
	if got[0].Stage != want {
		c.t.Fatalf("after %v the link is at %q, want %q", c.stages, got[0].Stage, want)
	}
	c.stages = append(c.stages, want)
}

func (c *chain) deliver(handler http.HandlerFunc, topic string) {
	c.t.Helper()
	payload, _ := json.Marshal(map[string]string{"portal_link_id": "l1"})
	body, _ := json.Marshal(db.EventEnvelope{Topic: topic, Payload: payload})
	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body)))
	if rec.Code != http.StatusAccepted {
		c.t.Fatalf("%s: %d %s", topic, rec.Code, rec.Body.String())
	}
}

// settle makes the link look unchanged for a while (the sweeper leaves a link alone that changed in the last 10 minutes).
func (c *chain) settle() {
	c.exec(`UPDATE portal_links SET updated_at = datetime('now', '-1 hour')`)
}

func (c *chain) sweep(want FlagSweepReport) {
	c.t.Helper()
	c.settle()
	got, err := SweepFlags()
	if err != nil || got != want {
		c.t.Fatalf("sweep after %v: %+v %v, want %+v", c.stages, got, err, want)
	}
}

// crawl plays one crawl of the given kind: running, then finished.
func (c *chain) crawl(id, kind string, whileRunning func(), finish func()) {
	c.exec(`INSERT INTO crawl_runs (id, portal_link_id, status, kind) VALUES (?, 'l1', 'running', ?)`, id, kind)
	whileRunning()
	finish()
	c.exec(`UPDATE crawl_runs SET status = 'completed', finished_at = datetime('now') WHERE id = ?`, id)
}

func (c *chain) flags() (listing, ai, detail bool) {
	c.t.Helper()
	if err := db.JobsDB.QueryRow(`SELECT listing_crawl_requested, job_detail_instructions_ai_requested, job_detail_crawl_requested FROM portal_links WHERE id = 'l1'`).Scan(&listing, &ai, &detail); err != nil {
		c.t.Fatal(err)
	}
	return
}

func (c *chain) wantFlags(listing, ai, detail bool) {
	c.t.Helper()
	l, a, d := c.flags()
	if l != listing || a != ai || d != detail {
		c.t.Fatalf("after %v the flags are listing=%v ai=%v detail=%v, want %v %v %v", c.stages, l, a, d, listing, ai, detail)
	}
}

func (c *chain) saveJobs() {
	c.exec(`INSERT INTO jobs (id, source_url, title, portal_link_id) VALUES ('j1', 'https://j/1', 'One', 'l1'), ('j2', 'https://j/2', 'Two', 'l1')`)
}

// detailInstructionsRun plays the AI run that writes the job-detail instructions: an attempt that runs, the save, the attempt's end and the
// consumed request flag.
func (c *chain) detailInstructionsRun() {
	c.t.Helper()
	c.exec(`INSERT INTO instruction_attempts (id, portal_link_id, kind, status, started_at) VALUES ('a-detail', 'l1', 'job_detail', 'running', datetime('now'))`)
	c.stage(StageDetailGenerating)
	if err := UpdatePortalLinkJobDetailCrawlInstructions("l1", validDetailYAML); err != nil {
		c.t.Fatal(err)
	}
	c.exec(`UPDATE instruction_attempts SET status = 'succeeded', finished_at = datetime('now') WHERE id = 'a-detail'`)
	if err := UpdatePortalLinkJobDetailInstructionsAIRequested("l1", false); err != nil {
		c.t.Fatal(err)
	}
}

func (c *chain) describeJobs() {
	c.exec(`UPDATE jobs SET description = 'the full posting'`)
}

// The chain with every event delivered, in order.
func TestChainFromListingInstructionsToCompleteWithEvents(t *testing.T) {
	setupAttemptsDB(t, "l1")
	c := &chain{t: t}

	c.stage(StageNoListing)
	if err := UpdatePortalLinkCrawlInstructions("l1", validListingYAML); err != nil {
		t.Fatal(err)
	}
	c.stage(StageListingReady) // saved, event not delivered yet
	c.deliver(ListingInstructionsReadyEventHandler, "career.portal_link.listing_instructions_ready")
	c.stage(StageListingCrawlWaiting)
	c.wantFlags(true, false, false)

	// the follower consumes the flag and starts the crawl
	if err := UpdatePortalLinkListingCrawlRequested("l1", false); err != nil {
		t.Fatal(err)
	}
	c.crawl("r1", "listing", func() { c.stage(StageListingCrawling) }, c.saveJobs)
	c.stage(StageDetailMissing)
	c.deliver(JobsCrawledEventHandler, "career.portal_link.jobs_crawled")
	c.stage(StageDetailRequested)
	c.wantFlags(false, true, true)

	c.detailInstructionsRun()
	c.stage(StageDetailCrawlWaiting) // the detail crawl flag was raised by the same event
	c.wantFlags(false, false, true)

	if err := UpdatePortalLinkJobDetailCrawlRequested("l1", false); err != nil {
		t.Fatal(err)
	}
	c.crawl("r2", "job_detail", func() { c.stage(StageDetailCrawling) }, c.describeJobs)
	c.stage(StageComplete)
	c.wantFlags(false, false, false)

	// a complete link is left alone by the sweeper
	c.sweep(FlagSweepReport{})
	c.stage(StageComplete)
}

// The same chain with BOTH events lost: the sweeper raises each flag a lost event would have raised, and nothing is raised twice.
func TestChainFromListingInstructionsToCompleteWithBothEventsLost(t *testing.T) {
	setupAttemptsDB(t, "l1")
	c := &chain{t: t}

	if err := UpdatePortalLinkCrawlInstructions("l1", validListingYAML); err != nil {
		t.Fatal(err)
	}
	c.stage(StageListingReady)
	// a sweep right after the save leaves it alone (an event may still be on its way)
	if got, err := SweepFlags(); err != nil || got != (FlagSweepReport{}) {
		t.Fatalf("a link that just changed: %+v %v", got, err)
	}
	c.sweep(FlagSweepReport{ListingCrawl: 1})
	c.stage(StageListingCrawlWaiting)
	c.sweep(FlagSweepReport{}) // idempotent

	if err := UpdatePortalLinkListingCrawlRequested("l1", false); err != nil {
		t.Fatal(err)
	}
	c.crawl("r1", "listing", func() { c.stage(StageListingCrawling) }, c.saveJobs)
	// the jobs_crawled event is lost: no flags, the link would wait for ever
	c.stage(StageDetailMissing)
	c.wantFlags(false, false, false)
	c.sweep(FlagSweepReport{DetailAI: 1})
	c.stage(StageDetailRequested)
	c.sweep(FlagSweepReport{})

	c.detailInstructionsRun()
	// the detail crawl flag was never raised either
	c.stage(StageDetailCrawlPending)
	c.sweep(FlagSweepReport{DetailCrawl: 1})
	c.stage(StageDetailCrawlWaiting)
	c.sweep(FlagSweepReport{})

	if err := UpdatePortalLinkJobDetailCrawlRequested("l1", false); err != nil {
		t.Fatal(err)
	}
	c.crawl("r2", "job_detail", func() { c.stage(StageDetailCrawling) }, c.describeJobs)
	c.stage(StageComplete)
	c.sweep(FlagSweepReport{})
}

// A crawl that ends with some jobs still lacking a description is NOT crawled again by the sweeper (no loop), and a link whose crawl
// found nothing is not re-requested either.
func TestChainSweeperDoesNotLoopOnAFinishedCrawl(t *testing.T) {
	setupAttemptsDB(t, "l1")
	c := &chain{t: t}
	if err := UpdatePortalLinkCrawlInstructions("l1", validListingYAML); err != nil {
		t.Fatal(err)
	}
	c.crawl("r1", "listing", func() {}, func() {}) // the crawl ran and found nothing
	c.sweep(FlagSweepReport{})
	c.stage(StageListingReady)

	c.saveJobs()
	if err := UpdatePortalLinkJobDetailCrawlInstructions("l1", validDetailYAML); err != nil {
		t.Fatal(err)
	}
	c.crawl("r2", "job_detail", func() {}, func() { c.exec(`UPDATE jobs SET description = 'x' WHERE id = 'j1'`) }) // one page had no text
	c.sweep(FlagSweepReport{})
	c.stage(StageDetailCrawlPending)
}
