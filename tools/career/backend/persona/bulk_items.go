package persona

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
)

// Bulk tools of a persona's own details (plan/ai/tools/career/persona-ai-tools.md): the single-item tools take one entry per call, and a
// chat turn allows only so many tool rounds, so a list of skills or jobs could not be entered in one turn.

// maxBulkSkills is how many skills one add_career_skills call may carry.
const maxBulkSkills = 50

// BulkSkillsResult says what a bulk add did: the skills that were added (as stored, trimmed) and the ones skipped because the persona
// had them already or the call repeated them (the first spelling in the call is the one that counts).
type BulkSkillsResult struct {
	Added   []string `json:"added"`
	Skipped []string `json:"skipped"`
}

// ErrBulkRefused is returned when a bulk call is refused as a whole: nothing was changed. The message names every entry that was the
// reason (up to a few), so the caller can fix them and send the call again.
var ErrBulkRefused = errors.New("nothing was added")

// maxBulkProblems is how many refused entries the message names.
const maxBulkProblems = 8

// refusal is the error of a bulk call refused as a whole: it names the entries that were the reason, at most maxBulkProblems of them.
func refusal(problems []string) error {
	more := ""
	if len(problems) > maxBulkProblems {
		more = fmt.Sprintf(" (and %d more)", len(problems)-maxBulkProblems)
		problems = problems[:maxBulkProblems]
	}
	return fmt.Errorf("%w: %s%s", ErrBulkRefused, strings.Join(problems, "; "), more)
}

// AddCareerSkills adds skills to the persona's own skills in one transaction: all or nothing. Every skill is trimmed and must be plain
// text of a sane length; one the persona already has (compared without case, as the AI persona proposal does) or one repeated in the
// call is skipped, not an error. Like add_career_skill it touches only the persona's own skills, never the profile's library.
func AddCareerSkills(personaID string, skills []string) (BulkSkillsResult, error) {
	var res BulkSkillsResult
	if err := requirePersonaExists(personaID); err != nil {
		return res, err
	}
	switch {
	case len(skills) == 0:
		return res, fmt.Errorf("%w: skills is empty", ErrBulkRefused)
	case len(skills) > maxBulkSkills:
		return res, fmt.Errorf("%w: too many skills (max %d per call, got %d): send them in several calls", ErrBulkRefused, maxBulkSkills, len(skills))
	}

	var problems []string
	trimmed := make([]string, len(skills))
	for i, s := range skills {
		t := strings.TrimSpace(s)
		trimmed[i] = t
		switch {
		case t == "":
			problems = append(problems, fmt.Sprintf("skills[%d] is empty", i))
		case utf8.RuneCountInString(t) > maxShortField:
			problems = append(problems, fmt.Sprintf("skills[%d] is too long (max %d characters)", i, maxShortField))
		case hasMarkup(t):
			problems = append(problems, fmt.Sprintf("skills[%d] must be plain text, no markup", i))
		}
	}
	if len(problems) > 0 {
		return res, refusal(problems)
	}

	tx, err := db.CareerDB.Begin()
	if err != nil {
		return res, err
	}
	defer func() { _ = tx.Rollback() }()

	have := map[string]bool{}
	rows, err := tx.Query(`SELECT skill FROM career_skills WHERE persona_id = ?`, personaID)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			_ = rows.Close()
			return res, err
		}
		have[strings.ToLower(strings.TrimSpace(s))] = true
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return res, err
	}
	_ = rows.Close()

	res.Added, res.Skipped = []string{}, []string{}
	for _, s := range trimmed {
		key := strings.ToLower(s)
		if have[key] {
			res.Skipped = append(res.Skipped, s)
			continue
		}
		have[key] = true
		if _, err := tx.Exec(`INSERT INTO career_skills (id, persona_id, skill) VALUES (?, ?, ?)`, uuid.NewString(), personaID, s); err != nil {
			return BulkSkillsResult{}, err
		}
		res.Added = append(res.Added, s)
	}
	if err := tx.Commit(); err != nil {
		return BulkSkillsResult{}, err
	}
	if len(res.Added) > 0 { // one event for the call, and only when something changed
		publishPersonaChanged(personaID, ChangeSkillAdded)
	}
	return res, nil
}

