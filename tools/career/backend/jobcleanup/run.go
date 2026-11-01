package jobcleanup

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"sync"
	"time"

	"career-tool-backend/comments"
	"career-tool-backend/db"
	"career-tool-backend/deletedjobs"
	"career-tool-backend/jobs"
)

// The rules (step 131): jobs added to the database more than N days ago, jobs that score under X percent for every
// profile that can judge them, or either of the two.
const (
	RuleOlder    = "older"
	RuleLowMatch = "low_match"
	RuleBoth     = "both"
)

// BatchSize is how many jobs one transaction deletes, so a run never holds the SQLite write lock for long while
// a crawl is inserting.
const BatchSize = 200

// SampleSize is how many would-be-deleted jobs a preview lists.
const SampleSize = 50

// now is the clock, a var so a test can fix it (the same convention as autodiscovery's tickInterval).
var now = time.Now

// runMu lets one cleanup run at a time; the manual "Delete now" and, later, the background ticker both take it.
var runMu sync.Mutex

var (
	// ErrBusy: another cleanup is running (a 409).
	ErrBusy = errors.New("a cleanup is already running")
	// ErrUnknownRule: the rule is not one this version knows (a 400).
	ErrUnknownRule = errors.New(`rule must be "older", "low_match" or "both"`)
)

// Options is what one preview or run uses: the saved settings, with the two values a caller may override.
type Options struct {
	Rule            string
	OlderThanDays   int
	MinMatchPercent int
	IncludeManual   bool
}

func (o Options) validate() error {
	if o.Rule != RuleOlder && o.Rule != RuleLowMatch && o.Rule != RuleBoth {
		return ErrUnknownRule
	}
	if o.OlderThanDays < MinOlderThanDays || o.OlderThanDays > MaxOlderThanDays ||
		o.MinMatchPercent < MinMatchPercentMin || o.MinMatchPercent > MinMatchPercentMax {
		return ErrInvalid
	}
	return nil
}

func (o Options) runsOlder() bool    { return o.Rule == RuleOlder || o.Rule == RuleBoth }
func (o Options) runsLowMatch() bool { return o.Rule == RuleLowMatch || o.Rule == RuleBoth }

// cutoff is the oldest crawled_at that is kept, in the format SQLite's datetime('now') writes (UTC), so the
// comparison is a plain text comparison.
func (o Options) cutoff() string {
	return now().UTC().AddDate(0, 0, -o.OlderThanDays).Format("2006-01-02 15:04:05")
}

// Sample is one job a preview says would go. BestPercent is the highest score any profile gave it, set only for a
// job the low-match rule picked (an older job that the low-match rule did not need to judge has none).
type Sample struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Company     string `json:"company"`
	CrawledAt   string `json:"crawledAt"`
	BestPercent *int   `json:"bestPercent,omitempty"`
	reason      string
}

// Kept counts the jobs a rule picked that the safety rules keep, each under the first reason that applies, in this
// order. Unjudged is separate: with the low-match rule on, a job that is not too old and that no profile could
// judge (no profile has personas, or none has enough data for it) is kept as "unknown", not as "bad".
type Kept struct {
	OnBoard    int `json:"onBoard"`
	HasCv      int `json:"hasCv"`
	InProgress int `json:"inProgress"`
	Manual     int `json:"manual"`
	Unjudged   int `json:"unjudged"`
}

// ByRule counts what each rule picked. A job both rules pick counts under Older, so the two add up to the total.
type ByRule struct {
	Older    int `json:"older"`
	LowMatch int `json:"lowMatch"`
}

// Preview is the answer of a dry run.
type Preview struct {
	// TotalJobs is how many jobs exist, so the page can warn when a run would remove most of them.
	TotalJobs   int      `json:"totalJobs"`
	WouldDelete int      `json:"wouldDelete"`
	ByRule      ByRule   `json:"byRule"`
	Kept        Kept     `json:"kept"`
	Sample      []Sample `json:"sample"`
}

// RunResult is the answer of a real run.
type RunResult struct {
	Deleted    int    `json:"deleted"`
	ByRule     ByRule `json:"byRule"`
	DurationMs int64  `json:"durationMs"`
}

// The safety rules, as SQL on a jobs row aliased j. A job is protected when any profile has it on a board, when
// it has a generated CV (or any CV generation record), or when its match is still running. A generation that is
// still running has a row in job_cv_generations, so it is covered by the second condition.
const (
	onBoardSQL    = `EXISTS (SELECT 1 FROM job_board_entries be WHERE be.job_id = j.id)`
	hasCvSQL      = `(EXISTS (SELECT 1 FROM job_cv_pdfs cp WHERE cp.job_id = j.id) OR EXISTS (SELECT 1 FROM job_cv_generations cg WHERE cg.job_id = j.id))`
	inProgressSQL = `EXISTS (SELECT 1 FROM job_matches jm WHERE jm.job_id = j.id AND jm.status = 'matching')`
)

