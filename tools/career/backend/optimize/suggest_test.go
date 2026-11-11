package optimize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
	"career-tool-backend/hobbies"
	"career-tool-backend/profile/profiledetails"
	"career-tool-backend/profile/profileitems"
)

// world is a profile with one text of each kind and an open (generating) run for it.
type world struct {
	pid, run, expID, eduID, courseID, hobbyID string
}

func newWorld(t *testing.T) world {
	t.Helper()
	pid := setup(t)
	w := world{pid: pid}
	if _, err := profiledetails.Update(pid, profiledetails.Patch{Summary: ptr("I build things.")}); err != nil {
		t.Fatal(err)
	}
	w.expID = must(profileitems.AddExperience(pid, profileitems.ExperienceInput{Company: "Acme", Title: "Dev", StartDate: "2019", Description: "Intro\n- wrote Go\n- fixed bugs"})).ID
	w.hobbyID = must(hobbies.Add(pid, hobbies.Input{Name: "Chess", Description: "club player"})).ID
	w.run = must(CreateRun(SubjectProfile, pid, "conv", "", "make it short"))
	return w
}

func rowsOf(t *testing.T, runID string) []string {
	t.Helper()
	rows, err := db.CareerDB.Query(`SELECT position, kind, item_id, field, before_text, after_text, reason FROM optimize_suggestions WHERE run_id = ? ORDER BY position`, runID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var pos int
		var k, i, f, b, a, r string
		if err := rows.Scan(&pos, &k, &i, &f, &b, &a, &r); err != nil {
			t.Fatal(err)
		}
		out = append(out, fmt.Sprintf("%d|%s|%s|%s|%s|%s|%s", pos, k, i, f, b, a, r))
	}
	return out
}

func TestASuggestionIsStoredWithTheTextTheAIWasShown(t *testing.T) {
	w := newWorld(t)
	st, err := AddSuggestion(w.run, KindProfileDetails, "", FieldSummary, "  I build reliable systems.  ", "  clearer  ")
	if err != nil || st.Suggested != 1 || st.Replaced || st.SuggestionID == "" {
		t.Fatalf("%+v %v", st, err)
	}
	st2, err := AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, "plays in a chess club", "")
	if err != nil || st2.Suggested != 2 {
		t.Fatalf("%+v %v", st2, err)
	}
	got := rowsOf(t, w.run)
	want := []string{
		"0|profileDetails||summary|I build things.|I build reliable systems.|clearer",
		"1|hobby|" + w.hobbyID + "|description|club player|plays in a chess club|",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("rows:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// Nothing was written to the profile.
	d, _ := profiledetails.Get(w.pid)
	if d.Summary != "I build things." {
		t.Errorf("a suggestion changed the profile: %q", d.Summary)
	}
}

func TestASuggestionIsRefusedWithASentenceTheAICanActOn(t *testing.T) {
	w := newWorld(t)
	otherKind := must(CreateRun(SubjectPersona, newPersona(t, w.pid), "c", "", ""))
	cases := []struct {
		name                         string
		run, kind, item, field, text string
		want                         string
	}{
		{"kind and field that do not go together", w.run, KindHobby, w.hobbyID, FieldTitle, "x", "does not exist"},
		{"unknown kind", w.run, "skill", "x", FieldSummary, "x", "does not exist"},
		{"an item that is not there", w.run, KindHobby, "nope", FieldDescription, "better", "no hobby description"},
		{"a kind the subject does not have", w.run, KindPersonaDetails, "", FieldSummary, "better", "no personaDetails summary"},
		{"a profile's item on a persona's run", otherKind, KindHobby, w.hobbyID, FieldDescription, "better", "no hobby description"},
		{"empty", w.run, KindHobby, w.hobbyID, FieldDescription, "  \n ", "empty"},
		{"unchanged", w.run, KindHobby, w.hobbyID, FieldDescription, "club player", "same as it is now"},
		{"unchanged but for white space", w.run, KindHobby, w.hobbyID, FieldDescription, "  club player \n", "same as it is now"},
		{"too long", w.run, KindHobby, w.hobbyID, FieldDescription, strings.Repeat("é", 4001), "too long"},
		{"a title over one line", w.run, KindExperience, w.expID, FieldTitle, "Senior\nDeveloper", "one line"},
		{"a title that is too long", w.run, KindExperience, w.expID, FieldTitle, strings.Repeat("x", 201), "one line"},
		{"a bullet point dropped", w.run, KindExperience, w.expID, FieldDescription, "Intro\n- wrote Go and fixed bugs", "bullet"},
	}
	for _, c := range cases {
		_, err := AddSuggestion(c.run, c.kind, c.item, c.field, c.text, "")
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if _, err := AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, "better", strings.Repeat("r", 301)); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "reason") {
		t.Errorf("long reason: %v", err)
	}
	if _, err := AddSuggestion("nope", KindHobby, w.hobbyID, FieldDescription, "better", ""); !errors.Is(err, ErrUnknownRun) {
		t.Errorf("unknown run: %v", err)
	}
	if n := len(rowsOf(t, w.run)); n != 0 {
		t.Errorf("%d suggestions were stored by refused calls", n)
	}
}

