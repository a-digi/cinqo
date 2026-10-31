package cvbuilder

import (
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	"career-tool-backend/cvbuilder/templates"
	"career-tool-backend/db"
)

// CvDocument is one generated CV — persona_id/template_id/title are
// plain columns, Data is data_json unmarshaled back into shape, and
// MediaFileID is the resulting PDF's own permanent Media file id (see
// plan/ai/career/cv-builder/step-01-overview-and-data-model.md).
// JobID is "" for every document the human-facing wizard creates —
// only ever set for one the AI generated for a specific job posting
// (jobs/cv_pdf.go's own save_cv_document). See
// plan/ai/career/cv-builder/step-08-ai-generated-cv-documents.md.
type CvDocument struct {
	ID          string           `json:"id"`
	PersonaID   string           `json:"personaId"`
	TemplateID  string           `json:"templateId"`
	Title       string           `json:"title"`
	Data        templates.CvData `json:"data"`
	MediaFileID string           `json:"mediaFileId"`
	JobID       string           `json:"jobId,omitempty"`
	// PhotoMediaFileID is the photo the CV was made with (step 111); missing when none was
	// chosen, which means the profile's default.
	PhotoMediaFileID string `json:"photoMediaFileId,omitempty"`
	// PersonaName is filled only by ListAllCvDocuments (the "Select existing"
	// modal's list), never by the other reads.
	PersonaName string `json:"personaName,omitempty"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

// ErrUnknownCvDocument mirrors persona.ErrUnknownPersona's own
// sentinel-error convention.
var ErrUnknownCvDocument = errors.New("unknown cv document id")

const cvDocumentColumns = `id, persona_id, template_id, title, data_json, media_file_id, job_id, photo_media_file_id, created_at, updated_at`

func scanCvDocument(scan func(dest ...any) error) (CvDocument, error) {
	var doc CvDocument
	var dataJSON string
	var jobID, photoID, updatedAt sql.NullString
	if err := scan(&doc.ID, &doc.PersonaID, &doc.TemplateID, &doc.Title, &dataJSON, &doc.MediaFileID, &jobID, &photoID, &doc.CreatedAt, &updatedAt); err != nil {
		return doc, err
	}
	doc.JobID = jobID.String
	doc.PhotoMediaFileID = photoID.String
	doc.UpdatedAt = updatedAt.String
	if err := json.Unmarshal([]byte(dataJSON), &doc.Data); err != nil {
		return doc, err
	}
	return doc, nil
}

// ListCvDocuments returns personaID's own CV documents, newest first —
// no pagination (step 3's own "a personal library isn't expected to
// grow into the hundreds" reasoning).
func ListCvDocuments(personaID string) ([]CvDocument, error) {
	rows, err := db.CareerDB.Query(`SELECT `+cvDocumentColumns+` FROM cv_documents WHERE persona_id = ? ORDER BY created_at DESC`, personaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	docs := []CvDocument{}
	for rows.Next() {
		doc, err := scanCvDocument(rows.Scan)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}

// MaxAllCvDocuments caps ListAllCvDocuments.
const MaxAllCvDocuments = 200

// ListAllCvDocuments returns every persona's CV documents, newest first, at
// most MaxAllCvDocuments, each with its persona's name — the library the Jobs
// page's "Select existing" modal picks from (step 98).
func ListAllCvDocuments() ([]CvDocument, error) {
	rows, err := db.CareerDB.Query(
		`SELECT d.id, d.persona_id, d.template_id, d.title, d.data_json, d.media_file_id, d.job_id, d.photo_media_file_id, d.created_at, d.updated_at, COALESCE(p.name, '')
		 FROM cv_documents d LEFT JOIN personas p ON p.id = d.persona_id
		 ORDER BY d.created_at DESC, d.id LIMIT ?`, MaxAllCvDocuments)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	docs := []CvDocument{}
	for rows.Next() {
		var doc CvDocument
		var dataJSON string
		var jobID, photoID, updatedAt sql.NullString
		if err := rows.Scan(&doc.ID, &doc.PersonaID, &doc.TemplateID, &doc.Title, &dataJSON, &doc.MediaFileID, &jobID, &photoID, &doc.CreatedAt, &updatedAt, &doc.PersonaName); err != nil {
			return nil, err
		}
		doc.JobID = jobID.String
		doc.PhotoMediaFileID = photoID.String
		doc.UpdatedAt = updatedAt.String
		if err := json.Unmarshal([]byte(dataJSON), &doc.Data); err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}

// GetCvDocument reads one document by id — ErrUnknownCvDocument if it
// doesn't exist, same convention as persona.ErrUnknownPersona.
func GetCvDocument(id string) (CvDocument, error) {
	row := db.CareerDB.QueryRow(`SELECT `+cvDocumentColumns+` FROM cv_documents WHERE id = ?`, id)
	doc, err := scanCvDocument(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return doc, ErrUnknownCvDocument
	}
	return doc, err
}

// InsertCvDocument records a newly generated CV — called only after a
// real, permanent Media file id already exists (handler.go's own
// pipeline, or jobs/cv_pdf.go's own AI-facing save_cv_document), so
// there's no partial/"generating" state to represent. jobID is ""
// for every human-driven (wizard) document; only the AI-facing path
// ever passes a real one.
func InsertCvDocument(personaID, templateID, title string, data templates.CvData, mediaFileID, jobID string) (CvDocument, error) {
	return InsertCvDocumentWithPhoto(personaID, templateID, title, data, mediaFileID, jobID, "")
}

// InsertCvDocumentWithPhoto is InsertCvDocument that also records which of the profile's photos
// the CV was made with (step 111); "" stores NULL, meaning no photo was chosen and the profile's
// default is used.
func InsertCvDocumentWithPhoto(personaID, templateID, title string, data templates.CvData, mediaFileID, jobID, photoMediaFileID string) (CvDocument, error) {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return CvDocument{}, err
	}

	id := uuid.NewString()
	if _, err := db.CareerDB.Exec(
		`INSERT INTO cv_documents (id, persona_id, template_id, title, data_json, media_file_id, job_id, photo_media_file_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
		id, personaID, templateID, title, string(dataJSON), mediaFileID, sqlNullIfEmpty(jobID), sqlNullIfEmpty(photoMediaFileID),
	); err != nil {
		return CvDocument{}, err
	}
	return GetCvDocument(id)
}

