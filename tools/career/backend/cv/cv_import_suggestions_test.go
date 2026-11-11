package cv

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
)

// Reading the suggestions of a CV import run (plan 257, step 1): ids for every row, the view with the state of each row, and the tool.

// oldProposalJSON is a proposal stored before ids existed.
const oldProposalJSON = `{"profile":{"action":"insert","possibleDuplicateOf":null,"name":"Ada Lovelace"},"persona":{"action":"insert","possibleDuplicateOf":null,"name":"Backend","title":"Engineer"},` +
	`"personalDetails":{"action":"insert","possibleDuplicateOf":null,"headline":"H","summary":"S","location":"L","desiredTitles":"","desiredLocations":"","minSalary":0},` +
	`"skills":[{"action":"insert","possibleDuplicateOf":null,"value":"Go"},{"action":"skip","possibleDuplicateOf":"Docker","value":"Docker"},{"action":"insert","possibleDuplicateOf":null,"value":"SQL"}],` +
	`"experience":[{"action":"insert","possibleDuplicateOf":null,"company":"Acme","title":"Lead","startDate":"2019","endDate":"2023","description":"a\nb"}],` +
	`"education":[{"action":"insert","possibleDuplicateOf":null,"institution":"TU","degree":"MSc","fieldOfStudy":"CS","startDate":"2010","endDate":"2012","description":""}]}`

