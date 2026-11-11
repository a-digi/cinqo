package optimize

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
)

// The runs of "Optimize with AI" (plan 258): one row per attempt of a profile or of a persona. The page starts the hidden conversation,
// records the run BEFORE the first message so a reload can resume it, and finishes it when the AI's turn ends. The suggestions wait on
// the run until the user decides on each of them; a run that is left without any is removed.

const (
	maxFocus    = 500
	maxRunError = 2000

	runGenerating = "generating"
	runReady      = "ready"
	runFailed     = "failed"

	// DriverLocal is a run done by the code driver of a small local model (no conversation); '' is the tool conversation of a remote model.
	DriverLocal = "local"

	// The AI turn runs in the core app, so a restart of this tool does not orphan it; a run still 'generating' after this long lost its
	// page (tab closed, app crashed) before the finish call.
	staleRunThreshold = 30 * time.Minute
	staleRunMessage   = "The run never finished (the page was closed before the AI answered). Open the chat to see what happened."
)

var (
	ErrUnknownRun  = errors.New("unknown optimize run id")
	ErrRunFinished = errors.New("this run is already finished")
	// ErrRunOpen: the subject already has a run that is generating or waiting for the user's decisions.
	ErrRunOpen = errors.New("this already has an optimization that is running or waiting for your review: finish or dismiss it first")
	// ErrInvalid is a request the server refuses (a missing field, a bad value): the message says which.
	ErrInvalid = errors.New("invalid request")
)

// Suggestion is one improved text waiting for the user. Stale is true when the subject no longer shows Before (it was changed after the
// AI read it): such a suggestion can only be declined.
type Suggestion struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	ItemID string `json:"itemId"`
	Field  string `json:"field"`
	Label  string `json:"label"`
	Before string `json:"before"`
	After  string `json:"after"`
	Reason string `json:"reason"`
	Stale  bool   `json:"stale"`
}

// Run is one optimization of a subject with the suggestions that still wait.
type Run struct {
	ID             string       `json:"id"`
	SubjectKind    string       `json:"subjectKind"`
	SubjectID      string       `json:"subjectId"`
	ConversationID string       `json:"conversationId"`
	Driver         string       `json:"driver"`
	Focus          string       `json:"focus"`
	Status         string       `json:"status"`
	Error          string       `json:"error,omitempty"`
	CreatedAt      string       `json:"createdAt"`
	FinishedAt     string       `json:"finishedAt,omitempty"`
	Suggestions    []Suggestion `json:"suggestions"`
}

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

func subjectExists(kind, id string) (bool, error) {
	var table string
	switch kind {
	case SubjectProfile:
		table = "profiles"
	case SubjectPersona:
		table = "personas"
	default:
		return false, nil
	}
	var one int
	switch err := db.CareerDB.QueryRow(`SELECT 1 FROM `+table+` WHERE id = ?`, id).Scan(&one); {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, err
	}
	return true, nil
}

// CreateRun records the start of a run (status 'generating'). A remote run needs its conversation; a local run has none. The subject
// may have only one open run: ErrRunOpen.
func CreateRun(subjectKind, subjectID, conversationID, driver, focus string) (string, error) {
	if subjectKind != SubjectProfile && subjectKind != SubjectPersona {
		return "", invalid("subjectKind must be %q or %q", SubjectProfile, SubjectPersona)
	}
	if driver != "" && driver != DriverLocal {
		return "", invalid("driver must be empty or %q", DriverLocal)
	}
	conversationID = strings.TrimSpace(conversationID)
	if driver == "" && conversationID == "" {
		return "", invalid("conversationId is required")
	}
	focus = strings.TrimSpace(focus)
	if utf8.RuneCountInString(focus) > maxFocus {
		return "", invalid("focus is too long (max %d characters)", maxFocus)
	}
	ok, err := subjectExists(subjectKind, subjectID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrUnknownSubject
	}
	// A run that lost its page must not block the next one.
	if _, err := ReconcileStale(); err != nil {
		return "", err
	}
	id := uuid.NewString()
	if _, err := db.CareerDB.Exec(
		`INSERT INTO optimize_runs (id, subject_kind, subject_id, conversation_id, driver, focus) VALUES (?, ?, ?, ?, ?, ?)`,
		id, subjectKind, subjectID, conversationID, driver, focus,
	); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return "", ErrRunOpen
		}
		return "", err
	}
	return id, nil
}

