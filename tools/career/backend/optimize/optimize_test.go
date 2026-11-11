package optimize

import (
	"encoding/json"
	"errors"
	"fmt"
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
	"career-tool-backend/profile"
	"career-tool-backend/profile/profiledetails"
	"career-tool-backend/profile/profileitems"
)

func setup(t *testing.T) (profileID string) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.JobsDB.Close()
		db.CareerDB.Close()
	})
	id, err := profile.CreateProfile("Ada", "Lovelace")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func ptr(s string) *string { return &s }

func newPersona(t *testing.T, profileID string) string {
	t.Helper()
	return must(persona.CreatePersona(profileID, "Backend", ""))
}

func exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := db.CareerDB.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func suggest(t *testing.T, runID string, pos int, kind, itemID, field, before, after string) string {
	t.Helper()
	id := fmt.Sprintf("s%d", 9-pos) // sorts the other way round than the position
	exec(t, `INSERT INTO optimize_suggestions (id, run_id, position, kind, item_id, field, before_text, after_text, reason) VALUES (?,?,?,?,?,?,?,?,?)`,
		id, runID, pos, kind, itemID, field, before, after, "better")
	return id
}

func textsOf(s Subject) []string {
	out := []string{}
	for _, x := range s.Texts {
		out = append(out, x.Kind+"/"+x.Field+"="+x.Text)
	}
	return out
}

func TestAProfilesTextsAreReadInOrderAndEmptyOnesAreLeftOut(t *testing.T) {
	pid := setup(t)
	if _, err := profiledetails.Update(pid, profiledetails.Patch{Summary: ptr("I build things.")}); err != nil {
		t.Fatal(err)
	}
	exp := must(profileitems.AddExperience(pid, profileitems.ExperienceInput{Company: "Acme", Title: "Dev", StartDate: "2019", EndDate: "2021", Description: "- wrote Go"}))
	noDesc := must(profileitems.AddExperience(pid, profileitems.ExperienceInput{Company: "Beta", Title: "Lead"}))
	edu := must(education.Add(pid, education.Input{Institution: "TU", Degree: "MSc", Description: "thesis on graphs"}))
	must(education.Add(pid, education.Input{Institution: "School"}))
	crs := must(courses.Add(pid, courses.Input{Title: "Go course", Provider: "Udemy", Description: "advanced"}))
	hob := must(hobbies.Add(pid, hobbies.Input{Name: "Chess", Description: "club player"}))

	s, err := ReadSubject(SubjectProfile, pid)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "Ada Lovelace" || s.Kind != SubjectProfile || s.ID != pid {
		t.Fatalf("subject = %+v", s)
	}
	got := strings.Join(textsOf(s), "|")
	for _, want := range []string{"profileDetails/summary=I build things.", "experience/title=Dev", "experience/description=- wrote Go", "experience/title=Lead", "education/description=thesis on graphs", "course/description=advanced", "hobby/description=club player"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if len(s.Texts) != 7 {
		t.Errorf("want 7 texts (empty descriptions left out), got %d: %s", len(s.Texts), got)
	}
	if strings.Index(got, "profileDetails") != 0 {
		t.Errorf("the summary comes first: %s", got)
	}
	for _, x := range s.Texts {
		switch x.ItemID {
		case exp.ID:
			if x.Label != "Acme — Dev" {
				t.Errorf("experience label %q", x.Label)
			}
			if x.Field == FieldDescription && x.Context != "2019 – 2021" {
				t.Errorf("experience context %q", x.Context)
			}
		case noDesc.ID:
			if x.Field == FieldDescription {
				t.Error("an empty description is not offered")
			}
		case edu.ID:
			if x.Label != "TU, MSc" {
				t.Errorf("education label %q", x.Label)
			}
		case crs.ID:
			if x.Label != "Go course, Udemy" {
				t.Errorf("course label %q", x.Label)
			}
		case hob.ID:
			if x.Label != "Chess" {
				t.Errorf("hobby label %q", x.Label)
			}
		}
	}
}

func TestAProfileWithNothingWrittenHasNoTexts(t *testing.T) {
	pid := setup(t)
	s, err := ReadSubject(SubjectProfile, pid)
	if err != nil || len(s.Texts) != 0 || s.Texts == nil {
		t.Fatalf("texts = %v, %v (want an empty, non-nil list)", s.Texts, err)
	}
}

func TestAPersonasTextsAreWhatItShowsNotWhatTheProfileHas(t *testing.T) {
	pid := setup(t)
	if _, err := profiledetails.Update(pid, profiledetails.Patch{Summary: ptr("Profile summary")}); err != nil {
		t.Fatal(err)
	}
	a := must(profileitems.AddExperience(pid, profileitems.ExperienceInput{Company: "Acme", Title: "Dev", Description: "profile text A"}))
	b := must(profileitems.AddExperience(pid, profileitems.ExperienceInput{Company: "Beta", Title: "Ops", Description: "profile text B"}))
	per := newPersona(t, pid)
	if err := profileitems.SetPersonaExperience(per, []profileitems.ExperienceUse{
		{ID: a.ID}, {ID: b.ID, TitleOverride: ptr("Platform engineer"), DescriptionOverride: ptr("persona text B")},
	}); err != nil {
		t.Fatal(err)
	}
	own := must(persona.AddCareerExperience(per, "Own Ltd", "Consultant", "", "", "own text"))

	s, err := ReadSubject(SubjectPersona, per)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "Backend" {
		t.Errorf("name %q", s.Name)
	}
	byKey := map[string]Text{}
	for _, x := range s.Texts {
		byKey[x.Kind+"/"+x.ItemID+"/"+x.Field] = x
	}
	if x := byKey["personaDetails//summary"]; x.Text != "Profile summary" {
		t.Errorf("a persona without its own summary shows the profile's: %+v", x)
	}
	if x := byKey["experience/"+a.ID+"/description"]; x.Text != "profile text A" {
		t.Errorf("followed text: %+v", x)
	}
	if x := byKey["experience/"+b.ID+"/description"]; x.Text != "persona text B" {
		t.Errorf("override text: %+v", x)
	}
	if x := byKey["experience/"+b.ID+"/title"]; x.Text != "Platform engineer" {
		t.Errorf("override title: %+v", x)
	}
	if x := byKey["customExperience/"+own+"/description"]; x.Text != "own text" {
		t.Errorf("custom experience: %+v", x)
	}

	// Its own summary replaces the profile's in what it shows.
	if err := persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{PersonaID: per, Summary: ptr("Persona summary")}); err != nil {
		t.Fatal(err)
	}
	s, _ = ReadSubject(SubjectPersona, per)
	if got := strings.Join(textsOf(s), "|"); !strings.Contains(got, "personaDetails/summary=Persona summary") || strings.Contains(got, "Profile summary") {
		t.Errorf("own summary: %s", got)
	}
}

