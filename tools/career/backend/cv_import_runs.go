// cv_import_runs.go — persists one row per completed CV analysis: the
// AI's own proposal (ai_proposal_json) and, once the user acts on it,
// what was actually saved (save_summary_json). Both are stored as
// opaque JSON blobs — this tool never re-parses or re-validates their
// contents, just round-trips whatever the frontend already computed
// (parseProposal.ts's own CVImportProposal, insertProposal.ts's own
// InsertResult) — so a schema change on either shape needs no backend
// change here. See plan/ai/media/step-05-career-history.md.
package main

import (
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

// errUnknownCVImportRun mirrors errUnknownProfile/errUnknownPersona's
// own established convention for a by-id operation on a missing row.
var errUnknownCVImportRun = errors.New("unknown cv import run id")

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
}

// createCVImportRun records a freshly-parsed AI proposal — called once
// the frontend successfully parses the AI's reply, before entering its
// own 'review' state.
func createCVImportRun(mediaFileID, originalFilename, conversationID string, aiProposal []byte) (string, error) {
	id := uuid.NewString()
	_, err := careerDB.Exec(
		`INSERT INTO cv_import_runs (id, media_file_id, original_filename, conversation_id, ai_proposal_json, created_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'))`,
		id, mediaFileID, originalFilename, conversationID, string(aiProposal),
	)
	if err != nil {
		return "", err
	}
	return id, nil
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
	_, err := careerDB.Exec(
		`UPDATE cv_import_runs SET save_summary_json = ?, updated_at = datetime('now') WHERE id = ?`,
		string(saveSummary), id,
	)
	return err
}

func requireCVImportRunExists(id string) error {
	var exists int
	err := careerDB.QueryRow(`SELECT 1 FROM cv_import_runs WHERE id = ?`, id).Scan(&exists)
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
func listCVImportRuns() ([]cvImportRun, error) {
	rows, err := careerDB.Query(
		`SELECT id, media_file_id, original_filename, conversation_id, ai_proposal_json, save_summary_json, created_at, updated_at
		FROM cv_import_runs ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := []cvImportRun{}
	for rows.Next() {
		var r cvImportRun
		var aiProposal string
		var saveSummary, updatedAt sql.NullString
		if err := rows.Scan(&r.ID, &r.MediaFileID, &r.OriginalFilename, &r.ConversationID, &aiProposal, &saveSummary, &r.CreatedAt, &updatedAt); err != nil {
			return nil, err
		}
		r.AIProposal = json.RawMessage(aiProposal)
		if saveSummary.Valid {
			r.SaveSummary = json.RawMessage(saveSummary.String)
		}
		r.UpdatedAt = updatedAt.String
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// deleteCVImportRun removes the row and returns the media_file_id it
// referenced, so the caller (deleteCVImportRunHandler) can best-effort
// forward a cleanup call to core Media before it's gone.
func deleteCVImportRun(id string) (mediaFileID string, err error) {
	if err := careerDB.QueryRow(`SELECT media_file_id FROM cv_import_runs WHERE id = ?`, id).Scan(&mediaFileID); err != nil {
		if err == sql.ErrNoRows {
			return "", errUnknownCVImportRun
		}
		return "", err
	}
	if _, err := careerDB.Exec(`DELETE FROM cv_import_runs WHERE id = ?`, id); err != nil {
		return "", err
	}
	return mediaFileID, nil
}
