// pipeline.go (plan/ai/tools/career/step-163-job-detail-instructions-pipeline.md, step 1) answers, for every portal link, "where is it in the
// chain from the listing instructions to a complete set of jobs?". The chain is: listing instructions written, listing crawled, job-detail
// instructions written, job details crawled. Nothing here changes anything: the stage is derived from data that already exists (the stored
// instructions, the request flags, the running attempts and crawls, the saved jobs), by a pure function, so it can be tested stage by stage.
package portal

import (
	"strconv"
	"time"

	"career-tool-backend/db"
)

// The stages, in the order the chain goes through them (a few are waiting states of the same step).
const (
	StageListingQuestion     = "listing_question"
	StageListingGenerating   = "listing_instructions_generating"
	StageNoListing           = "no_listing_instructions"
	StageListingCrawling     = "listing_crawling"
	StageDetailGenerating    = "detail_instructions_generating"
	StageDetailCrawling      = "detail_crawling"
	StageListingCrawlWaiting = "listing_crawl_waiting"
	StageListingReady        = "listing_ready"
	StageDetailRequested     = "detail_instructions_requested"
	StageDetailFailed        = "detail_instructions_failed"
	StageDetailMissing       = "detail_instructions_missing"
	StageDetailCrawlWaiting  = "detail_crawl_waiting"
	StageDetailCrawlPending  = "detail_crawl_pending"
	StageComplete            = "complete"
)

// What the link is waiting for.
const (
	WaitUser    = "user"    // a person has to do something (answer, generate, crawl)
	WaitPage    = "page"    // a request flag is set and an open Platforms page will act on it
	WaitAI      = "ai"      // an AI run is writing instructions
	WaitCrawl   = "crawl"   // a crawl is running
	WaitNothing = "nothing" // nothing is pending: the chain is complete
)

// PipelineSnapshot is everything the stage is derived from, for one link.
type PipelineSnapshot struct {
	HasListingInstructions bool
	HasDetailInstructions  bool
	// The three request flags of plan 65, raised by the events and consumed by an open page.
	ListingCrawlRequested bool
	DetailAIRequested     bool
	DetailCrawlRequested  bool
	// A question of the generation agent ("does this page list jobs?") is waiting for the person.
	QuestionPending bool
	// Running work.
	ListingGenerating bool
	DetailGenerating  bool
	ListingCrawling   bool
	DetailCrawling    bool
	// The saved jobs of the link, and how many of them have a description.
	Jobs                int
	JobsWithDescription int
	// How many job-detail instruction attempts failed.
	DetailFailedRuns int
}

// PipelineStage is where a link is and what it waits for.
type PipelineStage struct {
	Stage      string
	WaitingFor string
}

// StageOf is the decision table, first match wins. Running work comes before waiting flags, flags before the next missing piece.
func StageOf(s PipelineSnapshot) PipelineStage {
	switch {
	case s.QuestionPending:
		return PipelineStage{StageListingQuestion, WaitUser}
	case s.ListingGenerating:
		return PipelineStage{StageListingGenerating, WaitAI}
	case !s.HasListingInstructions:
		return PipelineStage{StageNoListing, WaitUser}
	case s.ListingCrawling:
		return PipelineStage{StageListingCrawling, WaitCrawl}
	case s.DetailGenerating:
		return PipelineStage{StageDetailGenerating, WaitAI}
	case s.DetailCrawling:
		return PipelineStage{StageDetailCrawling, WaitCrawl}
	case s.ListingCrawlRequested:
		return PipelineStage{StageListingCrawlWaiting, WaitPage}
	case s.Jobs == 0:
		return PipelineStage{StageListingReady, WaitUser}
	case !s.HasDetailInstructions:
		switch {
		case s.DetailAIRequested:
			return PipelineStage{StageDetailRequested, WaitPage}
		case s.DetailFailedRuns > 0:
			return PipelineStage{StageDetailFailed, WaitUser}
		default:
			return PipelineStage{StageDetailMissing, WaitUser}
		}
	case s.DetailCrawlRequested:
		return PipelineStage{StageDetailCrawlWaiting, WaitPage}
	case s.JobsWithDescription < s.Jobs:
		return PipelineStage{StageDetailCrawlPending, WaitUser}
	default:
		return PipelineStage{StageComplete, WaitNothing}
	}
}