func TestAnUnknownSubjectIsRefused(t *testing.T) {
	setup(t)
	for _, c := range [][2]string{{SubjectProfile, "nope"}, {SubjectPersona, "nope"}, {"job", "x"}, {"", ""}} {
		if _, err := ReadSubject(c[0], c[1]); err != ErrUnknownSubject {
			t.Errorf("%v: %v", c, err)
		}
	}
}

func TestARunIsRecordedAndItsInputIsChecked(t *testing.T) {
	pid := setup(t)
	per := newPersona(t, pid)
	bad := []struct {
		name                                   string
		kind, id, conv, driver, focus, wantErr string
	}{
		{"bad kind", "job", pid, "c", "", "", "subjectKind"},
		{"bad driver", SubjectProfile, pid, "c", "cloud", "", "driver"},
		{"remote needs a conversation", SubjectProfile, pid, "  ", "", "", "conversationId"},
		{"focus too long", SubjectProfile, pid, "c", "", strings.Repeat("x", 501), "focus"},
	}
	for _, c := range bad {
		_, err := CreateRun(c.kind, c.id, c.conv, c.driver, c.focus)
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if _, err := CreateRun(SubjectProfile, "nope", "c", "", ""); err != ErrUnknownSubject {
		t.Errorf("unknown profile: %v", err)
	}
	if _, err := CreateRun(SubjectPersona, pid, "c", "", ""); err != ErrUnknownSubject {
		t.Errorf("a profile id is not a persona id: %v", err)
	}
	id, err := CreateRun(SubjectPersona, per, " conv-1 ", "", "  shorter  ")
	if err != nil {
		t.Fatal(err)
	}
	runs, err := ListRuns(SubjectPersona, per)
	if err != nil || len(runs) != 1 {
		t.Fatalf("%v %v", runs, err)
	}
	r := runs[0]
	if r.ID != id || r.Status != "generating" || r.ConversationID != "conv-1" || r.Focus != "shorter" || r.Driver != "" || r.SubjectKind != SubjectPersona || r.SubjectID != per || r.Suggestions == nil {
		t.Errorf("run = %+v", r)
	}
	// A local run has no conversation.
	if _, err := CreateRun(SubjectProfile, pid, "", DriverLocal, ""); err != nil {
		t.Errorf("a local run needs no conversation: %v", err)
	}
}

func TestASubjectHasOnlyOneOpenRun(t *testing.T) {
	pid := setup(t)
	per := newPersona(t, pid)
	first, err := CreateRun(SubjectProfile, pid, "c1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateRun(SubjectProfile, pid, "c2", "", ""); err != ErrRunOpen {
		t.Fatalf("second run while generating: %v", err)
	}
	// Another subject is not blocked.
	if _, err := CreateRun(SubjectPersona, per, "c3", "", ""); err != nil {
		t.Fatal(err)
	}
	// A run that waits for the user's review still blocks.
	suggest(t, first, 0, KindProfileDetails, "", FieldSummary, "a", "b")
	if n, err := FinishRun(first, "ready", ""); err != nil || n != 1 {
		t.Fatalf("finish: %d %v", n, err)
	}
	if _, err := CreateRun(SubjectProfile, pid, "c2", "", ""); err != ErrRunOpen {
		t.Fatalf("second run while ready: %v", err)
	}
	// Dismissing it frees the subject.
	if err := DeleteRun(first); err != nil {
		t.Fatal(err)
	}
	failed, err := CreateRun(SubjectProfile, pid, "c4", "", "")
	if err != nil {
		t.Fatal(err)
	}
	// A failed run does not block either.
	if _, err := FinishRun(failed, "failed", "boom"); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateRun(SubjectProfile, pid, "c5", "", ""); err != nil {
		t.Fatalf("after a failed run: %v", err)
	}
}

func TestTwoRunsStartedAtOnceLeaveOne(t *testing.T) {
	pid := setup(t)
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = CreateRun(SubjectProfile, pid, "c", "", "")
		}(i)
	}
	wg.Wait()
	ok := 0
	for _, e := range errs {
		switch e {
		case nil:
			ok++
		case ErrRunOpen:
		default:
			t.Errorf("unexpected: %v", e)
		}
	}
	if ok != 1 {
		t.Fatalf("%d runs were started", ok)
	}
}

