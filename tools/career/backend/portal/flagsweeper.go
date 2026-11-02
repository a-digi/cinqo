// flagsweeper.go (plan/ai/tools/career/step-163-job-detail-instructions-pipeline.md, step 5) repairs the request flags that a lost event
// would have raised. The chain from the listing instructions to the job details is driven by events (listing instructions ready, jobs
// crawled) that set the flags the run follower and the Platforms page consume; an event is best effort (core drops one after a few tries,
// a tool that is stopped gets none), so without a repair a link could wait for ever. Every few minutes the sweeper looks at every link and
// raises the flag that is missing. It only raises flags (the same UPDATE the event handlers do); it cannot start an AI run or a crawl, the
// follower or the page does that. It is idempotent and never touches a link with running work or a link that changed lately.
package portal

import (
	"context"
	"log"
	"strconv"
	"time"

	"career-tool-backend/db"
)

// maxDetailFailedRuns is the number of failed job-detail instruction runs after which the link is left alone ("probably can't be crawled").
const maxDetailFailedRuns = 3

// FlagSweepSnapshot is what the repair decision needs: the link's pipeline snapshot plus how settled and how often it was crawled.
type FlagSweepSnapshot struct {
	PipelineSnapshot
	// Settled: the link row did not change in the last settleAfter, so an event that is still on its way has had time to arrive.
	Settled bool
	// How many crawl runs of each kind the link ever had (any outcome). A link that was crawled and found nothing, or whose detail pages
	// were visited and still lack a description, is not a lost flag: re-requesting it would crawl it again every sweep.
	ListingCrawlRuns int
	DetailCrawlRuns  int
}

// FlagRepairs are the flags to raise for one link.
type FlagRepairs struct {
	ListingCrawl bool
	DetailAI     bool
	DetailCrawl  bool
}

// RepairsFor is the pure decision. No flag is raised for a link with running work, a pending question, no listing instructions, or one that
// is not settled.
func RepairsFor(s FlagSweepSnapshot) FlagRepairs {
	var r FlagRepairs
	if !s.Settled || !s.HasListingInstructions || s.QuestionPending ||
		s.ListingGenerating || s.DetailGenerating || s.ListingCrawling || s.DetailCrawling {
		return r
	}
	// Listing instructions saved, the event lost, the link never crawled.
	r.ListingCrawl = !s.ListingCrawlRequested && s.Jobs == 0 && s.ListingCrawlRuns == 0
	// Jobs saved, no job-detail instructions, nobody asked for them, and not given up on after repeated failures.
	r.DetailAI = !s.HasDetailInstructions && !s.DetailAIRequested && s.Jobs > 0 && s.DetailFailedRuns < maxDetailFailedRuns
	// Job-detail instructions exist, jobs lack a description, the detail pages were never visited, nobody asked.
	r.DetailCrawl = s.HasDetailInstructions && !s.DetailCrawlRequested && s.JobsWithDescription < s.Jobs && s.DetailCrawlRuns == 0
	return r
}

// What the sweeper uses; vars so a test can shrink the times.
var (
	flagSweepStartDelay  = 45 * time.Second
	flagSweepInterval    = 5 * time.Minute
	flagSweepSettleAfter = 10 * time.Minute
	// Called after the links were read and before any flag is written; a test raises a flag here to play a late event.
	afterFlagSweepRead = func() {}
)

