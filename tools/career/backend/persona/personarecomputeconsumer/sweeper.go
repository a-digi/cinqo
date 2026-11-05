package personarecomputeconsumer

import (
	"context"
	"log"
	"time"

	"career-tool-backend/jobs"
)

// The catch-up for persona changes that were missed (step 142). Events are best effort: core drops one after three
// tries, a tool that is stopped gets none, and the waiting set is in memory. So every so often the sweeper compares
// each persona's fingerprint (what the matcher reads of it plus the engine version) with the one stored when its scores
// were last made, and notes the personas whose fingerprints differ, which the worker then recomputes as for an event.
// This also brings in the personas that have no state yet (the ones that exist when this is first installed) and every
// persona after an engine version change. A persona that is already waiting is left as it is, so the sweeper never delays
// a recompute that is on its way. See plan/ai/tools/career/step-142-persona-changed-rescore.md.

// sweepStartDelay is how long after the tool starts the first sweep runs (so the databases and core are up) and
// sweepInterval how often it runs after that. Vars so a test can shrink them.
var (
	sweepStartDelay = 30 * time.Second
	sweepInterval   = 10 * time.Minute
)

// stalePersonas is where the sweeper finds work; a var so a test can substitute it.
var stalePersonas = jobs.StalePersonas

// noteIfAbsent notes a persona that is not waiting yet and says whether it did; one that is waiting keeps its times.
func (c *Consumer) noteIfAbsent(personaID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.waiting[personaID]; ok {
		return false
	}
	now := c.now()
	c.waiting[personaID] = pending{first: now, last: now}
	return true
}

// Sweep notes the personas whose scores are stale and returns how many it newly noted.
func (c *Consumer) Sweep() (noted int, err error) {
	ids, err := stalePersonas()
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if c.noteIfAbsent(id) {
			noted++
		}
	}
	return noted, nil
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
			log.Printf("persona recompute: a sweep panicked: %v", r)
		}
	}()
	noted, err := c.Sweep()
	if err != nil {
		log.Printf("persona recompute: a sweep could not look for stale personas: %v", err)
		return
	}
	if noted > 0 {
		log.Printf("persona recompute: a sweep found %d persona(s) with stale scores", noted)
	}
}
