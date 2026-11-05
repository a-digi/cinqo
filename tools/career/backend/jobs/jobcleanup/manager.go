package jobcleanup

import (
	"context"
	"errors"
	"log"
	"time"

	"career-tool-backend/db"
)

// tickInterval is how often the manager looks at the settings. It only bounds how late a due run can start; the
// run itself is due by the saved interval_hours. A var so a test can shrink it (the convention of autodiscovery's
// own tickInterval).
var tickInterval = 15 * time.Minute

// StartManager launches the background cleanup as one goroutine that lives until ctx is cancelled (the process
// lifetime in main.go). Every tick it reads the settings and does nothing unless the automatic run is on, at least
// one rule is on, and interval_hours have passed since the last run started. A run that is already going (a manual
// "Delete now") makes the tick skip: the two share runMu. The returned channel is closed when the goroutine has
// stopped (main ignores it; a test waits on it). See plan/ai/tools/career/step-131-job-cleanup.md.
func StartManager(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				checkAndRunTick(ctx)
			}
		}
	}()
	return done
}

// checkAndRunTick is one tick's work. A panic is logged, not fatal: a bug in a sweep must not stop the tool.
func checkAndRunTick(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("job cleanup: tick panicked: %v", r)
		}
	}()
	s, err := GetSettings()
	if err != nil {
		log.Printf("job cleanup: failed to load settings: %v", err)
		return
	}
	o, ok := s.automaticOptions()
	if !ok || !s.due(now(), savedAt()) {
		return
	}
	if _, err := RunCleanupContext(ctx, o); err != nil && !errors.Is(err, ErrBusy) {
		log.Printf("job cleanup: automatic run failed: %v", err)
	}
}

// automaticOptions is what the automatic run does: false when it is off or no rule is on. The rule follows the two
// toggles; the thresholds and "include manual" are the saved ones.
func (s Settings) automaticOptions() (Options, bool) {
	if !s.Enabled || (!s.OlderEnabled && !s.LowMatchEnabled) {
		return Options{}, false
	}
	rule := RuleBoth
	switch {
	case !s.LowMatchEnabled:
		rule = RuleOlder
	case !s.OlderEnabled:
		rule = RuleLowMatch
	}
	return Options{Rule: rule, OlderThanDays: s.OlderThanDays, MinMatchPercent: s.MinMatchPercent, IncludeManual: s.IncludeManual}, true
}

// savedAt is when the settings were last saved from the page (zero when never). The automatic run never starts
// sooner than interval_hours after it, so switching the run on, or changing a threshold, always leaves time to
// look at the preview and change one's mind before anything is deleted.
func savedAt() time.Time {
	var raw *string
	if err := db.JobsDB.QueryRow(`SELECT updated_at FROM job_cleanup_settings WHERE id = 1`).Scan(&raw); err != nil || raw == nil {
		return time.Time{}
	}
	return parseTime(*raw)
}

func parseTime(raw string) time.Time {
	t, err := time.ParseInLocation(timeFormat, raw, time.UTC)
	if err != nil {
		return time.Time{}
	}
	return t
}

// due says whether interval_hours have passed since the later of the last run's start (a manual run counts: it
// also stamps the clock) and the last settings save. Neither ever happening, or an unreadable stamp, counts as
// long ago.
func (s Settings) due(at, saved time.Time) bool {
	ref := parseTime(s.LastRunAt)
	if saved.After(ref) {
		ref = saved
	}
	return !at.Before(ref.Add(time.Duration(s.IntervalHours) * time.Hour))
}
