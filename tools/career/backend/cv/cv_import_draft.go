// cv_import_draft.go — the AI hands a CV proposal over in small pieces (step 165). One tool call that carries the whole proposal of
// a long CV is the failure that fills the AI log: the call gets long, is cut off or sent in the wrong shape, and every error costs a
// full retype. Here each piece is its own small call (cv_import_set for the profile, persona and personal details,
// cv_import_add_skills, cv_import_add_experience with one job per call); the server keeps the pieces as a draft on the run and
// cv_import_finish validates the whole draft and stores it as the proposal, exactly what suggest_cv_import stores. Every piece is
// validated alone (so a wrong piece is corrected alone), a piece sent again replaces the same one, and a value may be an object or
// the JSON text of one (weak models send either). See plan/ai/tools/career/step-165-cv-import-failures.md.
package cv

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
)

var (
	// errNoGeneratingRun: no run is generating, so a call without a run id has nothing to belong to.
	errNoGeneratingRun = errors.New("no cv import run is generating")
	// errSeveralGeneratingRuns: a call without a run id is ambiguous.
	errSeveralGeneratingRuns = errors.New("several cv import runs are generating")
)

// The sections cv_import_set takes.
const (
	sectionProfile         = "profile"
	sectionPersona         = "persona"
	sectionPersonalDetails = "personalDetails"
)

// importDraft is the pieces handed over so far; a nil pointer is a section not sent yet.
type importDraft struct {
	Profile         *ProfileProposal     `json:"profile,omitempty"`
	Persona         *PersonaProposal     `json:"persona,omitempty"`
	PersonalDetails *DetailsProposal     `json:"personalDetails,omitempty"`
	Skills          []SkillProposal      `json:"skills,omitempty"`
	Experience      []ExperienceProposal `json:"experience,omitempty"`
}

// allSkipped says all three single sections were sent and every one is skipped.
func (d importDraft) allSkipped() bool {
	return d.Profile != nil && d.Persona != nil && d.PersonalDetails != nil &&
		d.Profile.Action == actionSkip && d.Persona.Action == actionSkip && d.PersonalDetails.Action == actionSkip
}

// missing lists what cv_import_finish still needs.
func (d importDraft) missing() []string {
	var out []string
	if d.Profile == nil {
		out = append(out, sectionProfile)
	}
	if d.Persona == nil {
		out = append(out, sectionPersona)
	}
	if d.PersonalDetails == nil {
		out = append(out, sectionPersonalDetails)
	}
	// Skills or jobs are needed unless everything was skipped: a document that is not a CV is answered with three skipped sections.
	if len(d.Skills) == 0 && len(d.Experience) == 0 && !d.allSkipped() {
		out = append(out, "skills or experience (at least one)")
	}
	return out
}

// summary is what a tool answers so the AI sees what is left to send.
func (d importDraft) summary() map[string]any {
	return map[string]any{
		"stored": true,
		"draft": map[string]any{
			sectionProfile: d.Profile != nil, sectionPersona: d.Persona != nil, sectionPersonalDetails: d.PersonalDetails != nil,
			"skills": len(d.Skills), "experience": len(d.Experience),
		},
		"stillMissing": d.missing(),
	}
}

// resolveRunID is the run a call belongs to: the one it names, or else the only run that is generating.
func resolveRunID(id string) (string, error) {
	if id = strings.TrimSpace(id); id != "" {
		return id, nil
	}
	rows, err := db.CareerDB.Query(`SELECT id FROM cv_import_runs WHERE status = 'generating' ORDER BY created_at DESC LIMIT 2`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return "", err
		}
		ids = append(ids, s)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	switch {
	case len(ids) == 0:
		return "", errNoGeneratingRun
	case len(ids) > 1:
		return "", errSeveralGeneratingRuns
	}
	return ids[0], nil
}

// rawJSON is the JSON bytes of a tool value that should be an object or a list but may arrive as the JSON text of it.
func rawJSON(raw any, what, shape string) ([]byte, error) {
	if raw == nil {
		return nil, problem("%s is required: %s", what, shape)
	}
	if text, ok := raw.(string); ok {
		data := bytes.TrimSpace([]byte(text))
		if len(data) == 0 || (data[0] != '{' && data[0] != '[') {
			return nil, problem("%s was a string that is not JSON: %s", what, shape)
		}
		return data, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, problem("%s could not be read: %s", what, shape)
	}
	return data, nil
}