// PipelineDetail are the numbers a stage's text needs.
type PipelineDetail struct {
	Jobs                int `json:"jobs"`
	JobsWithDescription int `json:"jobsWithDescription"`
	DetailFailedRuns    int `json:"detailFailedRuns"`
}

// PipelineLink is one link's stage as the route sends it. Since is when the running work started (empty for a stage that is not running).
type PipelineLink struct {
	PortalLinkID string         `json:"portalLinkId"`
	Stage        string         `json:"stage"`
	WaitingFor   string         `json:"waitingFor"`
	Since        string         `json:"since,omitempty"`
	Detail       PipelineDetail `json:"detail"`
}

// pipelineRow is one link with the snapshot its stage is derived from.
type pipelineRow struct {
	id   string
	snap PipelineSnapshot
}

// loadPipelineRows reads the snapshot of every link (oldest link first) and when the running attempts and crawls of each started
// (keyed "linkID|kind"). It only reads: a running attempt that is older than the stale threshold is left out (the portals list heals
// those by writing, this does not). Shared by the pipeline route and the flag sweeper.
func loadPipelineRows() ([]pipelineRow, map[string]string, map[string]string, error) {
	var links []pipelineRow

	rows, err := db.JobsDB.Query(
		`SELECT id,
		        COALESCE(TRIM(crawl_instructions), '') <> '',
		        COALESCE(TRIM(job_detail_crawl_instructions), '') <> '',
		        listing_crawl_requested, job_detail_instructions_ai_requested, job_detail_crawl_requested,
		        jobs_question_asked_at IS NOT NULL AND jobs_question_asked_at <> ''
		 FROM portal_links ORDER BY created_at ASC, rowid ASC`)
	if err != nil {
		return nil, nil, nil, err
	}
	for rows.Next() {
		var l pipelineRow
		var listing, detail, listingReq, detailAIReq, detailCrawlReq, question bool
		if err := rows.Scan(&l.id, &listing, &detail, &listingReq, &detailAIReq, &detailCrawlReq, &question); err != nil {
			rows.Close()
			return nil, nil, nil, err
		}
		l.snap = PipelineSnapshot{
			HasListingInstructions: listing, HasDetailInstructions: detail,
			ListingCrawlRequested: listingReq, DetailAIRequested: detailAIReq, DetailCrawlRequested: detailCrawlReq,
			QuestionPending: question,
		}
		links = append(links, l)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, nil, err
	}
	rows.Close() // one connection only: the next reads start after this result set is closed

	index := map[string]int{}
	for i, l := range links {
		index[l.id] = i
	}
	cutoff := "-" + strconv.FormatInt(int64(staleInstructionAttemptThreshold/time.Second), 10) + " seconds"
	attempts, err := db.JobsDB.Query(
		`SELECT portal_link_id, kind, started_at FROM instruction_attempts
		 WHERE status = 'running' AND started_at >= datetime('now', ?)`, cutoff)
	if err != nil {
		return nil, nil, nil, err
	}
	startedAttempt := map[string]string{}
	for attempts.Next() {
		var id, kind, started string
		if err := attempts.Scan(&id, &kind, &started); err != nil {
			attempts.Close()
			return nil, nil, nil, err
		}
		if i, ok := index[id]; ok {
			if kind == InstructionKindJobDetail {
				links[i].snap.DetailGenerating = true
			} else {
				links[i].snap.ListingGenerating = true
			}
			startedAttempt[id+"|"+kind] = started
		}
	}
	if err := attempts.Err(); err != nil {
		attempts.Close()
		return nil, nil, nil, err
	}
	attempts.Close()

	failed, err := db.JobsDB.Query(
		`SELECT portal_link_id, COUNT(*) FROM instruction_attempts WHERE kind = 'job_detail' AND status = 'failed' GROUP BY portal_link_id`)
	if err != nil {
		return nil, nil, nil, err
	}
	for failed.Next() {
		var id string
		var n int
		if err := failed.Scan(&id, &n); err != nil {
			failed.Close()
			return nil, nil, nil, err
		}
		if i, ok := index[id]; ok {
			links[i].snap.DetailFailedRuns = n
		}
	}
	if err := failed.Err(); err != nil {
		failed.Close()
		return nil, nil, nil, err
	}
	failed.Close()

	crawls, err := db.JobsDB.Query(`SELECT portal_link_id, kind, started_at FROM crawl_runs WHERE status = 'running'`)
	if err != nil {
		return nil, nil, nil, err
	}
	startedCrawl := map[string]string{}
	for crawls.Next() {
		var id, kind, started string
		if err := crawls.Scan(&id, &kind, &started); err != nil {
			crawls.Close()
			return nil, nil, nil, err
		}
		if i, ok := index[id]; ok {
			if kind == "job_detail" {
				links[i].snap.DetailCrawling = true
			} else {
				links[i].snap.ListingCrawling = true
			}
			startedCrawl[id+"|"+kind] = started
		}
	}
	if err := crawls.Err(); err != nil {
		crawls.Close()
		return nil, nil, nil, err
	}
	crawls.Close()

	jobs, err := db.JobsDB.Query(
		`SELECT portal_link_id, COUNT(*), COALESCE(SUM(COALESCE(TRIM(description), '') <> ''), 0)
		 FROM jobs WHERE portal_link_id IS NOT NULL GROUP BY portal_link_id`)
	if err != nil {
		return nil, nil, nil, err
	}
	for jobs.Next() {
		var id string
		var total, withDescription int
		if err := jobs.Scan(&id, &total, &withDescription); err != nil {
			jobs.Close()
			return nil, nil, nil, err
		}
		if i, ok := index[id]; ok {
			links[i].snap.Jobs, links[i].snap.JobsWithDescription = total, withDescription
		}
	}
	if err := jobs.Err(); err != nil {
		jobs.Close()
		return nil, nil, nil, err
	}
	jobs.Close()
	return links, startedAttempt, startedCrawl, nil
}

