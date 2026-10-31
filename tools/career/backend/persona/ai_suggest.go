package persona

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
)

// The AI hands its suggestions over one at a time through suggest_persona, which only stores a
// suggestion for the user to accept or decline: it creates no persona. The tool answers with the
// suggestion's id, and the AI's last message is a plain sentence, not data. When the run is
// finished without a proposal, the proposal is assembled from the suggestions stored for it.

var (
	ErrRunNotGenerating = errors.New("this run is not generating")
	errTooManySuggested = fmt.Errorf("a run suggests at most %d personas", MaxProposedPersonas)
)

// isEmptyJSON is true for no body at all and for a JSON null.
func isEmptyJSON(raw []byte) bool {
	t := bytes.TrimSpace(raw)
	return len(t) == 0 || string(t) == "null"
}

// AddSuggestion stores one suggested persona for a generating run and returns its id. The persona
// is validated like any proposal: ErrUnknownAiRun, ErrRunNotGenerating, ErrInvalidProposal, or too
// many / a repeated name.
func AddSuggestion(runID string, pp ProposedPersona) (string, error) {
	if err := ValidateProposedPersona(pp); err != nil {
		return "", err
	}
	if pp.Skills == nil {
		pp.Skills = []string{}
	}
	if pp.Experience == nil {
		pp.Experience = []ProposedRole{}
	}
	tx, err := db.CareerDB.Begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var status string
	switch err := tx.QueryRow(`SELECT status FROM persona_ai_runs WHERE id = ?`, runID).Scan(&status); {
	case err == sql.ErrNoRows:
		return "", ErrUnknownAiRun
	case err != nil:
		return "", err
	}
	if status != runGenerating {
		return "", ErrRunNotGenerating
	}
	existing, err := suggestionsOf(tx, runID)
	if err != nil {
		return "", err
	}
	if len(existing) >= MaxProposedPersonas {
		return "", errTooManySuggested
	}
	key := strings.ToLower(strings.TrimSpace(pp.Name))
	for _, e := range existing {
		if strings.ToLower(strings.TrimSpace(e.Name)) == key {
			return "", invalid("the name %q is already suggested in this run", pp.Name)
		}
	}
	raw, err := json.Marshal(pp)
	if err != nil {
		return "", err
	}
	id := uuid.NewString()
	if _, err := tx.Exec(
		`INSERT INTO persona_ai_suggestions (id, run_id, position, persona_json) VALUES (?, ?, ?, ?)`,
		id, runID, len(existing), string(raw),
	); err != nil {
		return "", err
	}
	return id, tx.Commit()
}

type queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// suggestionsOf returns the run's stored suggestions in the order they were made.
func suggestionsOf(q queryer, runID string) ([]ProposedPersona, error) {
	rows, err := q.Query(`SELECT persona_json FROM persona_ai_suggestions WHERE run_id = ? ORDER BY position`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProposedPersona{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var pp ProposedPersona
		if err := json.Unmarshal([]byte(raw), &pp); err != nil {
			return nil, err
		}
		out = append(out, pp)
	}
	return out, rows.Err()
}

// proposalFromSuggestions is the proposal of a run that was finished without one.
func proposalFromSuggestions(runID string) (Proposal, error) {
	list, err := suggestionsOf(db.CareerDB, runID)
	if err != nil {
		return Proposal{}, err
	}
	return Proposal{Personas: list}, nil
}

type suggestPersonaArgs struct {
	RunID   string          `json:"runId" jsonschema:"the run id given in the request"`
	Persona ProposedPersona `json:"persona" jsonschema:"one suggested persona, built only from data the user already filled in"`
}

func RegisterSuggestPersona(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "suggest_persona",
		Description: "Hand over ONE suggested persona to the user, who then accepts or declines it on the Personas page. " +
			"It creates no persona. Call it once per suggestion (at most 3 per run), with the runId from the request. " +
			"Returns the suggestion's id. Fails if the run is unknown or no longer running, the persona is invalid, " +
			"or its name was already suggested in this run.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args suggestPersonaArgs) (*mcp.CallToolResult, any, error) {
		if args.RunID == "" {
			return db.ErrResult("runId is required"), nil, nil
		}
		id, err := AddSuggestion(args.RunID, args.Persona)
		switch {
		case errors.Is(err, ErrUnknownAiRun):
			return db.ErrResult(fmt.Sprintf("unknown run id %q", args.RunID)), nil, nil
		case err != nil:
			return db.ErrResult(err.Error()), nil, nil
		}
		return db.JSONResult(map[string]any{"suggestionId": id})
	})
}