// lenientDecode reads a tool value that should be a JSON object (or list) but may arrive as the JSON text of it.
func lenientDecode(raw any, into any, what, shape string) error {
	data, err := rawJSON(raw, what, shape)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, into); err != nil {
		return problem("%s is not in the right shape (%v): %s", what, err, shape)
	}
	return nil
}

// skipPlaceholder is a proposal whose three single sections are valid and skipped, so one piece can be validated by the rules of
// the whole proposal.
func skipPlaceholder() Proposal {
	return Proposal{Profile: ProfileProposal{Action: actionSkip}, Persona: PersonaProposal{Action: actionSkip}, PersonalDetails: DetailsProposal{Action: actionSkip}}
}

// updateDraft applies change to the draft of a generating run in one transaction and returns the new draft.
func updateDraft(runID string, change func(*importDraft) error) (importDraft, error) {
	var d importDraft
	tx, err := db.CareerDB.Begin()
	if err != nil {
		return d, err
	}
	defer tx.Rollback() //nolint:errcheck // a rollback after a commit is a no-op
	var stored sql.NullString
	switch err := tx.QueryRow(`SELECT draft_json FROM cv_import_runs WHERE id = ? AND status = 'generating'`, runID).Scan(&stored); {
	case errors.Is(err, sql.ErrNoRows):
		tx.Rollback() //nolint:errcheck
		return d, notGeneratingOrUnknown(runID)
	case err != nil:
		return d, err
	}
	if stored.Valid && stored.String != "" {
		if err := json.Unmarshal([]byte(stored.String), &d); err != nil {
			return d, fmt.Errorf("the stored draft is unreadable: %w", err)
		}
	}
	if err := change(&d); err != nil {
		return importDraft{}, err
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return d, err
	}
	if _, err := tx.Exec(`UPDATE cv_import_runs SET draft_json = ?, updated_at = datetime('now') WHERE id = ?`, string(raw), runID); err != nil {
		return d, err
	}
	return d, tx.Commit()
}

// SetDraftSection validates one of the three single sections and keeps it on the run, replacing an earlier one.
func SetDraftSection(runID, section string, value any) (importDraft, error) {
	return updateDraft(runID, func(d *importDraft) error {
		p := skipPlaceholder()
		switch section {
		case sectionProfile:
			if err := lenientDecode(value, &p.Profile, "value", `an object {"action","possibleDuplicateOf","name"}`); err != nil {
				return err
			}
		case sectionPersona:
			if err := lenientDecode(value, &p.Persona, "value", `an object {"action","possibleDuplicateOf","name","title"}`); err != nil {
				return err
			}
		case sectionPersonalDetails:
			if err := lenientDecode(value, &p.PersonalDetails, "value", `an object {"action","possibleDuplicateOf","headline","summary","location","desiredTitles","desiredLocations","minSalary"}`); err != nil {
				return err
			}
		default:
			return problem(`section must be "profile", "persona" or "personalDetails" (skills and experience have their own tools: cv_import_add_skills, cv_import_add_experience), not %q`, section)
		}
		clean, err := validateProposal(p)
		if err != nil {
			return err
		}
		switch section {
		case sectionProfile:
			d.Profile = &clean.Profile
		case sectionPersona:
			d.Persona = &clean.Persona
		default:
			d.PersonalDetails = &clean.PersonalDetails
		}
		return nil
	})
}

// AddDraftSkills validates skills and adds them to the run's draft; a skill with the same text (any case) replaces the earlier one.
func AddDraftSkills(runID string, items any) (importDraft, error) {
	return updateDraft(runID, func(d *importDraft) error {
		var skills []SkillProposal
		if err := lenientDecode(items, &skills, "items", `a list of {"action","possibleDuplicateOf","value"} objects`); err != nil {
			return err
		}
		p := skipPlaceholder()
		p.Skills = skills
		clean, err := validateProposal(p)
		if err != nil {
			return problem("%v (counting from 0 in this call)", err)
		}
	next:
		for _, s := range clean.Skills {
			for i, old := range d.Skills {
				if strings.EqualFold(strings.TrimSpace(old.Value), strings.TrimSpace(s.Value)) {
					d.Skills[i] = s
					continue next
				}
			}
			d.Skills = append(d.Skills, s)
		}
		if len(d.Skills) > MaxProposalSkills {
			return problem("skills can have at most %d entries", MaxProposalSkills)
		}
		return nil
	})
}

// MaxJobsPerCall bounds one cv_import_add_experience call: the AI has a limited number of rounds, so it may send several jobs at once,
// but not so many that one call is the long call this design avoids.
const MaxJobsPerCall = 10

