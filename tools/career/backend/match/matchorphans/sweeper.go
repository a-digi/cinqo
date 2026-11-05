// Package matchorphans is the catch-up of the profile and persona deleted consumers (step 152). Events are best effort (core drops one
// after a few tries, a tool that is stopped gets none) and an AI match can finish after its profile was deleted, so every so often the
// sweeper removes the match rows that name a profile or persona that no longer exists (jobs.PurgeOrphans) and hands the jobs that
// lost their match to the job-match worker, so the profiles that remain match them. The first sweep also removes the orphans that
// were left behind before the consumers existed. See plan/ai/tools/career/step-152-profile-persona-deleted-events.md.
package matchorphans

import (
	"context"
	"log"
	"time"

	"career-tool-backend/jobs"
	"career-tool-backend/match/jobmatchconsumer"
)

// sweepStartDelay is how long after the tool starts the first sweep runs (so the databases and core are up) and sweepInterval how often
// it runs after that. Vars so a test can shrink them.
var (
	sweepStartDelay = 30 * time.Second
	sweepInterval   = 10 * time.Minute
)

// What the sweep uses; vars so a test can watch the calls and make them fail.
var (
	purgeOrphans = jobs.PurgeOrphans
	handOver     = jobmatchconsumer.Enqueue
)

// Sweep removes the orphan matches and hands the freed jobs over. It returns what was removed.
func Sweep() (jobs.OrphanReport, error) {
	report, freed, err := purgeOrphans()
	if err != nil {
		return report, err
	}
	if len(freed) > 0 {
		handOver(freed)
	}
	return report, nil
}

// StartSweeper runs Sweep once after sweepStartDelay and then every sweepInterval, until ctx is cancelled, and returns a channel that
// closes when it has stopped. A failing or panicking sweep is logged and the next one still runs.
func StartSweeper(ctx context.Context) <-chan struct{} {
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
				sweepOnce()
				timer.Reset(sweepInterval)
			}
		}
	}()
	return done
}

// sweepOnce is one sweep that can never stop the sweeper.
func sweepOnce() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("match orphans: a sweep panicked: %v", r)
		}
	}()
	report, err := Sweep()
	if err != nil {
		log.Printf("match orphans: a sweep could not remove the orphans: %v", err)
		return
	}
	if report.Matches+report.PersonaScores+report.States > 0 {
		log.Printf("match orphans: a sweep removed %d match(es), %d persona score(s) and %d state row(s) of %d deleted profile(s) and %d deleted persona(s)",
			report.Matches, report.PersonaScores, report.States, report.Profiles, report.Personas)
	}
}
