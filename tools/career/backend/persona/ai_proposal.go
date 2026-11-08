package persona

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// The proposal an AI run ends with (step 112): up to three personas, each with its details, skills
// and experience. It is untrusted text: only ever shown for review and validated here, before it is
// stored (PUT persona-ai-runs) and again before anything is created from it. See
// plan/ai/tools/career/step-112-create-personas-with-ai.md.

const (
	MaxProposedPersonas = 3
	maxShortField       = 300  // name, headline, location, titles, locations, company, title, dates
	maxLongField        = 4000 // summary, experience description
	maxProposedSkills   = 60
	maxProposedRoles    = 30
	maxNote             = 1000
)

type ProposedDetails struct {
	Headline         string `json:"headline"`
	Summary          string `json:"summary"`
	Location         string `json:"location"`
	DesiredTitles    string `json:"desiredTitles"`
	DesiredLocations string `json:"desiredLocations"`
	MinSalary        int    `json:"minSalary"`
}

type ProposedRole struct {
	Company     string `json:"company"`
	Title       string `json:"title"`
	StartDate   string `json:"startDate"`
	EndDate     string `json:"endDate"`
	Description string `json:"description"`
}

type ProposedPersona struct {
	Name                string          `json:"name"`
	Description         string          `json:"description"`
	PossibleDuplicateOf *string         `json:"possibleDuplicateOf"`
	Details             ProposedDetails `json:"details"`
	Skills              []string        `json:"skills"`
	Experience          []ProposedRole  `json:"experience"`
}

type Proposal struct {
	Personas []ProposedPersona `json:"personas"`
	Note     string            `json:"note"`
}

var ErrInvalidProposal = errors.New("invalid proposal")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidProposal, fmt.Sprintf(format, args...))
}

// hasMarkup says whether s contains something that looks like an HTML tag.
func hasMarkup(s string) bool {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '<' && (s[i+1] == '/' || s[i+1] == '!' || (s[i+1] >= 'a' && s[i+1] <= 'z') || (s[i+1] >= 'A' && s[i+1] <= 'Z')) {
			return true
		}
	}
	return false
}

func checkText(name, s string, max int) error {
	if utf8.RuneCountInString(s) > max {
		return invalid("%s is too long (max %d characters)", name, max)
	}
	if hasMarkup(s) {
		return invalid("%s must be plain text, no markup", name)
	}
	return nil
}

// ParseProposal decodes raw (the AI's JSON) and validates it. An empty list of personas is valid:
// it is how the AI says the data is too thin, with the reason in Note.
func ParseProposal(raw []byte) (Proposal, error) {
	var p Proposal
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	if err := dec.Decode(&p); err != nil {
		return p, invalid("not valid JSON: %v", err)
	}
	if p.Personas == nil {
		return p, invalid("personas is required (use an empty list when there is nothing to propose)")
	}
	return p, ValidateProposal(p)
}

func ValidateProposal(p Proposal) error {
	if len(p.Personas) > MaxProposedPersonas {
		return invalid("too many personas (max %d)", MaxProposedPersonas)
	}
	if err := checkText("note", p.Note, maxNote); err != nil {
		return err
	}
	names := map[string]bool{}
	for i, pp := range p.Personas {
		if err := ValidateProposedPersona(pp); err != nil {
			return fmt.Errorf("persona %d: %w", i+1, err)
		}
		key := strings.ToLower(strings.TrimSpace(pp.Name))
		if names[key] {
			return invalid("persona %d: the name %q is used twice", i+1, pp.Name)
		}
		names[key] = true
	}
	return nil
}

// ValidateProposedPersona checks one persona: also what the user edited in the review.
func ValidateProposedPersona(pp ProposedPersona) error {
	if strings.TrimSpace(pp.Name) == "" {
		return invalid("name is required")
	}
	for _, f := range []struct {
		name string
		val  string
		max  int
	}{
		{"name", pp.Name, maxShortField},
		{"description", pp.Description, maxShortField},
		{"headline", pp.Details.Headline, maxShortField},
		{"summary", pp.Details.Summary, maxLongField},
		{"location", pp.Details.Location, maxShortField},
		{"desiredTitles", pp.Details.DesiredTitles, maxShortField},
		{"desiredLocations", pp.Details.DesiredLocations, maxShortField},
	} {
		if err := checkText(f.name, f.val, f.max); err != nil {
			return err
		}
	}
	if pp.Details.MinSalary < 0 {
		return invalid("minSalary cannot be negative")
	}
	if len(pp.Skills) > maxProposedSkills {
		return invalid("too many skills (max %d)", maxProposedSkills)
	}
	for _, s := range pp.Skills {
		if strings.TrimSpace(s) == "" {
			return invalid("a skill is empty")
		}
		if err := checkText("a skill", s, maxShortField); err != nil {
			return err
		}
	}
	if len(pp.Experience) > maxProposedRoles {
		return invalid("too many experience entries (max %d)", maxProposedRoles)
	}
	for _, r := range pp.Experience {
		if strings.TrimSpace(r.Company) == "" || strings.TrimSpace(r.Title) == "" {
			return invalid("an experience entry needs a company and a title")
		}
		for _, f := range []struct {
			name string
			val  string
			max  int
		}{
			{"company", r.Company, maxShortField}, {"title", r.Title, maxShortField},
			{"startDate", r.StartDate, maxShortField}, {"endDate", r.EndDate, maxShortField},
			{"experience description", r.Description, maxLongField},
		} {
			if err := checkText(f.name, f.val, f.max); err != nil {
				return err
			}
		}
	}
	return nil
}