func statusOf(t *testing.T, id string) (status, errText string) {
	t.Helper()
	if err := db.CareerDB.QueryRow(`SELECT status, error FROM optimize_runs WHERE id = ?`, id).Scan(&status, &errText); err != nil {
		t.Fatal(err)
	}
	return
}

func TestFinishingARun(t *testing.T) {
	pid := setup(t)
	id := must(CreateRun(SubjectProfile, pid, "c", "", ""))
	if _, err := FinishRun(id, "generating", ""); err == nil || !strings.Contains(err.Error(), "status") {
		t.Errorf("bad status: %v", err)
	}
	if _, err := FinishRun(id, "failed", "  "); err == nil || !strings.Contains(err.Error(), "error is required") {
		t.Errorf("failed without a reason: %v", err)
	}
	if _, err := FinishRun("nope", "ready", ""); err != ErrUnknownRun {
		t.Errorf("unknown: %v", err)
	}
	suggest(t, id, 0, KindProfileDetails, "", FieldSummary, "a", "b")
	suggest(t, id, 1, KindHobby, "h1", FieldDescription, "c", "d")
	if n, err := FinishRun(id, "ready", "ignored"); err != nil || n != 2 {
		t.Fatalf("ready: %d %v", n, err)
	}
	if st, e := statusOf(t, id); st != "ready" || e != "" {
		t.Errorf("status %q error %q", st, e)
	}
	if _, err := FinishRun(id, "failed", "late"); err != ErrRunFinished {
		t.Errorf("a finished run cannot be finished again: %v", err)
	}
}

