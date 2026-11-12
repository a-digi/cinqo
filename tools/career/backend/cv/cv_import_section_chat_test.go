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

	"career-tool-backend/db"
)

// A chat bound to one section or some rows of a CV import's suggestions (plan 268, step 1).

func bind(t *testing.T, run, conv, section string, ids ...string) string {
	t.Helper()
	id, err := RecordSectionChat(run, conv, section, ids)
	if err != nil {
		t.Fatalf("bind %s %v: %v", section, ids, err)
	}
	return id
}

func chatPost(body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	SectionChatsHandler(rec, httptest.NewRequest(http.MethodPost, "/cv-import/section-chats", strings.NewReader(body)))
	return rec
}

func TestRecordASectionChatAndReadItBack(t *testing.T) {
	s := newSugg(t)
	g := s.get()
	id := bind(t, s.run, "conv-skills", "skills")
	c, err := sectionChatOf("conv-skills")
	if err != nil || c == nil || c.ID != id || c.RunID != s.run || c.Section != "skills" || len(c.ItemIDs) != 0 || c.describe() != "the Skills section" {
		t.Fatalf("%+v %v", c, err)
	}
	bind(t, s.run, "conv-rows", "skills", g.Skills[0].Id, g.Skills[2].Id, g.Skills[0].Id, " ")
	c, _ = sectionChatOf("conv-rows")
	if len(c.ItemIDs) != 2 || c.ItemIDs[0] != g.Skills[0].Id || c.ItemIDs[1] != g.Skills[2].Id || c.describe() != "2 rows of the Skills section" {
		t.Errorf("%+v", c)
	}
	bind(t, s.run, "conv-one", "experience", g.Experience[0].Id)
	if c, _ = sectionChatOf("conv-one"); c.describe() != "one row of the Experience section" || !c.isList() {
		t.Errorf("%+v", c)
	}
	bind(t, s.run, "conv-persona", "persona")
	if c, _ = sectionChatOf("conv-persona"); c.describe() != "the Target roles section" || c.isList() {
		t.Errorf("%+v", c)
	}
	if c, err = sectionChatOf("nobody"); c != nil || err != nil {
		t.Errorf("an unbound conversation: %+v %v", c, err)
	}
	if c, err = sectionChatOf(""); c != nil || err != nil {
		t.Errorf("no conversation: %+v %v", c, err)
	}
}

func TestSectionNamesAreThePageHeadings(t *testing.T) {
	for in, want := range map[string]string{"profile": "Profile", "persona": "Target roles", "personalDetails": "Personal Details", "skills": "Skills", "experience": "Experience", "education": "Education", "odd": "odd"} {
		if got := sectionName(in); got != want {
			t.Errorf("%s: %q, want %q", in, got, want)
		}
	}
}

func TestRecordASectionChatRefusals(t *testing.T) {
	s := newSugg(t)
	g := s.get()
	if _, err := RecordSectionChat(s.run, "bad conv!", "skills", nil); err == nil {
		t.Error("a conversation id with a space and a mark")
	} else {
		wantProblem(t, err, "conversationId")
	}
	if _, err := RecordSectionChat(s.run, strings.Repeat("c", 101), "skills", nil); err == nil {
		t.Error("a too long conversation id")
	}
	if _, err := RecordSectionChat(s.run, "", "skills", nil); err == nil {
		t.Error("no conversation id")
	}
	_, err := RecordSectionChat(s.run, "c1", "hobbies", nil)
	wantProblem(t, err, "section must be one of")
	_, err = RecordSectionChat(s.run, "c1", "persona", []string{"x"})
	wantProblem(t, err, "single section")
	_, err = RecordSectionChat(s.run, "c1", "skills", []string{"nope"})
	wantProblem(t, err, `no row with id "nope"`)
	_, err = RecordSectionChat(s.run, "c1", "skills", []string{g.Experience[0].Id})
	wantProblem(t, err, "no row with id")
	_, err = RecordSectionChat(s.run, "c1", "education", []string{g.Skills[0].Id})
	wantProblem(t, err, "no row with id")
	if _, err := RecordSectionChat("00000000-0000-0000-0000-000000000000", "c1", "skills", nil); !errors.Is(err, errUnknownCVImportRun) {
		t.Errorf("an unknown run: %v", err)
	}
	many := make([]string, MaxSectionChatRows+1)
	for i := range many {
		many[i] = fmt.Sprintf("id-%d", i)
	}
	_, err = RecordSectionChat(s.run, "c1", "skills", many)
	wantProblem(t, err, "at most 100 rows")
	if n := count(t, `SELECT COUNT(*) FROM cv_import_section_chats`); n != 0 {
		t.Errorf("%d bindings after only refusals", n)
	}
	bind(t, s.run, "c2", "skills")
	if _, err := RecordSectionChat(s.run, "c2", "experience", nil); !errors.Is(err, errChatAlreadyBound) {
		t.Errorf("a second binding for one conversation: %v", err)
	}
	if c, _ := sectionChatOf("c2"); c.Section != "skills" {
		t.Errorf("the first binding must stay: %+v", c)
	}
	exactly := make([]string, 0, MaxSectionChatRows)
	if _, err := RecordSectionChat(s.run, "c3", "skills", exactly); err != nil {
		t.Errorf("an empty list of ids is the whole section: %v", err)
	}
}

