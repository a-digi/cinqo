package jobs

import (
	"errors"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/jobmatcher"
)

// ProfileScorer scores jobs against every profile with the manual engine, in memory, and stores nothing (step
// 131). It is the read-only side of RunManualMatch: the same loaders and the same jobmatcher.Score, so a number it
// returns is the number the Jobs page would show for that profile. The corpus and the personas are read once, when
// the scorer is built, so one cleanup run costs one read, not one per job.
type ProfileScorer struct {
	corpus   jobmatcher.Corpus
	profiles [][]jobmatcher.Persona
	now      time.Time
}

// NewProfileScorer loads the corpus and the personas of every profile. A profile without personas is left out
// (it cannot score anything).
func NewProfileScorer() (*ProfileScorer, error) {
	corpus, err := loadCorpus()
	if err != nil {
		return nil, err
	}
	rows, err := db.CareerDB.Query(`SELECT id FROM profiles ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	s := &ProfileScorer{corpus: corpus, now: time.Now()}
	for _, id := range ids {
		personas, err := loadMatchPersonas(id)
		if err != nil {
			return nil, err
		}
		if len(personas) > 0 {
			s.profiles = append(s.profiles, personas)
		}
	}
	return s, nil
}

// Scores returns the best score (0 to 100) of each profile that could judge the job, in profile order. A profile
// with too little data for this job (jobmatcher.InsufficientDataError) is left out; an empty result means no profile
// could judge it. ErrUnknownJob is returned for a job that no longer exists.
func (s *ProfileScorer) Scores(jobID string) ([]int, error) {
	job, err := loadMatchJob(jobID)
	if err != nil {
		return nil, err
	}
	var out []int
	for _, personas := range s.profiles {
		r, err := jobmatcher.Score(job, personas, s.corpus, s.now)
		var insufficient *jobmatcher.InsufficientDataError
		if errors.As(err, &insufficient) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, r.Score)
	}
	return out, nil
}