func TestAReadyRunWithoutSuggestionsIsRemoved(t *testing.T) {
	pid := setup(t)
	id := must(CreateRun(SubjectProfile, pid, "c", "", ""))
	n, err := FinishRun(id, "ready", "")
	if err != nil || n != 0 {
		t.Fatalf("%d %v", n, err)
	}
	runs, _ := ListRuns(SubjectProfile, pid)
	if len(runs) != 0 {
		t.Fatalf("nothing to review, nothing kept: %+v", runs)
	}
	if _, err := CreateRun(SubjectProfile, pid, "c", "", ""); err != nil {
		t.Errorf("the subject is free again: %v", err)
	}
}

func TestAFailedRunKeepsItsErrorCutToALimit(t *testing.T) {
	pid := setup(t)
	id := must(CreateRun(SubjectProfile, pid, "c", "", ""))
	if n, err := FinishRun(id, "failed", "  "+strings.Repeat("é", 3000)+"  "); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	st, e := statusOf(t, id)
	if st != "failed" || len([]rune(e)) != 2000 || strings.HasPrefix(e, " ") {
		t.Errorf("status %q, error of %d characters", st, len([]rune(e)))
	}
}

func TestARunThatLostItsPageIsEndedAsFailed(t *testing.T) {
	pid := setup(t)
	old := must(CreateRun(SubjectProfile, pid, "c", "", ""))
	exec(t, `UPDATE optimize_runs SET created_at = datetime('now', '-31 minutes') WHERE id = ?`, old)
	fresh := must(CreateRun(SubjectPersona, newPersona(t, pid), "c", "", ""))

	runs, err := ListRuns(SubjectProfile, pid)
	if err != nil || len(runs) != 1 || runs[0].Status != "failed" || !strings.Contains(runs[0].Error, "never finished") || runs[0].FinishedAt == "" {
		t.Fatalf("%+v %v", runs, err)
	}
	if st, _ := statusOf(t, fresh); st != "generating" {
		t.Errorf("a young run is left alone: %s", st)
	}
	// The start-up call does the same and counts.
	other := must(CreateRun(SubjectProfile, pid, "c", "", "")) // allowed: the old one is failed
	exec(t, `UPDATE optimize_runs SET created_at = datetime('now', '-45 minutes') WHERE id = ?`, other)
	if n, err := ReconcileStale(); err != nil || n != 1 {
		t.Errorf("reconciled %d, %v", n, err)
	}
	// A run that was ready is never touched.
	r := must(CreateRun(SubjectProfile, pid, "c", "", ""))
	suggest(t, r, 0, KindHobby, "h", FieldDescription, "a", "b")
	if _, err := FinishRun(r, "ready", ""); err != nil {
		t.Fatal(err)
	}
	exec(t, `UPDATE optimize_runs SET created_at = datetime('now', '-3 hours') WHERE id = ?`, r)
	if n, _ := ReconcileStale(); n != 0 {
		t.Errorf("a ready run was ended: %d", n)
	}
}

func TestTheListShowsTheLabelAndWhetherASuggestionIsStale(t *testing.T) {
	pid := setup(t)
	if _, err := profiledetails.Update(pid, profiledetails.Patch{Summary: ptr("old summary")}); err != nil {
		t.Fatal(err)
	}
	h := must(hobbies.Add(pid, hobbies.Input{Name: "Chess", Description: "club player"}))
	e := must(profileitems.AddExperience(pid, profileitems.ExperienceInput{Company: "Acme", Title: "Dev", Description: "text"}))
	id := must(CreateRun(SubjectProfile, pid, "c", "", ""))
	suggest(t, id, 0, KindProfileDetails, "", FieldSummary, "old summary", "new summary")
	suggest(t, id, 1, KindHobby, h.ID, FieldDescription, "club player", "plays in a chess club")
	suggest(t, id, 2, KindExperience, e.ID, FieldDescription, "text", "better text")
	suggest(t, id, 3, KindHobby, "gone", FieldDescription, "x", "y")
	// The hobby text is changed by hand after the AI read it; the fourth suggestion is about an item that does not exist.
	if _, err := hobbies.Update(h.ID, hobbies.Patch{Description: ptr("changed by hand")}); err != nil {
		t.Fatal(err)
	}
	runs, err := ListRuns(SubjectProfile, pid)
	if err != nil || len(runs) != 1 || len(runs[0].Suggestions) != 4 {
		t.Fatalf("%+v %v", runs, err)
	}
	got := runs[0].Suggestions
	want := []struct {
		label string
		stale bool
	}{{"Summary", false}, {"Chess", true}, {"Acme — Dev", false}, {"", true}}
	for i, w := range want {
		if got[i].Label != w.label || got[i].Stale != w.stale {
			t.Errorf("suggestion %d: label %q stale %v, want %q %v", i, got[i].Label, got[i].Stale, w.label, w.stale)
		}
	}
	if got[0].Before != "old summary" || got[0].After != "new summary" || got[0].Reason != "better" || got[0].Kind != KindProfileDetails {
		t.Errorf("first suggestion: %+v", got[0])
	}
	if _, err := ListRuns("job", "x"); err != ErrUnknownSubject {
		t.Errorf("unknown subject: %v", err)
	}
}

