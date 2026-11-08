// job_match.go — "Job Match": lets an AI conversation assess how well
// one job posting fits a specific career Profile (by inspecting every
// Persona under it, via career.db's own get_persona_details) and
// record a 0-100 score. Mirrors the "Generate with AI" mechanic
// already established for crawl instructions (generateInstructions.ts,
// CrawlPanel.tsx) one level up: a hidden conversation, tracked via a
// conversation id field the frontend can resume watching across a
// reload, finishing by either a score (success) or a recorded error
// (failure).
//
// Split into two persistence paths, deliberately never overlapping:
//   - startJobMatch/updateJobMatchStatus (below) back the human-facing
//     PUT /jobs/match endpoint (http.go) — starts/clears the tracking
//     row, records a client-observed failure. Can NEVER set score or
//     persona_id.
//   - saveJobMatchResult (below) backs the AI-facing save_job_match
//     MCP tool — the ONLY place a score is ever written.
//
// See plan/ai/tools/career/step-XX-job-match.md.
package jobs

import (
	"career-tool-backend/db"
	"career-tool-backend/match/jobmatcher"
	"database/sql"
	"sync"
)

// OrNull turns a *string into a value database/sql writes as either
// the string itself or a real SQL NULL — nil in, nil out.
func OrNull(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// LoadMatchJob reads a job and its requirements as the engine sees them (ErrUnknownJob for an unknown id).
func LoadMatchJob(jobID string) (jobmatcher.Job, error) {
	var title, company, location, description sql.NullString
	err := db.JobsDB.QueryRow(`SELECT title, company, location, description FROM jobs WHERE id = ?`, jobID).
		Scan(&title, &company, &location, &description)
	if err == sql.ErrNoRows {
		return jobmatcher.Job{}, ErrUnknownJob
	}
	if err != nil {
		return jobmatcher.Job{}, err
	}
	reqs, err := readRequirements(jobID)
	if err != nil {
		return jobmatcher.Job{}, err
	}
	job := jobmatcher.Job{Title: title.String, Company: company.String, Location: location.String, Description: description.String}
	for _, r := range reqs {
		job.Requirements = append(job.Requirements, jobmatcher.Requirement{Term: r.Term, Tier: r.Tier})
	}
	return job, nil
}

// LoadCorpus is the title and description of every saved job, for the engine's skill weights and word rarity.
func LoadCorpus() (jobmatcher.Corpus, error) {
	rows, err := db.JobsDB.Query(`SELECT title, COALESCE(description, '') FROM jobs`)
	if err != nil {
		return jobmatcher.Corpus{}, err
	}
	defer rows.Close()
	var texts []string
	for rows.Next() {
		var title, desc string
		if err := rows.Scan(&title, &desc); err != nil {
			return jobmatcher.Corpus{}, err
		}
		texts = append(texts, title+"\n"+desc)
	}
	if err := rows.Err(); err != nil {
		return jobmatcher.Corpus{}, err
	}
	return jobmatcher.NewCorpus(texts), nil
}

var rematchLocks sync.Map // profile id -> *sync.Mutex

// LockProfile takes the profile's lock (a re-match and a purge of the profile's matches never overlap) and returns the unlock.
func LockProfile(profileID string) (unlock func()) {
	lock, _ := rematchLocks.LoadOrStore(profileID, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}
