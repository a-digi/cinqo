// cv_import_suggest.go — the AI hands the result of a CV analysis over through the suggest_cv_import tool instead of printing it
// as JSON in its last message (step 155). The tool only stores the proposal on the run the page recorded before the AI started;
// it creates no profile, persona, skill or experience (those are inserted only by the user's own review). The AI's last message is
// then one plain sentence, so the chat shows no data. Same idea as suggest_persona (plans 112 and 113). See
// plan/ai/tools/career/step-155-cv-import-result-by-tool-not-chat.md.
package cv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
)

// Limits on what the AI may hand over; a CV has no more than this, and a run that is out of range is a clear tool error the AI can
// correct in the same turn.
const (
	MaxProposalSkills     = 200
	MaxProposalExperience = 50
	MaxProposalTextRunes  = 4000
)

const (
	actionInsert = "insert"
	actionSkip   = "skip"
)

// Proposal is the AI's proposal of what to take from a CV; the JSON shape is the one the review page reads (parseProposal.ts).
type Proposal struct {
	Profile         ProfileProposal      `json:"profile" jsonschema:"the profile to create from the CV"`
	Persona         PersonaProposal      `json:"persona" jsonschema:"at most one persona for this run"`
	PersonalDetails DetailsProposal      `json:"personalDetails" jsonschema:"the persona's summary and goals"`
	Skills          []SkillProposal      `json:"skills" jsonschema:"the skills found in the CV"`
	Experience      []ExperienceProposal `json:"experience" jsonschema:"the jobs found in the CV"`
}

type ProfileProposal struct {
	Action              string  `json:"action" jsonschema:"insert, or skip when not relevant or not confident"`
	PossibleDuplicateOf *string `json:"possibleDuplicateOf" jsonschema:"the id or text of an existing item this looks like, otherwise null"`
	Name                string  `json:"name"`
}

type PersonaProposal struct {
	Action              string  `json:"action" jsonschema:"insert, or skip when not relevant or not confident"`
	PossibleDuplicateOf *string `json:"possibleDuplicateOf" jsonschema:"the id of an existing persona this resembles, otherwise null"`
	Name                string  `json:"name"`
	Title               string  `json:"title"`
}

type DetailsProposal struct {
	Action              string  `json:"action" jsonschema:"insert, or skip when not relevant or not confident"`
	PossibleDuplicateOf *string `json:"possibleDuplicateOf" jsonschema:"always null"`
	Headline            string  `json:"headline"`
	Summary             string  `json:"summary"`
	Location            string  `json:"location"`
	DesiredTitles       string  `json:"desiredTitles"`
	DesiredLocations    string  `json:"desiredLocations"`
	MinSalary           int64   `json:"minSalary" jsonschema:"0 when unknown"`
}

type SkillProposal struct {
	Action              string  `json:"action" jsonschema:"insert, or skip when not relevant or not confident"`
	PossibleDuplicateOf *string `json:"possibleDuplicateOf" jsonschema:"the text of an existing skill this matches, otherwise null"`
	Value               string  `json:"value"`
}

type ExperienceProposal struct {
	Action              string  `json:"action" jsonschema:"insert, or skip when not relevant or not confident"`
	PossibleDuplicateOf *string `json:"possibleDuplicateOf" jsonschema:"the id of an existing experience entry this matches, otherwise null"`
	Company             string  `json:"company"`
	Title               string  `json:"title"`
	StartDate           string  `json:"startDate"`
	EndDate             string  `json:"endDate"`
	Description         string  `json:"description"`
}

// proposalProblem is a proposal the tool refuses: what is wrong and where.
type proposalProblem struct{ msg string }

func (p *proposalProblem) Error() string { return p.msg }

func problem(format string, args ...any) error { return &proposalProblem{fmt.Sprintf(format, args...)} }

func checkAction(path, action string) error {
	if action != actionInsert && action != actionSkip {
		return problem(`%s.action must be "insert" or "skip"`, path)
	}
	return nil
}

func checkText(path, s string) error {
	if utf8.RuneCountInString(s) > MaxProposalTextRunes {
		return problem("%s must be at most %d characters", path, MaxProposalTextRunes)
	}
	return nil
}

func checkDuplicate(path string, d *string) error {
	if d == nil {
		return nil
	}
	return checkText(path+".possibleDuplicateOf", *d)
}