// Scorer gives the scores (0 to 100) of every profile that could judge a job; an empty answer means none could.
type Scorer interface {
	Scores(jobID string) ([]int, error)
}

// newScorer builds the scorer for one evaluation; a var so a test can substitute a fake.
var newScorer = func() (Scorer, error) { return jobs.NewProfileScorer() }

// candidates reads every job with its safety flags and decides, rule by rule, which ones the options pick and which
// of those the safety rules keep. Nothing is written. The low-match rule scores only the jobs the older rule did not
// already pick, and all of them (protected ones too) so the kept counts are exact.
func candidates(o Options) ([]Sample, Kept, error) {
	rows, err := db.JobsDB.Query(
		`SELECT j.id, j.title, COALESCE(j.company, ''), j.crawled_at, j.source = 'manual',
			` + onBoardSQL + `, ` + hasCvSQL + `, ` + inProgressSQL + `
		 FROM jobs j ORDER BY j.crawled_at, j.id`)
	if err != nil {
		return nil, Kept{}, err
	}
	type row struct {
		Sample
		manual, board, cv, progress bool
	}
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.ID, &r.Title, &r.Company, &r.CrawledAt, &r.manual, &r.board, &r.cv, &r.progress); err != nil {
			rows.Close()
			return nil, Kept{}, err
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, Kept{}, err
	}
	rows.Close()

	var scorer Scorer
	if o.runsLowMatch() {
		if scorer, err = newScorer(); err != nil {
			return nil, Kept{}, err
		}
	}
	cutoff := o.cutoff()
	var out []Sample
	var kept Kept
	for _, r := range all {
		reason := ""
		if o.runsOlder() && r.CrawledAt < cutoff {
			reason = RuleOlder
		} else if scorer != nil {
			scores, err := scorer.Scores(r.ID)
			if errors.Is(err, jobs.ErrUnknownJob) {
				continue // deleted since the list was read
			}
			if err != nil {
				return nil, Kept{}, err
			}
			if len(scores) == 0 {
				kept.Unjudged++
				continue
			}
			best := slices.Max(scores)
			if best >= o.MinMatchPercent {
				continue
			}
			reason = RuleLowMatch
			r.BestPercent = &best
		}
		if reason == "" {
			continue
		}
		switch {
		case r.board:
			kept.OnBoard++
		case r.cv:
			kept.HasCv++
		case r.progress:
			kept.InProgress++
		case r.manual && !o.IncludeManual:
			kept.Manual++
		default:
			r.reason = reason
			out = append(out, r.Sample)
		}
	}
	return out, kept, nil
}

// PreviewCleanup says what a run with these options would delete, without writing anything.
func PreviewCleanup(o Options) (Preview, error) {
	if err := o.validate(); err != nil {
		return Preview{}, err
	}
	all, kept, err := candidates(o)
	if err != nil {
		return Preview{}, err
	}
	p := Preview{WouldDelete: len(all), ByRule: countByRule(all), Kept: kept, Sample: []Sample{}}
	p.Sample = append(p.Sample, all[:min(len(all), SampleSize)]...)
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&p.TotalJobs); err != nil {
		return Preview{}, err
	}
	return p, nil
}

func countByRule(list []Sample) ByRule {
	var b ByRule
	for _, s := range list {
		if s.reason == RuleOlder {
			b.Older++
		} else {
			b.LowMatch++
		}
	}
	return b
}

// guardedDelete deletes one job only if it still qualifies, so whatever changed since the preview (a board
// entry, a CV, a match started, a re-crawl that refreshed crawled_at) wins over the cleanup. The age is checked
// for a job picked as too old (the first two arguments); a job picked by its scores is not re-scored here, the run
// scored it moments ago.
const guardedDelete = `DELETE FROM jobs AS j WHERE j.id = ? AND (? OR j.crawled_at < ?)
	AND NOT ` + onBoardSQL + ` AND NOT ` + hasCvSQL + ` AND NOT ` + inProgressSQL + `
	AND (? OR j.source <> 'manual')`

// RunCleanup deletes what PreviewCleanup lists, BatchSize jobs per transaction, re-checking every safety rule in
// the same statement that deletes. Each batch is atomic, the run is not: a failure stops the run and keeps the
// earlier batches. The comments of the deleted jobs are purged after their batch commits.
func RunCleanup(o Options) (RunResult, error) {
	return RunCleanupContext(context.Background(), o)
}

