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

// addCareerSkillsPlain is the shape of the call as the schema describes it; addCareerSkillsArgs is what the handler reads, which also
// accepts the list as a text (see bulk_loose.go).
type addCareerSkillsPlain struct {
	PersonaID string   `json:"personaId" jsonschema:"the persona to add these skills to, from create_persona or list_personas"`
	Skills    []string `json:"skills" jsonschema:"the skills to add, e.g. [\"Go\", \"Kubernetes\"]: plain text, at most 50 per call"`
}

type addCareerSkillsArgs struct {
	PersonaID string       `json:"personaId"`
	Skills    LooseStrings `json:"skills"`
}

func RegisterAddCareerSkills(server *mcp.Server) {
	schema, err := looseSchema[addCareerSkillsPlain]("skills", "Send a JSON list; a text holding the list or the skills separated by commas is read as well.")
	if err != nil {
		panic(err) // the argument type is fixed in the code: this cannot depend on any input
	}
	mcp.AddTool(server, &mcp.Tool{
		InputSchema: schema,
		Name:        "add_career_skills",
		Description: "Add several skills at once that belong only to this persona (custom skills of its own, not skills of the profile's library; add those with add_profile_skill and choose them with set_persona_skills). " +
			"Use this instead of calling add_career_skill many times. At most 50 per call; a skill the persona already has (any case) or one repeated in the call is skipped. " +
			"All or nothing: if any entry is refused, nothing is added and the answer says which. Returns {added, skipped}.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args addCareerSkillsArgs) (*mcp.CallToolResult, any, error) {
		if args.PersonaID == "" {
			return db.ErrResult("personaId is required"), nil, nil
		}
		result, err := AddCareerSkills(args.PersonaID, []string(args.Skills))
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

// BulkExperienceUpdated is an entry that matched an existing entry of the persona (same company and title) which had no start date: the
// existing entry got its blanks filled instead of a second entry being added. Filled names the fields that were filled.
type BulkExperienceUpdated struct {
	Index   int      `json:"index"`
	ID      string   `json:"id"`
	Company string   `json:"company"`
	Title   string   `json:"title"`
	Filled  []string `json:"filled"`
}

type BulkExperiencesResult struct {
	Added   []BulkExperienceAdded   `json:"added"`
	Updated []BulkExperienceUpdated `json:"updated"`
	Skipped []BulkExperienceSkipped `json:"skipped"`
}

// jobKey is what makes two entries the same job: company and title, without case and surrounding spaces.
func jobKey(company, title string) string {
	return strings.ToLower(strings.TrimSpace(company)) + "\x00" + strings.ToLower(strings.TrimSpace(title))
}

// ownRole is one of the persona's own entries while a bulk call works.
type ownRole struct {
	ID, Company, Title, Start, End, Description string
}

func experienceKey(company, title, start string) string {
	return strings.ToLower(strings.TrimSpace(company)) + "\x00" + strings.ToLower(strings.TrimSpace(title)) + "\x00" + strings.ToLower(strings.TrimSpace(start))
}

// AddCareerExperiences adds work experience entries to the persona's own entries in one transaction: all or nothing. Every entry is
// trimmed, needs a company and a title, and is plain text of a sane length (the persona proposal's limits: 300 characters, 4,000 for a
// description). Entries the persona already has are not added a second time, so that sending a call again after an error (or a model
// sending a stripped version of the jobs first and the complete version after) does not double them:
//   - an entry equal to an existing one or to an earlier one in the call (same company, title and start date, without case, after
//     trimming; a stored NULL start date counts as empty) is skipped and reported;
//   - an entry with a start date that matches an existing entry on company and title, where that entry has NO start date, fills the
//     existing entry's blanks (the start date; the end date and the description only if they are empty, nothing that is there is
//     overwritten) and is reported in updated;
//   - an entry with NO start date whose company and title the persona already has is skipped: it carries nothing the entry lacks.
//
// Like add_career_experience it adds to the persona's own entries only, never to the profile's library.
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

	var roles []*ownRole
	rows, err := tx.Query(`SELECT id, company, title, COALESCE(start_date, ''), COALESCE(end_date, ''), COALESCE(description, '') FROM career_experience WHERE persona_id = ? ORDER BY rowid`, personaID)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var r ownRole
		if err := rows.Scan(&r.ID, &r.Company, &r.Title, &r.Start, &r.End, &r.Description); err != nil {
			_ = rows.Close()
			return res, err
		}
		roles = append(roles, &r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return res, err
	}
	_ = rows.Close()
	same := map[string]int{} // entries by company, title and start date
	jobs := map[string]int{} // entries by company and title
	for _, r := range roles {
		same[experienceKey(r.Company, r.Title, r.Start)]++
		jobs[jobKey(r.Company, r.Title)]++
	}

	res.Added, res.Updated, res.Skipped = []BulkExperienceAdded{}, []BulkExperienceUpdated{}, []BulkExperienceSkipped{}
	for i, c := range clean {
		key := experienceKey(c.Company, c.Title, c.StartDate)
		job := jobKey(c.Company, c.Title)
		if same[key] > 0 || (c.StartDate == "" && jobs[job] > 0) {
			res.Skipped = append(res.Skipped, BulkExperienceSkipped{Index: i, Company: c.Company, Title: c.Title, StartDate: c.StartDate})
			continue
		}
		if c.StartDate != "" {
			var blank *ownRole
			for _, r := range roles {
				if r.Start == "" && jobKey(r.Company, r.Title) == job {
					blank = r
					break
				}
			}
			if blank != nil {
				oldKey := experienceKey(blank.Company, blank.Title, blank.Start)
				filled := []string{"startDate"}
				blank.Start = c.StartDate
				if blank.End == "" && c.EndDate != "" {
					blank.End = c.EndDate
					filled = append(filled, "endDate")
				}
				if blank.Description == "" && c.Description != "" {
					blank.Description = c.Description
					filled = append(filled, "description")
				}
				if _, err := tx.Exec(`UPDATE career_experience SET start_date = ?, end_date = ?, description = ? WHERE id = ? AND persona_id = ?`,
					blank.Start, blank.End, blank.Description, blank.ID, personaID); err != nil {
					return BulkExperiencesResult{}, err
				}
				same[oldKey]--
				same[key]++
				res.Updated = append(res.Updated, BulkExperienceUpdated{Index: i, ID: blank.ID, Company: blank.Company, Title: blank.Title, Filled: filled})
				continue
			}
		}
		id := uuid.NewString()
		if _, err := tx.Exec(
			`INSERT INTO career_experience (id, persona_id, company, title, start_date, end_date, description) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id, personaID, c.Company, c.Title, c.StartDate, c.EndDate, c.Description,
		); err != nil {
			return BulkExperiencesResult{}, err
		}
		roles = append(roles, &ownRole{ID: id, Company: c.Company, Title: c.Title, Start: c.StartDate, End: c.EndDate, Description: c.Description})
		same[key]++
		jobs[job]++
		res.Added = append(res.Added, BulkExperienceAdded{Index: i, ID: id, Company: c.Company, Title: c.Title})
	}
	if err := tx.Commit(); err != nil {
		return BulkExperiencesResult{}, err
	}
	// One event per kind of change, and only for a kind that happened.
	if len(res.Added) > 0 {
		publishPersonaChanged(personaID, ChangeExperienceAdded)
	}
	if len(res.Updated) > 0 {
		publishPersonaChanged(personaID, ChangeExperienceUpdated)
	}
	return res, nil
}

// addCareerExperiencesPlain is the shape of the call as the schema describes it; addCareerExperiencesArgs is what the handler reads,
// which also accepts the list as a text (see bulk_loose.go).
type addCareerExperiencesPlain struct {
	PersonaID   string           `json:"personaId" jsonschema:"the persona to add these experience entries to, from create_persona or list_personas"`
	Experiences []BulkExperience `json:"experiences" jsonschema:"the work experience entries to add, newest first as the CV lists them: at most 10 per call"`
}

type addCareerExperiencesArgs struct {
	PersonaID   string           `json:"personaId"`
	Experiences LooseExperiences `json:"experiences"`
}

func RegisterAddCareerExperiences(server *mcp.Server) {
	schema, err := looseSchema[addCareerExperiencesPlain]("experiences", "Send a JSON list of objects; a text holding that JSON list is read as well.")
	if err != nil {
		panic(err) // the argument type is fixed in the code: this cannot depend on any input
	}
	mcp.AddTool(server, &mcp.Tool{
		InputSchema: schema,
		Name:        "add_career_experiences",
		Description: "Add several work experience entries at once that belong only to this persona (custom entries of its own, not experiences of the profile's library; add those with add_profile_experience and choose them with set_persona_experience). " +
			"Use this instead of calling add_career_experience many times. At most 10 per call; every entry needs a company and a title. An entry the persona already has (same company, title and start date) or one repeated in the call is skipped; " +
			"an entry without a start date whose company and title the persona already has is skipped too. An entry WITH a start date whose company and title match an existing entry that has no start date fills that entry's blank fields instead of adding a second one (reported in updated). " +
			"All or nothing: if any entry is refused, nothing changes and the answer says which. Returns {added: [{index, id, company, title}], updated: [{index, id, company, title, filled}], skipped: [...]}.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args addCareerExperiencesArgs) (*mcp.CallToolResult, any, error) {
		if args.PersonaID == "" {
			return db.ErrResult("personaId is required"), nil, nil
		}
		result, err := AddCareerExperiences(args.PersonaID, []BulkExperience(args.Experiences))
		if err != nil {
			return personaAwareErrResult("add experiences", args.PersonaID, err), nil, nil
		}
		return db.JSONResult(result)
	})
}