func TestTheLimitsAreInclusive(t *testing.T) {
	w := newWorld(t)
	if _, err := AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, strings.Repeat("é", 4000), strings.Repeat("r", 300)); err != nil {
		t.Errorf("4000 characters and a 300 character reason are allowed: %v", err)
	}
	if _, err := AddSuggestion(w.run, KindExperience, w.expID, FieldTitle, strings.Repeat("x", 200), ""); err != nil {
		t.Errorf("a 200 character title is allowed: %v", err)
	}
}

func TestABulletPointMayBeReworded(t *testing.T) {
	w := newWorld(t)
	if _, err := AddSuggestion(w.run, KindExperience, w.expID, FieldDescription, "Introduction\n- wrote clean Go\n- fixed many bugs\n- new extra bullet", "better"); err != nil {
		t.Errorf("the same number of bullets or more is fine: %v", err)
	}
}

func TestASecondSuggestionForTheSameTextReplacesTheFirst(t *testing.T) {
	w := newWorld(t)
	first := must(AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, "one", "r1"))
	must(AddSuggestion(w.run, KindProfileDetails, "", FieldSummary, "other", ""))
	second, err := AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, "two", "r2")
	if err != nil || !second.Replaced || second.SuggestionID != first.SuggestionID || second.Suggested != 2 {
		t.Fatalf("%+v %v", second, err)
	}
	rows := rowsOf(t, w.run)
	if len(rows) != 2 || rows[0] != "0|hobby|"+w.hobbyID+"|description|club player|two|r2" {
		t.Errorf("%v", rows)
	}
	// The text it replaces the AI's own earlier one with is judged against the current text, so going back to it is refused.
	if _, err := AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, "club player", ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("back to the original: %v", err)
	}
}

func TestARunHoldsAtMostOneHundredSuggestions(t *testing.T) {
	pid := setup(t)
	ids := []string{}
	for i := 0; i < MaxSuggestionsPerRun+1; i++ {
		ids = append(ids, must(hobbies.Add(pid, hobbies.Input{Name: fmt.Sprintf("hobby %d", i), Description: "old"})).ID)
	}
	run := must(CreateRun(SubjectProfile, pid, "c", "", ""))
	for i := 0; i < MaxSuggestionsPerRun; i++ {
		if _, err := AddSuggestion(run, KindHobby, ids[i], FieldDescription, "new", ""); err != nil {
			t.Fatalf("suggestion %d: %v", i, err)
		}
	}
	if _, err := AddSuggestion(run, KindHobby, ids[MaxSuggestionsPerRun], FieldDescription, "new", ""); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "at most 100") {
		t.Errorf("the 101st: %v", err)
	}
	if st, err := AddSuggestion(run, KindHobby, ids[0], FieldDescription, "newer", ""); err != nil || !st.Replaced || st.Suggested != 100 {
		t.Errorf("replacing at the limit is fine: %+v %v", st, err)
	}
}

