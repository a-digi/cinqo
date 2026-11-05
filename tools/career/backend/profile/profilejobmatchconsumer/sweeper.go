package profilejobmatchconsumer

import (
	"context"
	"log"
	"time"

	"career-tool-backend/jobs"
)

// The catch-up for "career.job.description_changed" events that never arrived (step 164). Events are best effort: core drops
// one after three tries, the queue refuses one when it is full, a tool that is killed loses the ids still waiting. A job whose
// text changed after this consumer last looked at it (never, or at or before the change) is put on the queue again by the
// sweeper, where the same worker looks at it: it can only ADD the job to a board (a job on a board is never moved, one the
// person took off is never put back), so a repeat is harmless. A job the worker could not look at (a failure) stays offered; one
// it did look at, whatever it found, is not offered again until its text changes again.

const (
	// SweepLimit is how many jobs one sweep puts on the queue (the most recently changed first); the rest wait for the next one.
	SweepLimit = 200
)

// sweepStartDelay is how long after the tool starts the first sweep runs (so the databases and core are up) and sweepInterval
// how often it runs after that. Vars so a test can shrink them.
var (
	sweepStartDelay = 30 * time.Second
	sweepInterval   = 10 * time.Minute
)

// jobsToCheck is where the sweeper finds work; a var so a test can substitute it.
var jobsToCheck = jobs.JobsNeedingRulesCheck

// Sweep puts the ids of jobs whose text changed since this consumer last looked on the queue, at most SweepLimit, and returns how
// many it queued. It stops at the first id the queue has no room for (the next sweep tries again).
func (c *Consumer) Sweep() (queued int, err error) {
	ids, err := jobsToCheck(SweepLimit)
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
	return queued, nil
}

// StartSweeper launches the sweeper of the default consumer (see Consumer.StartSweeper).
func StartSweeper(ctx context.Context) <-chan struct{} { return Default.StartSweeper(ctx) }

// StartSweeper runs Sweep once after sweepStartDelay and then every sweepInterval, until ctx is cancelled, and returns a channel
// that closes when it has stopped. A failing or panicking sweep is logged and the next one still runs.
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
			log.Printf("profile job match consumer: a sweep panicked: %v", r)
		}
	}()
	queued, err := c.Sweep()
	if err != nil {
		log.Printf("profile job match consumer: a sweep could not look for changed jobs: %v", err)
		return
	}
	if queued > 0 {
		log.Printf("profile job match consumer: a sweep queued %d job(s) whose text changed since they were last checked", queued)
	}
}
