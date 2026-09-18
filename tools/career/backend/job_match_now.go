// job_match_now.go is "Job Match"'s own deterministic sibling — same
// data model (job_matches/job_match_skills, job_match.go), but no
// hidden AI conversation anywhere in the path. Reproduces every step
// of the AI's own prompt (buildJobMatchMessage.ts) in plain Go:
// crawl the job's own detail page if its description is missing
// (reusing crawlOneJobDetail, crawl_job_details_now.go — the exact
// same deterministic single-job crawl "Crawl job details now" already
// uses), score every persona under the chosen profile
// (job_match_algorithm.go), and keep whichever one scores highest —
// the deterministic substitute for the AI's own qualitative judgment
// call, since there's no model here to weigh "best fit" any other
// way. Tracked via kind='deterministic' (db.go's own
// job_matches.kind doc comment) so the Jobs page can tell the two
// mechanisms apart without a conversation id to poll.
//
// Deliberately not cancellable — unlike "Crawl now"/"Crawl job
// details now," this is a single job's own short operation (at most
// one page crawl plus a fast in-memory scoring pass), so no dedicated
// cancel endpoint or crawl_cancel.go registration was built for it.
// See plan/ai/tools/career/step-XX-deterministic-job-match.md.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// jobMatchNowHandler handles POST /jobs/match/now — starts a
// job_matches row (kind='deterministic') and launches the detached
// goroutine, responding immediately. Mirrors crawlJobDetailsNowHandler
// (crawl_job_details_now.go) in shape: every fast, synchronously-
// checkable precondition (unknown job, a profile with no personas) is
// rejected here, before anything starts, rather than surfacing only
// after the fact as an async failure.
func jobMatchNowHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		JobID     string `json:"jobId"`
		ProfileID string `json:"profileId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.JobID == "" || body.ProfileID == "" {
		http.Error(w, "jobId and profileId are required", http.StatusBadRequest)
		return
	}

	// The credential travels via the request's own Cookie header, same
	// reasoning as crawlJobDetailsNowHandler's own identical read —
	// needed here too, since the crawl-if-missing step below may call
	// out to the browser tool's own proxy routes.
	accessCookie, err := r.Cookie("access_token")
	if err != nil {
		http.Error(w, "no active session cookie found", http.StatusUnauthorized)
		return
	}

	personas, err := listPersonas(body.ProfileID)
	if err != nil {
		http.Error(w, "failed to list personas: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(personas) == 0 {
		http.Error(w, "this profile has no personas to match against", http.StatusBadRequest)
		return
	}

	if err := startJobMatch(body.JobID, body.ProfileID, "", "deterministic"); err != nil {
		if errors.Is(err, errUnknownJob) {
			http.Error(w, "unknown job id", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to start job match: "+err.Error(), http.StatusInternalServerError)
		return
	}

	go runJobMatchNow(context.Background(), body.JobID, body.ProfileID, personas, accessCookie.Value)

	w.WriteHeader(http.StatusAccepted)
}

// runJobMatchNow is the detached goroutine body — rooted in
// context.Background() by its caller, same "survives the initiating
// request" reasoning as runJobDetailCrawlNow, but with no cancel
// registration (see this file's own top comment for why).
func runJobMatchNow(ctx context.Context, jobID, profileID string, personas []persona, accessToken string) {
	fail := func(err error) {
		msg := err.Error()
		_ = updateJobMatchStatus(jobID, "failed", &msg)
	}

	job, err := getJobByID(jobID)
	if err != nil {
		fail(err)
		return
	}

	// Crawl-if-missing — the same rule the AI's own prompt follows:
	// best-effort, never fatal to the match. No portal link, no stored
	// job detail crawl instructions, or the crawl attempt itself
	// failing all fall through to scoring with whatever text is
	// already available (title/company/location) rather than refusing
	// to produce a score.
	if strings.TrimSpace(job.Description) == "" && job.PortalLinkID != "" {
		if req, reqErr := buildJobDetailCrawlRequest(job.PortalLinkID); reqErr == nil {
			if coreURL, urlErr := coreAPIURL(); urlErr == nil {
				if portalID, portalErr := getPortalIDForLink(job.PortalLinkID); portalErr == nil {
					target := jobDetailCrawlTarget{ID: job.ID, SourceURL: job.SourceURL, Title: job.Title}
					_, _ = crawlOneJobDetail(ctx, coreURL, portalID, uuid.NewString(), accessToken, req, target)
					if refreshed, refreshErr := getJobByID(jobID); refreshErr == nil {
						job = refreshed
					}
				}
			}
		}
	}

	// acquireActiveVectors (semantic_vectors.go) loads the active
	// semantic model on demand and is nil if none is selected/loaded —
	// computeJobMatch treats nil as "no semantic fallback," so this
	// call site behaves exactly as before whenever no model has been
	// downloaded/selected. Called once per run, not once per persona
	// below: it's the same job description being compared every time,
	// and re-acquiring per persona would just restart the same idle-
	// unload timer redundantly.
	vectors := acquireActiveVectors()

	// bestScore starts at -1 (never a real score) so even a 0-scoring
	// persona is recorded as "the best available" rather than being
	// silently skipped by a zero-valued default.
	var bestPersonaID string
	var bestScore = -1
	var bestSkills []string
	for _, p := range personas {
		details, err := fetchPersonaDetails(p.ID)
		if err != nil {
			fail(err)
			return
		}
		score, matched := computeJobMatch(job.Title, job.Description, details.Skills, vectors)
		if score > bestScore {
			bestScore = score
			bestPersonaID = p.ID
			bestSkills = matched
		}
	}

	if err := saveJobMatchResult(jobID, profileID, bestPersonaID, bestScore, bestSkills, "deterministic"); err != nil {
		fail(err)
	}
}