func TestOnlyAGeneratingRunTakesSuggestions(t *testing.T) {
	w := newWorld(t)
	must(AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, "better", ""))
	if _, err := FinishRun(w.run, "ready", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := AddSuggestion(w.run, KindProfileDetails, "", FieldSummary, "x", ""); !errors.Is(err, ErrRunNotGenerating) {
		t.Errorf("ready run: %v", err)
	}
	if _, err := GetSubjectForRun(w.run); !errors.Is(err, ErrRunNotGenerating) {
		t.Errorf("ready run read: %v", err)
	}
	if _, err := GetSubjectForRun("nope"); !errors.Is(err, ErrUnknownRun) {
		t.Errorf("unknown run read: %v", err)
	}
}

func TestTheAIIsGivenTheTextsTheFocusAndHowManyItHasSuggested(t *testing.T) {
	w := newWorld(t)
	s, err := GetSubjectForRun(w.run)
	if err != nil || s.Focus != "make it short" || s.Suggested != 0 || s.Name != "Ada Lovelace" || len(s.Texts) != 4 {
		t.Fatalf("%+v %v", s, err)
	}
	must(AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, "better", ""))
	if s, _ = GetSubjectForRun(w.run); s.Suggested != 1 {
		t.Errorf("suggested = %d", s.Suggested)
	}
	raw, _ := json.Marshal(s)
	if !strings.Contains(string(raw), `"subjectKind":"profile"`) || !strings.Contains(string(raw), `"texts":[`) {
		t.Errorf("the answer is flat: %s", raw)
	}
}