// loadFlagSweepSnapshots reads every link's snapshot (read only).
func loadFlagSweepSnapshots() ([]string, []FlagSweepSnapshot, error) {
	rows, _, _, err := loadPipelineRows()
	if err != nil {
		return nil, nil, err
	}
	index := map[string]int{}
	snaps := make([]FlagSweepSnapshot, len(rows))
	ids := make([]string, len(rows))
	for i, r := range rows {
		index[r.id] = i
		ids[i] = r.id
		snaps[i].PipelineSnapshot = r.snap
	}

	cutoff := "-" + strconv.FormatInt(int64(flagSweepSettleAfter/time.Second), 10) + " seconds"
	settled, err := db.JobsDB.Query(`SELECT id FROM portal_links WHERE updated_at IS NULL OR updated_at <= datetime('now', ?)`, cutoff)
	if err != nil {
		return nil, nil, err
	}
	for settled.Next() {
		var id string
		if err := settled.Scan(&id); err != nil {
			settled.Close()
			return nil, nil, err
		}
		if i, ok := index[id]; ok {
			snaps[i].Settled = true
		}
	}
	if err := settled.Err(); err != nil {
		settled.Close()
		return nil, nil, err
	}
	settled.Close()

	runs, err := db.JobsDB.Query(`SELECT portal_link_id, kind, COUNT(*) FROM crawl_runs GROUP BY portal_link_id, kind`)
	if err != nil {
		return nil, nil, err
	}
	for runs.Next() {
		var id, kind string
		var n int
		if err := runs.Scan(&id, &kind, &n); err != nil {
			runs.Close()
			return nil, nil, err
		}
		if i, ok := index[id]; ok {
			if kind == "job_detail" {
				snaps[i].DetailCrawlRuns = n
			} else {
				snaps[i].ListingCrawlRuns = n
			}
		}
	}
	if err := runs.Err(); err != nil {
		runs.Close()
		return nil, nil, err
	}
	runs.Close()
	return ids, snaps, nil
}

// FlagSweepReport counts the flags one sweep raised.
type FlagSweepReport struct {
	ListingCrawl int
	DetailAI     int
	DetailCrawl  int
}

// SweepFlags raises the missing flags of every link. A flag is raised only if it is still unset at the moment of the write (the UPDATE
// names the condition), so a flag a person or an event changed in between is never overwritten.
func SweepFlags() (FlagSweepReport, error) {
	var report FlagSweepReport
	ids, snaps, err := loadFlagSweepSnapshots()
	if err != nil {
		return report, err
	}
	afterFlagSweepRead()
	raise := func(id, column string) (bool, error) {
		res, err := db.JobsDB.Exec(`UPDATE portal_links SET `+column+` = 1, updated_at = datetime('now') WHERE id = ? AND `+column+` = 0`, id)
		if err != nil {
			return false, err
		}
		n, _ := res.RowsAffected()
		return n > 0, nil
	}
	for i, snap := range snaps {
		repairs := RepairsFor(snap)
		for _, step := range []struct {
			wanted bool
			column string
			count  *int
		}{
			{repairs.ListingCrawl, "listing_crawl_requested", &report.ListingCrawl},
			{repairs.DetailAI, "job_detail_instructions_ai_requested", &report.DetailAI},
			{repairs.DetailCrawl, "job_detail_crawl_requested", &report.DetailCrawl},
		} {
			if !step.wanted {
				continue
			}
			raised, err := raise(ids[i], step.column)
			if err != nil {
				return report, err
			}
			if raised {
				*step.count++
			}
		}
	}
	return report, nil
}

// StartFlagSweeper runs SweepFlags once after flagSweepStartDelay and then every flagSweepInterval, until ctx is cancelled, and returns a
// channel that closes when it has stopped. A failing or panicking sweep is logged and the next one still runs.
func StartFlagSweeper(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		timer := time.NewTimer(flagSweepStartDelay)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				sweepFlagsOnce()
				timer.Reset(flagSweepInterval)
			}
		}
	}()
	return done
}

func sweepFlagsOnce() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("portal flag sweeper: a sweep panicked: %v", r)
		}
	}()
	report, err := SweepFlags()
	if err != nil {
		log.Printf("portal flag sweeper: a sweep failed: %v", err)
		return
	}
	if report.ListingCrawl+report.DetailAI+report.DetailCrawl > 0 {
		log.Printf("portal flag sweeper: re-requested %d listing crawl(s), %d job-detail instruction run(s) and %d job-detail crawl(s) that an event should have requested",
			report.ListingCrawl, report.DetailAI, report.DetailCrawl)
	}
}
