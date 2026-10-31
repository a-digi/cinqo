package jobs

import (
	"database/sql"
	"encoding/json"
	"sort"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/jobmatcher"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

// The manual (no AI) Job Match engine's run (step 129). The AI engine is untouched: it is a conversation the Jobs
// page starts and `save_job_match` answers. This one scores in-process with the jobmatcher package from data
// already in the two databases, and writes the same job_matches / job_match_skills rows with kind = 'manual'.
// Nothing here calls an AI, the network or any other service. See plan/ai/tools/career/step-129-manual-job-match.md.

// loadMatchJob reads a job and its requirements as the engine sees them (ErrUnknownJob for an unknown id).
func loadMatchJob(jobID string) (jobmatcher.Job, error) {
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

// loadCorpus is the title and description of every saved job, for the engine's skill weights and word rarity.
func loadCorpus() (jobmatcher.Corpus, error) {
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

// loadMatchPersonas reads every persona of a profile with its details, skills and experience, oldest first.
func loadMatchPersonas(profileID string) ([]jobmatcher.Persona, error) {
	list, err := persona.ListPersonas(profileID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].CreatedAt != list[j].CreatedAt {
			return list[i].CreatedAt < list[j].CreatedAt
		}
		return list[i].ID < list[j].ID
	})
	out := make([]jobmatcher.Persona, 0, len(list))
	for _, p := range list {
		d, err := persona.FetchPersonaDetails(p.ID)
		if err != nil {
			return nil, err
		}
		mp := jobmatcher.Persona{ID: p.ID, Name: p.Name, Skills: d.Skills}
		if d.PersonaDetails != nil {
			mp.Headline = d.PersonaDetails.Headline
			mp.Summary = d.PersonaDetails.Summary
			mp.Location = d.PersonaDetails.Location
			mp.DesiredTitles = d.PersonaDetails.DesiredTitles
			mp.DesiredLocations = d.PersonaDetails.DesiredLocations
			mp.RemotePreference = d.PersonaDetails.RemotePreference
		}
		for _, e := range d.Experience {
			mp.Experience = append(mp.Experience, jobmatcher.Experience{Title: e.Title, Description: e.Description, StartDate: e.StartDate, EndDate: e.EndDate})
		}
		out = append(out, mp)
	}
	return out, nil
}

// RunManualMatch scores a job against every persona of a profile with the manual engine and stores the best
// result as the job's match (the same row an AI match uses: one per job, the last run of either engine wins).
// It returns jobmatcher.ErrNoPersonas or an *jobmatcher.InsufficientDataError, and ErrUnknownJob or
// profile.ErrUnknownProfile, without writing anything: a failed run never replaces an earlier result.
func RunManualMatch(jobID, profileID string) (jobmatcher.Result, error) {
	job, err := loadMatchJob(jobID)
	if err != nil {
		return jobmatcher.Result{}, err
	}
	if err := profile.RequireProfileExists(profileID); err != nil {
		return jobmatcher.Result{}, err
	}
	personas, err := loadMatchPersonas(profileID)
	if err != nil {
		return jobmatcher.Result{}, err
	}
	corpus, err := loadCorpus()
	if err != nil {
		return jobmatcher.Result{}, err
	}
	result, err := jobmatcher.Score(job, personas, corpus, time.Now())
	if err != nil {
		return jobmatcher.Result{}, err
	}
	if err := storeManualMatch(jobID, profileID, result); err != nil {
		return jobmatcher.Result{}, err
	}
	return result, nil
}

// storeManualMatch writes the result like saveJobMatchResult does for the AI, with kind = 'manual' and the whole
// result as breakdown_json.
func storeManualMatch(jobID, profileID string, r jobmatcher.Result) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(
		`INSERT INTO job_matches (job_id, profile_id, persona_id, score, status, conversation_id, error, kind, breakdown_json, updated_at)
		 VALUES (?, ?, ?, ?, 'completed', NULL, NULL, 'manual', ?, datetime('now'))
		 ON CONFLICT(job_id) DO UPDATE SET
			profile_id = excluded.profile_id,
			persona_id = excluded.persona_id,
			score = excluded.score,
			status = 'completed',
			conversation_id = NULL,
			error = NULL,
			kind = 'manual',
			breakdown_json = excluded.breakdown_json,
			updated_at = datetime('now')`,
		jobID, profileID, r.PersonaID, r.Score, string(raw),
	); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM job_match_skills WHERE job_id = ?`, jobID); err != nil {
		return err
	}
	for _, s := range r.MatchedSkills {
		if _, err := tx.Exec(`INSERT INTO job_match_skills (job_id, skill) VALUES (?, ?)`, jobID, s); err != nil {
			return err
		}
	}
	return tx.Commit()
}
