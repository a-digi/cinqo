package profilejobmatchconsumer

import (
	"context"
	"errors"
	"fmt"

	"career-tool-backend/jobboard"
	"career-tool-backend/jobs"
	"career-tool-backend/profile"
	"career-tool-backend/profilerules"
)

// Applying a profile's auto-add rules to the jobs that already exist (step 143). The rules otherwise act only on jobs found
// after they were made (this package's consumer). The pass is the consumer's own: the same fresh score of the profile for
// each job (the manual engine, no AI), the same "first rule of the list that fits" decision, the same insert-only board
// write, so a card that is on the board is never moved. See plan/ai/tools/career/step-143-apply-rules-to-existing-jobs.md.

// ErrNoRules is returned for a profile that has no rules: there is nothing to apply.
var ErrNoRules = errors.New("this profile has no rules")

// applyChunk is how many jobs one step of the pass reads; a var so a test can make it small.
var applyChunk = 200

// listJobIDs, isOnBoard, wasRemoved and afterApplyChunk are seams for tests: the job lister, the dry run's board checks, and a
// hook run after each chunk is done.
var (
	listJobIDs      = jobs.JobIDsAfter
	isOnBoard       = jobboard.IsOnBoard
	wasRemoved      = jobboard.WasRemoved
	afterApplyChunk = func() {}
)

// StatusCount is how many jobs were added to (or, in a dry run, would be added to) a status.
type StatusCount struct {
	StatusID   string `json:"statusId"`
	StatusName string `json:"statusName"`
	Count      int    `json:"count"`
}

// ApplySummary says what a pass did, or in a dry run what it would do. Jobs is how many jobs were looked at; each is in
// exactly one of NotJudged, Below, OnBoard, Removed or Total (added or to be added).
type ApplySummary struct {
	Jobs      int           `json:"jobs"`
	NotJudged int           `json:"notJudged"` // the profile cannot judge the job (no personas, nothing to compare)
	Below     int           `json:"below"`     // no rule fits the job's score
	OnBoard   int           `json:"onBoard"`   // already on this profile's board (any status), left alone
	Removed   int           `json:"removed"`   // fits a rule but the person took it off this board earlier: left off (step 144)
	Total     int           `json:"total"`     // added (or to be added), the sum of Statuses
	Statuses  []StatusCount `json:"statuses"`  // per status, in the order of the rules that use them, only those with a count
}

// ApplyToExisting runs the profile's rules over every existing job, in chunks of applyChunk by id: a job already on the
// board is left alone, one the profile cannot judge or that no rule fits is skipped, and the others are added with the
// status of the first rule of the list that fits. A job the person took off this board earlier is left off and counted
// as Removed, unless includeRemoved is set: then it is added like any other (and the removal is forgotten). With dryRun
// nothing is written and the summary says what a real run would do. Never moves, removes or changes a card, never touches another profile's board.
//
// persona-less profiles judge nothing (every job NotJudged). ErrNoRules for a profile without rules and
// profile.ErrUnknownProfile for an unknown one, both before anything is read. A job deleted meanwhile is skipped and not
// counted. The context is checked between chunks; on a cancellation or a failure the summary of what was done so far
// comes back with the error, and what was added stays (a repeat finds it on the board).
func ApplyToExisting(ctx context.Context, profileID string, dryRun, includeRemoved bool) (ApplySummary, error) {
	if err := profile.RequireProfileExists(profileID); err != nil {
		return ApplySummary{}, err
	}
	rules, err := profilerules.List(profileID)
	if err != nil {
		return ApplySummary{}, err
	}
	if len(rules) == 0 {
		return ApplySummary{}, ErrNoRules
	}
	sc, err := newScorer(map[string]bool{profileID: true})
	if err != nil {
		return ApplySummary{}, fmt.Errorf("could not read the profile: %w", err)
	}
	counts := map[string]int{}
	var sum ApplySummary
	finish := func() ApplySummary {
		sum.Statuses = []StatusCount{}
		seen := map[string]bool{}
		for _, r := range rules { // in the order of the rules, a status once however many rules use it
			if seen[r.StatusID] || counts[r.StatusID] == 0 {
				continue
			}
			seen[r.StatusID] = true
			sum.Statuses = append(sum.Statuses, StatusCount{StatusID: r.StatusID, StatusName: r.StatusName, Count: counts[r.StatusID]})
			sum.Total += counts[r.StatusID]
		}
		return sum
	}
	after := ""
	for {
		if err := ctx.Err(); err != nil {
			return finish(), err
		}
		ids, err := listJobIDs(after, applyChunk)
		if err != nil {
			return finish(), err
		}
		if len(ids) == 0 {
			break
		}
		for _, jobID := range ids {
			if err := applyOne(sc, rules, profileID, jobID, dryRun, includeRemoved, &sum, counts); err != nil {
				return finish(), err
			}
		}
		after = ids[len(ids)-1]
		afterApplyChunk()
	}
	return finish(), nil
}

// applyOne handles one job and updates the counts. A job that no longer exists is not counted at all.
func applyOne(sc scorer, rules []profilerules.Rule, profileID, jobID string, dryRun, includeRemoved bool, sum *ApplySummary, counts map[string]int) error {
	scores, err := sc.ScoreByProfile(jobID)
	if errors.Is(err, jobs.ErrUnknownJob) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("could not score job %s: %w", jobID, err)
	}
	score, judged := scores[profileID]
	if !judged {
		sum.Jobs++
		sum.NotJudged++
		return nil
	}
	rule, ok := profilerules.RuleFor(rules, score)
	if !ok {
		sum.Jobs++
		sum.Below++
		return nil
	}
	if dryRun {
		onBoard, err := isOnBoard(profileID, jobID)
		if err != nil {
			return err
		}
		removed := false
		if !onBoard && !includeRemoved {
			if removed, err = wasRemoved(profileID, jobID); err != nil {
				return err
			}
		}
		sum.Jobs++
		switch {
		case onBoard:
			sum.OnBoard++
		case removed:
			sum.Removed++
		default:
			counts[rule.StatusID]++
		}
		return nil
	}
	outcome, err := jobboard.AddEntry(profileID, jobID, rule.StatusID, includeRemoved)
	switch {
	case errors.Is(err, jobboard.ErrUnknownJob): // deleted since it was listed
		return nil
	case err != nil:
		return fmt.Errorf("could not add job %s: %w", jobID, err)
	}
	sum.Jobs++
	switch outcome {
	case jobboard.Added:
		counts[rule.StatusID]++
	case jobboard.RemovedByPerson:
		sum.Removed++
	default:
		sum.OnBoard++
	}
	return nil
}