// PipelineForLinks loads the stage of every link (oldest link first). It only reads; nothing is repaired or requested.
func PipelineForLinks() ([]PipelineLink, error) {
	links, startedAttempt, startedCrawl, err := loadPipelineRows()
	if err != nil {
		return nil, err
	}
	since := map[string]string{} // the start of the running work that decides the stage, by link
	out := make([]PipelineLink, 0, len(links))
	for _, l := range links {
		st := StageOf(l.snap)
		switch st.Stage {
		case StageListingGenerating:
			since[l.id] = startedAttempt[l.id+"|"+InstructionKindListing]
		case StageDetailGenerating:
			since[l.id] = startedAttempt[l.id+"|"+InstructionKindJobDetail]
		case StageListingCrawling:
			since[l.id] = startedCrawl[l.id+"|"+InstructionKindListing]
		case StageDetailCrawling:
			since[l.id] = startedCrawl[l.id+"|job_detail"]
		}
		out = append(out, PipelineLink{
			PortalLinkID: l.id, Stage: st.Stage, WaitingFor: st.WaitingFor, Since: since[l.id],
			Detail: PipelineDetail{Jobs: l.snap.Jobs, JobsWithDescription: l.snap.JobsWithDescription, DetailFailedRuns: l.snap.DetailFailedRuns},
		})
	}
	return out, nil
}