func readyRun(t *testing.T, proposal string) string {
	t.Helper()
	id, err := createCVImportRun("media-1", "cv.pdf", "conv-1", []byte(proposal))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func setSummary(t *testing.T, runID, summary string) {
	t.Helper()
	if err := updateCVImportRunSaveSummary(runID, []byte(summary)); err != nil {
		t.Fatal(err)
	}
}

func TestAssignIDsGivesEveryListItemItsOwnIdAndKeepsGoodOnes(t *testing.T) {
	p := Proposal{
		Skills:     []SkillProposal{{Value: "a"}, {Id: "keep-1", Value: "b"}, {Id: "keep-1", Value: "dup"}, {Id: "bad id", Value: "c"}, {Id: strings.Repeat("x", 65), Value: "long"}, {Id: "a/b", Value: "slash"}},
		Experience: []ExperienceProposal{{Company: "x"}, {Id: "exp_2", Company: "y"}},
		Education:  []EducationProposal{{Institution: "z"}, {Id: "keep-1", Institution: "clash with a skill"}},
	}
	assignIDs(&p)
	seen := map[string]bool{}
	all := []string{}
	for _, s := range p.Skills {
		all = append(all, s.Id)
	}
	for _, e := range p.Experience {
		all = append(all, e.Id)
	}
	for _, e := range p.Education {
		all = append(all, e.Id)
	}
	for _, id := range all {
		if !itemID.MatchString(id) || seen[id] {
			t.Errorf("id %q is not usable or not unique", id)
		}
		seen[id] = true
	}
	if p.Skills[1].Id != "keep-1" || p.Experience[1].Id != "exp_2" {
		t.Errorf("a good, unique id must be kept: %q %q", p.Skills[1].Id, p.Experience[1].Id)
	}
	if p.Skills[2].Id == "keep-1" || p.Education[1].Id == "keep-1" {
		t.Error("a second item with the same id must get a new one")
	}
	if len(p.Skills[0].Id) != 36 {
		t.Errorf("a new id is a UUID: %q", p.Skills[0].Id)
	}
}

func TestAssignIDsLeavesAnEmptyProposalAlone(t *testing.T) {
	p := Proposal{}
	assignIDs(&p)
	if len(p.Skills)+len(p.Experience)+len(p.Education) != 0 || !hasAllIDs(p) {
		t.Errorf("%+v", p)
	}
}

func TestHasAllIDs(t *testing.T) {
	if hasAllIDs(Proposal{Skills: []SkillProposal{{Id: "a"}, {Id: ""}}}) || hasAllIDs(Proposal{Skills: []SkillProposal{{Id: "a"}, {Id: "a"}}}) || hasAllIDs(Proposal{Education: []EducationProposal{{Id: "bad id"}}}) {
		t.Error("a missing, repeated or malformed id is not enough")
	}
	if !hasAllIDs(Proposal{Skills: []SkillProposal{{Id: "a"}}, Experience: []ExperienceProposal{{Id: "b"}}, Education: []EducationProposal{{Id: "c"}}}) {
		t.Error("ids that are well formed and unique are enough")
	}
}

func TestAStoredProposalHasIDsAndTheCallersListsAreNotChanged(t *testing.T) {
	setup(t)
	p := okProposal()
	run := newRun(t)
	if err := StoreProposal(run, p); err != nil {
		t.Fatal(err)
	}
	if p.Skills[0].Id != "" || p.Experience[0].Id != "" {
		t.Error("validateProposal must not write ids into the caller's lists")
	}
	var stored Proposal
	if err := json.Unmarshal(runOf(t, run).AIProposal, &stored); err != nil {
		t.Fatal(err)
	}
	if !hasAllIDs(stored) || len(stored.Skills) != 2 || stored.Skills[0].Id == stored.Skills[1].Id {
		t.Errorf("stored: %+v", stored)
	}
	// storing the same proposal again keeps the ids it carries
	again := stored
	run2 := newRun(t)
	if err := StoreProposal(run2, again); err != nil {
		t.Fatal(err)
	}
	var stored2 Proposal
	_ = json.Unmarshal(runOf(t, run2).AIProposal, &stored2)
	if stored2.Skills[0].Id != stored.Skills[0].Id {
		t.Error("an id that is sent along is kept")
	}
}

func TestGetSuggestionsGivesEveryRowItsIdAndAStateAndARevision(t *testing.T) {
	setup(t)
	run := readyRun(t, oldProposalJSON)
	s, err := GetSuggestions(run)
	if err != nil {
		t.Fatal(err)
	}
	if s.RunID != run || s.Status != "ready" || s.OriginalFilename != "cv.pdf" || len(s.Revision) != 12 {
		t.Errorf("header: %+v", s)
	}
	if s.Profile.Name != "Ada Lovelace" || s.Persona.Title != "Engineer" || s.PersonalDetails.Headline != "H" || s.Profile.State != "pending" || s.Persona.State != "pending" || s.PersonalDetails.State != "pending" {
		t.Errorf("sections: %+v %+v %+v", s.Profile, s.Persona, s.PersonalDetails)
	}
	if len(s.Skills) != 3 || s.Skills[0].Value != "Go" || s.Skills[1].Action != "skip" || *s.Skills[1].PossibleDuplicateOf != "Docker" || len(s.Experience) != 1 || s.Experience[0].Description != "a\nb" || len(s.Education) != 1 || s.Education[0].Institution != "TU" {
		t.Errorf("lists: %+v", s)
	}
	ids := map[string]bool{}
	for _, sk := range s.Skills {
		ids[sk.Id] = true
		if sk.State != "pending" {
			t.Errorf("state %q", sk.State)
		}
	}
	ids[s.Experience[0].Id], ids[s.Education[0].Id] = true, true
	if len(ids) != 5 || ids[""] {
		t.Errorf("ids: %v", ids)
	}
}

func TestAnOldProposalGetsItsIDsOnceAndNothingElseChanges(t *testing.T) {
	setup(t)
	run := readyRun(t, oldProposalJSON)
	first, err := GetSuggestions(run)
	if err != nil {
		t.Fatal(err)
	}
	stored := string(runOf(t, run).AIProposal)
	if !strings.Contains(stored, first.Skills[0].Id) {
		t.Fatal("the ids were not stored")
	}
	if !sameContent(t, stored, oldProposalJSON) {
		t.Errorf("the content changed:\n%s", stored)
	}
	second, err := GetSuggestions(run)
	if err != nil || second.Skills[0].Id != first.Skills[0].Id || second.Experience[0].Id != first.Experience[0].Id || second.Revision != first.Revision {
		t.Errorf("the second read must give the same ids and revision: %v %+v %+v", err, first.Revision, second.Revision)
	}
	if string(runOf(t, run).AIProposal) != stored {
		t.Error("reading again must not rewrite the proposal")
	}
}

// sameContent compares two proposals without their ids (as data, not as text).
func sameContent(t *testing.T, a, b string) bool {
	t.Helper()
	strip := func(s string) string {
		var p Proposal
		if err := json.Unmarshal([]byte(s), &p); err != nil {
			t.Fatal(err)
		}
		for i := range p.Skills {
			p.Skills[i].Id = ""
		}
		for i := range p.Experience {
			p.Experience[i].Id = ""
		}
		for i := range p.Education {
			p.Education[i].Id = ""
		}
		out, _ := json.Marshal(p)
		return string(out)
	}
	return strip(a) == strip(b)
}

func TestOnlyTheRowsWithoutAnIdGetOne(t *testing.T) {
	setup(t)
	run := readyRun(t, strings.Replace(oldProposalJSON, `"skills":[{"action":"insert"`, `"skills":[{"id":"mine-1","action":"insert"`, 1))
	s, err := GetSuggestions(run)
	if err != nil || s.Skills[0].Id != "mine-1" || s.Skills[1].Id == "" || s.Skills[1].Id == "mine-1" {
		t.Errorf("%v %+v", err, s.Skills)
	}
}

func TestTheRevisionChangesWhenTheSuggestionsChangeAndOnlyThen(t *testing.T) {
	setup(t)
	run := readyRun(t, oldProposalJSON)
	a, _ := GetSuggestions(run)
	b, _ := GetSuggestions(run)
	if a.Revision != b.Revision {
		t.Error("reading twice gave two revisions")
	}
	var p Proposal
	_ = json.Unmarshal(runOf(t, run).AIProposal, &p)
	p.Skills[0].Value = "Golang"
	raw, _ := json.Marshal(p)
	if _, err := db.CareerDB.Exec(`UPDATE cv_import_runs SET ai_proposal_json = ? WHERE id = ?`, string(raw), run); err != nil {
		t.Fatal(err)
	}
	c, _ := GetSuggestions(run)
	if c.Revision == a.Revision || c.Skills[0].Value != "Golang" {
		t.Errorf("revision %s -> %s, value %q", a.Revision, c.Revision, c.Skills[0].Value)
	}
	// a change of the save summary is not a change of the suggestions
	setSummary(t, run, `{"skills":[{"index":0,"ok":true,"value":"Golang"}]}`)
	d, _ := GetSuggestions(run)
	if d.Revision != c.Revision {
		t.Error("the save summary must not change the revision")
	}
}

func TestStatesComeFromTheSaveSummaryByIdOrByIndex(t *testing.T) {
	setup(t)
	run := readyRun(t, oldProposalJSON)
	first, _ := GetSuggestions(run)
	// an older summary: rows named by index only
	setSummary(t, run, `{"profile":{"ok":true},"persona":{"ok":false,"error":"name taken"},"skills":[{"index":0,"ok":true,"value":"Go"},{"index":2,"ok":false,"error":"too long","value":"SQL"}],"experience":[{"index":0,"ok":true,"label":"x"}],"education":[]}`)
	s, err := GetSuggestions(run)
	if err != nil {
		t.Fatal(err)
	}
	if s.Profile.State != "saved" || s.Persona.State != "failed" || s.Persona.Error != "name taken" || s.PersonalDetails.State != "pending" {
		t.Errorf("sections: %+v %+v %+v", s.Profile.itemState, s.Persona.itemState, s.PersonalDetails.itemState)
	}
	if s.Skills[0].State != "saved" || s.Skills[1].State != "pending" || s.Skills[2].State != "failed" || s.Skills[2].Error != "too long" || s.Experience[0].State != "saved" || s.Education[0].State != "pending" {
		t.Errorf("rows: %+v", s)
	}
	// a newer summary: rows named by id; the id wins over a stale index
	setSummary(t, run, `{"skills":[{"id":"`+first.Skills[1].Id+`","index":0,"ok":true,"value":"Docker"}]}`)
	s, _ = GetSuggestions(run)
	if s.Skills[0].State != "pending" || s.Skills[1].State != "saved" || s.Skills[2].State != "pending" {
		t.Errorf("by id: %+v", s.Skills)
	}
}

func TestAnUnreadableSaveSummaryMeansNothingSavedYet(t *testing.T) {
	setup(t)
	run := readyRun(t, oldProposalJSON)
	setSummary(t, run, `{not json`)
	s, err := GetSuggestions(run)
	if err != nil || s.Skills[0].State != "pending" || s.Profile.State != "pending" {
		t.Errorf("%v %+v", err, s.Skills)
	}
}

func TestGetSuggestionsSaysWhyARunHasNone(t *testing.T) {
	setup(t)
	if _, err := GetSuggestions("nope"); !errors.Is(err, errUnknownCVImportRun) {
		t.Errorf("unknown: %v", err)
	}
	gen := newRun(t)
	if _, err := GetSuggestions(gen); !errors.Is(err, errSuggestionsStillGenerating) {
		t.Errorf("generating: %v", err)
	}
	failed := newRun(t)
	if _, err := finishCVImportRun(failed, runFailed, "the AI did not read the CV"); err != nil {
		t.Fatal(err)
	}
	if _, err := GetSuggestions(failed); err == nil || !strings.Contains(err.Error(), "the AI did not read the CV") {
		t.Errorf("failed: %v", err)
	}
	if _, err := db.CareerDB.Exec(`UPDATE cv_import_runs SET error = NULL WHERE id = ?`, failed); err != nil {
		t.Fatal(err)
	}
	if _, err := GetSuggestions(failed); err == nil || !strings.Contains(err.Error(), "the analysis failed") {
		t.Errorf("failed without a reason: %v", err)
	}
	for name, stored := range map[string]string{"null": "null", "empty": "", "not json": "{nope", "an array": "[1]"} {
		run := readyRun(t, stored)
		if _, err := GetSuggestions(run); !errors.Is(err, errNoSuggestions) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestTwoReadsAtOnceOfAnOldProposalAgreeOnTheIDs(t *testing.T) {
	setup(t)
	run := readyRun(t, oldProposalJSON)
	var wg sync.WaitGroup
	got := make([]Suggestions, 2)
	errs := make([]error, 2)
	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got[i], errs[i] = GetSuggestions(run)
		}()
	}
	wg.Wait()
	if errs[0] != nil || errs[1] != nil || got[0].Skills[0].Id != got[1].Skills[0].Id || got[0].Revision != got[1].Revision {
		t.Errorf("%v %v %+v %+v", errs[0], errs[1], got[0].Revision, got[1].Revision)
	}
}

func TestTheViewIsFlatJSONWithTheStateBesideTheFields(t *testing.T) {
	setup(t)
	run := readyRun(t, oldProposalJSON)
	s, _ := GetSuggestions(run)
	raw, _ := json.Marshal(s)
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	skill := generic["skills"].([]any)[0].(map[string]any)
	for _, key := range []string{"id", "action", "possibleDuplicateOf", "value", "state"} {
		if _, ok := skill[key]; !ok {
			t.Errorf("a skill must have %q: %v", key, skill)
		}
	}
	if _, nested := skill["SkillProposal"]; nested {
		t.Error("the fields must not be nested")
	}
	if generic["runId"] != run || generic["revision"] == "" || generic["profile"].(map[string]any)["state"] != "pending" {
		t.Errorf("%v", generic)
	}
	if _, has := skill["error"]; has {
		t.Error("a row without an error has no error key")
	}
}

func TestGetCvImportSuggestionsTool(t *testing.T) {
	setup(t)
	run := readyRun(t, oldProposalJSON)
	gen := newRun(t)
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1"}, nil)
	RegisterGetCvImportSuggestions(server)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	call := func(args map[string]any) (string, bool) {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "get_cv_import_suggestions", Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		text := ""
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				text += tc.Text
			}
		}
		return text, res.IsError
	}
	if text, isErr := call(map[string]any{"runId": "  "}); !isErr || !strings.Contains(text, "runId is required") {
		t.Errorf("no id: %q %v", text, isErr)
	}
	if text, isErr := call(map[string]any{"runId": "nope"}); !isErr || !strings.Contains(text, `unknown CV import run id "nope"`) {
		t.Errorf("unknown: %q %v", text, isErr)
	}
	if text, isErr := call(map[string]any{"runId": gen}); !isErr || !strings.Contains(text, "still being analysed") {
		t.Errorf("generating: %q %v", text, isErr)
	}
	text, isErr := call(map[string]any{"runId": " " + run + " "})
	if isErr {
		t.Fatalf("a good call: %q", text)
	}
	var got Suggestions
	if err := json.Unmarshal([]byte(text), &got); err != nil || got.RunID != run || len(got.Skills) != 3 || got.Skills[0].Id == "" || got.Skills[0].State != "pending" || got.Revision == "" {
		t.Errorf("answer: %v %+v", err, got)
	}
	tools, err := cs.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 1 || !strings.Contains(tools.Tools[0].Description, "only reads") {
		t.Errorf("the description must say it only reads: %v", err)
	}
}