// FinishRun ends a generating run: 'ready' (the AI's turn ended) or 'failed' with an error. It returns how many suggestions wait. A ready
// run without any suggestion (the AI found nothing to improve) is removed, and 0 is returned: there is nothing to review.
func FinishRun(id, status, errText string) (int, error) {
	switch status {
	case runReady, runFailed:
	default:
		return 0, invalid("status must be ready or failed")
	}
	errText = strings.TrimSpace(errText)
	if status == runFailed {
		if errText == "" {
			return 0, invalid("error is required for a failed run")
		}
		if utf8.RuneCountInString(errText) > maxRunError {
			errText = string([]rune(errText)[:maxRunError])
		}
	} else {
		errText = ""
	}

	tx, err := db.CareerDB.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var current string
	switch err := tx.QueryRow(`SELECT status FROM optimize_runs WHERE id = ?`, id).Scan(&current); {
	case errors.Is(err, sql.ErrNoRows):
		return 0, ErrUnknownRun
	case err != nil:
		return 0, err
	}
	if current != runGenerating {
		return 0, ErrRunFinished
	}
	var waiting int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM optimize_suggestions WHERE run_id = ?`, id).Scan(&waiting); err != nil {
		return 0, err
	}
	if status == runReady && waiting == 0 {
		if _, err := tx.Exec(`DELETE FROM optimize_runs WHERE id = ?`, id); err != nil {
			return 0, err
		}
		return 0, tx.Commit()
	}
	if _, err := tx.Exec(
		`UPDATE optimize_runs SET status = ?, error = ?, finished_at = datetime('now') WHERE id = ?`, status, errText, id,
	); err != nil {
		return 0, err
	}
	return waiting, tx.Commit()
}

// DeleteRun dismisses a run and its suggestions. A no-op for an unknown id.
func DeleteRun(id string) error {
	_, err := db.CareerDB.Exec(`DELETE FROM optimize_runs WHERE id = ?`, id)
	return err
}

// ReconcileStale ends every run still 'generating' after the threshold as failed and returns how many. Called at start-up, by the list
// and before a new run, so a stuck row heals without a restart.
func ReconcileStale() (int64, error) {
	cutoff := fmt.Sprintf("-%d seconds", int64(staleRunThreshold/time.Second))
	res, err := db.CareerDB.Exec(
		`UPDATE optimize_runs SET status = 'failed', error = ?, finished_at = datetime('now')
		 WHERE status = 'generating' AND created_at < datetime('now', ?)`,
		staleRunMessage, cutoff,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ListRuns returns a subject's runs, newest first, each with its waiting suggestions. A suggestion carries the label of its text and
// whether it is stale (the subject no longer shows the text the AI read).
func ListRuns(subjectKind, subjectID string) ([]Run, error) {
	subject, err := ReadSubject(subjectKind, subjectID)
	if err != nil {
		return nil, err
	}
	if _, err := ReconcileStale(); err != nil {
		return nil, err
	}
	rows, err := db.CareerDB.Query(
		`SELECT id, subject_kind, subject_id, conversation_id, driver, focus, status, error, created_at, finished_at
		 FROM optimize_runs WHERE subject_kind = ? AND subject_id = ? ORDER BY created_at DESC, rowid DESC`, subjectKind, subjectID)
	if err != nil {
		return nil, err
	}
	runs := []Run{}
	for rows.Next() {
		var r Run
		var finished sql.NullString
		if err := rows.Scan(&r.ID, &r.SubjectKind, &r.SubjectID, &r.ConversationID, &r.Driver, &r.Focus, &r.Status, &r.Error, &r.CreatedAt, &finished); err != nil {
			rows.Close()
			return nil, err
		}
		r.FinishedAt = finished.String
		r.Suggestions = []Suggestion{}
		runs = append(runs, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	for i := range runs {
		sRows, err := db.CareerDB.Query(
			`SELECT id, kind, item_id, field, before_text, after_text, reason FROM optimize_suggestions WHERE run_id = ? ORDER BY position`, runs[i].ID)
		if err != nil {
			return nil, err
		}
		for sRows.Next() {
			var s Suggestion
			if err := sRows.Scan(&s.ID, &s.Kind, &s.ItemID, &s.Field, &s.Before, &s.After, &s.Reason); err != nil {
				sRows.Close()
				return nil, err
			}
			cur, found := subject.find(s.Kind, s.ItemID, s.Field)
			s.Label = cur.Label
			s.Stale = !found || cur.Text != s.Before
			runs[i].Suggestions = append(runs[i].Suggestions, s)
		}
		if err := sRows.Err(); err != nil {
			sRows.Close()
			return nil, err
		}
		sRows.Close()
	}
	return runs, nil
}

type createRunRequest struct {
	SubjectKind    string `json:"subjectKind"`
	SubjectID      string `json:"subjectId"`
	ConversationID string `json:"conversationId"`
	Driver         string `json:"driver"`
	Focus          string `json:"focus"`
}

type finishRunRequest struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error"`
}

func writeError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, ErrUnknownSubject), errors.Is(err, ErrUnknownRun):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrInvalid):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrRunFinished), errors.Is(err, ErrRunOpen):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
	}
}

// RunsHandler handles GET/POST/PUT/DELETE optimize-runs.
func RunsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		kind, id := r.URL.Query().Get("subjectKind"), r.URL.Query().Get("subjectId")
		if kind == "" || id == "" {
			http.Error(w, "subjectKind and subjectId query parameters are required", http.StatusBadRequest)
			return
		}
		runs, err := ListRuns(kind, id)
		if err != nil {
			writeError(w, "list runs", err)
			return
		}
		db.WriteJSON(w, map[string]any{"runs": runs})

	case http.MethodPost:
		var body createRunRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "subjectKind, subjectId and conversationId are required", http.StatusBadRequest)
			return
		}
		id, err := CreateRun(body.SubjectKind, body.SubjectID, body.ConversationID, body.Driver, body.Focus)
		if err != nil {
			writeError(w, "record run", err)
			return
		}
		db.WriteJSON(w, map[string]any{"id": id})

	case http.MethodPut:
		var body finishRunRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		n, err := FinishRun(body.ID, body.Status, body.Error)
		if err != nil {
			writeError(w, "finish run", err)
			return
		}
		db.WriteJSON(w, map[string]any{"suggestions": n})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := DeleteRun(id); err != nil {
			http.Error(w, "failed to delete run: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// SubjectHandler handles GET optimize-runs/subject: the texts the AI is given.
func SubjectHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	kind, id := r.URL.Query().Get("subjectKind"), r.URL.Query().Get("subjectId")
	if kind == "" || id == "" {
		http.Error(w, "subjectKind and subjectId query parameters are required", http.StatusBadRequest)
		return
	}
	s, err := ReadSubject(kind, id)
	if err != nil {
		writeError(w, "read the texts", err)
		return
	}
	db.WriteJSON(w, s)
}