func TestRecordASectionChatNeedsARunThatIsOpen(t *testing.T) {
	setup(t)
	gen, err := createGeneratingCVImportRunFor("m", "cv.pdf", "conv-g", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RecordSectionChat(gen, "c1", "skills", nil); !errors.Is(err, errSuggestionsStillGenerating) {
		t.Errorf("generating: %v", err)
	}
	saved := readyRun(t, oldProposalJSON)
	setSummary(t, saved, `{"skills":[{"index":0,"ok":true}]}`)
	if _, err := RecordSectionChat(saved, "c2", "skills", nil); !errors.Is(err, errChatRunClosed) {
		t.Errorf("saved: %v", err)
	}
	if n := count(t, `SELECT COUNT(*) FROM cv_import_section_chats`); n != 0 {
		t.Errorf("%d bindings", n)
	}
}

func TestBindingsGoWithTheirRun(t *testing.T) {
	s := newSugg(t)
	bind(t, s.run, "c1", "skills")
	bind(t, s.run, "c2", "experience")
	other := readyRun(t, oldProposalJSON)
	bind(t, other, "c3", "education")
	if _, err := deleteCVImportRun(s.run); err != nil {
		t.Fatal(err)
	}
	if n := count(t, `SELECT COUNT(*) FROM cv_import_section_chats`); n != 1 {
		t.Errorf("%d bindings left, want only the other run's", n)
	}
	if c, _ := sectionChatOf("c3"); c == nil || c.RunID != other {
		t.Errorf("%+v", c)
	}
}

func TestAStoredBindingThatCannotBeReadIsAnError(t *testing.T) {
	s := newSugg(t)
	bind(t, s.run, "c1", "skills")
	if _, err := db.CareerDB.Exec(`UPDATE cv_import_section_chats SET item_ids = 'not json' WHERE conversation_id = 'c1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := sectionChatOf("c1"); err == nil || !strings.Contains(err.Error(), "unreadable") {
		t.Errorf("%v", err)
	}
	if _, err := ChangeSuggestionsIn("c1", s.run, []SuggestionChange{add("skills", map[string]any{"value": "x"})}, ""); err == nil {
		t.Error("a change in a chat whose binding cannot be read must not go through")
	}
	if _, err := GetSuggestionsIn("c1", s.run, "", nil); err == nil {
		t.Error("a read in a chat whose binding cannot be read must not go through")
	}
}

// ---- the scope of a change

func TestAChatAboutAListMayChangeOnlyThatList(t *testing.T) {
	s := newSugg(t)
	g := s.get()
	bind(t, s.run, "c-skills", "skills")
	in := func(conv string, ch ...SuggestionChange) (SuggestionsChanged, error) {
		return ChangeSuggestionsIn(conv, s.run, ch, "")
	}
	out, err := in("c-skills", upd("skills", g.Skills[0].Id, map[string]any{"value": "Golang"}), add("skills", map[string]any{"value": "Rust"}), rem("skills", g.Skills[2].Id))
	if err != nil || out.Applied != 3 {
		t.Fatalf("%+v %v", out, err)
	}
	// every way out of the list is refused, whatever the operation is
	for name, ch := range map[string]SuggestionChange{
		"update of a single section": updSection("profile", map[string]any{"name": "X"}),
		"update of another list":     upd("experience", g.Experience[0].Id, map[string]any{"title": "X"}),
		"add to another list":        add("education", map[string]any{"institution": "X"}),
		"remove from another list":   rem("education", g.Education[0].Id),
		"update of the persona":      updSection("persona", map[string]any{"title": "X"}),
	} {
		before := s.get().Revision
		_, err := in("c-skills", ch)
		wantProblem(t, err, "this chat is about the Skills section, so it cannot change")
		if s.get().Revision != before {
			t.Errorf("%s: the refused call changed the suggestions", name)
		}
	}
}

func TestAChatAboutASingleSectionMayChangeOnlyThatSection(t *testing.T) {
	s := newSugg(t)
	g := s.get()
	bind(t, s.run, "c-pd", "personalDetails")
	if _, err := ChangeSuggestionsIn("c-pd", s.run, []SuggestionChange{updSection("personalDetails", map[string]any{"headline": "New"})}, ""); err != nil {
		t.Fatal(err)
	}
	if s.get().PersonalDetails.Headline != "New" {
		t.Error("the bound section was not changed")
	}
	for _, ch := range []SuggestionChange{updSection("persona", map[string]any{"title": "X"}), upd("skills", g.Skills[0].Id, map[string]any{"value": "X"}), add("skills", map[string]any{"value": "X"})} {
		_, err := ChangeSuggestionsIn("c-pd", s.run, []SuggestionChange{ch}, "")
		wantProblem(t, err, "this chat is about the Personal Details section")
	}
}

func TestAChatAboutRowsMayChangeOnlyThoseRows(t *testing.T) {
	s := newSugg(t)
	g := s.get()
	bind(t, s.run, "c-rows", "skills", g.Skills[0].Id, g.Skills[1].Id)
	if _, err := ChangeSuggestionsIn("c-rows", s.run, []SuggestionChange{upd("skills", g.Skills[1].Id, map[string]any{"value": "Dockerfile"}), rem("skills", g.Skills[0].Id)}, ""); err != nil {
		t.Fatal(err)
	}
	for name, ch := range map[string]SuggestionChange{
		"update of another row": upd("skills", g.Skills[2].Id, map[string]any{"value": "X"}),
		"remove of another row": rem("skills", g.Skills[2].Id),
	} {
		_, err := ChangeSuggestionsIn("c-rows", s.run, []SuggestionChange{ch}, "")
		wantProblem(t, err, "is not one of them")
		_ = name
	}
	_, err := ChangeSuggestionsIn("c-rows", s.run, []SuggestionChange{add("skills", map[string]any{"value": "X"})}, "")
	wantProblem(t, err, "cannot add a row")
	// a row of a bound list that the chat holds, named in the wrong list, is outside the scope too
	_, err = ChangeSuggestionsIn("c-rows", s.run, []SuggestionChange{upd("experience", g.Skills[1].Id, map[string]any{"title": "X"})}, "")
	wantProblem(t, err, "cannot change Experience")
}

func TestOneBadOperationRefusesTheWholeCallAndNamesTheOffender(t *testing.T) {
	s := newSugg(t)
	g := s.get()
	bind(t, s.run, "c-skills", "skills")
	before := s.get()
	_, err := ChangeSuggestionsIn("c-skills", s.run, []SuggestionChange{
		upd("skills", g.Skills[0].Id, map[string]any{"value": "A"}),
		updSection("persona", map[string]any{"title": "X"}),
		add("skills", map[string]any{"value": "B"}),
	}, "")
	wantProblem(t, err, "changes[1]: this chat is about the Skills section, so it cannot change Target roles")
	if after := s.get(); after.Revision != before.Revision || after.Skills[0].Value != before.Skills[0].Value || len(after.Skills) != len(before.Skills) {
		t.Errorf("nothing may be stored when one operation is outside: %+v", after.Skills)
	}
}

func TestACallOfAnotherRunIsRefusedInABoundChat(t *testing.T) {
	s := newSugg(t)
	other := readyRun(t, oldProposalJSON)
	bind(t, s.run, "c1", "skills")
	_, err := ChangeSuggestionsIn("c1", other, []SuggestionChange{add("skills", map[string]any{"value": "X"})}, "")
	wantProblem(t, err, "about another import")
	if _, err := GetSuggestionsIn("c1", other, "", nil); err == nil || !strings.Contains(err.Error(), "about another import") {
		t.Errorf("read of another run: %v", err)
	}
	g, _ := GetSuggestions(other)
	if len(g.Skills) != 3 {
		t.Errorf("the other run must be untouched: %+v", g.Skills)
	}
}

func TestAChatWithoutABindingBehavesAsBefore(t *testing.T) {
	s := newSugg(t)
	g := s.get()
	for _, conv := range []string{"", "never-bound"} {
		if _, err := ChangeSuggestionsIn(conv, s.run, []SuggestionChange{updSection("profile", map[string]any{"name": "Free"}), add("education", map[string]any{"institution": "ETH"})}, ""); err != nil {
			t.Fatalf("conv %q: %v", conv, err)
		}
	}
	got, err := GetSuggestionsIn("", s.run, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	full, ok := got.(Suggestions)
	if !ok || full.Profile.Name != "Free" || len(full.Education) != 3 || full.Revision == g.Revision {
		t.Errorf("%T %+v", got, got)
	}
}

func TestACloseRunStaysClosedInABoundChat(t *testing.T) {
	s := newSugg(t)
	bind(t, s.run, "c1", "skills")
	setSummary(t, s.run, `{"skills":[{"index":0,"ok":true}]}`)
	_, err := ChangeSuggestionsIn("c1", s.run, []SuggestionChange{add("skills", map[string]any{"value": "X"})}, "")
	if !errors.Is(err, errSuggestionsClosed) {
		t.Errorf("%v", err)
	}
}

func TestTheScopeIsCheckedBeforeBasedOnAndTheOrdinaryRulesStillApply(t *testing.T) {
	s := newSugg(t)
	g := s.get()
	bind(t, s.run, "c1", "skills")
	_, err := ChangeSuggestionsIn("c1", s.run, []SuggestionChange{updSection("persona", map[string]any{"title": "X"})}, "stale-revision")
	wantProblem(t, err, "cannot change Target roles")
	if _, err := ChangeSuggestionsIn("c1", s.run, []SuggestionChange{add("skills", map[string]any{"value": "X"})}, "stale-revision"); !errors.Is(err, errSuggestionsMoved) {
		t.Errorf("an old revision in a bound chat: %v", err)
	}
	_, err = ChangeSuggestionsIn("c1", s.run, []SuggestionChange{add("skills", map[string]any{"value": strings.Repeat("x", 5000)})}, g.Revision)
	if err == nil {
		t.Error("the validation of the result still applies")
	}
	// a malformed operation is left to the ordinary checks, which say what is wrong with it
	_, err = ChangeSuggestionsIn("c1", s.run, []SuggestionChange{{Op: "update"}}, "")
	wantProblem(t, err, "name a section")
}

// ---- the read

func TestTheReadIsCutToTheBoundSectionAndRows(t *testing.T) {
	s := newSugg(t)
	g := s.get()
	bind(t, s.run, "c-skills", "skills")
	bind(t, s.run, "c-rows", "skills", g.Skills[1].Id)
	bind(t, s.run, "c-exp", "experience")
	bind(t, s.run, "c-pd", "personalDetails")
	asMap := func(conv, section string, ids ...string) map[string]any {
		t.Helper()
		got, err := GetSuggestionsIn(conv, s.run, section, ids)
		if err != nil {
			t.Fatalf("%s %s %v: %v", conv, section, ids, err)
		}
		b, _ := json.Marshal(got)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		return m
	}
	m := asMap("c-skills", "")
	if len(m["skills"].([]any)) != 3 || m["revision"] != g.Revision || m["runId"] != s.run || m["status"] != "ready" || m["originalFilename"] != "cv.pdf" || m["section"] != "skills" || m["scope"] != "This chat is about the Skills section." {
		t.Errorf("%v", m)
	}
	for _, other := range []string{"profile", "persona", "personalDetails", "experience", "education"} {
		if _, has := m[other]; has {
			t.Errorf("a chat about the skills must not be shown %s", other)
		}
	}
	m = asMap("c-rows", "")
	rows := m["skills"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["id"] != g.Skills[1].Id || rows[0].(map[string]any)["state"] != "pending" || m["scope"] != "This chat is about one row of the Skills section." {
		t.Errorf("%v", m)
	}
	if m = asMap("c-exp", "experience"); len(m["experience"].([]any)) != 1 || m["skills"] != nil {
		t.Errorf("%v", m)
	}
	if m = asMap("c-pd", ""); m["personalDetails"] == nil || m["skills"] != nil || m["persona"] != nil || m["profile"] != nil {
		t.Errorf("%v", m)
	}
	// the same section or rows may be asked for explicitly; anything else is refused
	if m = asMap("c-rows", "skills", g.Skills[1].Id); len(m["skills"].([]any)) != 1 {
		t.Errorf("%v", m)
	}
	for name, args := range map[string]struct {
		conv, section string
		ids           []string
	}{
		"another section":      {"c-skills", "experience", nil},
		"another row":          {"c-rows", "", []string{g.Skills[0].Id}},
		"another row by name":  {"c-rows", "skills", []string{g.Skills[0].Id}},
		"a section's row list": {"c-pd", "personalDetails", []string{"x"}},
	} {
		if _, err := GetSuggestionsIn(args.conv, s.run, args.section, args.ids); err == nil {
			t.Errorf("%s was allowed", name)
		}
	}
}

func TestTheOptionalFilterWorksInAnUnboundChatToo(t *testing.T) {
	s := newSugg(t)
	g := s.get()
	got, err := GetSuggestionsIn("", s.run, "skills", []string{g.Skills[2].Id, g.Skills[0].Id})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	rows := m["skills"].([]any)
	if len(rows) != 2 || rows[0].(map[string]any)["id"] != g.Skills[0].Id || rows[1].(map[string]any)["id"] != g.Skills[2].Id || m["scope"] != nil || m["revision"] != g.Revision {
		t.Errorf("%v", m)
	}
	for _, c := range []struct {
		section string
		ids     []string
		want    string
	}{
		{"hobbies", nil, "section must be one of"},
		{"profile", []string{"x"}, "single section"},
		{"skills", []string{"nope"}, "not rows of skills"},
		{"skills", []string{g.Skills[0].Id, "nope"}, "not rows of skills"},
		{"", []string{g.Skills[0].Id}, "ids go with a section"},
	} {
		_, err := GetSuggestionsIn("", s.run, c.section, c.ids)
		wantProblem(t, err, c.want)
	}
	for _, section := range []string{"profile", "persona", "personalDetails", "experience", "education"} {
		got, err := GetSuggestionsIn("", s.run, section, nil)
		if err != nil {
			t.Fatalf("%s: %v", section, err)
		}
		b, _ := json.Marshal(got)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		if m[section] == nil || m["section"] != section {
			t.Errorf("%s: %v", section, m)
		}
	}
	// a duplicate id counts once
	if _, err := GetSuggestionsIn("", s.run, "skills", []string{g.Skills[0].Id, g.Skills[0].Id}); err != nil {
		t.Errorf("a repeated id: %v", err)
	}
	// ids that are blank are ignored
	if got, err := GetSuggestionsIn("", s.run, "", []string{" "}); err != nil {
		t.Errorf("blank ids: %v", err)
	} else if _, ok := got.(Suggestions); !ok {
		t.Errorf("blank ids must read as no ids: %T", got)
	}
	if got, err := GetSuggestionsIn("", s.run, "  ", nil); err != nil {
		t.Error(err)
	} else if _, ok := got.(Suggestions); !ok {
		t.Errorf("a blank section reads as none: %T", got)
	}
}

func TestAnEmptyListIsShownAsAnEmptyListInTheCut(t *testing.T) {
	setup(t)
	run := readyRun(t, `{"profile":{"action":"insert","possibleDuplicateOf":null,"name":"A"},"persona":{"action":"insert","possibleDuplicateOf":null,"name":"B","title":"T"},"personalDetails":{"action":"insert","possibleDuplicateOf":null,"headline":"","summary":"","location":"","desiredTitles":"","desiredLocations":"","minSalary":0},"skills":[],"experience":[],"education":[]}`)
	bind(t, run, "c1", "skills")
	got, err := GetSuggestionsIn("c1", run, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	if !strings.Contains(string(b), `"skills":[]`) {
		t.Errorf("%s", b)
	}
}

// ---- the route

func TestTheSectionChatRoute(t *testing.T) {
	s := newSugg(t)
	g := s.get()
	body := func(conv, section string, ids ...string) string {
		b, _ := json.Marshal(map[string]any{"runId": s.run, "conversationId": conv, "section": section, "itemIds": ids})
		return string(b)
	}
	if rec := chatPost(body("conv-a", "skills", g.Skills[0].Id)); rec.Code != http.StatusCreated || decode(t, rec)["id"] == "" || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("a good one: %d %s %s", rec.Code, rec.Body.String(), rec.Header().Get("Content-Type"))
	}
	if c, _ := sectionChatOf("conv-a"); c == nil || c.Section != "skills" || len(c.ItemIDs) != 1 {
		t.Errorf("%+v", c)
	}
	for name, c := range map[string]struct {
		body string
		code int
	}{
		"a second binding":      {body("conv-a", "experience"), http.StatusConflict},
		"an unknown section":    {body("conv-b", "hobbies"), http.StatusBadRequest},
		"ids on a single one":   {body("conv-b", "persona", "x"), http.StatusBadRequest},
		"an unknown row":        {body("conv-b", "skills", "nope"), http.StatusBadRequest},
		"a bad conversation id": {body("bad id", "skills"), http.StatusBadRequest},
		"no section":            {`{"runId":"` + s.run + `","conversationId":"c"}`, http.StatusBadRequest},
		"no conversation":       {`{"runId":"` + s.run + `","section":"skills"}`, http.StatusBadRequest},
		"no run":                {`{"conversationId":"c","section":"skills"}`, http.StatusBadRequest},
		"not json":              {`nope`, http.StatusBadRequest},
		"an unknown run":        {`{"runId":"00000000-0000-0000-0000-000000000000","conversationId":"c","section":"skills"}`, http.StatusNotFound},
	} {
		if rec := chatPost(c.body); rec.Code != c.code {
			t.Errorf("%s: %d %s, want %d", name, rec.Code, rec.Body.String(), c.code)
		}
	}
	rec := httptest.NewRecorder()
	SectionChatsHandler(rec, httptest.NewRequest(http.MethodGet, "/cv-import/section-chats", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", rec.Code)
	}
	setSummary(t, s.run, `{"skills":[{"index":0,"ok":true}]}`)
	if rec := chatPost(body("conv-c", "skills")); rec.Code != http.StatusConflict {
		t.Errorf("a closed run: %d %s", rec.Code, rec.Body.String())
	}
	gen, _ := createGeneratingCVImportRunFor("m", "cv.pdf", "conv-g", "", "")
	if rec := chatPost(fmt.Sprintf(`{"runId":%q,"conversationId":"conv-d","section":"skills"}`, gen)); rec.Code != http.StatusConflict {
		t.Errorf("a generating run: %d %s", rec.Code, rec.Body.String())
	}
	failed, _ := createGeneratingCVImportRunFor("m", "cv.pdf", "conv-f", "", "")
	if _, err := finishCVImportRun(failed, runFailed, "boom"); err != nil {
		t.Fatal(err)
	}
	if rec := chatPost(fmt.Sprintf(`{"runId":%q,"conversationId":"conv-e","section":"skills"}`, failed)); rec.Code != http.StatusConflict {
		t.Errorf("a failed run: %d %s", rec.Code, rec.Body.String())
	}
}

// ---- the tools, with the conversation the core names in the environment

func TestTheToolsKeepABoundChatInsideItsSectionThroughAnMCPSession(t *testing.T) {
	setup(t)
	run := readyRun(t, oldProposalJSON)
	g0, _ := GetSuggestions(run)
	bind(t, run, "conv-bound", "skills", g0.Skills[0].Id)
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
	change := func(ch map[string]any) (string, bool) {
		return call("update_cv_import_suggestions", map[string]any{"runId": run, "changes": []map[string]any{ch}})
	}
	// without TOOL_CONVERSATION_ID the call is an ordinary one
	t.Setenv("TOOL_CONVERSATION_ID", "")
	if text, isErr := change(map[string]any{"op": "update", "section": "persona", "fields": map[string]any{"title": "Free"}}); isErr {
		t.Fatalf("an unbound call: %q", text)
	}
	// a value that is not a plain id is no conversation either
	t.Setenv("TOOL_CONVERSATION_ID", "conv-bound; DROP")
	if text, isErr := change(map[string]any{"op": "update", "section": "persona", "fields": map[string]any{"title": "Free 2"}}); isErr {
		t.Fatalf("a malformed id: %q", text)
	}
	t.Setenv("TOOL_CONVERSATION_ID", "conv-bound")
	text, isErr := call("get_cv_import_suggestions", map[string]any{"runId": run})
	var m map[string]any
	if isErr || json.Unmarshal([]byte(text), &m) != nil || m["scope"] != "This chat is about one row of the Skills section." || m["persona"] != nil || len(m["skills"].([]any)) != 1 {
		t.Fatalf("the bound read: %q", text)
	}
	if text, isErr := call("get_cv_import_suggestions", map[string]any{"runId": run, "section": "experience"}); !isErr || !strings.Contains(text, "cannot read the Experience section") {
		t.Errorf("a read outside: %q", text)
	}
	if text, isErr := change(map[string]any{"op": "update", "section": "persona", "fields": map[string]any{"title": "Hacked"}}); !isErr || !strings.Contains(text, "this chat is about one row of the Skills section, so it cannot change Target roles") {
		t.Errorf("a change outside: %q", text)
	}
	if text, isErr := change(map[string]any{"op": "update", "list": "skills", "id": g0.Skills[0].Id, "fields": map[string]any{"value": "Golang"}}); isErr {
		t.Errorf("a change inside: %q", text)
	}
	after, _ := GetSuggestions(run)
	if after.Persona.Title != "Free 2" || after.Skills[0].Value != "Golang" {
		t.Errorf("%+v", after)
	}
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range tools.Tools {
		b, _ := json.Marshal(tl.InputSchema)
		if tl.Name == "get_cv_import_suggestions" && (!strings.Contains(string(b), `"section"`) || !strings.Contains(string(b), `"ids"`) || strings.Contains(string(b), `"required":["runId","section"`)) {
			t.Errorf("the read's schema: %s", b)
		}
		if tl.Name == "update_cv_import_suggestions" && !strings.Contains(tl.Description, "about one section") {
			t.Errorf("the description must tell about the scope: %s", tl.Description)
		}
	}
}

func TestTwoChatsChangingOneRunAtOnceDoNotLoseAnything(t *testing.T) {
	s := newSugg(t)
	bind(t, s.run, "c-a", "skills")
	bind(t, s.run, "c-b", "skills")
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i, conv := range []string{"c-a", "c-b"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = ChangeSuggestionsIn(conv, s.run, []SuggestionChange{add("skills", map[string]any{"value": fmt.Sprintf("N%d", i)})}, "")
		}()
	}
	wg.Wait()
	ok := 0
	for _, e := range results {
		if e == nil {
			ok++
		} else if !errors.Is(e, errSuggestionsMoved) {
			t.Errorf("an unexpected failure: %v", e)
		}
	}
	if ok < 1 || len(s.get().Skills) != 3+ok {
		t.Errorf("%d succeeded but %d skills", ok, len(s.get().Skills))
	}
}

func TestCallerConversationReadsOnlyAPlainId(t *testing.T) {
	for in, want := range map[string]string{"": "", "  ": "", "abc-1_X": "abc-1_X", " abc ": "abc", "a b": "", "a;b": "", strings.Repeat("a", 100): strings.Repeat("a", 100), strings.Repeat("a", 101): ""} {
		t.Setenv("TOOL_CONVERSATION_ID", in)
		if got := callerConversation(); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestAChatMayHoldExactlyTheMaximumOfRowsAndAWholeSectionStoresNoIds(t *testing.T) {
	setup(t)
	var skills []string
	for i := 0; i < MaxSectionChatRows+1; i++ {
		skills = append(skills, fmt.Sprintf(`{"action":"insert","possibleDuplicateOf":null,"value":"S%d"}`, i))
	}
	run := readyRun(t, `{"profile":{"action":"insert","possibleDuplicateOf":null,"name":"A"},"persona":{"action":"insert","possibleDuplicateOf":null,"name":"B","title":"T"},"personalDetails":{"action":"insert","possibleDuplicateOf":null,"headline":"","summary":"","location":"","desiredTitles":"","desiredLocations":"","minSalary":0},"skills":[`+strings.Join(skills, ",")+`],"experience":[],"education":[]}`)
	g, _ := GetSuggestions(run)
	ids := make([]string, 0, len(g.Skills))
	for _, r := range g.Skills {
		ids = append(ids, r.Id)
	}
	if _, err := RecordSectionChat(run, "c-max", "skills", ids[:MaxSectionChatRows]); err != nil {
		t.Errorf("exactly the maximum: %v", err)
	}
	_, err := RecordSectionChat(run, "c-over", "skills", ids)
	wantProblem(t, err, "at most 100 rows")
	bind(t, run, "c-whole", "skills")
	var stored string
	if err := db.CareerDB.QueryRow(`SELECT item_ids FROM cv_import_section_chats WHERE conversation_id = 'c-whole'`).Scan(&stored); err != nil || stored != "" {
		t.Errorf("a whole-section binding stores no ids: %q %v", stored, err)
	}
}
