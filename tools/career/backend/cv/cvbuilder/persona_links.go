package cvbuilder

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/persona"
)

// Attaching a CV to more personas and detaching it again (step 114). The persona a CV was built
// from is always linked and cannot be detached; any other persona of the same profile can be
// attached. See plan/ai/tools/career/step-114-cvs-for-many-personas.md.

var (
	// ErrPersonaOfOtherProfile: a CV holds one person's data, so it can only be attached to personas
	// of the profile that owns the persona it was built from.
	ErrPersonaOfOtherProfile = errors.New("this persona belongs to another profile than the CV")
	// ErrCannotDetachBuiltFrom: the persona the CV was built from stays linked.
	ErrCannotDetachBuiltFrom = errors.New("the persona the CV was built from cannot be detached")
)

// AttachCvToPersona links a CV to a persona. Idempotent. ErrUnknownCvDocument,
// persona.ErrUnknownPersona and ErrPersonaOfOtherProfile are returned as is.
func AttachCvToPersona(cvDocumentID, personaID string) error {
	tx, err := db.CareerDB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var cvProfile string
	switch err := tx.QueryRow(
		`SELECT p.profile_id FROM cv_documents d JOIN personas p ON p.id = d.persona_id WHERE d.id = ?`,
		cvDocumentID).Scan(&cvProfile); {
	case errors.Is(err, sql.ErrNoRows):
		return ErrUnknownCvDocument
	case err != nil:
		return err
	}
	var personaProfile string
	switch err := tx.QueryRow(`SELECT profile_id FROM personas WHERE id = ?`, personaID).Scan(&personaProfile); {
	case errors.Is(err, sql.ErrNoRows):
		return persona.ErrUnknownPersona
	case err != nil:
		return err
	}
	if personaProfile != cvProfile {
		return ErrPersonaOfOtherProfile
	}
	if _, err := tx.Exec(
		`INSERT INTO cv_document_personas (cv_document_id, persona_id) VALUES (?, ?)
		 ON CONFLICT(cv_document_id, persona_id) DO NOTHING`, cvDocumentID, personaID); err != nil {
		return err
	}
	return tx.Commit()
}

// DetachCvFromPersona removes the link between a CV and a persona. Idempotent: not linked is not
// an error. ErrUnknownCvDocument for an unknown CV, ErrCannotDetachBuiltFrom for the persona the
// CV was built from. The CV itself and its other links stay.
func DetachCvFromPersona(cvDocumentID, personaID string) error {
	var builtFrom string
	switch err := db.CareerDB.QueryRow(`SELECT persona_id FROM cv_documents WHERE id = ?`, cvDocumentID).Scan(&builtFrom); {
	case errors.Is(err, sql.ErrNoRows):
		return ErrUnknownCvDocument
	case err != nil:
		return err
	}
	if builtFrom == personaID {
		return ErrCannotDetachBuiltFrom
	}
	_, err := db.CareerDB.Exec(`DELETE FROM cv_document_personas WHERE cv_document_id = ? AND persona_id = ?`, cvDocumentID, personaID)
	return err
}

type attachCvRequest struct {
	CvDocumentID string `json:"cvDocumentId"`
	PersonaID    string `json:"personaId"`
}

// PersonaLinksHandler handles PUT (attach) and DELETE (detach, by ?cvDocumentId= and ?personaId=)
// on cv-documents/personas. Both answer 204.
func PersonaLinksHandler(w http.ResponseWriter, r *http.Request) {
	var cvID, personaID string
	switch r.Method {
	case http.MethodPut:
		var body attachCvRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		cvID, personaID = body.CvDocumentID, body.PersonaID
	case http.MethodDelete:
		cvID, personaID = r.URL.Query().Get("cvDocumentId"), r.URL.Query().Get("personaId")
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if cvID == "" || personaID == "" {
		http.Error(w, "cvDocumentId and personaId are required", http.StatusBadRequest)
		return
	}
	var err error
	if r.Method == http.MethodPut {
		err = AttachCvToPersona(cvID, personaID)
	} else {
		err = DetachCvFromPersona(cvID, personaID)
	}
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrUnknownCvDocument):
		http.Error(w, "unknown cv document id", http.StatusNotFound)
	case errors.Is(err, persona.ErrUnknownPersona):
		http.Error(w, "unknown persona id", http.StatusNotFound)
	case errors.Is(err, ErrPersonaOfOtherProfile), errors.Is(err, ErrCannotDetachBuiltFrom):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "failed to change the CV's personas: "+err.Error(), http.StatusInternalServerError)
	}
}
