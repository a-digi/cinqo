package cv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Changing the suggestions of a ready CV import run (plan 257, step 2).

type sugg struct {
	t   *testing.T
	run string
}

func newSugg(t *testing.T) sugg {
	setup(t)
	return sugg{t: t, run: readyRun(t, oldProposalJSON)}
}

func (s sugg) get() Suggestions {
	s.t.Helper()
	g, err := GetSuggestions(s.run)
	if err != nil {
		s.t.Fatal(err)
	}
	return g
}

func (s sugg) change(basedOn string, changes ...SuggestionChange) (SuggestionsChanged, error) {
	return ChangeSuggestions(s.run, changes, basedOn)
}

func upd(list, id string, fields any) SuggestionChange {
	return SuggestionChange{Op: "update", List: list, ID: id, Fields: fields}
}
func updSection(section string, fields any) SuggestionChange {
	return SuggestionChange{Op: "update", Section: section, Fields: fields}
}
func add(list string, item any) SuggestionChange {
	return SuggestionChange{Op: "add", List: list, Item: item}
}
func rem(list, id string) SuggestionChange { return SuggestionChange{Op: "remove", List: list, ID: id} }

func wantProblem(t *testing.T, err error, contains string) {
	t.Helper()
	var bad *proposalProblem
	if !errors.As(err, &bad) || !strings.Contains(bad.msg, contains) {
		t.Errorf("error %v, want a problem mentioning %q", err, contains)
	}
}

func TestUpdateASectionChangesOnlyTheFieldsNamed(t *testing.T) {
	s := newSugg(t)
	before := s.get()
	out, err := s.change(before.Revision,
		updSection("profile", map[string]any{"name": "Ada King"}),
		updSection("persona", map[string]any{"title": "Staff Engineer", "action": "skip"}),
		updSection("personalDetails", map[string]any{"summary": "New summary", "minSalary": 90000}),
	)
	if err != nil || out.Applied != 3 || out.Revision == before.Revision {
		t.Fatalf("%+v %v", out, err)
	}
	after := s.get()
	if after.Profile.Name != "Ada King" || after.Profile.Action != "insert" || after.Persona.Title != "Staff Engineer" || after.Persona.Action != "skip" || after.Persona.Name != "Backend" ||
		after.PersonalDetails.Summary != "New summary" || after.PersonalDetails.MinSalary != 90000 || after.PersonalDetails.Headline != "H" {
		t.Errorf("sections: %+v %+v %+v", after.Profile, after.Persona, after.PersonalDetails)
	}
	if out.Revision != after.Revision || len(after.Skills) != 3 || after.Skills[0].Id != before.Skills[0].Id {
		t.Error("the rest must stay as it was, with the same ids, and the answer must carry the new revision")
	}
}

func TestUpdateARowKeepsItsIdAndItsPlace(t *testing.T) {
	s := newSugg(t)
	b := s.get()
	if _, err := s.change("", upd("skills", b.Skills[1].Id, map[string]any{"value": "Podman", "action": "insert", "possibleDuplicateOf": nil})); err != nil {
		t.Fatal(err)
	}
	a := s.get()
	if a.Skills[1].Id != b.Skills[1].Id || a.Skills[1].Value != "Podman" || a.Skills[1].Action != "insert" || a.Skills[1].PossibleDuplicateOf != nil || a.Skills[0].Value != "Go" || a.Skills[2].Value != "SQL" {
		t.Errorf("%+v", a.Skills)
	}
	if _, err := s.change("", upd("experience", b.Experience[0].Id, map[string]any{"title": "Principal", "endDate": "2024", "description": "x\ny\nz"}),
		upd("education", b.Education[0].Id, map[string]any{"degree": "PhD", "endDate": ""})); err != nil {
		t.Fatal(err)
	}
	a = s.get()
	if a.Experience[0].Title != "Principal" || a.Experience[0].EndDate != "2024" || a.Experience[0].Description != "x\ny\nz" || a.Experience[0].Company != "Acme" || a.Education[0].Degree != "PhD" || a.Education[0].EndDate != "" || a.Education[0].Institution != "TU" {
		t.Errorf("%+v %+v", a.Experience[0], a.Education[0])
	}
}

