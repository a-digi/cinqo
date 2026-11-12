// cv_import_suggestions.go — the AI reads (and, in the next step, changes) the suggestions of a CV import run after it handed them over (plan 257). The suggestions are the
// proposal stored on the run; the run's UUID is their handle. Every list item has an id of its own so a tool can name a row; the review page and the tools read the same ids.
// Nothing here inserts anything into the profile: that stays the person's click on the review page.
package cv

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
)

var (
	errSuggestionsStillGenerating = errors.New("this CV import is still being analysed: its suggestions are not ready yet")
	errNoSuggestions              = errors.New("this CV import has no suggestions")
)

// itemID is what an id looks like: the ids the application makes are UUIDs; an id that was sent along is kept when it is this shape and unique.
var itemID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// assignIDs gives every list item that has no usable id a new one: an item keeps the id it has when it is well formed and no other item of the proposal has it. The lists
// are changed in place (callers pass copies).
func assignIDs(p *Proposal) {
	seen := map[string]bool{}
	next := func(current string) string {
		if itemID.MatchString(current) && !seen[current] {
			seen[current] = true
			return current
		}
		id := uuid.NewString()
		seen[id] = true
		return id
	}
	for i := range p.Skills {
		p.Skills[i].Id = next(p.Skills[i].Id)
	}
	for i := range p.Experience {
		p.Experience[i].Id = next(p.Experience[i].Id)
	}
	for i := range p.Education {
		p.Education[i].Id = next(p.Education[i].Id)
	}
}

// hasAllIDs says whether every list item of the proposal already has a usable, unique id.
func hasAllIDs(p Proposal) bool {
	seen := map[string]bool{}
	ok := func(id string) bool {
		if !itemID.MatchString(id) || seen[id] {
			return false
		}
		seen[id] = true
		return true
	}
	for _, s := range p.Skills {
		if !ok(s.Id) {
			return false
		}
	}
	for _, e := range p.Experience {
		if !ok(e.Id) {
			return false
		}
	}
	for _, e := range p.Education {
		if !ok(e.Id) {
			return false
		}
	}
	return true
}

// revisionOf is a short fingerprint of the stored proposal: it changes whenever the suggestions change, so a caller can say which version it read.
func revisionOf(storedProposal string) string {
	sum := sha256.Sum256([]byte(storedProposal))
	return hex.EncodeToString(sum[:])[:12]
}

// Suggestion states: what happened to a row on the review page.
const (
	statePending = "pending" // not saved to the profile yet
	stateSaved   = "saved"   // saved to the profile
	stateFailed  = "failed"  // the person tried to save it and it failed
)

