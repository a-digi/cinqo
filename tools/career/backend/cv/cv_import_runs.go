// cv_import_runs.go — persists one row per completed CV analysis: the
// AI's own proposal (ai_proposal_json) and, once the user acts on it,
// what was actually saved (save_summary_json). Both are stored as
// opaque JSON blobs — this tool never re-parses or re-validates their
// contents, just round-trips whatever the frontend already computed
// (parseProposal.ts's own CVImportProposal, insertProposal.ts's own
// InsertResult) — so a schema change on either shape needs no backend
// change here. See plan/ai/media/step-05-career-history.md.
package cv

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"career-tool-backend/db"
)

// errUnknownCVImportRun mirrors errUnknownProfile/errUnknownPersona's
// own established convention for a by-id operation on a missing row.
var errUnknownCVImportRun = errors.New("unknown cv import run id")

// The states of a run (step 155). A run is 'generating' from the moment the page records it until the AI's turn ends; the
// AI hands its proposal over with suggest_cv_import meanwhile, and the page then ends the run 'ready' or 'failed'.
const (
	runGenerating = "generating"
	runReady      = "ready"
	runFailed     = "failed"
)

var (
	// errRunNotGenerating: the run is not (or no longer) generating, so it takes no proposal and cannot be finished.
	errRunNotGenerating = errors.New("this cv import run is not generating")
	// errNoProposalStored: a run cannot end 'ready' when the AI never handed a proposal over.
	errNoProposalStored = errors.New("the AI did not hand over a proposal")
	// errInvalidFinish: a status other than ready/failed, or a failure without a reason.
	errInvalidFinish = errors.New("invalid cv import run outcome")
)

// staleCVImportRunThreshold: a run still 'generating' after this long is ended as failed (the AI's turn lives in the core app,
// so only the page that started it can finish the run; if nobody does, the server ends it, like the persona AI runs).
const staleCVImportRunThreshold = 30 * time.Minute

// afterFinishCheck runs between finishCVImportRun's check of the run and its update: a hook for tests (end the run in that gap).
var afterFinishCheck = func() {}

// noProposalJSON is what a generating run holds in ai_proposal_json (the column is NOT NULL and SQLite cannot relax that).
const noProposalJSON = "null"

type cvImportRun struct {
	ID               string `json:"id"`
	MediaFileID      string `json:"mediaFileId"`
	OriginalFilename string `json:"originalFilename"`
	ConversationID   string `json:"conversationId"`
	// AIProposal/SaveSummary are passed straight through as raw JSON —
	// json.RawMessage lets this tool store/return them without ever
	// unmarshaling into a concrete Go struct it would then have to keep
	// in lockstep with the frontend's own shapes.
	AIProposal  json.RawMessage `json:"aiProposal"`
	SaveSummary json.RawMessage `json:"saveSummary,omitempty"`
	CreatedAt   string          `json:"createdAt"`
	UpdatedAt   string          `json:"updatedAt,omitempty"`
	// Status is generating, ready or failed; Error says why a run failed (step 155).
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	// Driver is "code" for a run whose steps the page does itself (plan 209), empty for one the AI does with tools.
	Driver string `json:"driver,omitempty"`
	// Progress is what a generating run has stored so far (its draft), for the page that shows the run working; nil when nothing was stored.
	Progress *runProgress `json:"progress,omitempty"`
}