func TestFieldsAndItemsMayArriveAsJSONText(t *testing.T) {
	s := newSugg(t)
	b := s.get()
	out, err := s.change("", upd("skills", b.Skills[0].Id, `{"value":"Go 1.26"}`), add("skills", `{"value":"Rust"}`), updSection("profile", `{"name":"Via Text"}`))
	if err != nil || len(out.Added) != 1 {
		t.Fatalf("%+v %v", out, err)
	}
	a := s.get()
	if a.Skills[0].Value != "Go 1.26" || a.Skills[3].Value != "Rust" || a.Profile.Name != "Via Text" {
		t.Errorf("%+v %s", a.Skills, a.Profile.Name)
	}
}

func TestUpdateRefusesWhatIsNotAFieldOfTheRow(t *testing.T) {
	s := newSugg(t)
	b := s.get()
	for name, c := range map[string]SuggestionChange{
		"an unknown field":            upd("skills", b.Skills[0].Id, map[string]any{"company": "x"}),
		"a field of another section":  updSection("profile", map[string]any{"title": "x"}),
		"a value of the wrong kind":   updSection("personalDetails", map[string]any{"minSalary": "a lot"}),
		"the id":                      upd("skills", b.Skills[0].Id, map[string]any{"id": "other"}),
		"no fields":                   upd("skills", b.Skills[0].Id, map[string]any{}),
		"fields that are not objects": upd("skills", b.Skills[0].Id, "[1]"),
		"no fields at all":            upd("skills", b.Skills[0].Id, nil),
	} {
		if _, err := s.change("", c); err == nil {
			t.Errorf("%s: accepted", name)
		} else {
			var bad *proposalProblem
			if !errors.As(err, &bad) {
				t.Errorf("%s: %v is not a problem the AI can correct", name, err)
			}
		}
	}
	if s.get().Revision != b.Revision {
		t.Error("a refused call changed the suggestions")
	}
}

func TestTheRulesOfAProposalStillApplyToTheChangedResult(t *testing.T) {
	s := newSugg(t)
	b := s.get()
	_, err := s.change("", upd("skills", b.Skills[0].Id, map[string]any{"action": "maybe"}))
	wantProblem(t, err, "skills[0].action")
	_, err = s.change("", upd("experience", b.Experience[0].Id, map[string]any{"description": strings.Repeat("x", MaxProposalTextRunes+1)}))
	wantProblem(t, err, "experience[0].description")
	_, err = s.change("", upd("experience", b.Experience[0].Id, map[string]any{"bulletsInCv": 10}))
	wantProblem(t, err, "bullet points")
	_, err = s.change("", upd("education", b.Education[0].Id, map[string]any{"startDate": "not a date"}))
	wantProblem(t, err, "education[0]")
	_, err = s.change("", updSection("personalDetails", map[string]any{"minSalary": -1}))
	wantProblem(t, err, "minSalary")
	if s.get().Revision != b.Revision {
		t.Error("a refused call changed the suggestions")
	}
}

func TestAddAppendsARowWithANewIdAndAnInsertActionByDefault(t *testing.T) {
	s := newSugg(t)
	b := s.get()
	out, err := s.change(b.Revision,
		add("skills", map[string]any{"value": "Rust"}),
		add("skills", map[string]any{"value": "COBOL", "action": "skip"}),
		add("experience", map[string]any{"company": "Initech", "title": "Dev", "startDate": "2015", "endDate": "2018", "description": "a\nb"}),
		add("education", map[string]any{"institution": "MIT", "degree": "BSc", "startDate": "2005", "endDate": "2009"}),
	)
	if err != nil || len(out.Added) != 4 {
		t.Fatalf("%+v %v", out, err)
	}
	a := s.get()
	if len(a.Skills) != 5 || a.Skills[3].Value != "Rust" || a.Skills[3].Action != "insert" || a.Skills[4].Action != "skip" || a.Skills[3].Id != out.Added[0] || a.Skills[4].Id != out.Added[1] ||
		len(a.Experience) != 2 || a.Experience[1].Id != out.Added[2] || a.Experience[1].Company != "Initech" || len(a.Education) != 2 || a.Education[1].Id != out.Added[3] || a.Education[1].Institution != "MIT" {
		t.Errorf("%+v", a)
	}
	if a.Skills[3].State != "pending" || a.Skills[3].PossibleDuplicateOf != nil {
		t.Errorf("a new row is pending with no duplicate: %+v", a.Skills[3])
	}
	ids := map[string]bool{}
	for _, id := range out.Added {
		ids[id] = true
	}
	if len(ids) != 4 {
		t.Errorf("the new ids must differ: %v", out.Added)
	}
}

