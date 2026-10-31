package persona

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/google/uuid"

	"career-tool-backend/db"
)

// Deciding on a proposal (steps 112 and 113): each proposed persona is accepted or declined by the
// user, one at a time or several at once. An accepted persona (possibly edited) is created, with its
// details, skills and experience, under the run's own profile and linked to the run; a declined one
// is recorded on the run and nothing is created. A request is all or nothing. The run is closed
// (decided_at) once EVERY proposed persona has a decision, so a persona is never decided twice and
// the AI itself never creates one. See plan/ai/tools/career/step-112-create-personas-with-ai.md and
// plan/ai/tools/career/step-113-human-in-the-loop-for-ai-personas.md.

var (
	ErrAiRunNotReady = errors.New("this run has no proposal to decide on")
	ErrAiRunDecided  = errors.New("this run was already decided")
	// ErrPersonaDecided: a persona in the request already has a decision.
	ErrPersonaDecided = errors.New("this persona was already decided")
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
	decided   map[int]bool    // the proposed personas that already have a decision
	declined  []aiRunDeclined // the ones declined so far
}

// loadDecidableRun reads the run inside tx and checks it can still be decided: it exists, it is
// 'ready' with something proposed, and it is not closed yet.
func loadDecidableRun(tx *sql.Tx, runID string) (decidableRun, error) {
	run := decidableRun{decided: map[int]bool{}}
	var status string
	var decidedAt, proposalJSON, indexesJSON, declinedJSON sql.NullString
	switch err := tx.QueryRow(
		`SELECT profile_id, status, decided_at, proposal_json, decided_indexes, declined_json FROM persona_ai_runs WHERE id = ?`, runID,
	).Scan(&run.profileID, &status, &decidedAt, &proposalJSON, &indexesJSON, &declinedJSON); {
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
	var indexes []int
	if indexesJSON.Valid {
		_ = json.Unmarshal([]byte(indexesJSON.String), &indexes)
	}
	for _, i := range indexes {
		run.decided[i] = true
	}
	if declinedJSON.Valid {
		_ = json.Unmarshal([]byte(declinedJSON.String), &run.declined)
	}
	return run, nil
}

// ApplyDecisions records the user's decisions and creates the accepted personas, returned in the
// order of the proposal. The request must name at least one proposed persona, each at most once and
// none that already has a decision; an accepted one carries its final content (validated like a
// proposal, with skills trimmed and de-duplicated), a declined one carries none. The run is closed
// when every proposed persona has a decision.
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
	accepted, declined, indexes, err := resolveDecisions(run, decisions)
	if err != nil {
		return nil, err
	}
	return commitDecision(tx, run, runID, accepted, declined, indexes)
}

// resolveDecisions checks the decisions against the proposal and what is already decided, and splits
// them: the accepted personas (normalized, in proposal order), the names of the declined ones (as
// proposed) and the positions decided by this request.
func resolveDecisions(run decidableRun, decisions []Decision) ([]ProposedPersona, []string, []int, error) {
	if len(decisions) == 0 {
		return nil, nil, nil, invalid("no decision was sent")
	}
	byIndex := make(map[int]Decision, len(decisions))
	for _, d := range decisions {
		if d.Index < 0 || d.Index >= len(run.proposed) {
			return nil, nil, nil, invalid("decision for persona %d: there is no such proposed persona", d.Index)
		}
		if _, dup := byIndex[d.Index]; dup {
			return nil, nil, nil, invalid("persona %d has more than one decision", d.Index)
		}
		if run.decided[d.Index] {
			return nil, nil, nil, fmt.Errorf("%w: persona %d", ErrPersonaDecided, d.Index)
		}
		switch d.Decision {
		case decisionAccepted:
			if d.Persona == nil {
				return nil, nil, nil, invalid("persona %d is accepted but has no content", d.Index)
			}
		case decisionDeclined:
			if d.Persona != nil {
				return nil, nil, nil, invalid("persona %d is declined, so it must not carry content", d.Index)
			}
		default:
			return nil, nil, nil, invalid("persona %d: the decision must be %q or %q", d.Index, decisionAccepted, decisionDeclined)
		}
		byIndex[d.Index] = d
	}
	indexes := make([]int, 0, len(byIndex))
	for i := range byIndex {
		indexes = append(indexes, i)
	}
	sort.Ints(indexes)
	var accepted []ProposedPersona
	var declined []string
	for _, i := range indexes {
		d := byIndex[i]
		if d.Decision == decisionAccepted {
			accepted = append(accepted, normalizePersona(*d.Persona))
		} else {
			declined = append(declined, run.proposed[i].Name)
		}
	}
	if len(accepted) > 0 {
		if err := ValidateProposal(Proposal{Personas: accepted}); err != nil {
			return nil, nil, nil, err
		}
	}
	return accepted, declined, indexes, nil
}

// commitDecision creates the accepted personas, records the declined names and the positions now
// decided, closes the run when nothing is left to decide, and commits, all in tx.
func commitDecision(tx *sql.Tx, run decidableRun, runID string, accepted []ProposedPersona, newDeclined []string, indexes []int) ([]aiRunPersona, error) {
	created := make([]aiRunPersona, 0, len(accepted))
	for _, p := range accepted {
		id, err := insertAppliedPersona(tx, run.profileID, runID, p)
		if err != nil {
			return nil, fmt.Errorf("failed to create persona %q: %w", p.Name, err)
		}
		created = append(created, aiRunPersona{ID: id, Name: p.Name})
	}
	declined := run.declined
	for _, n := range newDeclined {
		declined = append(declined, aiRunDeclined{Name: n})
	}
	var declinedJSON any // NULL when nothing was declined
	if len(declined) > 0 {
		raw, err := json.Marshal(declined)
		if err != nil {
			return nil, err
		}
		declinedJSON = string(raw)
	}
	for _, i := range indexes {
		run.decided[i] = true
	}
	all := make([]int, 0, len(run.decided))
	for i := range run.decided {
		all = append(all, i)
	}
	sort.Ints(all)
	rawIndexes, err := json.Marshal(all)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(
		`UPDATE persona_ai_runs SET declined_json = ?, decided_indexes = ? WHERE id = ?`, declinedJSON, string(rawIndexes), runID,
	); err != nil {
		return nil, err
	}
	if len(all) == len(run.proposed) {
		if _, err := tx.Exec(`UPDATE persona_ai_runs SET decided_at = datetime('now') WHERE id = ?`, runID); err != nil {
			return nil, err
		}
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
		http.Error(w, "decisions is required", http.StatusBadRequest)
		return
	}
	created, err := ApplyDecisions(body.RunID, body.Decisions)
	if err != nil {
		switch {
		case errors.Is(err, ErrUnknownAiRun):
			http.Error(w, "unknown run id", http.StatusBadRequest)
		case errors.Is(err, ErrInvalidProposal):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, ErrAiRunNotReady), errors.Is(err, ErrAiRunDecided), errors.Is(err, ErrPersonaDecided):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			http.Error(w, "failed to decide on the run: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}
	db.WriteJSON(w, map[string]any{"personas": created})
}
