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
type CvDocument struct {
	ID          string           `json:"id"`
	PersonaID   string           `json:"personaId"`
	TemplateID  string           `json:"templateId"`
	Title       string           `json:"title"`
	Data        templates.CvData `json:"data"`
	MediaFileID string           `json:"mediaFileId"`
	CreatedAt   string           `json:"createdAt"`
	UpdatedAt   string           `json:"updatedAt,omitempty"`
}

// ErrUnknownCvDocument mirrors persona.ErrUnknownPersona's own
// sentinel-error convention.
var ErrUnknownCvDocument = errors.New("unknown cv document id")

const cvDocumentColumns = `id, persona_id, template_id, title, data_json, media_file_id, created_at, updated_at`

func scanCvDocument(scan func(dest ...any) error) (CvDocument, error) {
	var doc CvDocument
	var dataJSON string
	var updatedAt sql.NullString
	if err := scan(&doc.ID, &doc.PersonaID, &doc.TemplateID, &doc.Title, &dataJSON, &doc.MediaFileID, &doc.CreatedAt, &updatedAt); err != nil {
		return doc, err
	}
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
// pipeline), so there's no partial/"generating" state to represent.
func InsertCvDocument(personaID, templateID, title string, data templates.CvData, mediaFileID string) (CvDocument, error) {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return CvDocument{}, err
	}

	id := uuid.NewString()
	if _, err := db.CareerDB.Exec(
		`INSERT INTO cv_documents (id, persona_id, template_id, title, data_json, media_file_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, datetime('now'))`,
		id, personaID, templateID, title, string(dataJSON), mediaFileID,
	); err != nil {
		return CvDocument{}, err
	}
	return GetCvDocument(id)
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
