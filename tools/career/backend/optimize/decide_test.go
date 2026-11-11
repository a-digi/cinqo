package optimize

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"career-tool-backend/courses"
	"career-tool-backend/db"
	"career-tool-backend/education"
	"career-tool-backend/hobbies"
	"career-tool-backend/persona"
	"career-tool-backend/profile/profileitems"
)

// readyRun gives a world's run its suggestions and ends the AI's turn.
func readyRun(t *testing.T, runID string, sugs ...[5]string) []string {
	t.Helper()
	ids := []string{}
	for _, s := range sugs {
		st, err := AddSuggestion(runID, s[0], s[1], s[2], s[3], s[4])
		if err != nil {
			t.Fatalf("%v: %v", s, err)
		}
		ids = append(ids, st.SuggestionID)
	}
	if n, err := FinishRun(runID, "ready", ""); err != nil || n != len(sugs) {
		t.Fatalf("finish: %d %v", n, err)
	}
	return ids
}

func textOf(t *testing.T, kind, id string, k, item, field string) string {
	t.Helper()
	s, err := ReadSubject(kind, id)
	if err != nil {
		t.Fatal(err)
	}
	x, ok := s.find(k, item, field)
	if !ok {
		t.Fatalf("no %s %s %s", k, item, field)
	}
	return x.Text
}

