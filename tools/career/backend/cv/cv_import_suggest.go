// cv_import_suggest.go — the AI hands the result of a CV analysis over through the suggest_cv_import tool instead of printing it
// as JSON in its last message (step 155). The tool only stores the proposal on the run the page recorded before the AI started;
// it creates no profile, persona, skill or experience (those are inserted only by the user's own review). The AI's last message is
// then one plain sentence, so the chat shows no data. Same idea as suggest_persona (plans 112 and 113). See
// plan/ai/tools/career/step-155-cv-import-result-by-tool-not-chat.md.
package cv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
	"career-tool-backend/education"
)

// Limits on what the AI may hand over; a CV has no more than this, and a run that is out of range is a clear tool error the AI can
// correct in the same turn.
const (
	MaxProposalSkills     = 200
	MaxProposalExperience = 50
	MaxProposalEducation  = 30
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
	// Education (plan 175) is optional: a CV without any sends none, and a proposal stored before it existed has none.
	Education []EducationProposal `json:"education,omitempty" jsonschema:"the education found in the CV (schools, degrees, courses); leave it out when the CV has none"`
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

// FlexCount is a whole number the AI may send as a number or as a text of digits (weak models send either).
type FlexCount int64

func (c *FlexCount) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if len(text) > 1 && text[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		text = strings.TrimSpace(s)
	}
	if text == "" || text == "null" {
		return errors.New("a count must be a whole number")
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		f, ferr := strconv.ParseFloat(text, 64)
		if ferr != nil || f != float64(int64(f)) {
			return errors.New("a count must be a whole number")
		}
		n = int64(f)
	}
	*c = FlexCount(n)
	return nil
}

// Bullet points of a job: the AI counts them in the CV and the description must have a line for at least this share of them
// (plan 177). Numerator and denominator of 70 %.
const (
	bulletShareNum = 7
	bulletShareDen = 10
	// shortDescriptionRunes is the length under which a one-line description is warned about when no bullet count was given.
	shortDescriptionRunes = 200
)

