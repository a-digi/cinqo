// cv_import_draft_handler.go — the same draft pieces the cv_import_* tools store, over HTTP (plan 209, step 1).
//
// The tools are for the AI. A page that does the import itself, in code (the code-driven import for a model that cannot call tools),
// cannot call a tool, so it puts each piece on the run's draft through these two routes. Nothing is validated here: every piece goes
// through the function the tool uses (SetDraftSection, AddDraftSkills, AddDraftExperience, AddDraftEducation, FinishDraft), so a piece
// is accepted or refused by exactly the same rules, whoever sends it.
package cv

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"career-tool-backend/db"
)

// maxDraftBody bounds one request: a job with its whole text is a few kilobytes.
const maxDraftBody = 1 << 20

type draftPieceRequest struct {
	RunID   string          `json:"runId"`
	Section string          `json:"section"`
	Value   json.RawMessage `json:"value"`
	Items   json.RawMessage `json:"items"`
}

type draftFinishRequest struct {
	RunID string `json:"runId"`
}

// CVImportDraftHandler handles POST /cv-import/draft: {runId, section, value} for profile, persona and personalDetails, and
// {runId, section, items} (or value, for one entry) for skills, experience and education. It answers with the draft's summary.
func CVImportDraftHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body draftPieceRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxDraftBody)).Decode(&body); err != nil {
		http.Error(w, "the body must be JSON: runId, section and value or items", http.StatusBadRequest)
		return
	}
	runID, ok := requireRunID(w, body.RunID)
	if !ok {
		return
	}
	var (
		d   importDraft
		err error
	)
	switch section := strings.TrimSpace(body.Section); section {
	case sectionProfile, sectionPersona, sectionPersonalDetails:
		d, err = SetDraftSection(runID, section, anyOf(body.Value))
	case "skills":
		d, err = AddDraftSkills(runID, anyOf(firstRaw(body.Items, body.Value)))
	case "experience":
		d, err = AddDraftExperience(runID, anyOf(firstRaw(body.Items, body.Value)))
	case "education":
		d, err = AddDraftEducation(runID, anyOf(firstRaw(body.Items, body.Value)))
	default:
		http.Error(w, `section must be "profile", "persona", "personalDetails", "skills", "experience" or "education"`, http.StatusBadRequest)
		return
	}
	if err != nil {
		writeDraftError(w, err)
		return
	}
	db.WriteJSON(w, d.summary())
}

// CVImportDraftFinishHandler handles POST /cv-import/draft/finish: {runId}. It checks that the draft is complete and stores it as the
// run's proposal; the page then ends the run as ready, as it does for a run the AI handed over.
func CVImportDraftFinishHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body draftFinishRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxDraftBody)).Decode(&body); err != nil {
		http.Error(w, "the body must be JSON: runId", http.StatusBadRequest)
		return
	}
	runID, ok := requireRunID(w, body.RunID)
	if !ok {
		return
	}
	if err := FinishDraft(runID); err != nil {
		writeDraftError(w, err)
		return
	}
	db.WriteJSON(w, map[string]any{"ok": true})
}

// requireRunID is the run the request names. The run is never guessed (the tools may take "the only generating run"; a page always knows).
func requireRunID(w http.ResponseWriter, id string) (string, bool) {
	id = strings.TrimSpace(id)
	if _, err := uuid.Parse(id); err != nil {
		http.Error(w, "runId is required and must be a uuid", http.StatusBadRequest)
		return "", false
	}
	return id, true
}

// writeDraftError answers with what the tools would say: a refused piece is 400 with the reason, an unknown run 404, a run that is no
// longer generating 409.
func writeDraftError(w http.ResponseWriter, err error) {
	var bad *proposalProblem
	switch {
	case errors.As(err, &bad):
		http.Error(w, bad.msg, http.StatusBadRequest)
	case errors.Is(err, errUnknownCVImportRun):
		http.Error(w, "unknown cv import run id", http.StatusNotFound)
	case errors.Is(err, errRunNotGenerating):
		http.Error(w, runEndedMessage, http.StatusConflict)
	default:
		http.Error(w, "failed to store the piece: "+err.Error(), http.StatusInternalServerError)
	}
}

// firstRaw is the first of the values that was sent (not absent, not null).
func firstRaw(values ...json.RawMessage) json.RawMessage {
	for _, v := range values {
		if s := strings.TrimSpace(string(v)); s != "" && s != "null" {
			return v
		}
	}
	return nil
}

// anyOf is the decoded value, or nil when nothing was sent; the draft functions accept an object, a list or the JSON text of one.
func anyOf(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}
