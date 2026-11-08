package cvbuilder

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
	"career-tool-backend/persona"
)

// The history of "create a CV with AI" runs (step 115): one row per attempt for a persona, with the
// hidden conversation that did it, the optional note on what the CV is for, how it ended and the CV
// the AI saved. The frontend starts the conversation, creates the row before the first message so a
// reload can resume, and finishes it when the turn ends. See
// plan/ai/tools/career/step-115-create-cv-with-ai.md.

const (
	maxCvAiFocus  = 500
	maxCvAiError  = 2000
	cvAiRunning   = "generating"
	cvAiReady     = "ready"
	cvAiFailed    = "failed"
	staleCvAiRun  = 30 * time.Minute
	staleCvAiText = "The run never finished (the page was closed before the AI answered). Open the chat to see what happened."
)

var (
	ErrUnknownCvAiRun    = errors.New("unknown cv ai run id")
	ErrCvAiRunFinished   = errors.New("this run is already finished")
	ErrCvAiRunInProgress = errors.New("an AI run is already creating a CV for this persona")
	ErrCvAiNothingSaved  = errors.New("the AI did not save a CV in this run")
	errInvalidCvAiRun    = errors.New("invalid cv ai run")
)

type CvAiRun struct {
	ID             string `json:"id"`
	PersonaID      string `json:"personaId"`
	ConversationID string `json:"conversationId"`
	Focus          string `json:"focus"`
	Status         string `json:"status"`
	CvDocumentID   string `json:"cvDocumentId,omitempty"`
	Error          string `json:"error,omitempty"`
	CreatedAt      string `json:"createdAt"`
	FinishedAt     string `json:"finishedAt,omitempty"`
}

// CreateCvAiRun records the start of a run (status 'generating'). Stale runs of the persona are
// ended first, so a stuck one never blocks a new run. ErrCvAiRunInProgress when one still works.
func CreateCvAiRun(personaID, conversationID, focus string) (string, error) {
	if strings.TrimSpace(conversationID) == "" {
		return "", fmt.Errorf("%w: conversationId is required", errInvalidCvAiRun)
	}
	focus = strings.TrimSpace(focus)
	if utf8.RuneCountInString(focus) > maxCvAiFocus {
		return "", fmt.Errorf("%w: focus is too long (max %d characters)", errInvalidCvAiRun, maxCvAiFocus)
	}
	if err := requirePersonaExists(personaID); err != nil {
		return "", err
	}
	if _, err := ReconcileStaleCvAiRuns(); err != nil {
		return "", err
	}
	id := uuid.NewString()
	if _, err := db.CareerDB.Exec(
		`INSERT INTO cv_ai_runs (id, persona_id, conversation_id, focus) VALUES (?, ?, ?, ?)`,
		id, personaID, conversationID, focus,
	); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return "", ErrCvAiRunInProgress
		}
		return "", err
	}
	return id, nil
}

// FinishCvAiRun ends a generating run: 'ready' only when the AI saved a CV in it, or 'failed' with an
// error. ErrUnknownCvAiRun, ErrCvAiRunFinished, ErrCvAiNothingSaved (the run is left generating, so
// the caller can record it as failed).
func FinishCvAiRun(id, status, errText string) error {
	switch status {
	case cvAiReady:
		var cv sql.NullString
		var current string
		switch err := db.CareerDB.QueryRow(`SELECT status, cv_document_id FROM cv_ai_runs WHERE id = ?`, id).Scan(&current, &cv); {
		case errors.Is(err, sql.ErrNoRows):
			return ErrUnknownCvAiRun
		case err != nil:
			return err
		}
		if current != cvAiRunning {
			return ErrCvAiRunFinished
		}
		if !cv.Valid {
			return ErrCvAiNothingSaved
		}
		errText = ""
	case cvAiFailed:
		errText = strings.TrimSpace(errText)
		if errText == "" {
			return fmt.Errorf("%w: error is required for a failed run", errInvalidCvAiRun)
		}
		if utf8.RuneCountInString(errText) > maxCvAiError {
			errText = string([]rune(errText)[:maxCvAiError])
		}
	default:
		return fmt.Errorf("%w: status must be ready or failed", errInvalidCvAiRun)
	}
	res, err := db.CareerDB.Exec(
		`UPDATE cv_ai_runs SET status = ?, error = ?, finished_at = datetime('now') WHERE id = ? AND status = 'generating'`,
		status, sqlNullIfEmpty(errText), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return nil
	}
	var exists int
	switch err := db.CareerDB.QueryRow(`SELECT 1 FROM cv_ai_runs WHERE id = ?`, id).Scan(&exists); {
	case errors.Is(err, sql.ErrNoRows):
		return ErrUnknownCvAiRun
	case err != nil:
		return err
	}
	return ErrCvAiRunFinished
}