func TestRunsGoWhenTheirProfileOrPersonaIsDeleted(t *testing.T) {
	pid := setup(t)
	keep := must(profile.CreateProfile("Keep", "Me"))
	per := newPersona(t, pid)
	per2 := newPersona(t, pid)
	keepRun := must(CreateRun(SubjectProfile, keep, "c", "", ""))
	pRun := must(CreateRun(SubjectProfile, pid, "c", "", ""))
	perRun := must(CreateRun(SubjectPersona, per, "c", "", ""))
	per2Run := must(CreateRun(SubjectPersona, per2, "c", "", ""))
	suggest(t, perRun, 0, KindHobby, "h", FieldDescription, "a", "b")

	count := func() (runs, sugs int) {
		_ = db.CareerDB.QueryRow(`SELECT COUNT(*) FROM optimize_runs`).Scan(&runs)
		_ = db.CareerDB.QueryRow(`SELECT COUNT(*) FROM optimize_suggestions`).Scan(&sugs)
		return
	}
	if err := persona.DeletePersona(per); err != nil {
		t.Fatal(err)
	}
	if r, s := count(); r != 3 || s != 0 {
		t.Fatalf("deleting a persona: %d runs, %d suggestions left", r, s)
	}
	if err := profile.DeleteProfile(pid); err != nil {
		t.Fatal(err)
	}
	r, s := count()
	if r != 1 || s != 0 {
		t.Fatalf("deleting a profile: %d runs, %d suggestions left", r, s)
	}
	var left string
	if err := db.CareerDB.QueryRow(`SELECT id FROM optimize_runs`).Scan(&left); err != nil || left != keepRun {
		t.Errorf("only the other profile's run stays: %q %v (gone: %s %s)", left, err, pRun, per2Run)
	}
}

