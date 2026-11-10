package jobmatcher

import (
	"errors"
	"strings"
)

// Requirement is one thing a job asks for: a term or a short sentence, with a tier (3 must-have, 2 important,
// 1 nice-to-have).
type Requirement struct {
	Term string `json:"term"`
	Tier int    `json:"tier"`
}

// Job is what the engine reads of a job.
type Job struct {
	Title        string
	Company      string
	Location     string
	Description  string
	Requirements []Requirement
}

// Experience is one work history entry; the dates are as stored ("2021-03", "2021", "03/2021"), an empty end
// date means "still there".
type Experience struct {
	Title       string
	Description string
	StartDate   string
	EndDate     string
}

// Persona is what the engine reads of a persona. Callers pass personas oldest first: that order breaks ties.
type Persona struct {
	ID               string
	Name             string
	Headline         string
	Summary          string
	Location         string
	DesiredTitles    string
	DesiredLocations string
	// RemotePreference (step 130) is "" (not set), "onsite", "open" or "remote_only"; anything else reads as not set.
	RemotePreference string
	Skills           []string
	Experience       []Experience
}

// Component is one part of a score: its weight, the points it gave (0 to Weight), or why it could not be judged.
type Component struct {
	Key       string  `json:"key"`
	Label     string  `json:"label"`
	Weight    int     `json:"weight"`
	Points    float64 `json:"points"`
	Evaluable bool    `json:"evaluable"`
	Note      string  `json:"note"`
}

// Missing is a requirement of tier 2 or 3 that nothing in the persona covers.
type Missing struct {
	Term string `json:"term"`
	Tier int    `json:"tier"`
}

// The two ways the Skills component is scored.
const (
	ModeRequirements = "requirements"
	ModeText         = "text"
)

// Result is a score with everything needed to explain it.
type Result struct {
	Score         int      `json:"score"`
	PersonaID     string   `json:"personaId"`
	PersonaName   string   `json:"personaName"`
	MatchedSkills []string `json:"matchedSkills"`
	Mode          string   `json:"mode"`
	Coverage      float64  `json:"coverage"`
	LowConfidence bool     `json:"lowConfidence"`
	// RelevanceScale is 1, or less when the job hardly resembles the persona's field (Skills and Title fit earned
	// under RelevanceFloor points) and the score was scaled down.
	RelevanceScale float64 `json:"relevanceScale"`
	// DealbreakerScale is 1, or DealbreakerScale when the persona's explicit remote preference rules the job out (a
	// remote-only persona and an on-site job, an on-site-only persona and a remote job) and the score was scaled down.
	DealbreakerScale float64     `json:"dealbreakerScale"`
	Components       []Component `json:"components"`
	Missing          []Missing   `json:"missing"`
	Gaps             []string    `json:"gaps"`
	EngineVersion    string      `json:"engineVersion"`
}

// ErrNoPersonas is returned for a profile without personas.
var ErrNoPersonas = errors.New("this profile has no target roles")

// InsufficientDataError says why no score could be made: neither skills nor title could be judged for any persona.
// Every reason is something a person can fix.
type InsufficientDataError struct{ Reasons []string }

func (e *InsufficientDataError) Error() string {
	return "not enough data to match: " + strings.Join(e.Reasons, "; ")
}