// ListCvAiRuns returns a persona's runs, newest first. Stale runs are ended first.
func ListCvAiRuns(personaID string) ([]CvAiRun, error) {
	if _, err := ReconcileStaleCvAiRuns(); err != nil {
		return nil, err
	}
	rows, err := db.CareerDB.Query(
		`SELECT id, persona_id, conversation_id, focus, status, cv_document_id, error, created_at, finished_at
		 FROM cv_ai_runs WHERE persona_id = ? ORDER BY created_at DESC, rowid DESC`, personaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []CvAiRun{}
	for rows.Next() {
		var r CvAiRun
		var cv, errText, finished sql.NullString
		if err := rows.Scan(&r.ID, &r.PersonaID, &r.ConversationID, &r.Focus, &r.Status, &cv, &errText, &r.CreatedAt, &finished); err != nil {
			return nil, err
		}
		r.CvDocumentID, r.Error, r.FinishedAt = cv.String, errText.String, finished.String
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// DeleteCvAiRun removes a run from the history; the CV it made stays. A no-op for an unknown id.
func DeleteCvAiRun(id string) error {
	_, err := db.CareerDB.Exec(`DELETE FROM cv_ai_runs WHERE id = ?`, id)
	return err
}

// ReconcileStaleCvAiRuns ends every run still 'generating' after staleCvAiRun as failed and returns
// how many it ended.
func ReconcileStaleCvAiRuns() (int64, error) {
	cutoff := fmt.Sprintf("-%d seconds", int64(staleCvAiRun/time.Second))
	res, err := db.CareerDB.Exec(
		`UPDATE cv_ai_runs SET status = 'failed', error = ?, finished_at = datetime('now')
		 WHERE status = 'generating' AND created_at < datetime('now', ?)`, staleCvAiText, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

type createCvAiRunRequest struct {
	PersonaID      string `json:"personaId"`
	ConversationID string `json:"conversationId"`
	Focus          string `json:"focus"`
}

type finishCvAiRunRequest struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error"`
}

// CvAiRunsHandler handles GET/POST/PUT/DELETE cv-ai-runs.
func CvAiRunsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		personaID := r.URL.Query().Get("personaId")
		if personaID == "" {
			http.Error(w, "personaId query parameter is required", http.StatusBadRequest)
			return
		}
		if err := requirePersonaExists(personaID); err != nil {
			writeCvAiRunError(w, "load runs", err)
			return
		}
		runs, err := ListCvAiRuns(personaID)
		if err != nil {
			http.Error(w, "failed to list runs: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"runs": runs})

	case http.MethodPost:
		var body createCvAiRunRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PersonaID == "" || body.ConversationID == "" {
			http.Error(w, "personaId and conversationId are required", http.StatusBadRequest)
			return
		}
		id, err := CreateCvAiRun(body.PersonaID, body.ConversationID, body.Focus)
		if err != nil {
			writeCvAiRunError(w, "record run", err)
			return
		}
		db.WriteJSON(w, map[string]any{"id": id})

	case http.MethodPut:
		var body finishCvAiRunRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if err := FinishCvAiRun(body.ID, body.Status, body.Error); err != nil {
			writeCvAiRunError(w, "finish run", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := DeleteCvAiRun(id); err != nil {
			http.Error(w, "failed to delete run: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeCvAiRunError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, persona.ErrUnknownPersona):
		http.Error(w, "unknown persona id", http.StatusNotFound)
	case errors.Is(err, ErrUnknownCvAiRun):
		http.Error(w, "unknown run id", http.StatusNotFound)
	case errors.Is(err, ErrCvAiRunFinished), errors.Is(err, ErrCvAiRunInProgress):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, ErrCvAiNothingSaved), errors.Is(err, errInvalidCvAiRun):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, fmt.Sprintf("failed to %s: %v", action, err), http.StatusInternalServerError)
	}
}