// RunCleanupContext is RunCleanup that stops between batches when ctx is cancelled (the background manager's
// shutdown), keeping what earlier batches deleted and saying so in the last-run line.
func RunCleanupContext(ctx context.Context, o Options) (RunResult, error) {
	if err := o.validate(); err != nil {
		return RunResult{}, err
	}
	if !runMu.TryLock() {
		return RunResult{}, ErrBusy
	}
	defer runMu.Unlock()

	started := now()
	markStarted(started)
	list, _, err := candidates(o)
	if err != nil {
		return RunResult{}, recordFailure(err)
	}
	cutoff := o.cutoff()
	var byRule ByRule
	deleted := 0
	for from := 0; from < len(list); from += BatchSize {
		if err := ctx.Err(); err != nil {
			recordResult(deleted, fmt.Sprintf("%s: %d deleted, stopped: %v", o.describe(), deleted, err))
			return RunResult{Deleted: deleted, ByRule: byRule, DurationMs: since(started)}, err
		}
		batch := list[from:min(len(list), from+BatchSize)]
		done, err := deleteBatch(batch, cutoff, o.IncludeManual)
		for _, s := range done {
			deleted++
			if s.reason == RuleOlder {
				byRule.Older++
			} else {
				byRule.LowMatch++
			}
			comments.PurgeEntity(comments.JobType, s.ID)
		}
		if err != nil {
			recordResult(deleted, fmt.Sprintf("%s: %d deleted, stopped by an error: %v", o.describe(), deleted, err))
			return RunResult{Deleted: deleted, ByRule: byRule, DurationMs: since(started)}, err
		}
	}
	summary := fmt.Sprintf("%s: %d deleted", o.describe(), deleted)
	if byRule.Older > 0 && byRule.LowMatch > 0 {
		summary += fmt.Sprintf(" (%d old, %d low match)", byRule.Older, byRule.LowMatch)
	}
	recordResult(deleted, summary)
	log.Printf("job cleanup: %s", summary)
	return RunResult{Deleted: deleted, ByRule: byRule, DurationMs: since(started)}, nil
}

// describe is the rule and thresholds as the last-run line words them.
func (o Options) describe() string {
	switch o.Rule {
	case RuleOlder:
		return fmt.Sprintf("older than %d days", o.OlderThanDays)
	case RuleLowMatch:
		return fmt.Sprintf("under %d%% for every profile", o.MinMatchPercent)
	}
	return fmt.Sprintf("older than %d days or under %d%% for every profile", o.OlderThanDays, o.MinMatchPercent)
}

func deleteBatch(batch []Sample, cutoff string, includeManual bool) ([]Sample, error) {
	// A job picked for matching no profile well is remembered in the deleted-jobs database before it is deleted (step
	// 133), so the next crawl does not bring it back to be matched and deleted again. A job picked by age is not: if it
	// returns it is a genuine repost. This happens before the transaction (the memory reads JobsDB, which has one
	// connection) and is taken back for every job the guarded delete then leaves alone.
	forgets := make(map[string]func())
	for _, s := range batch {
		if s.reason == RuleLowMatch {
			forgets[s.ID] = jobs.RememberBeforeDelete(s.ID, deletedjobs.ReasonLowMatch)
		}
	}
	// takeBack undoes the entries of every remembered job that was not deleted.
	takeBack := func(deleted []Sample) {
		gone := make(map[string]bool, len(deleted))
		for _, s := range deleted {
			gone[s.ID] = true
		}
		for id, forget := range forgets {
			if !gone[id] {
				forget()
			}
		}
	}

	tx, err := db.JobsDB.Begin()
	if err != nil {
		takeBack(nil)
		return nil, err
	}
	var done []Sample
	for _, s := range batch {
		res, err := tx.Exec(guardedDelete, s.ID, s.reason != RuleOlder, cutoff, includeManual)
		if err != nil {
			_ = tx.Rollback()
			takeBack(nil)
			return nil, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			done = append(done, s)
		}
	}
	if err := tx.Commit(); err != nil {
		takeBack(nil) // nothing was deleted
		return nil, err
	}
	takeBack(done) // what the guard held back is not deleted, so it is not remembered either
	return done, nil
}

func since(t time.Time) int64 { return now().Sub(t).Milliseconds() }

// markStarted stamps last_run_at at the start of a run (UTC text) like the auto-discovery clock, so a crash
// cannot make the background run loop.
func markStarted(t time.Time) {
	if _, err := db.JobsDB.Exec(`UPDATE job_cleanup_settings SET last_run_at = ? WHERE id = 1`, t.UTC().Format(timeFormat)); err != nil {
		log.Printf("job cleanup: could not stamp the start: %v", err)
	}
}

func recordResult(deleted int, summary string) {
	if _, err := db.JobsDB.Exec(`UPDATE job_cleanup_settings SET last_run_deleted = ?, last_run_summary = ? WHERE id = 1`, deleted, summary); err != nil {
		log.Printf("job cleanup: could not save the result: %v", err)
	}
}

func recordFailure(err error) error {
	recordResult(0, "failed: "+err.Error())
	return err
}
