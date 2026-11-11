// cv_import_suggestions_update.go — the AI changes the suggestions of a ready CV import run (plan 257, step 2): update, add and remove, all or nothing, with the same rules as
// the proposal had when it was handed over. Only a run the person is still reviewing can be changed (once something was saved from it, or it was declined, the Import page shows it as
// a finished record), and nothing here inserts anything into the profile: that stays the person's click on the review page.
package cv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
)

// MaxSuggestionChanges is how many operations one call may hold.
const MaxSuggestionChanges = 100

const (
	opUpdate = "update"
	opAdd    = "add"
	opRemove = "remove"
)

var (
	sections = []string{"profile", "persona", "personalDetails"}
	lists    = []string{"skills", "experience", "education"}
)

// SuggestionChange is one operation of update_cv_import_suggestions.
type SuggestionChange struct {
	Op      string `json:"op" jsonschema:"update, add or remove"`
	Section string `json:"section,omitempty" jsonschema:"for update of a single section: profile, persona or personalDetails"`
	List    string `json:"list,omitempty" jsonschema:"for a row: skills, experience or education"`
	ID      string `json:"id,omitempty" jsonschema:"for update or remove of a row: its id, from get_cv_import_suggestions"`
	// Fields (update) and Item (add) are objects; the JSON text of one is accepted too, as weak models alternate between the two.
	Fields any `json:"fields,omitempty" jsonschema:"for update: an object with only the fields to change, for example {\"value\": \"Go\"} or {\"action\": \"skip\"}"`
	Item   any `json:"item,omitempty" jsonschema:"for add: the new row as an object (action defaults to insert)"`
}

// SuggestionsChanged is what update answers: the new revision, the ids of the rows added, and how many operations were applied.
type SuggestionsChanged struct {
	RunID    string   `json:"runId"`
	Revision string   `json:"revision"`
	Applied  int      `json:"applied"`
	Added    []string `json:"added,omitempty"`
}

var (
	errSuggestionsClosed = errors.New("this import was already saved from or declined, so its suggestions can no longer be changed (the Import page shows them as a finished record); start a new import if something is missing")
	errSuggestionsMoved  = errors.New("the suggestions changed since you read them: read them again with get_cv_import_suggestions and make your changes on that version")
)

// decodeObject reads a JSON object strictly into target: a field the row does not have is a problem the AI can correct.
func decodeObject(b []byte, path, what string, target any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return problem("%s: %s is not valid: %v", path, what, err)
	}
	return nil
}