// createCVImportRun records a freshly-parsed AI proposal — called once
// the frontend successfully parses the AI's reply, before entering its
// own 'review' state.
func createCVImportRun(mediaFileID, originalFilename, conversationID string, aiProposal []byte) (string, error) {
	id := uuid.NewString()
	_, err := db.CareerDB.Exec(
		`INSERT INTO cv_import_runs (id, media_file_id, original_filename, conversation_id, ai_proposal_json, status, created_at)
		VALUES (?, ?, ?, ?, ?, 'ready', datetime('now'))`,
		id, mediaFileID, originalFilename, conversationID, string(aiProposal),
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

// createGeneratingCVImportRun records the start of an analysis (step 155): the run exists before the AI has said anything, with
// no proposal yet.
func createGeneratingCVImportRun(mediaFileID, originalFilename, conversationID string) (string, error) {
	return createGeneratingCVImportRunBy(mediaFileID, originalFilename, conversationID, "")
}

// createGeneratingCVImportRunBy is createGeneratingCVImportRun for a run with a driver ("code", or "" for the AI with tools).
func createGeneratingCVImportRunBy(mediaFileID, originalFilename, conversationID, driver string) (string, error) {
	id := uuid.NewString()
	var drv any
	if driver != "" {
		drv = driver
	}
	_, err := db.CareerDB.Exec(
		`INSERT INTO cv_import_runs (id, media_file_id, original_filename, conversation_id, ai_proposal_json, status, driver, created_at)
		VALUES (?, ?, ?, ?, ?, 'generating', ?, datetime('now'))`,
		id, mediaFileID, originalFilename, conversationID, noProposalJSON, drv,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

// storeCVImportProposal keeps the proposal the AI handed over on a generating run (a second call while the run is generating
// replaces the first). errUnknownCVImportRun for an unknown run, errRunNotGenerating for one that is ready or failed.
func storeCVImportProposal(runID string, proposal []byte) error {
	res, err := db.CareerDB.Exec(
		`UPDATE cv_import_runs SET ai_proposal_json = ?, updated_at = datetime('now') WHERE id = ? AND status = 'generating'`,
		string(proposal), runID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return notGeneratingOrUnknown(runID)
	}
	return nil
}

// notGeneratingOrUnknown says why an update of a generating run changed nothing.
func notGeneratingOrUnknown(runID string) error {
	if err := requireCVImportRunExists(runID); err != nil {
		return err
	}
	return errRunNotGenerating
}

// finishCVImportRun ends a generating run and returns it: 'ready' needs a stored proposal (errNoProposalStored otherwise),
// 'failed' needs a reason; any other status is errInvalidFinish. errUnknownCVImportRun, errRunNotGenerating as above.
func finishCVImportRun(id, status, errText string) (cvImportRun, error) {
	errText = strings.TrimSpace(errText)
	switch status {
	case runReady:
		errText = ""
	case runFailed:
		if errText == "" {
			return cvImportRun{}, fmt.Errorf("%w: a failed run needs an error", errInvalidFinish)
		}
	default:
		return cvImportRun{}, fmt.Errorf("%w: status must be ready or failed", errInvalidFinish)
	}
	var stored string
	switch err := db.CareerDB.QueryRow(`SELECT ai_proposal_json FROM cv_import_runs WHERE id = ? AND status = 'generating'`, id).Scan(&stored); {
	case errors.Is(err, sql.ErrNoRows):
		return cvImportRun{}, notGeneratingOrUnknown(id)
	case err != nil:
		return cvImportRun{}, err
	}
	if status == runReady && strings.TrimSpace(stored) == noProposalJSON {
		// The AI sent every piece but never called cv_import_finish (a weak model stops after the last piece, plan 165): a complete
		// draft is finished here, so the run does not fail for a missing last call. An incomplete one says what is missing.
		if err := FinishDraft(id); err != nil {
			var bad *proposalProblem
			if errors.As(err, &bad) {
				return cvImportRun{}, fmt.Errorf("%w (%s)", errNoProposalStored, bad.msg)
			}
			return cvImportRun{}, err
		}
	}
	var errArg any
	if errText != "" {
		errArg = errText
	}
	afterFinishCheck()
	res, err := db.CareerDB.Exec(
		`UPDATE cv_import_runs SET status = ?, error = ?, updated_at = datetime('now') WHERE id = ? AND status = 'generating'`,
		status, errArg, id,
	)
	if err != nil {
		return cvImportRun{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return cvImportRun{}, notGeneratingOrUnknown(id)
	}
	return getCVImportRun(id)
}

// ReconcileStaleCVImportRuns ends every run still 'generating' after staleCVImportRunThreshold as failed, and returns how many.
// Called at start, like the other runs' reconcile; never fatal.
func ReconcileStaleCVImportRuns() (int64, error) {
	cutoff := fmt.Sprintf("-%d seconds", int64(staleCVImportRunThreshold/time.Second))
	res, err := db.CareerDB.Exec(
		`UPDATE cv_import_runs SET status = 'failed', error = ?, updated_at = datetime('now')
		 WHERE status = 'generating' AND created_at < datetime('now', ?)`,
		"the analysis did not finish (the page was closed or the AI never answered)", cutoff,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// getCVImportRun reads one run.
func getCVImportRun(id string) (cvImportRun, error) {
	runs, err := queryCVImportRuns(`WHERE id = ?`, id)
	if err != nil {
		return cvImportRun{}, err
	}
	if len(runs) == 0 {
		return cvImportRun{}, errUnknownCVImportRun
	}
	return runs[0], nil
}

// updateCVImportRunSaveSummary records the current, full outcome of the
// user's own review — called after every "Insert" attempt (including
// retries), always with the complete accumulated state, so a retry's
// second call simply overwrites with the more-complete picture rather
// than needing to merge deltas server-side.
func updateCVImportRunSaveSummary(id string, saveSummary []byte) error {
	if err := requireCVImportRunExists(id); err != nil {
		return err
	}
	_, err := db.CareerDB.Exec(
		`UPDATE cv_import_runs SET save_summary_json = ?, updated_at = datetime('now') WHERE id = ?`,
		string(saveSummary), id,
	)
	return err
}

func requireCVImportRunExists(id string) error {
	var exists int
	err := db.CareerDB.QueryRow(`SELECT 1 FROM cv_import_runs WHERE id = ?`, id).Scan(&exists)
	switch {
	case err == sql.ErrNoRows:
		return errUnknownCVImportRun
	case err != nil:
		return err
	}
	return nil
}

// listCVImportRuns returns every run, newest first — this tool has no
// per-user scoping of its own data today (profiles/personas have none
// either), so this is every run this install has ever recorded, not
// scoped to "the current caller" the way core Media's own
// GET /api/v1/media/mine is.
func listCVImportRuns() ([]cvImportRun, error) { return queryCVImportRuns("") }

// queryCVImportRuns reads runs, newest first; where is "" or a "WHERE …" with its arguments.
func queryCVImportRuns(where string, args ...any) ([]cvImportRun, error) {
	rows, err := db.CareerDB.Query(
		`SELECT id, media_file_id, original_filename, conversation_id, ai_proposal_json, save_summary_json, created_at, updated_at, status, error, driver, draft_json
		FROM cv_import_runs `+where+` ORDER BY created_at DESC`, args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := []cvImportRun{}
	for rows.Next() {
		var r cvImportRun
		var aiProposal string
		var saveSummary, updatedAt, errText, driver, draft sql.NullString
		if err := rows.Scan(&r.ID, &r.MediaFileID, &r.OriginalFilename, &r.ConversationID, &aiProposal, &saveSummary, &r.CreatedAt, &updatedAt, &r.Status, &errText, &driver, &draft); err != nil {
			return nil, err
		}
		r.AIProposal = json.RawMessage(aiProposal)
		if saveSummary.Valid {
			r.SaveSummary = json.RawMessage(saveSummary.String)
		}
		r.UpdatedAt = updatedAt.String
		r.Error = errText.String
		r.Driver = driver.String
		if r.Status == runGenerating && draft.Valid {
			r.Progress = progressOf(draft.String)
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// deleteCVImportRun removes the row and returns the media_file_id it
// referenced, so the caller (deleteCVImportRunHandler) can best-effort
// forward a cleanup call to core Media before it's gone.
func deleteCVImportRun(id string) (mediaFileID string, err error) {
	if err := db.CareerDB.QueryRow(`SELECT media_file_id FROM cv_import_runs WHERE id = ?`, id).Scan(&mediaFileID); err != nil {
		if err == sql.ErrNoRows {
			return "", errUnknownCVImportRun
		}
		return "", err
	}
	if _, err := db.CareerDB.Exec(`DELETE FROM cv_import_runs WHERE id = ?`, id); err != nil {
		return "", err
	}
	return mediaFileID, nil
}

// cvImportMediaInUse says whether any run still references the media file.
func cvImportMediaInUse(mediaFileID string) (bool, error) {
	var n int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM cv_import_runs WHERE media_file_id = ?`, mediaFileID).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}
