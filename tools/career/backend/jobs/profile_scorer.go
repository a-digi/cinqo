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
	profiles []scorerProfile
	now      time.Time
}

// scorerProfile is one profile with its personas, in the order profiles are listed (oldest first).
type scorerProfile struct {
	id       string
	personas []jobmatcher.Persona
}

// NewProfileScorer loads the corpus and the personas of every profile. A profile without personas is left out
// (it cannot score anything).
func NewProfileScorer() (*ProfileScorer, error) { return NewProfileScorerFor(nil) }

// NewProfileScorerFor is NewProfileScorer for the listed profiles only (step 140: the profiles that have auto-add rules),
// so scoring a job does not cost the personas of every other profile. A nil set means every profile; a profile that is
// listed but does not exist is simply absent.
func NewProfileScorerFor(only map[string]bool) (*ProfileScorer, error) {
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
		if only == nil || only[id] {
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	s := &ProfileScorer{now: time.Now()}
	for _, id := range ids {
		personas, err := loadMatchPersonas(id)
		if err != nil {
			return nil, err
		}
		if len(personas) > 0 {
			s.profiles = append(s.profiles, scorerProfile{id: id, personas: personas})
		}
	}
	if len(s.profiles) > 0 { // with no profile that can judge anything the corpus is never used, so it is not read
		corpus, err := loadCorpus()
		if err != nil {
			return nil, err
		}
		s.corpus = corpus
	}
	return s, nil
}

// profileResult is one profile's best result for a job.
type profileResult struct {
	profileID string
	result    jobmatcher.Result
}

// scoreAll scores a job for every profile that can judge it, in profile order. A profile with too little data for
// this job (jobmatcher.InsufficientDataError) is left out. ErrUnknownJob is returned for a job that no longer exists.
func (s *ProfileScorer) scoreAll(jobID string) ([]profileResult, error) {
	job, err := loadMatchJob(jobID)
	if err != nil {
		return nil, err
	}
	return s.scoreJob(job)
}

// scoreJob is scoreAll for a job already in memory: the engine's call, shared by the saved-job and the candidate paths so both give
// the same number for the same text.
func (s *ProfileScorer) scoreJob(job jobmatcher.Job) ([]profileResult, error) {
	var out []profileResult
	for _, p := range s.profiles {
		r, err := jobmatcher.Score(job, p.personas, s.corpus, s.now)
		var insufficient *jobmatcher.InsufficientDataError
		if errors.As(err, &insufficient) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, profileResult{profileID: p.id, result: r})
	}
	return out, nil
}

// ScoreCandidate returns the best score (0 to 100) of each profile that could judge a job the crawler found and has not inserted
// yet (step 150), in profile order, from what the crawl knows of it (title, company, location, description; a job that is not saved
// has no requirements). For the same text it is the number Scores gives once the job is saved. An empty result means no profile
// could judge it.
func (s *ProfileScorer) ScoreCandidate(c Candidate) ([]int, error) {
	all, err := s.scoreJob(jobmatcher.Job{Title: c.Title, Company: c.Company, Location: c.Location, Description: c.Description})
	if err != nil {
		return nil, err
	}
	var out []int
	for _, r := range all {
		out = append(out, r.result.Score)
	}
	return out, nil
}

// Scores returns the best score (0 to 100) of each profile that could judge the job, in profile order. An empty
// result means no profile could judge it. ErrUnknownJob is returned for a job that no longer exists.
func (s *ProfileScorer) Scores(jobID string) ([]int, error) {
	all, err := s.scoreAll(jobID)
	if err != nil {
		return nil, err
	}
	var out []int
	for _, r := range all {
		out = append(out, r.result.Score)
	}
	return out, nil
}

// ScoreByProfile returns each profile's best score (0 to 100) for the job, by profile id; a profile that could not judge
// the job (no personas, too little data) is absent. ErrUnknownJob is returned for a job that no longer exists.
func (s *ProfileScorer) ScoreByProfile(jobID string) (map[string]int, error) {
	all, err := s.scoreAll(jobID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(all))
	for _, r := range all {
		out[r.profileID] = r.result.Score
	}
	return out, nil
}

// Best returns the profile that scores the job highest, with that profile's whole result (its best persona, the
// matched skills, the breakdown). A tie goes to the profile listed first (the oldest), so the answer never depends
// on chance. ok is false when no profile could judge the job.
func (s *ProfileScorer) Best(jobID string) (profileID string, result jobmatcher.Result, ok bool, err error) {
	all, err := s.scoreAll(jobID)
	if err != nil {
		return "", jobmatcher.Result{}, false, err
	}
	for _, r := range all {
		if !ok || r.result.Score > result.Score {
			profileID, result, ok = r.profileID, r.result, true
		}
	}
	return profileID, result, ok, nil
}
