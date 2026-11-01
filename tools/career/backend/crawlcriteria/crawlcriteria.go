// Package crawlcriteria holds the CriteriaMatchers the crawler asks about every new job before inserting it (step 150). The interface
// itself is declared in package jobs (the funnel every crawled job passes through, which cannot import this package: the matchers
// score with jobs); a source built from the settings injects the enabled matchers with jobs.UseCriteria. See
// plan/ai/tools/career/step-150-crawler-settings-ignore-jobs.md.
package crawlcriteria

import (
	"errors"
	"fmt"
	"slices"

	"career-tool-backend/jobs"
)

// MinProfileMatchName is the name of the minimum-profile-match matcher, as the log and the settings page show it.
const MinProfileMatchName = "min-profile-match"

// The bounds of the limit, the same as the cleanup's low-match limit.
const (
	MinPercent = 1
	MaxPercent = 100
)

// ErrInvalidPercent is a limit outside 1 to 100.
var ErrInvalidPercent = errors.New("the minimum match must be 1 to 100 percent")

// CandidateScorer scores a job that is not saved yet for every profile that can judge it (jobs.ProfileScorer is one); a seam so the
// matcher can be tested with exact scores.
type CandidateScorer interface {
	ScoreCandidate(jobs.Candidate) ([]int, error)
}

// MinProfileMatch keeps a job only if at least one profile scores it at or above Percent: a job that matches no profile well is not
// inserted. A job no profile could judge (no profile, no persona, too little data) is kept: "cannot tell" never throws a job away.
type MinProfileMatch struct {
	Percent int
	Scorer  CandidateScorer
}

// NewMinProfileMatch builds the matcher with the given limit (1 to 100) and scorer.
func NewMinProfileMatch(percent int, scorer CandidateScorer) (*MinProfileMatch, error) {
	if percent < MinPercent || percent > MaxPercent {
		return nil, ErrInvalidPercent
	}
	if scorer == nil {
		return nil, errors.New("a scorer is required")
	}
	return &MinProfileMatch{Percent: percent, Scorer: scorer}, nil
}

// NewMinProfileMatchFromData is NewMinProfileMatch with a scorer built from the saved profiles and jobs (read once, so one crawl
// pays for it once).
func NewMinProfileMatchFromData(percent int) (*MinProfileMatch, error) {
	scorer, err := jobs.NewProfileScorer()
	if err != nil {
		return nil, fmt.Errorf("could not read the profiles to score the jobs: %w", err)
	}
	return NewMinProfileMatch(percent, scorer)
}

// Name implements jobs.CriteriaMatcher.
func (m *MinProfileMatch) Name() string { return MinProfileMatchName }

// Match implements jobs.CriteriaMatcher.
func (m *MinProfileMatch) Match(c jobs.Candidate) (jobs.Verdict, error) {
	scores, err := m.Scorer.ScoreCandidate(c)
	if err != nil {
		return jobs.Verdict{}, err
	}
	if len(scores) == 0 {
		return jobs.Verdict{Keep: true, Reason: "no profile could judge it"}, nil
	}
	best := slices.Max(scores)
	if best >= m.Percent {
		return jobs.Verdict{Keep: true, Reason: fmt.Sprintf("best profile match %d%% reaches the limit of %d%%", best, m.Percent)}, nil
	}
	return jobs.Verdict{Keep: false, Reason: fmt.Sprintf("best profile match %d%% is under the limit of %d%%", best, m.Percent)}, nil
}

var _ jobs.CriteriaMatcher = (*MinProfileMatch)(nil)
