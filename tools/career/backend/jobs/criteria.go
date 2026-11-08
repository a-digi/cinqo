package jobs

import (
	"errors"
	"fmt"
	"log"
	"sync"
)

// Criteria for the jobs a crawl may insert (step 150). The crawler asks a list of CriteriaMatchers about every NEW job before it is
// inserted; a job is inserted only if every matcher keeps it. The interface is declared here, in the package every crawled job passes
// through, and the implementations live in a package that imports this one (the scoring needs jobs), injected once at start-up with
// UseCriteria like cvbuilder.UseJobLinks. See plan/ai/tools/career/step-150-crawler-settings-ignore-jobs.md.

// Candidate is a job the crawler found and is about to insert: what the crawl knows of it (the description is already plain text).
type Candidate struct {
	SourceURL    string
	Title        string
	Company      string
	Location     string
	Description  string
	PostedAt     string
	PortalLinkID string // empty for a job the AI saved without a portal link
}

// Verdict is a matcher's answer: Keep false means "do not insert this job", with the reason in plain words for the log.
type Verdict struct {
	Keep   bool
	Reason string
}

// CriteriaMatcher is one rule about which jobs to insert. Name says which rule it is (the log and the settings page show it).
type CriteriaMatcher interface {
	Name() string
	Match(Candidate) (Verdict, error)
}

// CriteriaSource builds the matchers for one crawl: it reads the settings and whatever scoring data the matchers need, once, so a
// crawl of many jobs does not pay for it per job. It returns only the matchers that are switched on.
type CriteriaSource interface {
	Matchers() ([]CriteriaMatcher, error)
}

var (
	criteriaMu     sync.RWMutex
	criteriaSource CriteriaSource
	filteredOutLog func(Candidate, *FilteredOutError)
)

// UseFilteredOutLog injects a function that is told about every job a matcher refused (the crawler records them so the person can see
// what the rules keep out). It must not fail the crawl: a panic in it is recovered. nil removes it.
func UseFilteredOutLog(fn func(Candidate, *FilteredOutError)) {
	criteriaMu.Lock()
	filteredOutLog = fn
	criteriaMu.Unlock()
}

func reportFilteredOut(cand Candidate, refused *FilteredOutError) {
	criteriaMu.RLock()
	fn := filteredOutLog
	criteriaMu.RUnlock()
	if fn == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("crawler criteria: the log of ignored jobs panicked: %v", r)
		}
	}()
	fn(cand, refused)
}

// UseCriteria injects the source of the matchers (nil removes it: every job is kept). Called once at start-up.
func UseCriteria(s CriteriaSource) {
	criteriaMu.Lock()
	criteriaSource = s
	criteriaMu.Unlock()
}

// ErrFilteredOut is returned (wrapped in a *FilteredOutError) by a crawl's save for a new job a matcher refused: nothing is inserted
// and no event is sent. Callers count it as "ignored by the rules", not as a failure.
var ErrFilteredOut = errors.New("this job does not meet the crawler's criteria and is not added")

// FilteredOutError says which matcher refused a job and why. errors.Is(err, ErrFilteredOut) is true for it.
type FilteredOutError struct {
	Matcher string
	Reason  string
}

func (e *FilteredOutError) Error() string {
	return fmt.Sprintf("%v (%s: %s)", ErrFilteredOut, e.Matcher, e.Reason)
}

func (e *FilteredOutError) Is(target error) bool { return target == ErrFilteredOut }

// Criteria is the chain of matchers for one crawl. A nil Criteria, or one without matchers, keeps every job.
type Criteria struct {
	matchers []CriteriaMatcher
}

// NewCriteria builds the chain of one crawl from the injected source, asking it once. With no source the chain keeps everything; a
// source that fails is logged and the chain keeps everything too: a broken rule must never lose jobs.
func NewCriteria() *Criteria {
	criteriaMu.RLock()
	src := criteriaSource
	criteriaMu.RUnlock()
	if src == nil {
		return &Criteria{}
	}
	ms, err := src.Matchers()
	if err != nil {
		log.Printf("crawler criteria: could not build the matchers, keeping every job: %v", err)
		return &Criteria{}
	}
	return &Criteria{matchers: ms}
}

// NewCriteriaFrom builds a chain from the given matchers (for tests and for callers that have their own).
func NewCriteriaFrom(ms ...CriteriaMatcher) *Criteria { return &Criteria{matchers: ms} }

// Check asks the matchers in order and stops at the first that refuses the job, returning why. A matcher that fails (an error, or a
// panic) is logged and counts as keeping the job, and the next matcher is still asked. nil means every matcher keeps the job.
func (c *Criteria) Check(cand Candidate) *FilteredOutError {
	if c == nil {
		return nil
	}
	for _, m := range c.matchers {
		v, err := safeMatch(m, cand)
		if err != nil {
			log.Printf("crawler criteria: matcher %q failed for %q, keeping the job: %v", m.Name(), cand.Title, err)
			continue
		}
		if !v.Keep {
			refused := &FilteredOutError{Matcher: m.Name(), Reason: v.Reason}
			reportFilteredOut(cand, refused)
			return refused
		}
	}
	return nil
}

func safeMatch(m CriteriaMatcher, cand Candidate) (v Verdict, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return m.Match(cand)
}
