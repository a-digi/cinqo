package jobmatchconsumer

import (
	"errors"
	"net/http"
	"sync"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
)

// "Match unmatched jobs" now (step 134): the same catch-up the sweeper runs on a timer, run when a person asks, with the
// outcome counted so the Jobs page can say what happened. It matches in the request, in chunks of BatchSize, through the
// same matcher (jobs.AutoMatchJobs), so it can never disagree with the timer and never replaces a match that exists.

// MatchNowLimit is the most jobs one press handles (newest first); More tells the page there are others to press again for.
const MatchNowLimit = 1000

// ErrBusy: a press is already being handled (a 409).
var ErrBusy = errors.New("matching is already running")

var matchNowMu sync.Mutex

// MatchNowResult is what one press did. Unmatched is how many jobs without a match it looked at; every one of them is
// counted in exactly one of the others.
type MatchNowResult struct {
	Unmatched       int  `json:"unmatched"`
	Matched         int  `json:"matched"`
	Unjudged        int  `json:"unjudged"`        // no profile could judge the job yet; it is retried by the next catch-up
	Failed          int  `json:"failed"`          // reading, scoring or storing failed for that job
	SkippedExisting int  `json:"skippedExisting"` // got a match between the look and the matching (an event, another press)
	More            bool `json:"more"`            // there were more unmatched jobs than one press handles
}

// MatchNow matches the jobs that have no match. It returns ErrBusy when another press is running; a failure that stops it
// early comes back as an error together with what had been counted so far.
func MatchNow() (MatchNowResult, error) {
	if !matchNowMu.TryLock() {
		return MatchNowResult{}, ErrBusy
	}
	defer matchNowMu.Unlock()

	ids, err := unmatchedJobIDs(MatchNowLimit + 1)
	if err != nil {
		return MatchNowResult{}, err
	}
	var res MatchNowResult
	if len(ids) > MatchNowLimit {
		res.More = true
		ids = ids[:MatchNowLimit]
	}
	res.Unmatched = len(ids)
	for from := 0; from < len(ids); from += BatchSize {
		outcomes, err := autoMatch(ids[from:min(len(ids), from+BatchSize)])
		if err != nil {
			return res, err
		}
		for _, o := range outcomes {
			switch o.Status {
			case jobs.AutoMatched:
				res.Matched++
			case jobs.AutoUnjudged, jobs.AutoUnknownJob:
				res.Unjudged++
			case jobs.AutoSkippedExisting:
				res.SkippedExisting++
			default:
				res.Failed++
			}
		}
	}
	return res, nil
}

// MatchNowHandler handles POST /jobs/match/unmatched.
func MatchNowHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	res, err := MatchNow()
	switch {
	case errors.Is(err, ErrBusy):
		http.Error(w, err.Error(), http.StatusConflict)
	case err != nil:
		http.Error(w, "could not match the unmatched jobs: "+err.Error(), http.StatusInternalServerError)
	default:
		db.WriteJSON(w, res)
	}
}
