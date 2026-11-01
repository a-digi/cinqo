package personamatchconsumer

import (
	"context"
	"log"
	"time"

	"career-tool-backend/personamatch"
)

// The catch-up for jobs that have no persona scores (step 141): the jobs that existed before this feature, and those whose
// event was missed (events are best effort: core drops one after three tries, the queue refuses one when it is full, a
// tool that is killed loses the ids still waiting). Every so often the sweeper looks for jobs without any row and puts
// them on the same queue, where the same worker scores them. A job a persona already has a row for is never offered. A
// job that no persona can judge never gets a row, so it is offered again at every sweep: that costs one cheap scoring.

const (
	// SweepLimit is how many jobs one sweep puts on the queue (newest first); the rest wait for the next one.
	SweepLimit = 500
)

// sweepStartDelay is how long after the tool starts the first sweep runs (so the databases and core are up) and
// sweepInterval how often it runs after that. Vars so a test can shrink them.
var (
	sweepStartDelay = 30 * time.Second
	sweepInterval   = 10 * time.Minute
)

// jobsWithoutRows is where the sweeper finds work; a var so a test can substitute it.
var jobsWithoutRows = personamatch.JobsWithoutRows

// Sweep puts the ids of jobs without rows on the queue, at most SweepLimit, newest first, and returns how many it queued.
// It stops at the first id the queue has no room for (the next sweep tries again).
func (c *Consumer) Sweep() (queued int, err error) {
	ids, err := jobsWithoutRows(SweepLimit)
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
			log.Printf("persona match consumer: a sweep panicked: %v", r)
		}
	}()
	queued, err := c.Sweep()
	if err != nil {
		log.Printf("persona match consumer: a sweep could not look for jobs without scores: %v", err)
		return
	}
	if queued > 0 {
		log.Printf("persona match consumer: a sweep queued %d job(s) without persona scores", queued)
	}
}