// AddDraftExperience validates one job, or a list of up to MaxJobsPerCall jobs, and adds them to the run's draft; a job with the same
// company, title and start date replaces the earlier one.
func AddDraftExperience(runID string, item any) (importDraft, error) {
	return updateDraft(runID, func(d *importDraft) error {
		const shape = `one job {"action","possibleDuplicateOf","company","title","startDate","endDate","description"}, or a list of up to 10 of them`
		data, err := rawJSON(item, "item", shape)
		if err != nil {
			return err
		}
		var jobs []ExperienceProposal
		if data[0] == '[' {
			err = json.Unmarshal(data, &jobs)
		} else {
			var one ExperienceProposal
			if err = json.Unmarshal(data, &one); err == nil {
				jobs = []ExperienceProposal{one}
			}
		}
		if err != nil {
			return problem("item is not in the right shape (%v): %s", err, shape)
		}
		if len(jobs) == 0 {
			return problem("item has no job: %s", shape)
		}
		if len(jobs) > MaxJobsPerCall {
			return problem("send at most %d jobs per call, not %d", MaxJobsPerCall, len(jobs))
		}
		p := skipPlaceholder()
		p.Experience = jobs
		clean, err := validateProposal(p)
		if err != nil {
			return problem("%v (counting from 0 in this call)", err)
		}
		key := func(x ExperienceProposal) string {
			return strings.ToLower(strings.TrimSpace(x.Company) + "|" + strings.TrimSpace(x.Title) + "|" + strings.TrimSpace(x.StartDate))
		}
	next:
		for _, e := range clean.Experience {
			for i, old := range d.Experience {
				if key(old) == key(e) {
					d.Experience[i] = e
					continue next
				}
			}
			d.Experience = append(d.Experience, e)
		}
		if len(d.Experience) > MaxProposalExperience {
			return problem("experience can have at most %d entries", MaxProposalExperience)
		}
		return nil
	})
}

// FinishDraft validates the whole draft and stores it as the run's proposal, then drops the draft. A draft that is not complete is a
// *proposalProblem naming what is missing; the run errors are those of StoreProposal.
func FinishDraft(runID string) error {
	d, err := updateDraft(runID, func(*importDraft) error { return nil })
	if err != nil {
		return err
	}
	if m := d.missing(); len(m) > 0 {
		return problem("the draft is not complete, still missing: %s. Send them with cv_import_set, cv_import_add_skills or cv_import_add_experience, then call cv_import_finish again", strings.Join(m, ", "))
	}
	if err := StoreProposal(runID, Proposal{Profile: *d.Profile, Persona: *d.Persona, PersonalDetails: *d.PersonalDetails, Skills: d.Skills, Experience: d.Experience}); err != nil {
		return err
	}
	_, err = db.CareerDB.Exec(`UPDATE cv_import_runs SET draft_json = NULL WHERE id = ? AND status = 'generating'`, runID)
	return err
}

// refusal turns what went wrong into a tool error the AI can act on; false when err is not one it knows.
func refusal(runID string, err error) (*mcp.CallToolResult, bool) {
	var bad *proposalProblem
	switch {
	case errors.Is(err, errNoGeneratingRun):
		return db.ErrResult("no import run is generating, so nothing was stored. Do not call this tool again and do not say the analysis is done: reply with one plain sentence that the import run has ended and the user must start a new import"), true
	case errors.Is(err, errSeveralGeneratingRuns):
		return db.ErrResult("runId is required: several import runs are generating; use the runId from the request"), true
	case errors.Is(err, errUnknownCVImportRun):
		return db.ErrResult(fmt.Sprintf("unknown run id %q", runID)), true
	case errors.Is(err, errRunNotGenerating):
		return db.ErrResult(runEndedMessage), true
	case errors.As(err, &bad):
		return db.ErrResult("not valid: " + bad.msg), true
	}
	return nil, false
}

// runEndedMessage is the answer for a run that is no longer generating (plan 165 step 1b): a resend from the chat window re-sends a
// prompt for a run the page ended as failed, and the AI must neither retry nor claim success.
const runEndedMessage = "this run is no longer generating, so it takes no proposal and nothing was stored. Do not call this tool again and do not say the analysis is done: reply with one plain sentence that the import run has ended and the user must start a new import"

func draftResult(runID string, d importDraft, err error) (*mcp.CallToolResult, any, error) {
	if err == nil {
		return db.JSONResult(d.summary())
	}
	if res, ok := refusal(runID, err); ok {
		return res, nil, nil
	}
	return db.ErrResult("could not store the piece: " + err.Error()), nil, nil
}

