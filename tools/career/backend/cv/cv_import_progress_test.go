package cv

import (
	"net/http"
	"strings"
	"testing"
)

func TestProgressOfReadsTheDraftInCounts(t *testing.T) {
	got := progressOf(`{"profile":{"action":"insert","name":"A"},"skills":[{"action":"insert","value":"Go"},{"action":"insert","value":"PHP"}],"experience":[{"action":"insert","company":"X"}],"education":[]}`)
	want := runProgress{Profile: true, Skills: 2, Experience: 1}
	if got == nil || *got != want {
		t.Errorf("%+v, want %+v", got, want)
	}
	all := progressOf(`{"profile":{},"persona":{},"personalDetails":{}}`)
	if all == nil || !all.Profile || !all.Persona || !all.PersonalDetails {
		t.Errorf("%+v", all)
	}
	for _, in := range []string{"", "nope", "[1]", `"x"`, "{"} {
		if p := progressOf(in); p != nil {
			t.Errorf("%q gave %+v, want nil: an empty or unreadable draft shows nothing", in, p)
		}
	}
}

func TestAGeneratingRunShowsWhatItHasStoredAndNothingElseDoes(t *testing.T) {
	setup(t)
	id := newRun(t)
	if r := runOf(t, id); r.Progress != nil {
		t.Fatalf("no piece yet: %+v", r.Progress)
	}
	if _, err := SetDraftSection(id, sectionProfile, asObject(t, okProposal().Profile)); err != nil {
		t.Fatal(err)
	}
	if _, err := AddDraftSkills(id, asObject(t, okProposal().Skills)); err != nil {
		t.Fatal(err)
	}
	if _, err := AddDraftExperience(id, asObject(t, okProposal().Experience[0])); err != nil {
		t.Fatal(err)
	}
	p := runOf(t, id).Progress
	if p == nil || *p != (runProgress{Profile: true, Skills: 2, Experience: 1}) {
		t.Fatalf("%+v", p)
	}
	// it is in the list too, and in the JSON of the run
	rec := call(http.MethodGet, "/cv-import/runs?id="+id, "")
	if !strings.Contains(rec.Body.String(), `"progress":{"profile":true,"persona":false,"personalDetails":false,"skills":2,"experience":1,"education":0}`) {
		t.Errorf("%s", rec.Body.String())
	}
	// a ready run shows none
	sendAllRest(t, id)
	if err := FinishDraft(id); err != nil {
		t.Fatal(err)
	}
	if _, err := finishCVImportRun(id, runReady, ""); err != nil {
		t.Fatal(err)
	}
	if r := runOf(t, id); r.Progress != nil || strings.Contains(call(http.MethodGet, "/cv-import/runs?id="+id, "").Body.String(), "progress") {
		t.Errorf("a finished run has no progress: %+v", r.Progress)
	}
	// nor does a failed one
	failed := newRun(t)
	if _, err := SetDraftSection(failed, sectionProfile, asObject(t, okProposal().Profile)); err != nil {
		t.Fatal(err)
	}
	if _, err := finishCVImportRun(failed, "failed", "stopped"); err != nil {
		t.Fatal(err)
	}
	if r := runOf(t, failed); r.Progress != nil {
		t.Errorf("a failed run has no progress: %+v", r.Progress)
	}
}

func sendAllRest(t *testing.T, id string) {
	t.Helper()
	p := okProposal()
	if _, err := SetDraftSection(id, sectionPersona, asObject(t, p.Persona)); err != nil {
		t.Fatal(err)
	}
	if _, err := SetDraftSection(id, sectionPersonalDetails, asObject(t, p.PersonalDetails)); err != nil {
		t.Fatal(err)
	}
}

func TestARunRecordsItsDriverAndOnlyKnownOnes(t *testing.T) {
	setup(t)
	code := createViaAPI(t, `{"fileId":"m1","originalFilename":"cv.pdf","conversationId":"c1","driver":"code"}`)
	plain := createViaAPI(t, `{"fileId":"m1","originalFilename":"cv.pdf","conversationId":"c1"}`)
	if r := runOf(t, code); r.Driver != "code" || r.Status != "generating" {
		t.Errorf("%+v", r)
	}
	if r := runOf(t, plain); r.Driver != "" {
		t.Errorf("%+v", r)
	}
	if body := call(http.MethodGet, "/cv-import/runs?id="+plain, "").Body.String(); strings.Contains(body, "driver") {
		t.Errorf("no driver must not be in the JSON: %s", body)
	}
	if body := call(http.MethodGet, "/cv-import/runs?id="+code, "").Body.String(); !strings.Contains(body, `"driver":"code"`) {
		t.Errorf("%s", body)
	}
	for _, d := range []string{"magic", "Code", " code"} {
		rec := call(http.MethodPost, "/cv-import/runs", `{"fileId":"m1","originalFilename":"cv.pdf","conversationId":"c1","driver":"`+d+`"}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("driver %q: %d", d, rec.Code)
		}
	}
	// the list carries it too
	list := call(http.MethodGet, "/cv-import/runs", "").Body.String()
	if strings.Count(list, `"driver":"code"`) != 1 {
		t.Errorf("%s", list)
	}
}
