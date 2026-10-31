package persona

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"career-tool-backend/db"
)

// Applying a reviewed proposal (step 112): the personas the user accepted (possibly edited) are
// created in one transaction, each with its details, skills and experience, under the run's own
// profile and linked to the run. Either all of them exist afterwards or none. See
// plan/ai/tools/career/step-112-create-personas-with-ai.md.

var (
	ErrAiRunNotReady = errors.New("this run has no proposal to apply")
	ErrAiRunApplied  = errors.New("this run was already applied")
)

// ApplyAiRun creates the personas under the run's profile and returns them in the order given.
// The run must be 'ready' and must not have personas yet (applied is read from the personas, not
// stored twice, so deleting what it created makes it applicable again). The personas are
// validated exactly like a proposal; skills are trimmed and de-duplicated, text trimmed.
func ApplyAiRun(runID string, personas []ProposedPersona) ([]aiRunPersona, error) {
	if len(personas) == 0 {
		return nil, invalid("there is nothing to apply (no persona was accepted)")
	}
	normalized := make([]ProposedPersona, len(personas))
	for i, p := range personas {
		normalized[i] = normalizePersona(p)
	}
	if err := ValidateProposal(Proposal{Personas: normalized}); err != nil {
		return nil, err
	}

	tx, err := db.CareerDB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var profileID, status string
	switch err := tx.QueryRow(`SELECT profile_id, status FROM persona_ai_runs WHERE id = ?`, runID).Scan(&profileID, &status); {
	case err == sql.ErrNoRows:
		return nil, ErrUnknownAiRun
	case err != nil:
		return nil, err
	}
	if status != runReady {
		return nil, ErrAiRunNotReady
	}
	var already int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM personas WHERE ai_run_id = ?`, runID).Scan(&already); err != nil {
		return nil, err
	}
	if already > 0 {
		return nil, ErrAiRunApplied
	}

	created := make([]aiRunPersona, 0, len(normalized))
	for _, p := range normalized {
		id, err := insertAppliedPersona(tx, profileID, runID, p)
		if err != nil {
			return nil, fmt.Errorf("failed to create persona %q: %w", p.Name, err)
		}
		created = append(created, aiRunPersona{ID: id, Name: p.Name})
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

func insertAppliedPersona(tx *sql.Tx, profileID, runID string, p ProposedPersona) (string, error) {
	id := uuid.NewString()
	if _, err := tx.Exec(
		`INSERT INTO personas (id, profile_id, name, description, created_at, ai_run_id) VALUES (?, ?, ?, ?, datetime('now'), ?)`,
		id, profileID, p.Name, nullIfEmpty(p.Description), runID,
	); err != nil {
		return "", err
	}
	if _, err := tx.Exec(
		`INSERT INTO persona_details (persona_id, headline, summary, location, desired_titles, desired_locations, min_salary, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
		id, p.Details.Headline, p.Details.Summary, p.Details.Location, p.Details.DesiredTitles, p.Details.DesiredLocations, p.Details.MinSalary,
	); err != nil {
		return "", err
	}
	for _, skill := range p.Skills {
		if _, err := tx.Exec(`INSERT INTO career_skills (id, persona_id, skill) VALUES (?, ?, ?)`, uuid.NewString(), id, skill); err != nil {
			return "", err
		}
	}
	for _, r := range p.Experience {
		if _, err := tx.Exec(
			`INSERT INTO career_experience (id, persona_id, company, title, start_date, end_date, description) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), id, r.Company, r.Title, r.StartDate, r.EndDate, r.Description,
		); err != nil {
			return "", err
		}
	}
	return id, nil
}

// normalizePersona trims every text and drops duplicate skills (case-insensitive, the first
// spelling stays); what is empty after trimming is left for the validation to refuse.
func normalizePersona(p ProposedPersona) ProposedPersona {
	p.Name = strings.TrimSpace(p.Name)
	p.Description = strings.TrimSpace(p.Description)
	d := &p.Details
	d.Headline, d.Summary, d.Location = strings.TrimSpace(d.Headline), strings.TrimSpace(d.Summary), strings.TrimSpace(d.Location)
	d.DesiredTitles, d.DesiredLocations = strings.TrimSpace(d.DesiredTitles), strings.TrimSpace(d.DesiredLocations)

	seen := map[string]bool{}
	skills := make([]string, 0, len(p.Skills))
	for _, s := range p.Skills {
		s = strings.TrimSpace(s)
		key := strings.ToLower(s)
		if s != "" && seen[key] {
			continue
		}
		seen[key] = true
		skills = append(skills, s)
	}
	p.Skills = skills

	roles := make([]ProposedRole, len(p.Experience))
	for i, r := range p.Experience {
		roles[i] = ProposedRole{
			Company: strings.TrimSpace(r.Company), Title: strings.TrimSpace(r.Title),
			StartDate: strings.TrimSpace(r.StartDate), EndDate: strings.TrimSpace(r.EndDate), Description: strings.TrimSpace(r.Description),
		}
	}
	p.Experience = roles
	return p
}

type applyAiRunRequest struct {
	RunID    string            `json:"runId"`
	Personas []ProposedPersona `json:"personas"`
}

// AiRunApplyHandler handles POST persona-ai-runs/apply.
func AiRunApplyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body applyAiRunRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RunID == "" {
		http.Error(w, "runId and personas are required", http.StatusBadRequest)
		return
	}
	created, err := ApplyAiRun(body.RunID, body.Personas)
	if err != nil {
		switch {
		case errors.Is(err, ErrUnknownAiRun):
			http.Error(w, "unknown run id", http.StatusBadRequest)
		case errors.Is(err, ErrInvalidProposal):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, ErrAiRunNotReady), errors.Is(err, ErrAiRunApplied):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			http.Error(w, "failed to apply the run: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}
	db.WriteJSON(w, map[string]any{"personas": created})
}