// overlay returns the row with the given fields changed: the row as JSON, the fields laid over it, read back strictly (an unknown field, or a value of the wrong kind, is refused).
func overlay(current any, fields any, path string, target any) error {
	b, err := rawJSON(fields, path+".fields", `an object such as {"value": "Go"}`)
	if err != nil {
		return err
	}
	var patch map[string]json.RawMessage
	if err := json.Unmarshal(b, &patch); err != nil || len(patch) == 0 {
		return problem("%s.fields must be an object with at least one field to change", path)
	}
	if _, has := patch["id"]; has {
		return problem("%s.fields: the id of a row cannot be changed", path)
	}
	cur, err := json.Marshal(current)
	if err != nil {
		return err
	}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(cur, &merged); err != nil {
		return err
	}
	for k, v := range patch {
		merged[k] = v
	}
	out, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return problem("%s.fields: %v", path, err)
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// applyChanges applies the operations, in order, to a copy of the proposal and returns it with the ids of the rows added. It stops at the first problem and then changes
// nothing (the caller stores the result only when there is none). The result is validated by validateProposal as a whole.
func applyChanges(p Proposal, changes []SuggestionChange) (Proposal, []string, error) {
	if len(changes) == 0 {
		return Proposal{}, nil, problem("changes is empty: give at least one operation")
	}
	if len(changes) > MaxSuggestionChanges {
		return Proposal{}, nil, problem("a call can have at most %d operations (it has %d)", MaxSuggestionChanges, len(changes))
	}
	p.Skills = append([]SkillProposal{}, p.Skills...)
	p.Experience = append([]ExperienceProposal{}, p.Experience...)
	p.Education = append([]EducationProposal{}, p.Education...)
	var added []string
	for i, c := range changes {
		path := fmt.Sprintf("changes[%d]", i)
		switch c.Op {
		case opUpdate, opAdd, opRemove:
		default:
			return Proposal{}, nil, problem(`%s.op must be "update", "add" or "remove"`, path)
		}
		if c.Section != "" && c.List != "" {
			return Proposal{}, nil, problem("%s: name a section or a list, not both", path)
		}
		if c.Section != "" {
			if c.Op != opUpdate {
				return Proposal{}, nil, problem("%s: a section (%s) can only be updated, not %sed", path, strings.Join(sections, ", "), c.Op)
			}
			if !contains(sections, c.Section) {
				return Proposal{}, nil, problem("%s.section must be one of %s", path, strings.Join(sections, ", "))
			}
			var err error
			switch c.Section {
			case "profile":
				err = overlay(p.Profile, c.Fields, path, &p.Profile)
			case "persona":
				err = overlay(p.Persona, c.Fields, path, &p.Persona)
			default:
				err = overlay(p.PersonalDetails, c.Fields, path, &p.PersonalDetails)
			}
			if err != nil {
				return Proposal{}, nil, err
			}
			continue
		}
		if !contains(lists, c.List) {
			return Proposal{}, nil, problem("%s: name a section (%s) or a list (%s)", path, strings.Join(sections, ", "), strings.Join(lists, ", "))
		}
		if c.Op == opAdd {
			if strings.TrimSpace(c.ID) != "" {
				return Proposal{}, nil, problem("%s: a new row gets its id from the application, leave id out", path)
			}
			id := uuid.NewString()
			added = append(added, id)
			if err := addRow(&p, c.List, c.Item, path, id); err != nil {
				return Proposal{}, nil, err
			}
			continue
		}
		id := strings.TrimSpace(c.ID)
		if id == "" {
			return Proposal{}, nil, problem("%s.id is required to %s a row of %s: take it from get_cv_import_suggestions", path, c.Op, c.List)
		}
		if err := changeRow(&p, c, id, path); err != nil {
			return Proposal{}, nil, err
		}
	}
	clean, err := validateProposal(p)
	if err != nil {
		return Proposal{}, nil, err
	}
	return clean, added, nil
}

// addRow appends a new row to a list; the action defaults to insert and possibleDuplicateOf to none.
func addRow(p *Proposal, list string, item any, path, id string) error {
	b, err := rawJSON(item, path+".item", "an object")
	if err != nil {
		return err
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(b, &probe); err != nil {
		return problem("%s.item must be an object", path)
	}
	if _, has := probe["id"]; has {
		return problem("%s.item: leave id out, the application gives the new row its id", path)
	}
	if _, has := probe["action"]; !has {
		probe["action"] = json.RawMessage(`"insert"`)
		if b, err = json.Marshal(probe); err != nil {
			return err
		}
	}
	switch list {
	case "skills":
		var s SkillProposal
		if err := decodeObject(b, path+".item", "the skill", &s); err != nil {
			return err
		}
		s.Id = id
		p.Skills = append(p.Skills, s)
	case "experience":
		var e ExperienceProposal
		if err := decodeObject(b, path+".item", "the job", &e); err != nil {
			return err
		}
		e.Id = id
		p.Experience = append(p.Experience, e)
	default:
		var e EducationProposal
		if err := decodeObject(b, path+".item", "the education entry", &e); err != nil {
			return err
		}
		e.Id = id
		p.Education = append(p.Education, e)
	}
	return nil
}

// changeRow updates or removes the row with this id.
func changeRow(p *Proposal, c SuggestionChange, id, path string) error {
	notFound := problem("%s: there is no row with id %q in %s: read the suggestions again with get_cv_import_suggestions", path, id, c.List)
	switch c.List {
	case "skills":
		for i := range p.Skills {
			if p.Skills[i].Id != id {
				continue
			}
			if c.Op == opRemove {
				p.Skills = append(p.Skills[:i], p.Skills[i+1:]...)
				return nil
			}
			next := p.Skills[i]
			if err := overlay(next, c.Fields, path, &next); err != nil {
				return err
			}
			p.Skills[i] = next
			return nil
		}
	case "experience":
		for i := range p.Experience {
			if p.Experience[i].Id != id {
				continue
			}
			if c.Op == opRemove {
				p.Experience = append(p.Experience[:i], p.Experience[i+1:]...)
				return nil
			}
			next := p.Experience[i]
			if err := overlay(next, c.Fields, path, &next); err != nil {
				return err
			}
			p.Experience[i] = next
			return nil
		}
	default:
		for i := range p.Education {
			if p.Education[i].Id != id {
				continue
			}
			if c.Op == opRemove {
				p.Education = append(p.Education[:i], p.Education[i+1:]...)
				return nil
			}
			next := p.Education[i]
			if err := overlay(next, c.Fields, path, &next); err != nil {
				return err
			}
			p.Education[i] = next
			return nil
		}
	}
	return notFound
}

// ChangeSuggestions applies the operations to a ready run's suggestions and stores the result (plan 257, step 2). basedOn, when given, is the revision the caller read: if the
// suggestions changed since, nothing is changed. Everything is checked before anything is stored.
func ChangeSuggestions(runID string, changes []SuggestionChange, basedOn string) (SuggestionsChanged, error) {
	run, p, stored, err := readyProposal(runID)
	if err != nil {
		return SuggestionsChanged{}, err
	}
	if basedOn = strings.TrimSpace(basedOn); basedOn != "" && basedOn != revisionOf(stored) {
		return SuggestionsChanged{}, errSuggestionsMoved
	}
	// The Import page shows a run that was saved from (or declined) as a read-only result: its suggestions are a record of what was saved, and a change made now could never be
	// reviewed or saved. A run is only open to changes while the person is still reviewing it (no save summary yet).
	if !noRawJSON(run.SaveSummary) {
		return SuggestionsChanged{}, errSuggestionsClosed
	}
	next, added, err := applyChanges(p, changes)
	if err != nil {
		return SuggestionsChanged{}, err
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return SuggestionsChanged{}, err
	}
	// stored only while the suggestions are still the ones the changes were made on: a change made meanwhile (another chat, the id assignment of another read) is never overwritten
	res, err := db.CareerDB.Exec(`UPDATE cv_import_runs SET ai_proposal_json = ?, updated_at = datetime('now') WHERE id = ? AND status = 'ready' AND ai_proposal_json = ?`, string(raw), runID, stored)
	if err != nil {
		return SuggestionsChanged{}, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return SuggestionsChanged{}, errSuggestionsMoved
	}
	return SuggestionsChanged{RunID: runID, Revision: revisionOf(string(raw)), Applied: len(changes), Added: added}, nil
}

type updateSuggestionsArgs struct {
	RunID   string             `json:"runId" jsonschema:"the id of the CV import run (a UUID), from the first message of the import chat"`
	Changes []SuggestionChange `json:"changes" jsonschema:"the operations to apply, all or nothing: update (a section, or a row by id), add (a row) or remove (a row by id)"`
	BasedOn string             `json:"basedOn,omitempty" jsonschema:"the revision you read with get_cv_import_suggestions; when the suggestions changed since, nothing is changed"`
}

// RegisterUpdateCvImportSuggestions adds update_cv_import_suggestions.
func RegisterUpdateCvImportSuggestions(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "update_cv_import_suggestions",
		Description: "Change the suggestions of a CV import while talking to the user: update a field of a suggestion (for example fix a job's dates, or set action to skip to leave a row out), " +
			"add a suggestion or remove one. First read them with get_cv_import_suggestions (it gives every row's id and the revision) and pass that revision as basedOn. " +
			"All the operations of one call are applied together or not at all, and they must follow the same rules as the original suggestions (each row has an action, insert or skip, " +
			"limits, dates). Once the user has saved from the import (or declined it) the suggestions are closed and cannot be changed. This only edits the suggestions the user reviews on the Import page: it saves nothing to the profile, " +
			"and you must never say that something was saved; the user saves on the Import page. Afterwards tell the user in one or two sentences what you changed.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updateSuggestionsArgs) (*mcp.CallToolResult, any, error) {
		runID := strings.TrimSpace(args.RunID)
		if runID == "" {
			return db.ErrResult("runId is required: the id of the CV import run, a UUID"), nil, nil
		}
		out, err := ChangeSuggestions(runID, args.Changes, args.BasedOn)
		switch {
		case errors.Is(err, errUnknownCVImportRun):
			return db.ErrResult(fmt.Sprintf("unknown CV import run id %q", runID)), nil, nil
		case err != nil:
			return db.ErrResult(err.Error()), nil, nil
		}
		return db.JSONResult(out)
	})
}