func TestAddRefusesABadRow(t *testing.T) {
	s := newSugg(t)
	b := s.get()
	for name, c := range map[string]SuggestionChange{
		"an unknown field":     add("skills", map[string]any{"value": "x", "level": 5}),
		"an id in the item":    add("skills", map[string]any{"value": "x", "id": "mine"}),
		"an id in the change":  {Op: "add", List: "skills", ID: "mine", Item: map[string]any{"value": "x"}},
		"no item":              add("skills", nil),
		"an item that is text": add("skills", "just text"),
		"an invalid action":    add("skills", map[string]any{"value": "x", "action": "maybe"}),
		"an invalid education": add("education", map[string]any{"institution": "", "degree": "", "fieldOfStudy": ""}),
	} {
		if _, err := s.change("", c); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// the list limits
	many := make([]SuggestionChange, 0, MaxProposalSkills)
	for i := 0; i < MaxProposalSkills; i++ {
		many = append(many, add("skills", map[string]any{"value": fmt.Sprintf("s%d", i)}))
	}
	if _, err := s.change("", many[:100]...); err != nil {
		t.Fatalf("a full call: %v", err)
	}
	if _, err := s.change("", many[:MaxProposalSkills-103]...); err != nil {
		t.Fatalf("up to the limit of the list: %v", err)
	}
	if n := len(s.get().Skills); n != MaxProposalSkills {
		t.Fatalf("%d skills, want %d", n, MaxProposalSkills)
	}
	_, err := s.change("", many[:1]...)
	wantProblem(t, err, "skills can have at most")
	if len(s.get().Skills) != MaxProposalSkills {
		t.Error("a call over the limit changed the suggestions")
	}
	_ = b
}

func TestRemoveTakesTheRowOutAndKeepsTheOthersInOrder(t *testing.T) {
	s := newSugg(t)
	b := s.get()
	if _, err := s.change(b.Revision, rem("skills", b.Skills[1].Id), rem("experience", b.Experience[0].Id), rem("education", b.Education[0].Id)); err != nil {
		t.Fatal(err)
	}
	a := s.get()
	if len(a.Skills) != 2 || a.Skills[0].Id != b.Skills[0].Id || a.Skills[1].Id != b.Skills[2].Id || len(a.Experience) != 0 || len(a.Education) != 0 {
		t.Errorf("%+v", a)
	}
	var stored Proposal
	_ = json.Unmarshal(runOf(t, s.run).AIProposal, &stored)
	if stored.Experience == nil || !strings.Contains(string(runOf(t, s.run).AIProposal), `"experience":[]`) {
		t.Error("an emptied list is [], never null")
	}
	// the same row twice, or a row that is gone
	_, err := s.change("", rem("skills", b.Skills[1].Id))
	wantProblem(t, err, "no row with id")
	_, err = s.change("", rem("skills", a.Skills[0].Id), upd("skills", a.Skills[0].Id, map[string]any{"value": "x"}))
	wantProblem(t, err, "no row with id")
}

func TestARowIsFoundInItsOwnListOnly(t *testing.T) {
	s := newSugg(t)
	b := s.get()
	_, err := s.change("", rem("experience", b.Skills[0].Id))
	wantProblem(t, err, "no row with id")
	_, err = s.change("", upd("education", b.Experience[0].Id, map[string]any{"degree": "x"}))
	wantProblem(t, err, "no row with id")
}

func TestTheShapeOfAnOperationIsChecked(t *testing.T) {
	s := newSugg(t)
	b := s.get()
	id := b.Skills[0].Id
	for name, c := range map[string]struct {
		changes []SuggestionChange
		want    string
	}{
		"no operations":            {nil, "changes is empty"},
		"a bad op":                 {[]SuggestionChange{{Op: "delete", List: "skills", ID: id}}, `changes[0].op must be`},
		"no op":                    {[]SuggestionChange{{List: "skills", ID: id}}, `changes[0].op must be`},
		"a section and a list":     {[]SuggestionChange{{Op: "update", Section: "profile", List: "skills", Fields: map[string]any{"name": "x"}}}, "not both"},
		"a section to remove":      {[]SuggestionChange{{Op: "remove", Section: "profile"}}, "can only be updated"},
		"a section to add":         {[]SuggestionChange{{Op: "add", Section: "profile", Item: map[string]any{"name": "x"}}}, "can only be updated"},
		"an unknown section":       {[]SuggestionChange{updSection("education", map[string]any{"a": 1})}, "section must be one of"},
		"neither section nor list": {[]SuggestionChange{{Op: "update", Fields: map[string]any{"a": 1}}}, "name a section"},
		"an unknown list":          {[]SuggestionChange{upd("hobbies", id, map[string]any{"a": 1})}, "name a section"},
		"no id":                    {[]SuggestionChange{{Op: "remove", List: "skills"}}, "id is required"},
		"a blank id":               {[]SuggestionChange{rem("skills", "   ")}, "id is required"},
	} {
		_, err := s.change("", c.changes...)
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		wantProblem(t, err, c.want)
	}
	too := make([]SuggestionChange, MaxSuggestionChanges+1)
	for i := range too {
		too[i] = add("skills", map[string]any{"value": fmt.Sprintf("s%d", i)})
	}
	_, err := s.change("", too...)
	wantProblem(t, err, "at most 100 operations")
	if s.get().Revision != b.Revision {
		t.Error("a refused call changed the suggestions")
	}
}

func TestACallIsAllOrNothing(t *testing.T) {
	s := newSugg(t)
	b := s.get()
	stored := string(runOf(t, s.run).AIProposal)
	_, err := s.change("", upd("skills", b.Skills[0].Id, map[string]any{"value": "changed"}), add("skills", map[string]any{"value": "added"}), rem("skills", "no-such-id"))
	if err == nil {
		t.Fatal("accepted")
	}
	if string(runOf(t, s.run).AIProposal) != stored {
		t.Error("the first operations were stored although the last one failed")
	}
	// and a call whose result is not valid as a whole stores nothing
	_, err = s.change("", upd("skills", b.Skills[0].Id, map[string]any{"value": "ok"}), upd("skills", b.Skills[1].Id, map[string]any{"action": "bad"}))
	if err == nil || string(runOf(t, s.run).AIProposal) != stored {
		t.Errorf("%v", err)
	}
}

func TestARunThatWasSavedFromOrDeclinedCannotBeChanged(t *testing.T) {
	s := newSugg(t)
	b := s.get()
	ops := []SuggestionChange{add("skills", map[string]any{"value": "late"}), upd("skills", b.Skills[0].Id, map[string]any{"value": "x"}), rem("skills", b.Skills[1].Id), updSection("profile", map[string]any{"name": "x"})}
	for name, summary := range map[string]string{
		"a partial save":    `{"skills":[{"index":0,"ok":true,"value":"Go"}]}`,
		"a save of nothing": `{"skills":[],"experience":[],"education":[]}`,
		"a failed save":     `{"persona":{"ok":false,"error":"name taken"},"skills":[],"experience":[],"education":[]}`,
		"a declined import": `{"declined":true,"skills":[],"experience":[],"education":[]}`,
	} {
		setSummary(t, s.run, summary)
		for _, op := range ops {
			if _, err := s.change("", op); !errors.Is(err, errSuggestionsClosed) {
				t.Errorf("%s, %s: %v", name, op.Op, err)
			}
		}
	}
	if s.get().Revision != b.Revision {
		t.Error("a refused call changed the suggestions")
	}
	// the suggestions can still be read, with the states of what was saved
	setSummary(t, s.run, `{"skills":[{"index":0,"ok":true,"value":"Go"}]}`)
	if g := s.get(); g.Skills[0].State != "saved" {
		t.Errorf("a saved run can still be read: %+v", g.Skills[0])
	}
	// a summary that is null or empty means the review has not started
	for _, empty := range []string{"null", " null "} {
		setSummary(t, s.run, empty)
		if _, err := s.change("", add("skills", map[string]any{"value": "ok " + empty})); err != nil {
			t.Errorf("a run with a %q summary is still being reviewed: %v", empty, err)
		}
	}
}

func TestBasedOnRefusesAChangeMadeOnAnOldVersion(t *testing.T) {
	s := newSugg(t)
	first := s.get()
	if _, err := s.change(first.Revision, add("skills", map[string]any{"value": "one"})); err != nil {
		t.Fatal(err)
	}
	_, err := s.change(first.Revision, add("skills", map[string]any{"value": "two"}))
	if !errors.Is(err, errSuggestionsMoved) || !strings.Contains(err.Error(), "get_cv_import_suggestions") {
		t.Errorf("an old revision: %v", err)
	}
	if got := s.get(); len(got.Skills) != 4 {
		t.Errorf("a refused call changed the suggestions: %d skills", len(got.Skills))
	}
	if _, err := s.change("  "+s.get().Revision+" ", add("skills", map[string]any{"value": "three"})); err != nil {
		t.Errorf("the current revision (with spaces): %v", err)
	}
	if _, err := s.change("", add("skills", map[string]any{"value": "four"})); err != nil {
		t.Errorf("no revision at all is allowed: %v", err)
	}
	if _, err := s.change("0123456789ab", add("skills", map[string]any{"value": "five"})); !errors.Is(err, errSuggestionsMoved) {
		t.Errorf("a revision that never existed: %v", err)
	}
}

func TestOnlyAReadyRunCanBeChanged(t *testing.T) {
	setup(t)
	if _, err := ChangeSuggestions("nope", []SuggestionChange{add("skills", map[string]any{"value": "x"})}, ""); !errors.Is(err, errUnknownCVImportRun) {
		t.Errorf("unknown: %v", err)
	}
	gen := newRun(t)
	if _, err := ChangeSuggestions(gen, []SuggestionChange{add("skills", map[string]any{"value": "x"})}, ""); !errors.Is(err, errSuggestionsStillGenerating) {
		t.Errorf("generating: %v", err)
	}
	failed := newRun(t)
	if _, err := finishCVImportRun(failed, runFailed, "boom"); err != nil {
		t.Fatal(err)
	}
	if _, err := ChangeSuggestions(failed, []SuggestionChange{add("skills", map[string]any{"value": "x"})}, ""); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("failed: %v", err)
	}
}