func call(h http.HandlerFunc, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func TestTheRunsEndpoints(t *testing.T) {
	pid := setup(t)
	if w := call(RunsHandler, "POST", "/optimize-runs", `{"subjectKind":"profile","subjectId":"`+pid+`","conversationId":"c1","focus":"short"}`); w.Code != 200 {
		t.Fatalf("POST: %d %s", w.Code, w.Body)
	} else {
		var out struct{ ID string }
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		if out.ID == "" {
			t.Fatal("no id")
		}
		// a second one is a conflict
		if w2 := call(RunsHandler, "POST", "/optimize-runs", `{"subjectKind":"profile","subjectId":"`+pid+`","conversationId":"c2"}`); w2.Code != http.StatusConflict {
			t.Errorf("second POST: %d", w2.Code)
		}
		list := call(RunsHandler, "GET", "/optimize-runs?subjectKind=profile&subjectId="+pid, "")
		var l struct{ Runs []Run }
		_ = json.Unmarshal(list.Body.Bytes(), &l)
		if list.Code != 200 || len(l.Runs) != 1 || l.Runs[0].ID != out.ID || l.Runs[0].Suggestions == nil {
			t.Errorf("GET: %d %s", list.Code, list.Body)
		}
		if w3 := call(RunsHandler, "PUT", "/optimize-runs", `{"id":"`+out.ID+`","status":"ready"}`); w3.Code != 200 || !strings.Contains(w3.Body.String(), `"suggestions":0`) {
			t.Errorf("PUT ready without suggestions: %d %s", w3.Code, w3.Body)
		}
		if w4 := call(RunsHandler, "PUT", "/optimize-runs", `{"id":"`+out.ID+`","status":"ready"}`); w4.Code != http.StatusBadRequest {
			t.Errorf("PUT for a run that is gone: %d", w4.Code)
		}
	}
	id2 := must(CreateRun(SubjectProfile, pid, "c", "", ""))
	if w := call(RunsHandler, "PUT", "/optimize-runs", `{"id":"`+id2+`","status":"failed","error":"boom"}`); w.Code != 200 {
		t.Errorf("PUT failed: %d %s", w.Code, w.Body)
	}
	if w := call(RunsHandler, "PUT", "/optimize-runs", `{"id":"`+id2+`","status":"failed","error":"again"}`); w.Code != http.StatusConflict {
		t.Errorf("PUT twice: %d", w.Code)
	}
	if w := call(RunsHandler, "PUT", "/optimize-runs", `{"id":"`+id2+`","status":"nope"}`); w.Code != http.StatusBadRequest {
		t.Errorf("PUT bad status: %d", w.Code)
	}
	if w := call(RunsHandler, "PUT", "/optimize-runs", `{}`); w.Code != http.StatusBadRequest {
		t.Errorf("PUT without id: %d", w.Code)
	}
	for _, c := range []struct {
		method, target, body string
		want                 int
	}{
		{"POST", "/optimize-runs", `not json`, 400},
		{"POST", "/optimize-runs", `{"subjectKind":"profile","subjectId":"nope","conversationId":"c"}`, 400},
		{"GET", "/optimize-runs", "", 400},
		{"GET", "/optimize-runs?subjectKind=profile&subjectId=nope", "", 400},
		{"DELETE", "/optimize-runs", "", 400},
		{"DELETE", "/optimize-runs?id=" + id2, "", 204},
		{"DELETE", "/optimize-runs?id=" + id2, "", 204},
		{"PATCH", "/optimize-runs", "", 405},
	} {
		if w := call(RunsHandler, c.method, c.target, c.body); w.Code != c.want {
			t.Errorf("%s %s: %d, want %d (%s)", c.method, c.target, w.Code, c.want, w.Body)
		}
	}
}

func TestTheSubjectEndpoint(t *testing.T) {
	pid := setup(t)
	must(hobbies.Add(pid, hobbies.Input{Name: "Chess", Description: "club player"}))
	w := call(SubjectHandler, "GET", "/optimize-runs/subject?subjectKind=profile&subjectId="+pid, "")
	var s Subject
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil || w.Code != 200 || s.Name != "Ada Lovelace" || len(s.Texts) != 1 || s.Texts[0].Text != "club player" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	for _, c := range []struct {
		method, target string
		want           int
	}{
		{"GET", "/optimize-runs/subject", 400},
		{"GET", "/optimize-runs/subject?subjectKind=profile&subjectId=nope", 400},
		{"POST", "/optimize-runs/subject?subjectKind=profile&subjectId=" + pid, 405},
	} {
		if w := call(SubjectHandler, c.method, c.target, ""); w.Code != c.want {
			t.Errorf("%s %s: %d, want %d", c.method, c.target, w.Code, c.want)
		}
	}
}

func TestTheManifestDeclaresTheRoutes(t *testing.T) {
	raw, err := os.ReadFile("../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Routes []struct {
			Method        string `json:"method"`
			PathSuffix    string `json:"path_suffix"`
			RequiredScope string `json:"required_scope"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	have := map[string]string{}
	for _, r := range m.Routes {
		have[r.Method+" "+r.PathSuffix] = r.RequiredScope
	}
	for _, k := range []string{"GET optimize-runs", "POST optimize-runs", "PUT optimize-runs", "DELETE optimize-runs", "GET optimize-runs/subject"} {
		if have[k] != "tool:career:profile" {
			t.Errorf("%s: scope %q", k, have[k])
		}
	}
}

func TestRunsComeNewestFirstAndSuggestionsInTheirOrder(t *testing.T) {
	pid := setup(t)
	a := must(CreateRun(SubjectProfile, pid, "c", "", ""))
	suggest(t, a, 1, KindHobby, "h2", FieldDescription, "x", "y")
	suggest(t, a, 0, KindHobby, "h1", FieldDescription, "x", "y")
	if _, err := FinishRun(a, "ready", ""); err != nil {
		t.Fatal(err)
	}
	exec(t, `UPDATE optimize_runs SET status = 'failed', error = 'x' WHERE id = ?`, a)
	b := must(CreateRun(SubjectProfile, pid, "c", "", ""))
	exec(t, `UPDATE optimize_runs SET created_at = datetime('now', '-1 minutes') WHERE id = ?`, a)
	runs, err := ListRuns(SubjectProfile, pid)
	if err != nil || len(runs) != 2 || runs[0].ID != b || runs[1].ID != a {
		t.Fatalf("%+v %v", runs, err)
	}
	if s := runs[1].Suggestions; len(s) != 2 || s[0].ItemID != "h1" || s[1].ItemID != "h2" {
		t.Errorf("suggestions out of order: %+v", s)
	}
}

func TestAStaleRunDoesNotBlockANewOneEvenWithoutAListInBetween(t *testing.T) {
	pid := setup(t)
	old := must(CreateRun(SubjectProfile, pid, "c", "", ""))
	exec(t, `UPDATE optimize_runs SET created_at = datetime('now', '-31 minutes') WHERE id = ?`, old)
	if _, err := CreateRun(SubjectProfile, pid, "c2", "", ""); err != nil {
		t.Fatalf("the run that lost its page must not block: %v", err)
	}
	if st, _ := statusOf(t, old); st != "failed" {
		t.Errorf("old run: %s", st)
	}
}

func TestOnlyTextWithSomethingInItIsOffered(t *testing.T) {
	pid := setup(t)
	must(hobbies.Add(pid, hobbies.Input{Name: "Chess", Description: "   \n  "}))
	must(profileitems.AddExperience(pid, profileitems.ExperienceInput{Company: "Acme", Title: "Dev", StartDate: "2022-01", Description: "ongoing"}))
	s, _ := ReadSubject(SubjectProfile, pid)
	for _, x := range s.Texts {
		if x.Kind == KindHobby {
			t.Errorf("a blank description is not offered: %+v", x)
		}
		if x.Kind == KindExperience && x.Field == FieldDescription && x.Context != "2022-01 – now" {
			t.Errorf("a job that is not over says so: %q", x.Context)
		}
	}
}

func TestASuggestionIsAboutOneKindOfItem(t *testing.T) {
	pid := setup(t)
	h := must(hobbies.Add(pid, hobbies.Input{Name: "Chess", Description: "club player"}))
	id := must(CreateRun(SubjectProfile, pid, "c", "", ""))
	suggest(t, id, 0, KindEducation, h.ID, FieldDescription, "club player", "x")
	runs, _ := ListRuns(SubjectProfile, pid)
	if g := runs[0].Suggestions[0]; !g.Stale || g.Label != "" {
		t.Errorf("an education suggestion cannot be matched to a hobby: %+v", g)
	}
}

func TestTheTitleAndTheDescriptionOfOneJobAreTwoSuggestions(t *testing.T) {
	pid := setup(t)
	e := must(profileitems.AddExperience(pid, profileitems.ExperienceInput{Company: "Acme", Title: "Dev", Description: "text"}))
	id := must(CreateRun(SubjectProfile, pid, "c", "", ""))
	suggest(t, id, 0, KindExperience, e.ID, FieldTitle, "Dev", "Developer")
	suggest(t, id, 1, KindExperience, e.ID, FieldDescription, "text", "better")
	if _, err := db.CareerDB.Exec(`INSERT INTO optimize_suggestions (id, run_id, position, kind, item_id, field, before_text, after_text) VALUES ('dup', ?, 2, ?, ?, ?, 'a', 'b')`, id, KindExperience, e.ID, FieldTitle); err == nil {
		t.Error("a second suggestion for the same text must be refused by the table")
	}
	if n, err := FinishRun(id, "ready", ""); err != nil || n != 2 {
		t.Errorf("%d %v", n, err)
	}
}

func TestFinishingOverHTTPSaysHowManySuggestionsWait(t *testing.T) {
	pid := setup(t)
	id := must(CreateRun(SubjectProfile, pid, "c", "", ""))
	suggest(t, id, 0, KindHobby, "h1", FieldDescription, "a", "b")
	suggest(t, id, 1, KindHobby, "h2", FieldDescription, "a", "b")
	if w := call(RunsHandler, "PUT", "/optimize-runs", `{"id":"`+id+`","status":"ready"}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"suggestions":2`) {
		t.Errorf("%d %s", w.Code, w.Body)
	}
}
