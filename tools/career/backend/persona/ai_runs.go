package persona

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"career-tool-backend/db"
	"career-tool-backend/profile"
)

// The history of "create personas with AI" runs (step 112): one row per attempt of a profile,
// with the hidden conversation that did it, what the user asked for, how it ended and the AI's
// proposal. The frontend starts the conversation, creates the row before the first message so a
// reload can resume, and finishes it when the turn ends (ready with a proposal, or failed). See
// plan/ai/tools/career/step-112-create-personas-with-ai.md.

const (
	maxFocus      = 500
	maxRunError   = 2000
	runGenerating = "generating"
	runReady      = "ready"
	runFailed     = "failed"

	// The AI turn runs in the core app, not here, so a restart of this tool does not orphan it.
	// A run still 'generating' after this long lost its page (tab closed, app crashed) before
	// the finish call, and is ended as failed.
	staleAiRunThreshold    = 30 * time.Minute
	staleAiRunErrorMessage = "The run never finished (the page was closed before the AI answered). Open the chat to see what happened."
)

var (
	ErrUnknownAiRun  = errors.New("unknown persona ai run id")
	ErrAiRunFinished = errors.New("this run is already finished")
)

type aiRunPersona struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type aiRun struct {
	ID             string          `json:"id"`
	ProfileID      string          `json:"profileId"`
	ConversationID string          `json:"conversationId"`
	Focus          string          `json:"focus"`
	Status         string          `json:"status"`
	Proposal       json.RawMessage `json:"proposal,omitempty"`
	Error          string          `json:"error,omitempty"`
	CreatedAt      string          `json:"createdAt"`
	FinishedAt     string          `json:"finishedAt,omitempty"`
	// The personas created from this run; empty until a proposal is applied.
	Personas []aiRunPersona `json:"personas"`
}

// CreateAiRun records the start of a run (status 'generating').
func CreateAiRun(profileID, conversationID, focus string) (string, error) {
	if strings.TrimSpace(conversationID) == "" {
		return "", fmt.Errorf("conversationId is required")
	}
	focus = strings.TrimSpace(focus)
	if utf8.RuneCountInString(focus) > maxFocus {
		return "", fmt.Errorf("focus is too long (max %d characters)", maxFocus)
	}
	if err := profile.RequireProfileExists(profileID); err != nil {
		return "", err
	}
	id := uuid.NewString()
	if _, err := db.CareerDB.Exec(
		`INSERT INTO persona_ai_runs (id, profile_id, conversation_id, focus) VALUES (?, ?, ?, ?)`,
		id, profileID, conversationID, focus,
	); err != nil {
		return "", err
	}
	return id, nil
}

