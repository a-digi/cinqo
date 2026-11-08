package jobs

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"career-tool-backend/db"
	"career-tool-backend/match/jobmatcher"
)

// MaxRequirementTermLength is the longest requirement, in characters.
const MaxRequirementTermLength = 120

// JobRequirement is one thing a job asks for, with its tier (3 must-have, 2 important, 1 nice-to-have).
type JobRequirement struct {
	ID   string `json:"id"`
	Term string `json:"term"`
	Tier int    `json:"tier"`
}

// RequirementInput is one requirement as a caller sends it.
type RequirementInput struct {
	Term string `json:"term"`
	Tier int    `json:"tier"`
}

// RequirementsError is a refused requirements list, with the reason a person can act on.
type RequirementsError struct{ Message string }

func (e *RequirementsError) Error() string { return e.Message }

// IsRequirementsError reports whether err is a refused requirements list.
func IsRequirementsError(err error) (*RequirementsError, bool) {
	var re *RequirementsError
	if errors.As(err, &re) {
		return re, true
	}
	return nil, false
}

// cleanRequirementInputs trims and checks a whole list: at most jobmatcher.MaxRequirements terms of at most
// MaxRequirementTermLength characters, each with a tier of 1 to 3, none twice (ignoring case).
func cleanRequirementInputs(in []RequirementInput) ([]RequirementInput, error) {
	if len(in) > jobmatcher.MaxRequirements {
		return nil, &RequirementsError{fmt.Sprintf("a job can have at most %d requirements", jobmatcher.MaxRequirements)}
	}
	out := make([]RequirementInput, 0, len(in))
	seen := map[string]bool{}
	for _, r := range in {
		term := strings.TrimSpace(r.Term)
		switch {
		case term == "":
			return nil, &RequirementsError{"a requirement cannot be empty"}
		case utf8.RuneCountInString(term) > MaxRequirementTermLength:
			return nil, &RequirementsError{fmt.Sprintf("a requirement can have at most %d characters", MaxRequirementTermLength)}
		case r.Tier < 1 || r.Tier > 3:
			return nil, &RequirementsError{fmt.Sprintf("the tier of %q must be 1, 2 or 3", term)}
		}
		key := strings.ToLower(term)
		if seen[key] {
			return nil, &RequirementsError{fmt.Sprintf("%q is listed twice", term)}
		}
		seen[key] = true
		out = append(out, RequirementInput{Term: term, Tier: r.Tier})
	}
	return out, nil
}

// GetJobRequirements reads a job's requirements in the order they were saved (ErrUnknownJob for an unknown job).
func GetJobRequirements(jobID string) ([]JobRequirement, error) {
	if err := RequireJobExists(jobID); err != nil {
		return nil, err
	}
	return readRequirements(jobID)
}

func readRequirements(jobID string) ([]JobRequirement, error) {
	rows, err := db.JobsDB.Query(`SELECT id, term, tier FROM job_requirements WHERE job_id = ? ORDER BY rowid`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []JobRequirement{}
	for rows.Next() {
		var r JobRequirement
		if err := rows.Scan(&r.ID, &r.Term, &r.Tier); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

// SetJobRequirements replaces a job's whole requirements list (an empty list clears it) and returns what was
// saved. A refused list changes nothing.
func SetJobRequirements(jobID string, in []RequirementInput) ([]JobRequirement, error) {
	if err := RequireJobExists(jobID); err != nil {
		return nil, err
	}
	clean, err := cleanRequirementInputs(in)
	if err != nil {
		return nil, err
	}
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM job_requirements WHERE job_id = ?`, jobID); err != nil {
		return nil, err
	}
	for _, r := range clean {
		if _, err := tx.Exec(`INSERT INTO job_requirements (id, job_id, term, tier) VALUES (?, ?, ?, ?)`, uuid.NewString(), jobID, r.Term, r.Tier); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return readRequirements(jobID)
}

// SuggestJobRequirements offers the job's most distinctive words as candidate requirements (a word count
// against the saved jobs, never an AI call), leaving out words an existing requirement already uses.
func SuggestJobRequirements(jobID string) ([]string, error) {
	job, err := LoadMatchJob(jobID)
	if err != nil {
		return nil, err
	}
	corpus, err := LoadCorpus()
	if err != nil {
		return nil, err
	}
	return jobmatcher.Suggest(job, corpus), nil
}
