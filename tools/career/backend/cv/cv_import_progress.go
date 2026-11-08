// cv_import_progress.go — what a generating import run has stored so far (plan 209): the page that shows the run working reads it, so a person sees
// the import move (the AI's own conversation shows nothing while a page drives it in code, or while it works in steps in other conversations).
package cv

import "encoding/json"

// runProgress is the draft of a generating run in counts: which single sections are on it and how many skills, jobs and education entries.
type runProgress struct {
	Profile         bool `json:"profile"`
	Persona         bool `json:"persona"`
	PersonalDetails bool `json:"personalDetails"`
	Skills          int  `json:"skills"`
	Experience      int  `json:"experience"`
	Education       int  `json:"education"`
}

// progressOf reads a stored draft; nil when it is empty or cannot be read (nothing to show, never an error on a page that only looks).
func progressOf(draftJSON string) *runProgress {
	if draftJSON == "" {
		return nil
	}
	var d importDraft
	if err := json.Unmarshal([]byte(draftJSON), &d); err != nil {
		return nil
	}
	return &runProgress{
		Profile: d.Profile != nil, Persona: d.Persona != nil, PersonalDetails: d.PersonalDetails != nil,
		Skills: len(d.Skills), Experience: len(d.Experience), Education: len(d.Education),
	}
}