type addCareerSkillsArgs struct {
	PersonaID string   `json:"personaId" jsonschema:"the persona to add these skills to, from create_persona or list_personas"`
	Skills    []string `json:"skills" jsonschema:"the skills to add, e.g. [\"Go\", \"Kubernetes\"]: plain text, at most 50 per call"`
}

func RegisterAddCareerSkills(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "add_career_skills",
		Description: "Add several skills at once that belong only to this persona (custom skills of its own, not skills of the profile's library; add those with add_profile_skill and choose them with set_persona_skills). " +
			"Use this instead of calling add_career_skill many times. At most 50 per call; a skill the persona already has (any case) or one repeated in the call is skipped. " +
			"All or nothing: if any entry is refused, nothing is added and the answer says which. Returns {added, skipped}.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args addCareerSkillsArgs) (*mcp.CallToolResult, any, error) {
		if args.PersonaID == "" {
			return db.ErrResult("personaId is required"), nil, nil
		}
		result, err := AddCareerSkills(args.PersonaID, args.Skills)
		if err != nil {
			return personaAwareErrResult("add skills", args.PersonaID, err), nil, nil
		}
		return db.JSONResult(result)
	})
}

// maxBulkExperiences is how many work experience entries one add_career_experiences call may carry.
const maxBulkExperiences = 10

// BulkExperience is one entry of an add_career_experiences call: the fields of add_career_experience. The dates are free text there, and
// stay free text here (one behaviour for both tools).
type BulkExperience struct {
	Company     string `json:"company" jsonschema:"the employer's name"`
	Title       string `json:"title" jsonschema:"the job title held there"`
	StartDate   string `json:"startDate,omitempty" jsonschema:"when this position started"`
	EndDate     string `json:"endDate,omitempty" jsonschema:"when this position ended: omit for a current position"`
	Description string `json:"description,omitempty" jsonschema:"what the role involved"`
}

// BulkExperienceAdded is an entry that was added. Index is its place in the call (from 0).
type BulkExperienceAdded struct {
	Index   int    `json:"index"`
	ID      string `json:"id"`
	Company string `json:"company"`
	Title   string `json:"title"`
}

// BulkExperienceSkipped is an entry that was not added because the same entry (company, title and start date, without case) is already
// among the persona's own entries or came earlier in the call.
type BulkExperienceSkipped struct {
	Index     int    `json:"index"`
	Company   string `json:"company"`
	Title     string `json:"title"`
	StartDate string `json:"startDate"`
}

type BulkExperiencesResult struct {
	Added   []BulkExperienceAdded   `json:"added"`
	Skipped []BulkExperienceSkipped `json:"skipped"`
}

func experienceKey(company, title, start string) string {
	return strings.ToLower(strings.TrimSpace(company)) + "\x00" + strings.ToLower(strings.TrimSpace(title)) + "\x00" + strings.ToLower(strings.TrimSpace(start))
}