func decide(t *testing.T, runID string, approve bool, ids ...string) Decided {
	t.Helper()
	ds := []Decision{}
	for _, id := range ids {
		ds = append(ds, Decision{SuggestionID: id, Approve: approve})
	}
	res, err := ApplyDecisions(runID, ds)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestDecliningDeletesTheSuggestionAndChangesNothing(t *testing.T) {
	w := newWorld(t)
	ids := readyRun(t, w.run,
		[5]string{KindProfileDetails, "", FieldSummary, "I build reliable systems.", ""},
		[5]string{KindHobby, w.hobbyID, FieldDescription, "plays chess", ""})
	res := decide(t, w.run, false, ids[0])
	if len(res.Outcomes) != 1 || !res.Outcomes[0].OK || res.Remaining != 1 || res.Closed {
		t.Fatalf("%+v", res)
	}
	if got := textOf(t, SubjectProfile, w.pid, KindProfileDetails, "", FieldSummary); got != "I build things." {
		t.Errorf("a declined text was written: %q", got)
	}
	if n := len(rowsOf(t, w.run)); n != 1 {
		t.Errorf("%d suggestions left", n)
	}
	res = decide(t, w.run, false, ids[1])
	if res.Remaining != 0 || !res.Closed {
		t.Fatalf("the run closes with its last suggestion: %+v", res)
	}
	var n int
	_ = db.CareerDB.QueryRow(`SELECT COUNT(*) FROM optimize_runs WHERE id = ?`, w.run).Scan(&n)
	if n != 0 {
		t.Error("the closed run is still there")
	}
	if _, err := CreateRun(SubjectProfile, w.pid, "c", "", ""); err != nil {
		t.Errorf("the profile is free for a new run: %v", err)
	}
}

func TestApprovingAProfilesTextsReplacesThem(t *testing.T) {
	w := newWorld(t)
	w.eduID = must(education.Add(w.pid, education.Input{Institution: "TU", Degree: "MSc", Description: "graphs"})).ID
	w.courseID = must(courses.Add(w.pid, courses.Input{Title: "Go", Provider: "Udemy", Description: "advanced"})).ID
	_ = must(FinishRun(w.run, "failed", "x")) // the world's own run is not needed here
	run := must(CreateRun(SubjectProfile, w.pid, "c", "", ""))
	want := map[[3]string]string{
		{KindProfileDetails, "", FieldSummary}:      "I build reliable systems.",
		{KindExperience, w.expID, FieldTitle}:       "Developer",
		{KindExperience, w.expID, FieldDescription}: "Introduction\n- wrote Go\n- fixed bugs",
		{KindEducation, w.eduID, FieldDescription}:  "thesis on graph theory",
		{KindCourse, w.courseID, FieldDescription}:  "an advanced course",
		{KindHobby, w.hobbyID, FieldDescription}:    "plays chess in a club",
	}
	sugs := [][5]string{}
	for k, v := range want {
		sugs = append(sugs, [5]string{k[0], k[1], k[2], v, "better"})
	}
	ids := readyRun(t, run, sugs...)
	res := decide(t, run, true, ids...)
	for _, o := range res.Outcomes {
		if !o.OK {
			t.Errorf("%+v", o)
		}
	}
	if !res.Closed || res.Remaining != 0 {
		t.Errorf("%+v", res)
	}
	for k, v := range want {
		if got := textOf(t, SubjectProfile, w.pid, k[0], k[1], k[2]); got != v {
			t.Errorf("%v = %q, want %q", k, got, v)
		}
	}
}

func personaWorld(t *testing.T) (w world, per, other, expID string) {
	t.Helper()
	w = newWorld(t)
	_ = must(FinishRun(w.run, "failed", "x"))
	per = newPersona(t, w.pid)
	other = must(persona.CreatePersona(w.pid, "Other", ""))
	expID = w.expID
	for _, p := range []string{per, other} {
		if err := profileitems.SetPersonaExperience(p, []profileitems.ExperienceUse{{ID: expID}}); err != nil {
			t.Fatal(err)
		}
	}
	return
}

func TestApprovingAPersonasTextsWritesItsOwnOverridesNotTheProfile(t *testing.T) {
	w, per, other, expID := personaWorld(t)
	own := must(persona.AddCareerExperience(per, "Own Ltd", "Consultant", "", "", "own text"))
	run := must(CreateRun(SubjectPersona, per, "c", "", ""))
	ids := readyRun(t, run,
		[5]string{KindPersonaDetails, "", FieldSummary, "I build reliable systems.", ""},
		[5]string{KindExperience, expID, FieldDescription, "Introduction\n- wrote Go\n- fixed bugs", ""},
		[5]string{KindExperience, expID, FieldTitle, "Developer", ""},
		[5]string{KindCustomExperience, own, FieldDescription, "better own text", ""},
		[5]string{KindCustomExperience, own, FieldTitle, "Freelance consultant", ""})
	res := decide(t, run, true, ids...)
	for _, o := range res.Outcomes {
		if !o.OK {
			t.Fatalf("%+v", o)
		}
	}
	if got := textOf(t, SubjectPersona, per, KindExperience, expID, FieldDescription); got != "Introduction\n- wrote Go\n- fixed bugs" {
		t.Errorf("persona description: %q", got)
	}
	if got := textOf(t, SubjectPersona, per, KindExperience, expID, FieldTitle); got != "Developer" {
		t.Errorf("persona title: %q", got)
	}
	if got := textOf(t, SubjectPersona, per, KindPersonaDetails, "", FieldSummary); got != "I build reliable systems." {
		t.Errorf("persona summary: %q", got)
	}
	if got := textOf(t, SubjectPersona, per, KindCustomExperience, own, FieldDescription); got != "better own text" {
		t.Errorf("custom description: %q", got)
	}
	if got := textOf(t, SubjectPersona, per, KindCustomExperience, own, FieldTitle); got != "Freelance consultant" {
		t.Errorf("custom title: %q", got)
	}
	// The profile's own library and the other persona are untouched.
	if got := textOf(t, SubjectProfile, w.pid, KindExperience, expID, FieldDescription); got != "Intro\n- wrote Go\n- fixed bugs" {
		t.Errorf("the profile's text changed: %q", got)
	}
	if got := textOf(t, SubjectProfile, w.pid, KindExperience, expID, FieldTitle); got != "Dev" {
		t.Errorf("the profile's title changed: %q", got)
	}
	if got := textOf(t, SubjectProfile, w.pid, KindProfileDetails, "", FieldSummary); got != "I build things." {
		t.Errorf("the profile's summary changed: %q", got)
	}
	if got := textOf(t, SubjectPersona, other, KindExperience, expID, FieldDescription); got != "Intro\n- wrote Go\n- fixed bugs" {
		t.Errorf("the other persona changed: %q", got)
	}
	if got := textOf(t, SubjectPersona, other, KindPersonaDetails, "", FieldSummary); got != "I build things." {
		t.Errorf("the other persona's summary changed: %q", got)
	}
}

func TestATextChangedAfterTheAIReadItIsNotOverwritten(t *testing.T) {
	w := newWorld(t)
	ids := readyRun(t, w.run,
		[5]string{KindHobby, w.hobbyID, FieldDescription, "plays chess", ""},
		[5]string{KindProfileDetails, "", FieldSummary, "I build reliable systems.", ""},
		[5]string{KindExperience, w.expID, FieldDescription, "Intro\n- wrote good Go\n- fixed bugs", ""})
	if _, err := hobbies.Update(w.hobbyID, hobbies.Patch{Description: ptr("changed by hand")}); err != nil {
		t.Fatal(err)
	}
	if err := profileitems.UpdateExperience(w.expID, profileitems.ExperiencePatch{Description: ptr("")}); err != nil { // emptied: the text is gone from the list
		t.Fatal(err)
	}
	res := decide(t, w.run, true, ids...)
	if res.Outcomes[0].OK || !strings.Contains(res.Outcomes[0].Error, "changed after the AI read it") {
		t.Errorf("changed text: %+v", res.Outcomes[0])
	}
	if !res.Outcomes[1].OK {
		t.Errorf("an untouched text is still applied: %+v", res.Outcomes[1])
	}
	if res.Outcomes[2].OK || !strings.Contains(res.Outcomes[2].Error, "changed after the AI read it") {
		t.Errorf("a text that is gone: %+v", res.Outcomes[2])
	}
	if res.Remaining != 2 || res.Closed {
		t.Errorf("the stale ones stay for the user to decline: %+v", res)
	}
	if got := textOf(t, SubjectProfile, w.pid, KindHobby, w.hobbyID, FieldDescription); got != "changed by hand" {
		t.Errorf("the hand-made change was overwritten: %q", got)
	}
	// They can be declined afterwards, and then the run closes.
	res = decide(t, w.run, false, ids[0], ids[2])
	if !res.Closed {
		t.Errorf("%+v", res)
	}
}

func TestOneRefusedDecisionDoesNotStopTheOthers(t *testing.T) {
	w := newWorld(t)
	ids := readyRun(t, w.run,
		[5]string{KindHobby, w.hobbyID, FieldDescription, "plays chess", ""},
		[5]string{KindProfileDetails, "", FieldSummary, "I build reliable systems.", ""})
	other := must(CreateRun(SubjectPersona, newPersona(t, w.pid), "c", "", ""))
	foreign := readyRun(t, other, [5]string{KindPersonaDetails, "", FieldSummary, "x y z", ""})
	// A suggestion of another run cannot be decided through this one, not even declined.
	if res2, err := ApplyDecisions(w.run, []Decision{{SuggestionID: foreign[0], Approve: false}}); err != nil || res2.Outcomes[0].OK {
		t.Errorf("declining another run's suggestion: %+v %v", res2, err)
	}
	res, err := ApplyDecisions(w.run, []Decision{
		{SuggestionID: "nope", Approve: true},
		{SuggestionID: foreign[0], Approve: true},
		{SuggestionID: ids[0], Approve: true},
		{SuggestionID: ids[0], Approve: true},
		{SuggestionID: ids[1], Approve: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	ok := []bool{}
	for _, o := range res.Outcomes {
		ok = append(ok, o.OK)
	}
	if fmt.Sprint(ok) != "[false false true false true]" {
		t.Errorf("outcomes: %+v", res.Outcomes)
	}
	if res.Outcomes[2].SuggestionID != ids[0] || !strings.Contains(res.Outcomes[0].Error, "not in this run") {
		t.Errorf("%+v", res.Outcomes)
	}
	// A suggestion of another run cannot be decided through this one.
	if n := len(rowsOf(t, other)); n != 1 {
		t.Errorf("the other run's suggestion: %d", n)
	}
	if !res.Closed {
		t.Errorf("%+v", res)
	}
}

func TestDecisionsAreOnlyTakenOnAReadyRun(t *testing.T) {
	w := newWorld(t)
	must(AddSuggestion(w.run, KindHobby, w.hobbyID, FieldDescription, "better", ""))
	if _, err := ApplyDecisions(w.run, []Decision{{SuggestionID: "x", Approve: true}}); !errors.Is(err, ErrRunNotReady) {
		t.Errorf("generating: %v", err)
	}
	if _, err := ApplyDecisions("nope", []Decision{{SuggestionID: "x"}}); !errors.Is(err, ErrUnknownRun) {
		t.Errorf("unknown: %v", err)
	}
	if _, err := FinishRun(w.run, "ready", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyDecisions(w.run, nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("no decisions: %v", err)
	}
	failed := must(CreateRun(SubjectPersona, newPersona(t, w.pid), "c", "", ""))
	if _, err := FinishRun(failed, "failed", "boom"); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyDecisions(failed, []Decision{{SuggestionID: "x"}}); !errors.Is(err, ErrRunNotReady) {
		t.Errorf("failed: %v", err)
	}
}

func TestApprovingGoesThroughTheWritersSoTheMatcherHearsOfIt(t *testing.T) {
	w, per, _, expID := personaWorld(t)
	var mu sync.Mutex
	bodies := []string{}
	core := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
	}))
	defer core.Close()
	t.Setenv("CORE_API_URL", core.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "t")
	count := func() int { mu.Lock(); defer mu.Unlock(); return len(bodies) }

	// A change of the profile's experience reaches every persona that uses it.
	run := must(CreateRun(SubjectProfile, w.pid, "c", "", ""))
	ids := readyRun(t, run, [5]string{KindExperience, expID, FieldDescription, "Intro\n- wrote good Go\n- fixed bugs", ""})
	before := count()
	decide(t, run, true, ids...)
	if count() <= before {
		t.Error("approving a profile experience announced nothing")
	}
	// A declined suggestion announces nothing.
	run2 := must(CreateRun(SubjectPersona, per, "c", "", ""))
	ids2 := readyRun(t, run2, [5]string{KindExperience, expID, FieldDescription, "Intro\n- wrote great Go\n- fixed bugs", ""})
	before = count()
	decide(t, run2, false, ids2...)
	if count() != before {
		t.Error("declining announced something")
	}
	// A persona's own override does.
	run3 := must(CreateRun(SubjectPersona, per, "c", "", ""))
	ids3 := readyRun(t, run3, [5]string{KindPersonaDetails, "", FieldSummary, "I build reliable systems.", ""})
	before = count()
	decide(t, run3, true, ids3...)
	if count() <= before {
		t.Error("approving a persona summary announced nothing")
	}
}

func TestTheDecideEndpoint(t *testing.T) {
	w := newWorld(t)
	ids := readyRun(t, w.run,
		[5]string{KindHobby, w.hobbyID, FieldDescription, "plays chess", ""},
		[5]string{KindProfileDetails, "", FieldSummary, "I build reliable systems.", ""})
	r := call(DecideHandler, "POST", "/optimize-runs/decide", fmt.Sprintf(`{"runId":%q,"decisions":[{"suggestionId":%q,"approve":true}]}`, w.run, ids[0]))
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"remaining":1`) || !strings.Contains(r.Body.String(), `"closed":false`) || !strings.Contains(r.Body.String(), `"ok":true`) {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
	for _, c := range []struct {
		method, body string
		want         int
	}{
		{"POST", `not json`, 400},
		{"POST", `{"decisions":[]}`, 400},
		{"POST", fmt.Sprintf(`{"runId":%q,"decisions":[]}`, w.run), 400},
		{"POST", `{"runId":"nope","decisions":[{"suggestionId":"x"}]}`, 400},
		{"GET", ``, 405},
	} {
		if r := call(DecideHandler, c.method, "/optimize-runs/decide", c.body); r.Code != c.want {
			t.Errorf("%s %s: %d want %d (%s)", c.method, c.body, r.Code, c.want, r.Body)
		}
	}
	gen := must(CreateRun(SubjectPersona, newPersona(t, w.pid), "c", "", ""))
	if r := call(DecideHandler, "POST", "/optimize-runs/decide", fmt.Sprintf(`{"runId":%q,"decisions":[{"suggestionId":"x"}]}`, gen)); r.Code != http.StatusConflict {
		t.Errorf("a run that is still generating: %d", r.Code)
	}
}

func TestTheManifestDeclaresTheStepTwoRoutesAndTools(t *testing.T) {
	raw := mustRead(t, "../../manifest.json")
	for _, want := range []string{`"path_suffix": "optimize-runs/suggestions"`, `"path_suffix": "optimize-runs/decide"`, `"name": "get_optimize_subject"`, `"name": "suggest_optimization"`} {
		if !strings.Contains(raw, want) {
			t.Errorf("manifest lacks %s", want)
		}
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