// descriptionLines counts the non-empty lines of a description.
func descriptionLines(description string) int {
	n := 0
	for _, line := range strings.Split(description, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

// requiredLines is how many lines a description needs for a job the CV lists this many bullet points for: 70 %, rounded up.
func requiredLines(bullets int64) int {
	return int((bullets*bulletShareNum + bulletShareDen - 1) / bulletShareDen)
}

// shortDescriptionWarnings names the jobs to insert whose description is short and has no line break while the AI gave no bullet count:
// a hint to send the job again with all of its text, never a refusal.
func shortDescriptionWarnings(jobs []ExperienceProposal) []string {
	var out []string
	for _, e := range jobs {
		if e.Action != actionInsert || e.BulletsInCv != nil || strings.Contains(strings.TrimSpace(e.Description), "\n") {
			continue
		}
		if n := utf8.RuneCountInString(strings.TrimSpace(e.Description)); n < shortDescriptionRunes {
			out = append(out, fmt.Sprintf("the description of %q is only %d characters: include every bullet point and the technologies of this job (you may improve the wording, never cut it), and give bulletsInCv, then send the job again (the same company, title and start date replaces it)", e.Company, n))
		}
	}
	return out
}

type ExperienceProposal struct {
	Action              string  `json:"action" jsonschema:"insert, or skip when not relevant or not confident"`
	PossibleDuplicateOf *string `json:"possibleDuplicateOf" jsonschema:"the id of an existing experience entry this matches, otherwise null"`
	Company             string  `json:"company"`
	Title               string  `json:"title"`
	StartDate           string  `json:"startDate"`
	EndDate             string  `json:"endDate"`
	Description         string  `json:"description"`
	// BulletsInCv is how many bullet points the CV lists under this job (plan 177): when given, the description must keep a line for at
	// least 70 % of them. 0 for a job the CV describes in a sentence.
	BulletsInCv *FlexCount `json:"bulletsInCv,omitempty" jsonschema:"how many bullet points the CV lists under this job; the description must keep a line for most of them"`
}

// EducationProposal is one school, degree or course found in the CV (plan 175). Dates are YYYY, YYYY-MM or YYYY-MM-DD; an empty end date
// means still studying there.
type EducationProposal struct {
	Action              string  `json:"action" jsonschema:"insert, or skip when not relevant or not confident"`
	PossibleDuplicateOf *string `json:"possibleDuplicateOf" jsonschema:"the id of an existing education entry this matches (from list_education), otherwise null"`
	Institution         string  `json:"institution"`
	Degree              string  `json:"degree"`
	FieldOfStudy        string  `json:"fieldOfStudy"`
	StartDate           string  `json:"startDate" jsonschema:"YYYY, YYYY-MM or YYYY-MM-DD, or empty"`
	EndDate             string  `json:"endDate" jsonschema:"YYYY, YYYY-MM or YYYY-MM-DD, or empty while still studying"`
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
		if e.BulletsInCv != nil {
			if *e.BulletsInCv < 0 {
				return Proposal{}, problem("%s.bulletsInCv must be 0 or more", path)
			}
			if e.Action == actionInsert {
				if need, have := requiredLines(int64(*e.BulletsInCv)), descriptionLines(e.Description); have < need {
					return Proposal{}, problem("%s (%s): you said the CV lists %d bullet points but the description has %d lines (at least %d are needed); include every bullet point as its own line, you may improve the wording but not cut it, and send the job again (the same company, title and start date replaces it)", path, e.Company, int64(*e.BulletsInCv), have, need)
				}
			}
		}
	}
	if len(p.Education) > MaxProposalEducation {
		return Proposal{}, problem("education can have at most %d entries", MaxProposalEducation)
	}
	for i, e := range p.Education {
		path := fmt.Sprintf("education[%d]", i)
		if err := checkAction(path, e.Action); err != nil {
			return Proposal{}, err
		}
		if err := checkDuplicate(path, e.PossibleDuplicateOf); err != nil {
			return Proposal{}, err
		}
		for field, v := range map[string]string{"institution": e.Institution, "degree": e.Degree, "fieldOfStudy": e.FieldOfStudy, "startDate": e.StartDate, "endDate": e.EndDate, "description": e.Description} {
			if err := checkText(path+"."+field, v); err != nil {
				return Proposal{}, err
			}
		}
		// An entry to insert must be one the profile accepts (the same check as adding it by hand); a skipped one is not inserted, so it may be incomplete.
		if e.Action == actionInsert {
			if err := education.CheckInput(education.Input{Institution: e.Institution, Degree: e.Degree, FieldOfStudy: e.FieldOfStudy, StartDate: e.StartDate, EndDate: e.EndDate, Description: e.Description}); err != nil {
				return Proposal{}, problem("%s: %v (dates are YYYY, YYYY-MM or YYYY-MM-DD; leave the end date empty while still studying)", path, err)
			}
		}
	}
	if p.Skills == nil {
		p.Skills = []SkillProposal{}
	}
	if p.Experience == nil {
		p.Experience = []ExperienceProposal{}
	}
	if p.Education == nil {
		p.Education = []EducationProposal{}
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
	RunID    string   `json:"runId,omitempty" jsonschema:"the run id given in the request"`
	Proposal Proposal `json:"proposal" jsonschema:"what to take from the CV, built only from the CV text and the data already stored; an object (the JSON text of one is accepted too)"`
}

// UnmarshalJSON reads a proposal sent as an object or as the JSON text of one: weak models alternate between the two, and the
// string form used to be rejected before the tool ran (the "keeps sending invalid tool arguments" failure, plan 165).
func (p *Proposal) UnmarshalJSON(data []byte) error {
	type plain Proposal
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return err
		}
		trimmed = bytes.TrimSpace([]byte(text))
		if len(trimmed) == 0 || trimmed[0] != '{' {
			return errors.New("proposal was a string that is not a JSON object; send it as an object")
		}
	}
	return json.Unmarshal(trimmed, (*plain)(p))
}

// suggestCVImport is the tool's work, apart from the MCP plumbing so it can be tested on its own.
func suggestCVImport(args suggestCVImportArgs) (*mcp.CallToolResult, any, error) {
	runID, err := resolveRunID(args.RunID)
	if err == nil {
		err = StoreProposal(runID, args.Proposal)
	}
	if err == nil {
		return db.JSONResult(map[string]any{"stored": true})
	}
	if res, ok := refusal(runID, err); ok {
		return res, nil, nil
	}
	return db.ErrResult("could not store the proposal: " + err.Error()), nil, nil
}

// suggestSchema is the tool's input schema with the proposal allowed as an object or as text (see Proposal.UnmarshalJSON).
func suggestSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[suggestCVImportArgs](nil)
	if err != nil {
		panic(err) // a programming error caught by the first test run
	}
	if prop := schema.Properties["proposal"]; prop != nil {
		prop.Type, prop.Types = "", []string{"object", "string"}
	}
	return schema
}

// RegisterSuggestCVImport registers suggest_cv_import.
func RegisterSuggestCVImport(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		InputSchema: suggestSchema(),
		Name:        "suggest_cv_import",
		Description: "Hand over the result of a CV analysis to the user, who then reviews it on the Import page. " +
			"It creates no profile, persona, skill or experience. Call it ONCE, with the runId from the request and the whole proposal " +
			"(profile, persona, personalDetails, skills, experience, and education when the CV has any; each item with action insert or skip and possibleDuplicateOf). " +
			"Fails with a message when the run is unknown or no longer running, or when the proposal is not valid; correct it and call again.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args suggestCVImportArgs) (*mcp.CallToolResult, any, error) {
		return suggestCVImport(args)
	})
}
