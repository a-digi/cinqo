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

// Deciding on a proposal (steps 112 and 113): every proposed persona is accepted or declined by the
// user. The accepted ones (possibly edited) are created in one transaction, each with its details,
// skills and experience, under the run's own profile and linked to the run; the declined ones are
// recorded on the run. Either all of it happens or none of it, and the run is then closed
// (decided_at), so it cannot be decided twice. See
// plan/ai/tools/career/step-112-create-personas-with-ai.md and
// plan/ai/tools/career/step-113-human-in-the-loop-for-ai-personas.md.

var (
	ErrAiRunNotReady = errors.New("this run has no proposal to decide on")
	ErrAiRunDecided  = errors.New("this run was already decided")
)

const (
	decisionAccepted = "accepted"
	decisionDeclined = "declined"
)

// Decision is the user's decision on the proposed persona at Index (its position in the run's
// proposal): accepted with the final, possibly edited content, or declined with none.
type Decision struct {
	Index    int              `json:"index"`
	Decision string           `json:"decision"`
	Persona  *ProposedPersona `json:"persona,omitempty"`
}

type decidableRun struct {
	profileID string
	proposed  []ProposedPersona
}

// loadDecidableRun reads the run inside tx and checks it can still be decided: it exists, it is
// 'ready' with something proposed, and it has not been decided yet.
func loadDecidableRun(tx *sql.Tx, runID string) (decidableRun, error) {
	var run decidableRun
	var status string
	var decidedAt, proposalJSON sql.NullString
	switch err := tx.QueryRow(
		`SELECT profile_id, status, decided_at, proposal_json FROM persona_ai_runs WHERE id = ?`, runID,
	).Scan(&run.profileID, &status, &decidedAt, &proposalJSON); {
	case err == sql.ErrNoRows:
		return run, ErrUnknownAiRun
	case err != nil:
		return run, err
	}
	if status != runReady {
		return run, ErrAiRunNotReady
	}
	if decidedAt.Valid {
		return run, ErrAiRunDecided
	}
	var proposal Proposal
	if err := json.Unmarshal([]byte(proposalJSON.String), &proposal); err != nil || len(proposal.Personas) == 0 {
		return run, ErrAiRunNotReady
	}
	run.proposed = proposal.Personas
	return run, nil
}

// ApplyDecisions records the user's decision on every proposed persona of the run and creates the
// accepted ones, returned in the order of the proposal. The decisions must cover every proposed
// persona exactly once; an accepted one carries its final content (validated like a proposal, with
// skills trimmed and de-duplicated), a declined one carries none. Declining everything is allowed:
// nothing is created and the run is closed.
func ApplyDecisions(runID string, decisions []Decision) ([]aiRunPersona, error) {
	tx, err := db.CareerDB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	run, err := loadDecidableRun(tx, runID)
	if err != nil {
		return nil, err
	}
	accepted, declined, err := resolveDecisions(run.proposed, decisions)
	if err != nil {
		return nil, err
	}
	return commitDecision(tx, run.profileID, runID, accepted, declined)
}

// resolveDecisions checks the decisions against the proposal and splits them: the accepted
// personas (normalized, in proposal order) and the names of the declined ones (as proposed).
func resolveDecisions(proposed []ProposedPersona, decisions []Decision) ([]ProposedPersona, []string, error) {
	if len(decisions) != len(proposed) {
		return nil, nil, invalid("a decision is needed for every proposed persona (%d), got %d", len(proposed), len(decisions))
	}
	byIndex := make(map[int]Decision, len(decisions))
	for _, d := range decisions {
		if d.Index < 0 || d.Index >= len(proposed) {
			return nil, nil, invalid("decision for persona %d: there is no such proposed persona", d.Index)
		}
		if _, dup := byIndex[d.Index]; dup {
			return nil, nil, invalid("persona %d has more than one decision", d.Index)
		}
		switch d.Decision {
		case decisionAccepted:
			if d.Persona == nil {
				return nil, nil, invalid("persona %d is accepted but has no content", d.Index)
			}
		case decisionDeclined:
			if d.Persona != nil {
				return nil, nil, invalid("persona %d is declined, so it must not carry content", d.Index)
			}
		default:
			return nil, nil, invalid("persona %d: the decision must be %q or %q", d.Index, decisionAccepted, decisionDeclined)
		}
		byIndex[d.Index] = d
	}
	var accepted []ProposedPersona
	var declined []string
	for i := range proposed {
		d := byIndex[i]
		if d.Decision == decisionAccepted {
			accepted = append(accepted, normalizePersona(*d.Persona))
		} else {
			declined = append(declined, proposed[i].Name)
		}
	}
	if len(accepted) > 0 {
		if err := ValidateProposal(Proposal{Personas: accepted}); err != nil {
			return nil, nil, err
		}
	}
	return accepted, declined, nil
}

// commitDecision creates the accepted personas, records the declined names, closes the run and
// commits, all in tx.
func commitDecision(tx *sql.Tx, profileID, runID string, accepted []ProposedPersona, declinedNames []string) ([]aiRunPersona, error) {
	created := make([]aiRunPersona, 0, len(accepted))
	for _, p := range accepted {
		id, err := insertAppliedPersona(tx, profileID, runID, p)
		if err != nil {
			return nil, fmt.Errorf("failed to create persona %q: %w", p.Name, err)
		}
		created = append(created, aiRunPersona{ID: id, Name: p.Name})
	}
	var declinedJSON any // NULL when nothing was declined
	if len(declinedNames) > 0 {
		list := make([]aiRunDeclined, len(declinedNames))
		for i, n := range declinedNames {
			list[i] = aiRunDeclined{Name: n}
		}
		raw, err := json.Marshal(list)
		if err != nil {
			return nil, err
		}
		declinedJSON = string(raw)
	}
	if _, err := tx.Exec(
		`UPDATE persona_ai_runs SET decided_at = datetime('now'), declined_json = ? WHERE id = ?`, declinedJSON, runID,
	); err != nil {
		return nil, err
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
	RunID string `json:"runId"`
	// Decisions is one decision for every proposed persona. The step 112 body (only the accepted
	// personas, as "personas") is no longer accepted: it has no decision for the others.
	Decisions []Decision `json:"decisions"`
}

// AiRunApplyHandler handles POST persona-ai-runs/apply.
func AiRunApplyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body applyAiRunRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RunID == "" {
		http.Error(w, "runId and decisions are required", http.StatusBadRequest)
		return
	}
	if body.Decisions == nil {
		http.Error(w, "decisions is required: a decision for every proposed persona", http.StatusBadRequest)
		return
	}
	created, err := ApplyDecisions(body.RunID, body.Decisions)
	if err != nil {
		switch {
		case errors.Is(err, ErrUnknownAiRun):
			http.Error(w, "unknown run id", http.StatusBadRequest)
		case errors.Is(err, ErrInvalidProposal):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, ErrAiRunNotReady), errors.Is(err, ErrAiRunDecided):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			http.Error(w, "failed to decide on the run: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}
	db.WriteJSON(w, map[string]any{"personas": created})
}
