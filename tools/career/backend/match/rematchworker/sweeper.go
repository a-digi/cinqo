package rematchworker

import (
	"career-tool-backend/match"
	"context"
	"log"
	"time"
)

// The catch-up for profiles and personas whose events were missed (step 151). Events are best effort: core drops one after three tries,
// a tool that is stopped gets none, and the waiting set is in memory. So every so often the sweeper compares each profile's fingerprint
// (its set of personas) with the one stored when its jobs were last re-matched and notes the profiles whose fingerprints differ, which
// the worker then re-matches as for an event. The first time it runs it only gives the profiles that already exist their current
// fingerprint (jobs.SeedProfileMatchState), so installing the feature changes none of the existing matches. A profile that is already
// waiting is left as it is, so the sweeper never delays a re-match that is on its way. See
// plan/ai/tools/career/step-151-profile-persona-created-rematch.md.

// sweepStartDelay is how long after the tool starts the first sweep runs (so the databases and core are up) and sweepInterval how often
// it runs after that. Vars so a test can shrink them.
var (
	sweepStartDelay = 30 * time.Second
	sweepInterval   = 10 * time.Minute
)

// The sweeper's sources; vars so a test can substitute them.
var (
	seedState     = match.SeedProfileMatchState
	staleProfiles = match.StaleProfiles
)

// noteIfAbsent notes a profile that is not waiting yet and says whether it did; one that is waiting keeps its times.
func (w *Worker) noteIfAbsent(profileID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.waiting[profileID]; ok {
		return false
	}
	now := w.now()
	w.waiting[profileID] = pending{first: now, last: now}
	return true
}

// Sweep seeds the state on the first run, then notes the profiles that are stale and returns how many it newly noted. A seeding that
// fails stops the sweep (nothing would be known to be current), and the next sweep tries again.
func (w *Worker) Sweep() (noted int, err error) {
	if _, err := seedState(); err != nil {
		return 0, err
	}
	ids, err := staleProfiles()
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if w.noteIfAbsent(id) {
			noted++
		}
	}
	return noted, nil
}

// StartSweeper launches the sweeper of the default worker (see Worker.StartSweeper).
func StartSweeper(ctx context.Context) <-chan struct{} { return Default.StartSweeper(ctx) }

// StartSweeper runs Sweep once after sweepStartDelay and then every sweepInterval, until ctx is cancelled, and returns a channel that
// closes when it has stopped. A failing or panicking sweep is logged and the next one still runs.
func (w *Worker) StartSweeper(ctx context.Context) <-chan struct{} {
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
				w.sweepOnce()
				timer.Reset(sweepInterval)
			}
		}
	}()
	return done
}

// sweepOnce is one sweep that can never stop the sweeper.
func (w *Worker) sweepOnce() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("profile rematch: a sweep panicked: %v", r)
		}
	}()
	noted, err := w.Sweep()
	if err != nil {
		log.Printf("profile rematch: a sweep could not look for stale profiles: %v", err)
		return
	}
	if noted > 0 {
		log.Printf("profile rematch: a sweep found %d profile(s) to re-match", noted)
	}
}
