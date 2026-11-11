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
	ErrCvAiRunInProgress = errors.New("an AI run is already creating a CV for this target role")
	ErrCvAiNothingSaved  = errors.New("the AI did not save a CV in this run")
	// Attaching a CV a person-less driver made (plan 253): the CV must be one of the run's own target role, and belong to no other run.
	ErrCvNotOfRun     = errors.New("this CV was not built from the run's target role")
	ErrCvOfAnotherRun = errors.New("this CV already belongs to another run")
	errInvalidCvAiRun = errors.New("invalid cv ai run")
)

type CvAiRun struct {
	ID             string `json:"id"`
	PersonaID      string `json:"personaId"`
	ConversationID string `json:"conversationId"`
	Focus          string `json:"focus"`
	Status         string `json:"status"`
	CvDocumentID   string `json:"cvDocumentId,omitempty"`
	// Choice is what a model-driven run handed over with hand_over_cv (plan 256) and the page makes the CV from; absent until then.
	Choice     *CvAiChoice `json:"choice,omitempty"`
	Error      string      `json:"error,omitempty"`
	CreatedAt  string      `json:"createdAt"`
	FinishedAt string      `json:"finishedAt,omitempty"`
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

// AttachCvToRun ties a CV that already exists to a generating run (plan 253). The model-driven path ties its CV in save_persona_cv; a run driven by code (a small
// local model that is only asked which design to use) makes the CV through the same route as the wizard and then needs this second half only. The guards are those of
// SaveCvAiDocument: the run exists, still works and has no CV; on top of them the CV must exist, must be built from the run's own target role (so a run cannot take
// another role's CV) and must not belong to another run. One UPDATE decides a race: the run takes the CV only while it still works and has none.
func AttachCvToRun(runID, cvDocumentID string) error {
	var personaID, status string
	var saved sql.NullString
	switch err := db.CareerDB.QueryRow(`SELECT persona_id, status, cv_document_id FROM cv_ai_runs WHERE id = ?`, runID).Scan(&personaID, &status, &saved); {
	case errors.Is(err, sql.ErrNoRows):
		return ErrUnknownCvAiRun
	case err != nil:
		return err
	}
	if status != cvAiRunning {
		return ErrCvAiRunFinished
	}
	if saved.Valid {
		return ErrCvAiAlreadySaved
	}
	doc, err := GetCvDocument(cvDocumentID)
	if err != nil {
		return err
	}
	if doc.PersonaID != personaID {
		return ErrCvNotOfRun
	}
	var other int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM cv_ai_runs WHERE cv_document_id = ? AND id <> ?`, cvDocumentID, runID).Scan(&other); err != nil {
		return err
	}
	if other > 0 {
		return ErrCvOfAnotherRun
	}
	res, err := db.CareerDB.Exec(
		`UPDATE cv_ai_runs SET cv_document_id = ? WHERE id = ? AND status = 'generating' AND cv_document_id IS NULL`, cvDocumentID, runID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrCvAiAlreadySaved
	}
	return nil
}

type attachCvToRunRequest struct {
	RunID        string `json:"runId"`
	CvDocumentID string `json:"cvDocumentId"`
}

// CvAiRunAttachHandler handles POST cv-ai-runs/attach {runId, cvDocumentId}: 204 when the CV is now the run's, 400 for a missing field, 404 for an unknown run or CV,
// 409 when the run is finished or already has a CV, or the CV is another role's or another run's.
func CvAiRunAttachHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body attachCvToRunRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RunID == "" || body.CvDocumentID == "" {
		http.Error(w, "runId and cvDocumentId are required", http.StatusBadRequest)
		return
	}
	if err := AttachCvToRun(body.RunID, body.CvDocumentID); err != nil {
		writeCvAiRunError(w, "attach the CV to the run", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListCvAiRuns returns a persona's runs, newest first. Stale runs are ended first.
func ListCvAiRuns(personaID string) ([]CvAiRun, error) {
	if _, err := ReconcileStaleCvAiRuns(); err != nil {
		return nil, err
	}
	rows, err := db.CareerDB.Query(
		`SELECT id, persona_id, conversation_id, focus, status, cv_document_id, error, created_at, finished_at, choice_json
		 FROM cv_ai_runs WHERE persona_id = ? ORDER BY created_at DESC, rowid DESC`, personaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []CvAiRun{}
	for rows.Next() {
		var r CvAiRun
		var cv, errText, finished, choice sql.NullString
		if err := rows.Scan(&r.ID, &r.PersonaID, &r.ConversationID, &r.Focus, &r.Status, &cv, &errText, &r.CreatedAt, &finished, &choice); err != nil {
			return nil, err
		}
		r.CvDocumentID, r.Error, r.FinishedAt = cv.String, errText.String, finished.String
		if choice.Valid {
			var c CvAiChoice
			// a stored choice that cannot be read is left out (the page then ends the run as failed), never a failure of the whole list
			if json.Unmarshal([]byte(choice.String), &c) == nil {
				r.Choice = &c
			}
		}
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
		http.Error(w, "unknown target role id", http.StatusNotFound)
	case errors.Is(err, ErrUnknownCvAiRun):
		http.Error(w, "unknown run id", http.StatusNotFound)
	case errors.Is(err, ErrUnknownCvDocument):
		http.Error(w, "unknown cv document id", http.StatusNotFound)
	case errors.Is(err, ErrCvAiRunFinished), errors.Is(err, ErrCvAiRunInProgress), errors.Is(err, ErrCvAiAlreadySaved), errors.Is(err, ErrCvNotOfRun), errors.Is(err, ErrCvOfAnotherRun):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, ErrCvAiNothingSaved), errors.Is(err, errInvalidCvAiRun):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, fmt.Sprintf("failed to %s: %v", action, err), http.StatusInternalServerError)
	}
}
