package optimize

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"career-tool-backend/courses"
	"career-tool-backend/db"
	"career-tool-backend/education"
	"career-tool-backend/hobbies"
	"career-tool-backend/persona"
	"career-tool-backend/profile/profiledetails"
	"career-tool-backend/profile/profileitems"
)

// The user's decisions on the suggestions of a ready run (plan 258, 2.5). DECLINE deletes the suggestion. APPROVE writes the text through
// the same writers the pages use (so the job match hears about it) and deletes the suggestion; a text that is no longer what the AI read
// is not written (it is stale: the suggestion stays so that the user sees why and can decline it). The run goes when its last
// suggestion is decided.

// ErrRunNotReady: decisions are taken on a run whose AI turn has ended and that has suggestions waiting.
var ErrRunNotReady = errors.New("this run is not ready for review")

// Decision is the user's answer to one suggestion.
type Decision struct {
	SuggestionID string `json:"suggestionId"`
	Approve      bool   `json:"approve"`
}

// Outcome is what happened to one decision. A refused one says why and leaves its suggestion where it was.
type Outcome struct {
	SuggestionID string `json:"suggestionId"`
	OK           bool   `json:"ok"`
	Error        string `json:"error,omitempty"`
}

// Decided is the answer to a batch of decisions.
type Decided struct {
	Outcomes []Outcome `json:"outcomes"`
	// Remaining is how many suggestions still wait; Closed is true when the run was removed because none is left.
	Remaining int  `json:"remaining"`
	Closed    bool `json:"closed"`
}

type waitingSuggestion struct {
	id, kind, itemID, field, before, after string
}

// ApplyDecisions takes the user's decisions on a ready run, one after the other. A decision that cannot be carried out does not stop the
// others. A decision for a suggestion that is not (or no longer) in the run is an Outcome with an error.
func ApplyDecisions(runID string, decisions []Decision) (Decided, error) {
	run, err := loadRun(db.CareerDB, runID)
	if err != nil {
		return Decided{}, err
	}
	if run.status != runReady {
		return Decided{}, ErrRunNotReady
	}
	if len(decisions) == 0 {
		return Decided{}, invalid("decisions is empty")
	}
	out := Decided{Outcomes: []Outcome{}}
	for _, d := range decisions {
		out.Outcomes = append(out.Outcomes, decideOne(run, runID, d))
	}
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM optimize_suggestions WHERE run_id = ?`, runID).Scan(&out.Remaining); err != nil {
		return out, err
	}
	if out.Remaining == 0 {
		if err := DeleteRun(runID); err != nil {
			return out, err
		}
		out.Closed = true
	}
	return out, nil
}

func decideOne(run runRow, runID string, d Decision) Outcome {
	o := Outcome{SuggestionID: d.SuggestionID}
	var s waitingSuggestion
	switch err := db.CareerDB.QueryRow(
		`SELECT id, kind, item_id, field, before_text, after_text FROM optimize_suggestions WHERE id = ? AND run_id = ?`, d.SuggestionID, runID,
	).Scan(&s.id, &s.kind, &s.itemID, &s.field, &s.before, &s.after); {
	case errors.Is(err, sql.ErrNoRows):
		o.Error = "this suggestion is not in this run (it was already decided)"
		return o
	case err != nil:
		o.Error = err.Error()
		return o
	}
	if d.Approve {
		if err := apply(run, s); err != nil {
			o.Error = err.Error()
			return o
		}
	}
	if _, err := db.CareerDB.Exec(`DELETE FROM optimize_suggestions WHERE id = ?`, s.id); err != nil {
		o.Error = err.Error()
		return o
	}
	o.OK = true
	return o
}

// apply writes an approved suggestion, after checking that the subject still shows the text the AI read.
func apply(run runRow, s waitingSuggestion) error {
	subject, err := ReadSubject(run.subjectKind, run.subjectID)
	if err != nil {
		return errors.New("the profile or target role this was about no longer exists")
	}
	cur, ok := subject.find(s.kind, s.itemID, s.field)
	if !ok || cur.Text != s.before {
		return errors.New("this text changed after the AI read it: decline this suggestion and ask again")
	}
	text := s.after
	id := run.subjectID
	switch run.subjectKind {
	case SubjectProfile:
		switch s.kind {
		case KindProfileDetails:
			_, err = profiledetails.Update(id, profiledetails.Patch{Summary: &text})
		case KindExperience:
			p := profileitems.ExperiencePatch{}
			if s.field == FieldTitle {
				p.Title = &text
			} else {
				p.Description = &text
			}
			err = profileitems.UpdateExperience(s.itemID, p)
		case KindEducation:
			_, err = education.Update(s.itemID, education.Patch{Description: &text})
		case KindCourse:
			_, err = courses.Update(s.itemID, courses.Patch{Description: &text})
		case KindHobby:
			_, err = hobbies.Update(s.itemID, hobbies.Patch{Description: &text})
		default:
			err = fmt.Errorf("a profile has no %s to change", s.kind)
		}
	case SubjectPersona:
		switch s.kind {
		case KindPersonaDetails:
			err = persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{PersonaID: id, Summary: &text})
		case KindExperience:
			err = overrideExperience(id, s, text)
		case KindCustomExperience:
			a := persona.UpdateCareerExperienceArgs{PersonaID: id, ID: s.itemID}
			if s.field == FieldTitle {
				a.Title = &text
			} else {
				a.Description = &text
			}
			err = persona.UpdateCareerExperience(a)
		default:
			err = fmt.Errorf("a target role has no %s to change", s.kind)
		}
	}
	return err
}

// overrideExperience writes the text as the persona's OWN title or description of a profile experience it uses; the profile's text and
// the other personas stay as they are.
func overrideExperience(personaID string, s waitingSuggestion, text string) error {
	use, err := profileitems.GetPersonaUse(personaID)
	if err != nil {
		return err
	}
	found := false
	for i := range use.Experience {
		if use.Experience[i].ID != s.itemID {
			continue
		}
		found = true
		if s.field == FieldTitle {
			use.Experience[i].TitleOverride = &text
		} else {
			use.Experience[i].DescriptionOverride = &text
		}
	}
	if !found {
		return errors.New("this target role no longer uses that experience")
	}
	return profileitems.SetPersonaExperience(personaID, use.Experience)
}

type decideRequest struct {
	RunID     string     `json:"runId"`
	Decisions []Decision `json:"decisions"`
}

// DecideHandler handles POST optimize-runs/decide.
func DecideHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body decideRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.RunID) == "" {
		http.Error(w, "runId and decisions are required", http.StatusBadRequest)
		return
	}
	res, err := ApplyDecisions(body.RunID, body.Decisions)
	if err != nil {
		writeError(w, "apply the decisions", err)
		return
	}
	db.WriteJSON(w, res)
}
