// cv_link.go — attaching a CV to a job without the AI (step 98): a CV the
// user built by hand in the CV Builder ("manual"), or one picked from the
// library ("existing"). A job has one current CV (job_cv_pdfs, one row per
// job), so linking replaces whatever was there; unlinking deletes that row.
// Neither touches the CV document or its PDF, and neither writes
// job_cv_generations, which stays "every AI conversation". See
// plan/ai/tools/career/step-98-manual-and-existing-cv-for-a-job.md.
//
// cv_documents lives in career.db and job_cv_pdfs in jobs.db, so the link is
// a plain id (no cross-file foreign key). The media file id is always taken
// from the stored document, never from a request.
package jobs

import (
	"database/sql"
	"errors"

	"career-tool-backend/cvbuilder"
	"career-tool-backend/db"
)

const (
	CvSourceManual   = "manual"
	CvSourceExisting = "existing"
)

var (
	// ErrCvGenerationRunning: an AI generation owns the job's CV slot right now.
	ErrCvGenerationRunning = errors.New("an AI CV generation is running for this job")
	// ErrCvDocumentHasNoPdf: the document has no rendered PDF to attach.
	ErrCvDocumentHasNoPdf = errors.New("this CV has no PDF yet")
	// ErrInvalidCvLinkSource: source is neither "manual" nor "existing".
	ErrInvalidCvLinkSource = errors.New(`source must be "manual" or "existing"`)
	// ErrCvPersonaMissing: the document's persona (and so its profile) is gone.
	ErrCvPersonaMissing = errors.New("the persona of this CV no longer exists")
)

// LinkCv makes cvDocumentID the job's current CV. Idempotent: linking the same
// document again changes nothing but the timestamp. cvbuilder.ErrUnknownCvDocument
// and ErrUnknownJob pass through unchanged.
func LinkCv(jobID, cvDocumentID, source string) error {
	if source != CvSourceManual && source != CvSourceExisting {
		return ErrInvalidCvLinkSource
	}
	if err := RequireJobExists(jobID); err != nil {
		return err
	}
	doc, err := cvbuilder.GetCvDocument(cvDocumentID)
	if err != nil {
		return err
	}
	if doc.MediaFileID == "" {
		return ErrCvDocumentHasNoPdf
	}
	var profileID string
	err = db.CareerDB.QueryRow(`SELECT profile_id FROM personas WHERE id = ?`, doc.PersonaID).Scan(&profileID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrCvPersonaMissing
	}
	if err != nil {
		return err
	}

	tx, err := db.JobsDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := refuseWhileGenerating(tx, jobID); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO job_cv_pdfs (job_id, profile_id, status, conversation_id, media_file_id, cv_document_id, source, error, updated_at)
		 VALUES (?, ?, 'completed', NULL, ?, ?, ?, NULL, datetime('now'))
		 ON CONFLICT(job_id) DO UPDATE SET
			profile_id = excluded.profile_id,
			status = 'completed',
			conversation_id = NULL,
			media_file_id = excluded.media_file_id,
			cv_document_id = excluded.cv_document_id,
			source = excluded.source,
			error = NULL,
			updated_at = datetime('now')`,
		jobID, profileID, doc.MediaFileID, doc.ID, source,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// UnlinkCv removes the job's current CV (whichever way it was attached),
// leaving the CV document and its PDF alone. Idempotent: nothing linked is not
// an error. Refused while an AI generation is running.
func UnlinkCv(jobID string) error {
	if err := RequireJobExists(jobID); err != nil {
		return err
	}
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := refuseWhileGenerating(tx, jobID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM job_cv_pdfs WHERE job_id = ?`, jobID); err != nil {
		return err
	}
	return tx.Commit()
}

func refuseWhileGenerating(tx *sql.Tx, jobID string) error {
	var status string
	err := tx.QueryRow(`SELECT status FROM job_cv_pdfs WHERE job_id = ?`, jobID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if status == "generating" {
		return ErrCvGenerationRunning
	}
	return nil
}

// JobRef is a job as the delete-CV confirmation lists it.
type JobRef = cvbuilder.JobRef

// DocumentLinks implements cvbuilder.JobLinks over this package's functions;
// main.go registers it with cvbuilder.UseJobLinks.
type DocumentLinks struct{}

func (DocumentLinks) JobsUsing(documentID, mediaFileID string) ([]cvbuilder.JobRef, error) {
	return JobsUsingCv(documentID, mediaFileID)
}

func (DocumentLinks) UnlinkDocument(documentID, mediaFileID string) error {
	return UnlinkCvDocument(documentID, mediaFileID)
}

// usesDocument matches the jobs whose current CV is the given document: by
// document id, or by its Media file id (which also catches CVs the AI generated
// before job_cv_pdfs recorded the document, and rows from before the column).
const usesDocument = `(cv.cv_document_id = ? OR (cv.media_file_id IS NOT NULL AND cv.media_file_id <> '' AND cv.media_file_id = ?))`

// JobsUsingCv returns every job that uses the document, no cap.
func JobsUsingCv(documentID, mediaFileID string) ([]JobRef, error) {
	rows, err := db.JobsDB.Query(
		`SELECT j.id, j.title, COALESCE(j.company, '')
		 FROM job_cv_pdfs cv JOIN jobs j ON j.id = cv.job_id
		 WHERE `+usesDocument+`
		 ORDER BY j.title, j.id`,
		documentID, mediaFileID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []JobRef{}
	for rows.Next() {
		var r JobRef
		if err := rows.Scan(&r.ID, &r.Title, &r.Company); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UnlinkCvDocument removes the link of every job that uses the document. The
// audit rows in job_cv_generations are never touched.
func UnlinkCvDocument(documentID, mediaFileID string) error {
	_, err := db.JobsDB.Exec(
		`DELETE FROM job_cv_pdfs WHERE job_id IN (SELECT cv.job_id FROM job_cv_pdfs cv WHERE `+usesDocument+`)`,
		documentID, mediaFileID,
	)
	return err
}
