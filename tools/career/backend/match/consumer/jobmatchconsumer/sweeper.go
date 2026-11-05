package jobmatchconsumer

import (
	"career-tool-backend/match"
	profilematch "career-tool-backend/match/profile"
	"context"
	"log"
	"time"
)

// The catch-up for events that were missed (step 134). Events are best effort: core drops one after three tries, the
// queue refuses one when it is full, and a tool that is killed loses the ids still waiting. So every so often the
// sweeper looks for jobs that have no match at all and puts them on the same queue, where the same worker matches them.
// A job that has a match of any kind is never offered, so nothing is ever replaced. See
// plan/ai/tools/career/step-134-match-missed-jobs.md.

const (
	// SweepLimit is how many unmatched jobs one sweep puts on the queue (newest first); the rest wait for the next one.
	SweepLimit = 200
)

// sweepStartDelay is how long after the tool starts the first sweep runs (so the databases and core are up) and
// sweepInterval how often it runs after that. Vars so a test can shrink them.
var (
	sweepStartDelay = 30 * time.Second
	sweepInterval   = 10 * time.Minute
)

// unmatchedJobIDs and staleMatchJobIDs are where the sweeper finds work; vars so a test can substitute them. The second is the
// catch-up for a lost "career.job.description_changed" event (step 164): jobs whose manual match is older than their text.
var (
	unmatchedJobIDs  = profilematch.UnmatchedJobIDs
	staleMatchJobIDs = match.StaleManualMatchJobIDs
)

// Sweep puts the ids of jobs without a match on the queue for new jobs, at most SweepLimit, newest first, and the ids of jobs
// whose manual match is older than their text on the queue for changed jobs (step 164), at most SweepLimit, and returns how many
// it queued in all. It stops at the first id a queue has no room for (the next sweep tries again). An id that is already
// waiting from an event is queued a second time, which is harmless: the matcher skips a job that has a row, and the re-matcher
// makes the same result again. A failure to read the stale ones is logged and does not stop the rest.
func (c *Consumer) Sweep() (queued int, err error) {
	ids, err := unmatchedJobIDs(SweepLimit)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		select {
		case c.queue <- id:
			queued++
		default:
			return queued, nil
		}
	}
	stale, staleErr := staleMatchJobIDs(SweepLimit)
	if staleErr != nil {
		log.Printf("job match consumer: a sweep could not look for matches older than their job's text: %v", staleErr)
		return queued, nil
	}
	for _, id := range stale {
		select {
		case c.changed <- id:
			queued++
		default:
			return queued, nil
		}
	}
	return queued, nil
}

// StartSweeper launches the sweeper of the default consumer (see Consumer.StartSweeper).
func StartSweeper(ctx context.Context) <-chan struct{} { return Default.StartSweeper(ctx) }

// StartSweeper runs Sweep once after sweepStartDelay and then every sweepInterval, until ctx is cancelled, and returns a
// channel that closes when it has stopped. A failing or panicking sweep is logged and the next one still runs.
func (c *Consumer) StartSweeper(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		timer := time.NewTimer(sweepStartDelay)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				c.sweepOnce()
				timer.Reset(sweepInterval)
			}
		}
	}()
	return done
}

// sweepOnce is one sweep that can never stop the sweeper.
func (c *Consumer) sweepOnce() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("job match consumer: a sweep panicked: %v", r)
		}
	}()
	queued, err := c.Sweep()
	if err != nil {
		log.Printf("job match consumer: a sweep could not look for unmatched jobs: %v", err)
		return
	}
	if queued > 0 {
		log.Printf("job match consumer: a sweep queued %d unmatched job(s)", queued)
	}
}