func TestTheToolsOverARealMCPSession(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1"}, nil)
	RegisterGetOptimizeSubject(server)
	RegisterSuggestOptimization(server)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, tl := range list.Tools {
		names = append(names, tl.Name)
		if tl.Name == "suggest_optimization" && (!strings.Contains(tl.Description, "changes nothing") || !strings.Contains(tl.Description, "bullet")) {
			t.Errorf("description: %s", tl.Description)
		}
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "get_optimize_subject,suggest_optimization" {
		t.Fatalf("tools: %v", names)
	}

	callTool := func(name string, args map[string]any) (string, bool) {
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
	text, isErr := callTool("get_optimize_subject", map[string]any{"runId": w.run})
	var subj SubjectForAI
	if isErr || json.Unmarshal([]byte(text), &subj) != nil || len(subj.Texts) != 4 || subj.Focus != "make it short" {
		t.Fatalf("get: %q", text)
	}
	if _, isErr := callTool("get_optimize_subject", map[string]any{"runId": " "}); !isErr {
		t.Error("a blank run id is an error")
	}
	if text, isErr := callTool("get_optimize_subject", map[string]any{"runId": "nope"}); !isErr || !strings.Contains(text, "unknown optimization run") {
		t.Errorf("unknown run: %q", text)
	}
	args := map[string]any{"runId": w.run, "kind": "hobby", "itemId": w.hobbyID, "field": "description", "text": "plays in a chess club", "reason": "clearer"}
	text, isErr = callTool("suggest_optimization", args)
	var stored Stored
	if isErr || json.Unmarshal([]byte(text), &stored) != nil || stored.SuggestionID == "" || stored.Suggested != 1 {
		t.Fatalf("suggest: %q", text)
	}
	args["text"] = "club player"
	if text, isErr := callTool("suggest_optimization", args); !isErr || !strings.Contains(text, "same as it is now") {
		t.Errorf("an unchanged text: %q", text)
	}
	if _, isErr := callTool("suggest_optimization", map[string]any{"runId": "", "kind": "hobby"}); !isErr {
		t.Error("a missing run id is an error")
	}
	if text, isErr := callTool("suggest_optimization", map[string]any{"runId": "nope", "kind": "hobby", "itemId": "x", "field": "description", "text": "y", "reason": ""}); !isErr || !strings.Contains(text, "unknown optimization run") {
		t.Errorf("unknown run: %q", text)
	}
	if n := len(rowsOf(t, w.run)); n != 1 {
		t.Errorf("%d suggestions", n)
	}
}

func TestTheSuggestionsEndpoint(t *testing.T) {
	w := newWorld(t)
	body := fmt.Sprintf(`{"runId":%q,"kind":"hobby","itemId":%q,"field":"description","text":"plays in a chess club","reason":"clearer"}`, w.run, w.hobbyID)
	res := call(SuggestionsHandler, "POST", "/optimize-runs/suggestions", body)
	var st Stored
	if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &st) != nil || st.Suggested != 1 || st.SuggestionID == "" {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	for _, c := range []struct {
		method, body string
		want         int
	}{
		{"POST", `not json`, 400},
		{"POST", `{"runId":""}`, 400},
		{"POST", fmt.Sprintf(`{"runId":%q,"kind":"hobby","itemId":%q,"field":"description","text":"club player"}`, w.run, w.hobbyID), 400},
		{"POST", `{"runId":"nope","kind":"hobby","itemId":"x","field":"description","text":"y"}`, 400},
		{"GET", ``, 405},
	} {
		if r := call(SuggestionsHandler, c.method, "/optimize-runs/suggestions", c.body); r.Code != c.want {
			t.Errorf("%s %s: %d want %d (%s)", c.method, c.body, r.Code, c.want, r.Body)
		}
	}
	if _, err := FinishRun(w.run, "ready", ""); err != nil {
		t.Fatal(err)
	}
	if r := call(SuggestionsHandler, "POST", "/optimize-runs/suggestions", body); r.Code != http.StatusConflict {
		t.Errorf("a finished run: %d", r.Code)
	}
}

func TestAFinishedRunIsRefusedBeforeAnythingElseIsChecked(t *testing.T) {
	w := newWorld(t)
	must(AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, "better", ""))
	if _, err := FinishRun(w.run, "ready", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := AddSuggestion(w.run, "nonsense", "", "nonsense", "", ""); !errors.Is(err, ErrRunNotGenerating) {
		t.Errorf("the run's state is the first thing that is checked: %v", err)
	}
}

func TestWhiteSpaceAroundTheCurrentTextDoesNotMakeASuggestionDifferent(t *testing.T) {
	w := newWorld(t)
	exec(t, `UPDATE profile_hobbies SET description = ? WHERE id = ?`, "club player \n", w.hobbyID)
	if _, err := AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, "club player", ""); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "same as it is now") {
		t.Errorf("%v", err)
	}
	// The text the AI was shown is kept exactly as it was, so approving can compare it.
	if st, err := AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, "plays chess", ""); err != nil || st.Suggested != 1 {
		t.Fatal(st, err)
	}
	if rows := rowsOf(t, w.run); len(rows) != 1 || !strings.Contains(rows[0], "|club player \n|plays chess|") {
		t.Errorf("%q", rows)
	}
}

func TestIndentedBulletPointsCountToo(t *testing.T) {
	w := newWorld(t)
	exec(t, `UPDATE profile_experience SET description = ? WHERE id = ?`, "Intro\n  - one\n  - two", w.expID)
	if _, err := AddSuggestion(w.run, KindExperience, w.expID, FieldDescription, "Intro\n- one and two", ""); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "bullet") {
		t.Errorf("dropping an indented bullet: %v", err)
	}
	if _, err := AddSuggestion(w.run, KindExperience, w.expID, FieldDescription, "Introduction\n  - one\n- two", ""); err != nil {
		t.Errorf("keeping both: %v", err)
	}
}
