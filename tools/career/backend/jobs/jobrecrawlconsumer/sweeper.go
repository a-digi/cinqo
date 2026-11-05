package jobrecrawlconsumer

import (
	"context"
	"log"
	"time"

	"career-tool-backend/jobs/jobrecrawl"
)

// The catch-up for requests nobody is working on (step 148): events are best effort (core drops one after three tries, the queue
// refuses one when it is full, a tool that is killed loses the ids still waiting), and a job that waits for a signed-in session goes
// back to queued. Every so often the sweeper takes the queued requests that have waited a while and puts them on the same queue.

const (
	// SweepLimit is how many requests one sweep puts on the queue (oldest first); the rest wait for the next one.
	SweepLimit = 500
)

// sweepStartDelay is how long after the tool starts the first sweep runs, sweepInterval how often it runs after that and sweepAge
// how long a queued request must have waited before the sweeper takes it (so a request whose event is still on its way is left
// alone). Vars so a test can shrink them.
var (
	sweepStartDelay = 30 * time.Second
	sweepInterval   = 5 * time.Minute
	sweepAge        = 2 * time.Minute
)

// dueQueued is where the sweeper finds work; a var so a test can substitute it.
var dueQueued = jobrecrawl.DueQueued

// Sweep puts the ids of queued requests that have waited longer than sweepAge on the queue, at most SweepLimit, oldest first, and
// returns how many it queued. It stops at the first id the queue has no room for (the next sweep tries again). An id already
// waiting in the queue is not queued twice.
func (c *Consumer) Sweep() (queued int, err error) {
	ids, err := dueQueued(sweepAge, SweepLimit)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		switch c.enqueue(id) {
		case queuedNow:
			queued++
		case queueFull:
			return queued, nil
		}
	}
	return queued, nil
}

// StartSweeper launches the sweeper of the default consumer (see Consumer.StartSweeper).
func StartSweeper(ctx context.Context) <-chan struct{} { return Default.StartSweeper(ctx) }

// StartSweeper runs Sweep once after sweepStartDelay and then every sweepInterval, until ctx is cancelled, and returns a channel that
// closes when it has stopped. A failing or panicking sweep is logged and the next one still runs.
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
			log.Printf("job recrawl consumer: a sweep panicked: %v", r)
		}
	}()
	queued, err := c.Sweep()
	if err != nil {
		log.Printf("job recrawl consumer: a sweep could not look for waiting requests: %v", err)
		return
	}
	if queued > 0 {
		log.Printf("job recrawl consumer: a sweep queued %d waiting request(s)", queued)
	}
}