func TestTwoChangesAtOnceOnTheSameVersionOnlyOneWins(t *testing.T) {
	s := newSugg(t)
	rev := s.get().Revision
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = s.change(rev, add("skills", map[string]any{"value": fmt.Sprintf("racer %d", i)}))
		}()
	}
	wg.Wait()
	wins := 0
	for _, e := range errs {
		if e == nil {
			wins++
		} else if !errors.Is(e, errSuggestionsMoved) {
			t.Errorf("the loser got %v", e)
		}
	}
	if wins != 1 || len(s.get().Skills) != 4 {
		t.Errorf("%d won, %d skills (%v)", wins, len(s.get().Skills), errs)
	}
}

func TestAChangeLeavesTheRestOfTheRunAlone(t *testing.T) {
	s := newSugg(t)
	b := s.get()
	if _, err := s.change("", rem("skills", b.Skills[0].Id)); err != nil {
		t.Fatal(err)
	}
	r := runOf(t, s.run)
	if len(r.SaveSummary) != 0 || r.Status != "ready" || r.ConversationID != "conv-1" || r.MediaFileID != "media-1" || r.OriginalFilename != "cv.pdf" {
		t.Errorf("%+v", r)
	}
}

func TestUpdateCvImportSuggestionsTool(t *testing.T) {
	setup(t)
	run := readyRun(t, oldProposalJSON)
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1"}, nil)
	RegisterGetCvImportSuggestions(server)
	RegisterUpdateCvImportSuggestions(server)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	call := func(name string, args map[string]any) (string, bool) {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		text := ""
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				text += tc.Text
			}
		}
		return text, res.IsError
	}
	read := func() Suggestions {
		text, isErr := call("get_cv_import_suggestions", map[string]any{"runId": run})
		var g Suggestions
		if isErr || json.Unmarshal([]byte(text), &g) != nil {
			t.Fatalf("get: %q", text)
		}
		return g
	}
	g := read()
	if text, isErr := call("update_cv_import_suggestions", map[string]any{"runId": " ", "changes": []any{}}); !isErr || !strings.Contains(text, "runId is required") {
		t.Errorf("no id: %q", text)
	}
	if text, isErr := call("update_cv_import_suggestions", map[string]any{"runId": "nope", "changes": []map[string]any{{"op": "remove", "list": "skills", "id": "x"}}}); !isErr || !strings.Contains(text, `unknown CV import run id "nope"`) {
		t.Errorf("unknown: %q", text)
	}
	if text, isErr := call("update_cv_import_suggestions", map[string]any{"runId": run, "changes": []map[string]any{{"op": "remove", "list": "skills", "id": "x"}}}); !isErr || !strings.Contains(text, "no row with id") {
		t.Errorf("a bad id: %q", text)
	}
	text, isErr := call("update_cv_import_suggestions", map[string]any{
		"runId": run, "basedOn": g.Revision,
		"changes": []map[string]any{
			{"op": "update", "list": "skills", "id": g.Skills[0].Id, "fields": map[string]any{"value": "Golang"}},
			{"op": "add", "list": "skills", "item": map[string]any{"value": "Rust"}},
			{"op": "remove", "list": "skills", "id": g.Skills[2].Id},
			{"op": "update", "section": "persona", "fields": map[string]any{"title": "Lead"}},
		},
	})
	if isErr {
		t.Fatalf("a good call: %q", text)
	}
	var out SuggestionsChanged
	if err := json.Unmarshal([]byte(text), &out); err != nil || out.Applied != 4 || len(out.Added) != 1 || out.Revision == g.Revision {
		t.Errorf("answer: %v %+v", err, out)
	}
	after := read()
	if after.Revision != out.Revision || after.Skills[0].Value != "Golang" || len(after.Skills) != 3 || after.Skills[2].Value != "Rust" || after.Skills[2].Id != out.Added[0] || after.Persona.Title != "Lead" {
		t.Errorf("%+v", after)
	}
	if text, isErr := call("update_cv_import_suggestions", map[string]any{"runId": run, "basedOn": g.Revision, "changes": []map[string]any{{"op": "add", "list": "skills", "item": map[string]any{"value": "late"}}}}); !isErr || !strings.Contains(text, "changed since you read them") {
		t.Errorf("an old revision: %q", text)
	}
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range tools.Tools {
		if tl.Name != "update_cv_import_suggestions" {
			continue
		}
		if !strings.Contains(tl.Description, "saves nothing to the profile") || !strings.Contains(tl.Description, "never say that something was saved") || !strings.Contains(tl.Description, "cannot be changed") {
			t.Error("the description must say that it saves nothing to the profile")
		}
		schema, _ := json.Marshal(tl.InputSchema)
		var sc struct {
			Required []string `json:"required"`
		}
		_ = json.Unmarshal(schema, &sc)
		if !contains(sc.Required, "runId") || !contains(sc.Required, "changes") || contains(sc.Required, "basedOn") {
			t.Errorf("required: %v", sc.Required)
		}
	}
}