// sqlNullIfEmpty turns "" into a real SQL NULL rather than storing a
// literal empty string in job_id — keeps "no job" queryable the normal
// way (WHERE job_id IS NULL / WHERE job_id = ?), not two different
// falsy representations.
func sqlNullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// UpdateCvDocumentTitle is the one field a caller can rename after
// creation — same shape as core Media's own step 9 title-editing
// convention. ErrUnknownCvDocument if id doesn't exist.
func UpdateCvDocumentTitle(id, title string) (CvDocument, error) {
	res, err := db.CareerDB.Exec(`UPDATE cv_documents SET title = ?, updated_at = datetime('now') WHERE id = ?`, title, id)
	if err != nil {
		return CvDocument{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return CvDocument{}, err
	}
	if affected == 0 {
		return CvDocument{}, ErrUnknownCvDocument
	}
	return GetCvDocument(id)
}

// DeleteCvDocument removes the row and returns its own media_file_id
// so the caller (handler.go) can best-effort forward a cleanup delete
// to core Media — mirrors cv_import.go's own "look up media id, delete
// the row, THEN best-effort forward the Media delete" ordering exactly.
func DeleteCvDocument(id string) (mediaFileID string, err error) {
	doc, err := GetCvDocument(id)
	if err != nil {
		return "", err
	}
	if _, err := db.CareerDB.Exec(`DELETE FROM cv_documents WHERE id = ?`, id); err != nil {
		return "", err
	}
	return doc.MediaFileID, nil
}