// FinishAiRun ends a generating run: 'ready' with a valid proposal, or 'failed' with an error.
// ErrUnknownAiRun for an unknown id, ErrAiRunFinished when it is no longer generating,
// ErrInvalidProposal for a proposal that fails validation.
func FinishAiRun(id, status string, proposalRaw []byte, errText string) error {
	var stored any
	switch status {
	case runReady:
		p, err := ParseProposal(proposalRaw)
		if err != nil {
			return err
		}
		normalized, err := json.Marshal(p)
		if err != nil {
			return err
		}
		stored = string(normalized)
		errText = ""
	case runFailed:
		errText = strings.TrimSpace(errText)
		if errText == "" {
			return fmt.Errorf("%w: error is required for a failed run", ErrInvalidProposal)
		}
		if utf8.RuneCountInString(errText) > maxRunError {
			errText = string([]rune(errText)[:maxRunError])
		}
	default:
		return fmt.Errorf("%w: status must be ready or failed", ErrInvalidProposal)
	}

	res, err := db.CareerDB.Exec(
		`UPDATE persona_ai_runs SET status = ?, proposal_json = ?, error = ?, finished_at = datetime('now')
		 WHERE id = ? AND status = 'generating'`,
		status, stored, nullIfEmpty(errText), id,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return nil
	}
	var exists int
	switch err := db.CareerDB.QueryRow(`SELECT 1 FROM persona_ai_runs WHERE id = ?`, id).Scan(&exists); {
	case err == sql.ErrNoRows:
		return ErrUnknownAiRun
	case err != nil:
		return err
	}
	return ErrAiRunFinished
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ListAiRuns returns a profile's runs, newest first, each with the personas created from it.
// Runs that are stale are ended first, so a stuck one heals without a restart.
func ListAiRuns(profileID string) ([]aiRun, error) {
	if _, err := ReconcileStaleAiRuns(); err != nil {
		return nil, err
	}
	rows, err := db.CareerDB.Query(
		`SELECT id, profile_id, conversation_id, focus, status, proposal_json, error, created_at, finished_at
		 FROM persona_ai_runs WHERE profile_id = ? ORDER BY created_at DESC, rowid DESC`, profileID)
	if err != nil {
		return nil, err
	}
	runs := []aiRun{}
	index := map[string]int{}
	for rows.Next() {
		var r aiRun
		var proposal, errText, finished sql.NullString
		if err := rows.Scan(&r.ID, &r.ProfileID, &r.ConversationID, &r.Focus, &r.Status, &proposal, &errText, &r.CreatedAt, &finished); err != nil {
			rows.Close()
			return nil, err
		}
		if proposal.Valid {
			r.Proposal = json.RawMessage(proposal.String)
		}
		r.Error = errText.String
		r.FinishedAt = finished.String
		r.Personas = []aiRunPersona{}
		index[r.ID] = len(runs)
		runs = append(runs, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	pRows, err := db.CareerDB.Query(
		`SELECT ai_run_id, id, name FROM personas
		 WHERE ai_run_id IN (SELECT id FROM persona_ai_runs WHERE profile_id = ?)
		 ORDER BY created_at ASC, rowid ASC`, profileID)
	if err != nil {
		return nil, err
	}
	defer pRows.Close()
	for pRows.Next() {
		var runID string
		var p aiRunPersona
		if err := pRows.Scan(&runID, &p.ID, &p.Name); err != nil {
			return nil, err
		}
		if i, ok := index[runID]; ok {
			runs[i].Personas = append(runs[i].Personas, p)
		}
	}
	return runs, pRows.Err()
}

// DeleteAiRun removes a run from the history. The personas created from it stay (their
// ai_run_id becomes NULL). A no-op for an unknown id.
func DeleteAiRun(id string) error {
	_, err := db.CareerDB.Exec(`DELETE FROM persona_ai_runs WHERE id = ?`, id)
	return err
}

// ReconcileStaleAiRuns ends every run still 'generating' after staleAiRunThreshold as failed. It
// returns how many it ended. Called at boot and from the list, so a stuck row heals without a
// restart.
func ReconcileStaleAiRuns() (int64, error) {
	cutoff := fmt.Sprintf("-%d seconds", int64(staleAiRunThreshold/time.Second))
	res, err := db.CareerDB.Exec(
		`UPDATE persona_ai_runs SET status = 'failed', error = ?, finished_at = datetime('now')
		 WHERE status = 'generating' AND created_at < datetime('now', ?)`,
		staleAiRunErrorMessage, cutoff,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

type createAiRunRequest struct {
	ProfileID      string `json:"profileId"`
	ConversationID string `json:"conversationId"`
	Focus          string `json:"focus"`
}

type finishAiRunRequest struct {
	ID       string          `json:"id"`
	Status   string          `json:"status"`
	Proposal json.RawMessage `json:"proposal"`
	Error    string          `json:"error"`
}

// AiRunsHandler handles GET/POST/PUT/DELETE persona-ai-runs.
func AiRunsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		profileID := r.URL.Query().Get("profileId")
		if profileID == "" {
			http.Error(w, "profileId query parameter is required", http.StatusBadRequest)
			return
		}
		if err := profile.RequireProfileExists(profileID); err != nil {
			writeAiRunError(w, "load runs", err)
			return
		}
		runs, err := ListAiRuns(profileID)
		if err != nil {
			http.Error(w, "failed to list runs: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"runs": runs})

	case http.MethodPost:
		var body createAiRunRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProfileID == "" || body.ConversationID == "" {
			http.Error(w, "profileId and conversationId are required", http.StatusBadRequest)
			return
		}
		id, err := CreateAiRun(body.ProfileID, body.ConversationID, body.Focus)
		if err != nil {
			writeAiRunError(w, "record run", err)
			return
		}
		db.WriteJSON(w, map[string]any{"id": id})

	case http.MethodPut:
		var body finishAiRunRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if err := FinishAiRun(body.ID, body.Status, body.Proposal, body.Error); err != nil {
			writeAiRunError(w, "finish run", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := DeleteAiRun(id); err != nil {
			http.Error(w, "failed to delete run: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeAiRunError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, profile.ErrUnknownProfile):
		http.Error(w, "unknown profile id", http.StatusBadRequest)
	case errors.Is(err, ErrUnknownAiRun):
		http.Error(w, "unknown run id", http.StatusBadRequest)
	case errors.Is(err, ErrAiRunFinished):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, ErrInvalidProposal):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		// A validation message from CreateAiRun (required field, too long) is the caller's.
		if strings.HasPrefix(err.Error(), "conversationId") || strings.HasPrefix(err.Error(), "focus") {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
	}
}