// validateProposal checks a proposal the way the review page reads it (every item has an action insert or skip, the lists are
// lists) and bounds its size. It returns a normalised copy: empty lists are [], never null.
func validateProposal(p Proposal) (Proposal, error) {
	checks := []struct {
		path   string
		action string
		dup    *string
		texts  map[string]string
	}{
		{"profile", p.Profile.Action, p.Profile.PossibleDuplicateOf, map[string]string{"name": p.Profile.Name}},
		{"persona", p.Persona.Action, p.Persona.PossibleDuplicateOf, map[string]string{"name": p.Persona.Name, "title": p.Persona.Title}},
		{"personalDetails", p.PersonalDetails.Action, p.PersonalDetails.PossibleDuplicateOf, map[string]string{
			"headline": p.PersonalDetails.Headline, "summary": p.PersonalDetails.Summary, "location": p.PersonalDetails.Location,
			"desiredTitles": p.PersonalDetails.DesiredTitles, "desiredLocations": p.PersonalDetails.DesiredLocations,
		}},
	}
	for _, c := range checks {
		if err := checkAction(c.path, c.action); err != nil {
			return Proposal{}, err
		}
		if err := checkDuplicate(c.path, c.dup); err != nil {
			return Proposal{}, err
		}
		for field, v := range c.texts {
			if err := checkText(c.path+"."+field, v); err != nil {
				return Proposal{}, err
			}
		}
	}
	if p.PersonalDetails.MinSalary < 0 {
		return Proposal{}, problem("personalDetails.minSalary must be 0 or more")
	}
	if len(p.Skills) > MaxProposalSkills {
		return Proposal{}, problem("skills can have at most %d entries", MaxProposalSkills)
	}
	for i, s := range p.Skills {
		path := fmt.Sprintf("skills[%d]", i)
		if err := checkAction(path, s.Action); err != nil {
			return Proposal{}, err
		}
		if err := checkDuplicate(path, s.PossibleDuplicateOf); err != nil {
			return Proposal{}, err
		}
		if err := checkText(path+".value", s.Value); err != nil {
			return Proposal{}, err
		}
	}
	if len(p.Experience) > MaxProposalExperience {
		return Proposal{}, problem("experience can have at most %d entries", MaxProposalExperience)
	}
	for i, e := range p.Experience {
		path := fmt.Sprintf("experience[%d]", i)
		if err := checkAction(path, e.Action); err != nil {
			return Proposal{}, err
		}
		if err := checkDuplicate(path, e.PossibleDuplicateOf); err != nil {
			return Proposal{}, err
		}
		for field, v := range map[string]string{"company": e.Company, "title": e.Title, "startDate": e.StartDate, "endDate": e.EndDate, "description": e.Description} {
			if err := checkText(path+"."+field, v); err != nil {
				return Proposal{}, err
			}
		}
	}
	if p.Skills == nil {
		p.Skills = []SkillProposal{}
	}
	if p.Experience == nil {
		p.Experience = []ExperienceProposal{}
	}
	return p, nil
}

// StoreProposal validates a proposal and keeps it on the generating run. A proposal problem is a *proposalProblem; the run
// errors are errUnknownCVImportRun and errRunNotGenerating.
func StoreProposal(runID string, p Proposal) error {
	clean, err := validateProposal(p)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(clean)
	if err != nil {
		return err
	}
	return storeCVImportProposal(runID, raw)
}

type suggestCVImportArgs struct {
	RunID    string   `json:"runId" jsonschema:"the run id given in the request"`
	Proposal Proposal `json:"proposal" jsonschema:"what to take from the CV, built only from the CV text and the data already stored"`
}

// suggestCVImport is the tool's work, apart from the MCP plumbing so it can be tested on its own.
func suggestCVImport(args suggestCVImportArgs) (*mcp.CallToolResult, any, error) {
	if args.RunID == "" {
		return db.ErrResult("runId is required"), nil, nil
	}
	err := StoreProposal(args.RunID, args.Proposal)
	var bad *proposalProblem
	switch {
	case errors.Is(err, errUnknownCVImportRun):
		return db.ErrResult(fmt.Sprintf("unknown run id %q", args.RunID)), nil, nil
	case errors.Is(err, errRunNotGenerating):
		return db.ErrResult("this run is no longer generating, so it takes no proposal and nothing was stored. Do not call this tool again and do not say the analysis is done: reply with one plain sentence that the import run has ended and the user must start a new import"), nil, nil
	case errors.As(err, &bad):
		return db.ErrResult("the proposal is not valid: " + bad.msg), nil, nil
	case err != nil:
		return db.ErrResult("could not store the proposal: " + err.Error()), nil, nil
	}
	return db.JSONResult(map[string]any{"stored": true})
}

// RegisterSuggestCVImport registers suggest_cv_import.
func RegisterSuggestCVImport(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "suggest_cv_import",
		Description: "Hand over the result of a CV analysis to the user, who then reviews it on the Import page. " +
			"It creates no profile, persona, skill or experience. Call it ONCE, with the runId from the request and the whole proposal " +
			"(profile, persona, personalDetails, skills, experience; each item with action insert or skip and possibleDuplicateOf). " +
			"Fails with a message when the run is unknown or no longer running, or when the proposal is not valid; correct it and call again.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args suggestCVImportArgs) (*mcp.CallToolResult, any, error) {
		return suggestCVImport(args)
	})
}