// AddCareerExperiences adds work experience entries to the persona's own entries in one transaction: all or nothing. Every entry is
// trimmed, needs a company and a title, and is plain text of a sane length (the persona proposal's limits: 300 characters, 4,000 for a
// description). An entry equal to one the persona already has, or to an earlier one in the call (same company, title and start date,
// without case), is skipped and reported, so that sending a call again after an error does not add the jobs twice. Like
// add_career_experience it adds to the persona's own entries only, never to the profile's library.
func AddCareerExperiences(personaID string, entries []BulkExperience) (BulkExperiencesResult, error) {
	var res BulkExperiencesResult
	if err := requirePersonaExists(personaID); err != nil {
		return res, err
	}
	switch {
	case len(entries) == 0:
		return res, fmt.Errorf("%w: experiences is empty", ErrBulkRefused)
	case len(entries) > maxBulkExperiences:
		return res, fmt.Errorf("%w: too many experiences (max %d per call, got %d): send them in several calls", ErrBulkRefused, maxBulkExperiences, len(entries))
	}

	var problems []string
	clean := make([]BulkExperience, len(entries))
	for i, e := range entries {
		c := BulkExperience{
			Company: strings.TrimSpace(e.Company), Title: strings.TrimSpace(e.Title),
			StartDate: strings.TrimSpace(e.StartDate), EndDate: strings.TrimSpace(e.EndDate), Description: strings.TrimSpace(e.Description),
		}
		clean[i] = c
		if c.Company == "" {
			problems = append(problems, fmt.Sprintf("experiences[%d].company is empty", i))
		}
		if c.Title == "" {
			problems = append(problems, fmt.Sprintf("experiences[%d].title is empty", i))
		}
		for _, f := range []struct {
			name, val string
			max       int
		}{
			{"company", c.Company, maxShortField}, {"title", c.Title, maxShortField},
			{"startDate", c.StartDate, maxShortField}, {"endDate", c.EndDate, maxShortField},
			{"description", c.Description, maxLongField},
		} {
			switch {
			case utf8.RuneCountInString(f.val) > f.max:
				problems = append(problems, fmt.Sprintf("experiences[%d].%s is too long (max %d characters)", i, f.name, f.max))
			case hasMarkup(f.val):
				problems = append(problems, fmt.Sprintf("experiences[%d].%s must be plain text, no markup", i, f.name))
			}
		}
	}
	if len(problems) > 0 {
		return res, refusal(problems)
	}

	tx, err := db.CareerDB.Begin()
	if err != nil {
		return res, err
	}
	defer func() { _ = tx.Rollback() }()

	have := map[string]bool{}
	rows, err := tx.Query(`SELECT company, title, COALESCE(start_date, '') FROM career_experience WHERE persona_id = ?`, personaID)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var company, title, start string
		if err := rows.Scan(&company, &title, &start); err != nil {
			_ = rows.Close()
			return res, err
		}
		have[experienceKey(company, title, start)] = true
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return res, err
	}
	_ = rows.Close()

	res.Added, res.Skipped = []BulkExperienceAdded{}, []BulkExperienceSkipped{}
	for i, c := range clean {
		key := experienceKey(c.Company, c.Title, c.StartDate)
		if have[key] {
			res.Skipped = append(res.Skipped, BulkExperienceSkipped{Index: i, Company: c.Company, Title: c.Title, StartDate: c.StartDate})
			continue
		}
		have[key] = true
		id := uuid.NewString()
		if _, err := tx.Exec(
			`INSERT INTO career_experience (id, persona_id, company, title, start_date, end_date, description) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id, personaID, c.Company, c.Title, c.StartDate, c.EndDate, c.Description,
		); err != nil {
			return BulkExperiencesResult{}, err
		}
		res.Added = append(res.Added, BulkExperienceAdded{Index: i, ID: id, Company: c.Company, Title: c.Title})
	}
	if err := tx.Commit(); err != nil {
		return BulkExperiencesResult{}, err
	}
	if len(res.Added) > 0 { // one event for the call, and only when something changed
		publishPersonaChanged(personaID, ChangeExperienceAdded)
	}
	return res, nil
}

type addCareerExperiencesArgs struct {
	PersonaID   string           `json:"personaId" jsonschema:"the persona to add these experience entries to, from create_persona or list_personas"`
	Experiences []BulkExperience `json:"experiences" jsonschema:"the work experience entries to add, newest first as the CV lists them: at most 10 per call"`
}

func RegisterAddCareerExperiences(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "add_career_experiences",
		Description: "Add several work experience entries at once that belong only to this persona (custom entries of its own, not experiences of the profile's library; add those with add_profile_experience and choose them with set_persona_experience). " +
			"Use this instead of calling add_career_experience many times. At most 10 per call; every entry needs a company and a title. An entry the persona already has (same company, title and start date) or one repeated in the call is skipped. " +
			"All or nothing: if any entry is refused, nothing is added and the answer says which. Returns {added: [{index, id, company, title}], skipped: [...]}.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args addCareerExperiencesArgs) (*mcp.CallToolResult, any, error) {
		if args.PersonaID == "" {
			return db.ErrResult("personaId is required"), nil, nil
		}
		result, err := AddCareerExperiences(args.PersonaID, args.Experiences)
		if err != nil {
			return personaAwareErrResult("add experiences", args.PersonaID, err), nil, nil
		}
		return db.JSONResult(result)
	})
}
