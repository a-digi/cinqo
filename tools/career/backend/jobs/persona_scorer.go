package jobs

import (
	"time"

	"career-tool-backend/db"
	"career-tool-backend/jobs/jobmatcher"
	"career-tool-backend/persona/personamatch"
)

// PersonaScorer scores a job against every persona of every profile, each persona on its own, with the manual engine (no
// AI), and stores nothing (step 141). Like ProfileScorer it reads the personas and the corpus once, when it is built, so a
// batch of jobs costs one read. The rows it returns are what personamatch.Replace writes.
type PersonaScorer struct {
	corpus    jobmatcher.Corpus
	personas  []jobmatcher.Persona // profiles oldest first, each profile's personas oldest first
	profileOf map[string]string    // persona id -> its profile's id
	now       time.Time
}

// NewPersonaScorer loads every profile's personas (and, when there is at least one persona, the corpus).
func NewPersonaScorer() (*PersonaScorer, error) {
	rows, err := db.CareerDB.Query(`SELECT id FROM profiles ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	var profileIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		profileIDs = append(profileIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	s := &PersonaScorer{now: time.Now(), profileOf: map[string]string{}}
	for _, profileID := range profileIDs {
		personas, err := loadMatchPersonas(profileID)
		if err != nil {
			return nil, err
		}
		for _, p := range personas {
			s.personas = append(s.personas, p)
			s.profileOf[p.ID] = profileID
		}
	}
	if len(s.personas) > 0 { // with no persona the corpus is never used, so it is not read
		corpus, err := loadCorpus()
		if err != nil {
			return nil, err
		}
		s.corpus = corpus
	}
	return s, nil
}

// HasPersonas reports whether any persona exists to score a job.
func (s *PersonaScorer) HasPersonas() bool { return len(s.personas) > 0 }

// Rows scores the job for every persona and returns one row per persona that could judge it (a persona with nothing to
// compare has none). With no persona at all it returns no rows without reading the job. ErrUnknownJob for a job that no
// longer exists.
func (s *PersonaScorer) Rows(jobID string) ([]personamatch.Row, error) {
	if len(s.personas) == 0 {
		return nil, nil
	}
	job, err := loadMatchJob(jobID)
	if err != nil {
		return nil, err
	}
	results := jobmatcher.ScoreEach(job, s.personas, s.corpus, s.now)
	rows := make([]personamatch.Row, 0, len(results))
	for _, r := range results {
		rows = append(rows, personamatch.Row{PersonaID: r.PersonaID, ProfileID: s.profileOf[r.PersonaID], Score: r.Score, EngineVersion: r.EngineVersion})
	}
	return rows, nil
}