type cvImportSetArgs struct {
	RunID   string `json:"runId,omitempty" jsonschema:"the run id given in the request"`
	Section string `json:"section" jsonschema:"profile, persona or personalDetails"`
	Value   any    `json:"value" jsonschema:"that section as an object (profile: action, possibleDuplicateOf, name; persona: action, possibleDuplicateOf, name, title; personalDetails: action, possibleDuplicateOf, headline, summary, location, desiredTitles, desiredLocations, minSalary)"`
}

type cvImportAddSkillsArgs struct {
	RunID  string `json:"runId,omitempty" jsonschema:"the run id given in the request"`
	Items  any    `json:"items,omitempty" jsonschema:"a list of skills, each {action, possibleDuplicateOf, value}; send about 30 at most per call"`
	Skills any    `json:"skills,omitempty" jsonschema:"the same list under another name; send either items or skills"`
}

type cvImportAddExperienceArgs struct {
	RunID string `json:"runId,omitempty" jsonschema:"the run id given in the request"`
	Item  any    `json:"item,omitempty" jsonschema:"one job {action, possibleDuplicateOf, company, title, startDate, endDate, description}, or a list of up to 5 jobs"`
	Items any    `json:"items,omitempty" jsonschema:"the same under another name (a list of up to 5 jobs); send either item or items"`
}

type cvImportFinishArgs struct {
	RunID string `json:"runId,omitempty" jsonschema:"the run id given in the request"`
}

// RegisterCVImportDraftTools registers the four piece-by-piece tools.
func RegisterCVImportDraftTools(server *mcp.Server) {
	const common = " It creates no profile, persona, skill or experience; the user reviews the proposal afterwards. Fails with a message when the run has ended or the piece is not valid; correct that piece and send it again."
	mcp.AddTool(server, &mcp.Tool{
		Name: "cv_import_set",
		Description: "Hand over one section of the CV analysis (profile, persona or personalDetails) as an object; sending a section again replaces it. " +
			"Use this, cv_import_add_skills and cv_import_add_experience instead of one huge suggest_cv_import call for a long CV, then call cv_import_finish." + common,
	}, func(ctx context.Context, req *mcp.CallToolRequest, args cvImportSetArgs) (*mcp.CallToolResult, any, error) {
		runID, err := resolveRunID(args.RunID)
		if err != nil {
			return draftResult(args.RunID, importDraft{}, err)
		}
		d, err := SetDraftSection(runID, args.Section, args.Value)
		return draftResult(runID, d, err)
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "cv_import_add_skills",
		Description: "Add skills found in the CV to the analysis (a skill with the same text replaces the earlier one). Several calls are fine." + common,
	}, func(ctx context.Context, req *mcp.CallToolRequest, args cvImportAddSkillsArgs) (*mcp.CallToolResult, any, error) {
		runID, err := resolveRunID(args.RunID)
		if err != nil {
			return draftResult(args.RunID, importDraft{}, err)
		}
		d, err := AddDraftSkills(runID, firstOf(args.Items, args.Skills))
		return draftResult(runID, d, err)
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "cv_import_add_experience",
		Description: "Add jobs from the CV to the analysis: one job, or a list of up to 5 jobs per call (the same company, title and start date replaces the earlier one)." + common,
	}, func(ctx context.Context, req *mcp.CallToolRequest, args cvImportAddExperienceArgs) (*mcp.CallToolResult, any, error) {
		runID, err := resolveRunID(args.RunID)
		if err != nil {
			return draftResult(args.RunID, importDraft{}, err)
		}
		d, err := AddDraftExperience(runID, firstOf(args.Item, args.Items))
		return draftResult(runID, d, err)
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "cv_import_finish",
		Description: "Finish the analysis: checks that the profile, the persona, the personal details and at least one skill or job were sent and hands the whole " +
			"proposal to the user. Call it once, after the last piece." + common,
	}, func(ctx context.Context, req *mcp.CallToolRequest, args cvImportFinishArgs) (*mcp.CallToolResult, any, error) {
		runID, err := resolveRunID(args.RunID)
		if err != nil {
			return draftResult(args.RunID, importDraft{}, err)
		}
		if err := FinishDraft(runID); err != nil {
			return draftResult(runID, importDraft{}, err)
		}
		return db.JSONResult(map[string]any{"stored": true, "finished": true})
	})
}

// firstOf is the first value a weak model actually sent: it names the same argument differently from call to call ("item" and
// "items"), and the schema refuses an unknown name before the tool runs.
func firstOf(values ...any) any {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}