// itemState is the part of a row the review page adds: whether it was saved, and why not when it failed.
type itemState struct {
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

type profileSuggestion struct {
	ProfileProposal
	itemState
}
type personaSuggestion struct {
	PersonaProposal
	itemState
}
type detailsSuggestion struct {
	DetailsProposal
	itemState
}
type skillSuggestion struct {
	SkillProposal
	itemState
}
type experienceSuggestion struct {
	ExperienceProposal
	itemState
}
type educationSuggestion struct {
	EducationProposal
	itemState
}

// Suggestions is what get_cv_import_suggestions answers: the run, its revision, and every suggestion with its id and its state.
type Suggestions struct {
	RunID            string `json:"runId"`
	Status           string `json:"status"`
	OriginalFilename string `json:"originalFilename"`
	// Revision names the version of the suggestions that was read; pass it as basedOn when changing them (plan 257, step 2).
	Revision        string                 `json:"revision"`
	Profile         profileSuggestion      `json:"profile"`
	Persona         personaSuggestion      `json:"persona"`
	PersonalDetails detailsSuggestion      `json:"personalDetails"`
	Skills          []skillSuggestion      `json:"skills"`
	Experience      []experienceSuggestion `json:"experience"`
	Education       []educationSuggestion  `json:"education"`
}

// summaryItem is one entry of the review page's save summary: older summaries name a row by its index only, newer ones by its id too.
type summaryItem struct {
	ID    string `json:"id"`
	Index int    `json:"index"`
	Ok    bool   `json:"ok"`
	Error string `json:"error"`
}

type saveSummary struct {
	Profile         *summaryItem  `json:"profile"`
	Persona         *summaryItem  `json:"persona"`
	PersonalDetails *summaryItem  `json:"personalDetails"`
	Skills          []summaryItem `json:"skills"`
	Experience      []summaryItem `json:"experience"`
	Education       []summaryItem `json:"education"`
}

func stateOf(it *summaryItem) itemState {
	switch {
	case it == nil:
		return itemState{State: statePending}
	case it.Ok:
		return itemState{State: stateSaved}
	default:
		return itemState{State: stateFailed, Error: it.Error}
	}
}

// stateOfRow finds the row's entry in a list of summary items: by its id when the summary has ids, else by its index (an older summary).
func stateOfRow(items []summaryItem, id string, index int) itemState {
	for i := range items {
		if items[i].ID != "" && items[i].ID == id {
			return stateOf(&items[i])
		}
	}
	for i := range items {
		if items[i].ID == "" && items[i].Index == index {
			return stateOf(&items[i])
		}
	}
	return itemState{State: statePending}
}

// readyProposal reads the proposal of a ready run, giving every item an id (and storing them, once) when the proposal comes from before ids existed. It returns the stored
// proposal text the ids are in, so the revision is of what is stored.
func readyProposal(runID string) (cvImportRun, Proposal, string, error) {
	for attempt := 0; attempt < 3; attempt++ {
		run, err := getCVImportRun(runID)
		if err != nil {
			return cvImportRun{}, Proposal{}, "", err
		}
		switch run.Status {
		case runGenerating:
			return run, Proposal{}, "", errSuggestionsStillGenerating
		case runFailed:
			reason := strings.TrimSpace(run.Error)
			if reason == "" {
				reason = "the analysis failed"
			}
			return run, Proposal{}, "", fmt.Errorf("this CV import failed (%s): it has no suggestions", reason)
		}
		stored := strings.TrimSpace(string(run.AIProposal))
		if stored == "" || stored == noProposalJSON {
			return run, Proposal{}, "", errNoSuggestions
		}
		var p Proposal
		if err := json.Unmarshal([]byte(stored), &p); err != nil {
			return run, Proposal{}, "", errNoSuggestions
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
		if hasAllIDs(p) {
			return run, p, stored, nil
		}
		assignIDs(&p)
		raw, err := json.Marshal(p)
		if err != nil {
			return run, Proposal{}, "", err
		}
		// stored only while the proposal is still the one that was read, so a change made meanwhile is never overwritten; then read again
		res, err := db.CareerDB.Exec(`UPDATE cv_import_runs SET ai_proposal_json = ?, updated_at = datetime('now') WHERE id = ? AND status = 'ready' AND ai_proposal_json = ?`, string(raw), runID, string(run.AIProposal))
		if err != nil {
			return run, Proposal{}, "", err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			run.AIProposal = raw
			return run, p, string(raw), nil
		}
	}
	return cvImportRun{}, Proposal{}, "", errors.New("the suggestions keep changing: try again")
}

// ensureSuggestionIDs gives every list row of a run's suggestions an id of its own, once, when the run is one the person can still review (ready, nothing saved from it yet) and
// its proposal was stored before ids existed (plan 268): the review page needs a row's id for its Re-suggest button and for the local driver. It is the same guarded write the first
// read by a tool makes (readyProposal) and changes nothing but the ids. A run that is generating, failed, closed or already complete is left alone; nothing is reported.
func ensureSuggestionIDs(runID string) {
	run, err := getCVImportRun(runID)
	if err != nil || run.Status != runReady || !noRawJSON(run.SaveSummary) {
		return
	}
	_, _, _, _ = readyProposal(runID)
}

// GetSuggestions is the suggestions of a run as the review page shows them (plan 257). An unknown run is errUnknownCVImportRun; a run that is still being analysed, has
// failed or has no proposal says so.
func GetSuggestions(runID string) (Suggestions, error) {
	run, p, stored, err := readyProposal(runID)
	if err != nil {
		return Suggestions{}, err
	}
	var sum saveSummary
	if len(run.SaveSummary) > 0 {
		// a summary that cannot be read is treated as "nothing saved yet": it only decides the states
		_ = json.Unmarshal(run.SaveSummary, &sum)
	}
	out := Suggestions{
		RunID: run.ID, Status: run.Status, OriginalFilename: run.OriginalFilename, Revision: revisionOf(stored),
		Profile:         profileSuggestion{p.Profile, stateOf(sum.Profile)},
		Persona:         personaSuggestion{p.Persona, stateOf(sum.Persona)},
		PersonalDetails: detailsSuggestion{p.PersonalDetails, stateOf(sum.PersonalDetails)},
		Skills:          make([]skillSuggestion, 0, len(p.Skills)),
		Experience:      make([]experienceSuggestion, 0, len(p.Experience)),
		Education:       make([]educationSuggestion, 0, len(p.Education)),
	}
	for i, s := range p.Skills {
		out.Skills = append(out.Skills, skillSuggestion{s, stateOfRow(sum.Skills, s.Id, i)})
	}
	for i, e := range p.Experience {
		out.Experience = append(out.Experience, experienceSuggestion{e, stateOfRow(sum.Experience, e.Id, i)})
	}
	for i, e := range p.Education {
		out.Education = append(out.Education, educationSuggestion{e, stateOfRow(sum.Education, e.Id, i)})
	}
	return out, nil
}

type getSuggestionsArgs struct {
	RunID   string   `json:"runId" jsonschema:"the id of the CV import run (a UUID): it is in the first message of the import chat, after the word runId"`
	Section string   `json:"section,omitempty" jsonschema:"optional: read only this part: profile, persona, personalDetails, skills, experience or education (a chat about one section can only read that one)"`
	IDs     []string `json:"ids,omitempty" jsonschema:"optional, with section skills, experience or education: read only the rows with these ids"`
}

// RegisterGetCvImportSuggestions adds get_cv_import_suggestions.
func RegisterGetCvImportSuggestions(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_cv_import_suggestions",
		Description: "Read the suggestions of a CV import, the ones the user reviews on the Import page: the profile, the persona, the personal details, the skills, the jobs and the education, " +
			"each with its action (insert or skip), its fields, its own id (for the tool that changes them) and its state: pending (not saved to the profile yet), saved or failed. " +
			"Use it when the user wants to talk about the suggestions of an import. Pass the run's id (a UUID: it is in the first message of the import chat, after the word runId). " +
			"The answer also holds a revision. Give a section (and, for a list, row ids) to read only that part: shorter, and the only thing a chat about one section can read. It only reads; nothing is saved to the profile by it. Fails when the run is unknown, still being analysed, failed or has no suggestions.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getSuggestionsArgs) (*mcp.CallToolResult, any, error) {
		runID := strings.TrimSpace(args.RunID)
		if runID == "" {
			return db.ErrResult("runId is required: the id of the CV import run, a UUID"), nil, nil
		}
		s, err := GetSuggestionsIn(callerConversation(), runID, args.Section, args.IDs)
		switch {
		case errors.Is(err, errUnknownCVImportRun):
			return db.ErrResult(fmt.Sprintf("unknown CV import run id %q", runID)), nil, nil
		case errors.Is(err, sql.ErrNoRows):
			return db.ErrResult(fmt.Sprintf("unknown CV import run id %q", runID)), nil, nil
		case err != nil:
			return db.ErrResult(err.Error()), nil, nil
		}
		return db.JSONResult(s)
	})
}