// ---- the save summary says which version of the suggestions it is for ----

func TestTheSaveSummaryIsRefusedForSuggestionsThatChanged(t *testing.T) {
	s := newSugg(t)
	b := s.get()
	rec := call(http.MethodGet, "/cv-import/runs", "")
	runs := decode(t, rec)["runs"].([]any)
	if rev := runs[0].(map[string]any)["revision"]; rev != b.Revision {
		t.Fatalf("the run list must carry the revision of the suggestions: %v vs %v", rev, b.Revision)
	}
	put := func(rev string) *httptest.ResponseRecorder {
		body := fmt.Sprintf(`{"id":%q,"saveSummary":{"skills":[{"index":0,"ok":true,"value":"Go"}]},"proposalRevision":%q}`, s.run, rev)
		return call(http.MethodPut, "/cv-import/runs", body)
	}
	// the AI changes the suggestions after the page read them and before the person saved
	if _, err := s.change(b.Revision, add("skills", map[string]any{"value": "moved"})); err != nil {
		t.Fatal(err)
	}
	if rec := put(b.Revision); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "reload") {
		t.Errorf("the old revision: %d %s", rec.Code, rec.Body.String())
	}
	if len(runOf(t, s.run).SaveSummary) != 0 {
		t.Error("a refused summary was recorded")
	}
	// the page reads them again and saves with the new revision; from then on the suggestions are closed
	if rec := put(s.get().Revision); rec.Code != http.StatusNoContent {
		t.Errorf("the new revision: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := s.change("", add("skills", map[string]any{"value": "too late"})); !errors.Is(err, errSuggestionsClosed) {
		t.Errorf("after the save: %v", err)
	}
	// a later summary without a revision is recorded as before
	if rec := call(http.MethodPut, "/cv-import/runs", fmt.Sprintf(`{"id":%q,"saveSummary":{"skills":[]}}`, s.run)); rec.Code != http.StatusNoContent {
		t.Errorf("no revision: %d", rec.Code)
	}
}

func TestARunWithoutAProposalHasNoRevision(t *testing.T) {
	setup(t)
	newRun(t)
	runs := decode(t, call(http.MethodGet, "/cv-import/runs", ""))["runs"].([]any)
	if _, has := runs[0].(map[string]any)["revision"]; has {
		t.Error("a generating run has no proposal and no revision")
	}
}
