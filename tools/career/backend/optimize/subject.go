// Package optimize is "Optimize with AI" (plan 258): the AI reads the texts of a profile or of a persona and suggests better wording;
// the user approves (the text is replaced) or declines (the suggestion is deleted) each one. This file reads the texts the AI is
// shown: the text a profile or a persona SHOWS now (a persona's own override or the profile's text it follows).
package optimize

import (
	"database/sql"
	"errors"
	"strings"

	"career-tool-backend/courses"
	"career-tool-backend/db"
	"career-tool-backend/education"
	"career-tool-backend/hobbies"
	"career-tool-backend/persona"
	"career-tool-backend/profile/profiledetails"
	"career-tool-backend/profile/profileitems"
)

// What a run is about.
const (
	SubjectProfile = "profile"
	SubjectPersona = "persona"
)

// The kinds of text, and their fields (plan 258, 2.1).
const (
	KindProfileDetails   = "profileDetails"
	KindPersonaDetails   = "personaDetails"
	KindExperience       = "experience"
	KindCustomExperience = "customExperience"
	KindEducation        = "education"
	KindCourse           = "course"
	KindHobby            = "hobby"

	FieldSummary     = "summary"
	FieldDescription = "description"
	FieldTitle       = "title"
)

// ErrUnknownSubject is a profile or persona id that does not exist (or a subject kind that is neither).
var ErrUnknownSubject = errors.New("unknown profile or target role id")

// Text is one text the AI may improve. Text is what the subject shows now; Label and Context only say which text it is.
type Text struct {
	Kind    string `json:"kind"`
	ItemID  string `json:"itemId"`
	Field   string `json:"field"`
	Label   string `json:"label"`
	Context string `json:"context,omitempty"`
	Text    string `json:"text"`
}

// Subject is a profile or a persona and its texts.
type Subject struct {
	Kind  string `json:"subjectKind"`
	ID    string `json:"subjectId"`
	Name  string `json:"name"`
	Texts []Text `json:"texts"`
}

func dateRange(start, end string) string {
	switch {
	case start == "" && end == "":
		return ""
	case end == "":
		return start + " – now"
	default:
		return start + " – " + end
	}
}

func join(parts ...string) string {
	out := []string{}
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ", ")
}

// add appends a text unless it is empty: there is nothing to improve in an empty text.
func (s *Subject) add(kind, itemID, field, label, context, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	s.Texts = append(s.Texts, Text{Kind: kind, ItemID: itemID, Field: field, Label: label, Context: context, Text: text})
}

// ReadSubject reads the texts of a profile or of a persona. An unknown subject is ErrUnknownSubject.
func ReadSubject(kind, id string) (Subject, error) {
	switch kind {
	case SubjectProfile:
		return readProfile(id)
	case SubjectPersona:
		return readPersona(id)
	}
	return Subject{}, ErrUnknownSubject
}

func readProfile(id string) (Subject, error) {
	s := Subject{Kind: SubjectProfile, ID: id, Texts: []Text{}}
	var first, last string
	switch err := db.CareerDB.QueryRow(`SELECT first_name, last_name FROM profiles WHERE id = ?`, id).Scan(&first, &last); {
	case errors.Is(err, sql.ErrNoRows):
		return s, ErrUnknownSubject
	case err != nil:
		return s, err
	}
	s.Name = strings.TrimSpace(first + " " + last)

	d, err := profiledetails.Get(id)
	if err != nil {
		return s, err
	}
	s.add(KindProfileDetails, "", FieldSummary, "Summary", "", d.Summary)

	exp, err := profileitems.ListExperience(id)
	if err != nil {
		return s, err
	}
	for _, e := range exp {
		label := e.Company + " — " + e.Title
		ctx := dateRange(e.StartDate, e.EndDate)
		s.add(KindExperience, e.ID, FieldTitle, label, join(e.Company, ctx), e.Title)
		s.add(KindExperience, e.ID, FieldDescription, label, ctx, e.Description)
	}
	edu, err := education.List(id)
	if err != nil {
		return s, err
	}
	for _, e := range edu {
		s.add(KindEducation, e.ID, FieldDescription, join(e.Institution, e.Degree), dateRange(e.StartDate, e.EndDate), e.Description)
	}
	crs, err := courses.List(id)
	if err != nil {
		return s, err
	}
	for _, c := range crs {
		s.add(KindCourse, c.ID, FieldDescription, join(c.Title, c.Provider), c.CompletedDate, c.Description)
	}
	hob, err := hobbies.List(id)
	if err != nil {
		return s, err
	}
	for _, h := range hob {
		s.add(KindHobby, h.ID, FieldDescription, h.Name, "", h.Description)
	}
	return s, nil
}

func readPersona(id string) (Subject, error) {
	s := Subject{Kind: SubjectPersona, ID: id, Texts: []Text{}}
	var name string
	switch err := db.CareerDB.QueryRow(`SELECT name FROM personas WHERE id = ?`, id).Scan(&name); {
	case errors.Is(err, sql.ErrNoRows):
		return s, ErrUnknownSubject
	case err != nil:
		return s, err
	}
	s.Name = name

	r, err := persona.FetchPersonaDetails(id)
	if err != nil {
		return s, err
	}
	if r.PersonaDetails != nil {
		s.add(KindPersonaDetails, "", FieldSummary, "Summary", "", r.PersonaDetails.Summary)
	}
	for _, e := range r.Experience {
		kind := KindExperience
		if e.Source == persona.SourcePersona {
			kind = KindCustomExperience
		}
		label := e.Company + " — " + e.Title
		ctx := dateRange(e.StartDate, e.EndDate)
		s.add(kind, e.ID, FieldTitle, label, join(e.Company, ctx), e.Title)
		s.add(kind, e.ID, FieldDescription, label, ctx, e.Description)
	}
	return s, nil
}

// find returns the text a suggestion is about, if the subject still has it.
func (s Subject) find(kind, itemID, field string) (Text, bool) {
	for _, t := range s.Texts {
		if t.Kind == kind && t.ItemID == itemID && t.Field == field {
			return t, true
		}
	}
	return Text{}, false
}
