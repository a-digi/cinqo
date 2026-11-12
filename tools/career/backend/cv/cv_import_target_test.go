package cv

import (
	"net/http"
	"strings"
	"testing"

	"career-tool-backend/db"
)

func newProfile(t *testing.T, id string) {
	t.Helper()
	if _, err := db.CareerDB.Exec(`INSERT INTO profiles (id, first_name, last_name) VALUES (?, 'Ada', 'Lovelace')`, id); err != nil {
		t.Fatal(err)
	}
}

const runBody = `{"fileId":"m1","originalFilename":"cv.pdf","conversationId":"c1"`

// A run can name the profile it imports into; reading the run back (one, and the list) says so.
func TestPostWithATargetProfileRecordsItOnAGeneratingRun(t *testing.T) {
	setup(t)
	newProfile(t, "p-1")
	id := createViaAPI(t, runBody+`,"targetProfileId":"p-1"}`)
	if r := runOf(t, id); r.TargetProfileID != "p-1" || r.Status != "generating" {
		t.Errorf("%+v", r)
	}
	one := decode(t, call(http.MethodGet, "/cv-import/runs?id="+id, ""))["run"].(map[string]any)
	if one["targetProfileId"] != "p-1" {
		t.Errorf("one: %v", one)
	}
	list := decode(t, call(http.MethodGet, "/cv-import/runs", ""))["runs"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["targetProfileId"] != "p-1" {
		t.Errorf("list: %v", list)
	}
}

func TestPostWithATargetProfileAndAProposalRecordsItOnAReadyRun(t *testing.T) {
	setup(t)
	newProfile(t, "p-1")
	id := createViaAPI(t, runBody+`,"aiProposal":{"profile":{"action":"skip"}},"targetProfileId":"p-1"}`)
	if r := runOf(t, id); r.TargetProfileID != "p-1" || r.Status != "ready" {
		t.Errorf("%+v", r)
	}
}

// The code driver and a target together.
func TestPostWithATargetKeepsTheDriver(t *testing.T) {
	setup(t)
	newProfile(t, "p-1")
	id := createViaAPI(t, runBody+`,"driver":"code","targetProfileId":"p-1"}`)
	if r := runOf(t, id); r.Driver != "code" || r.TargetProfileID != "p-1" {
		t.Errorf("%+v", r)
	}
}

// Without one (absent, empty, blank) the run has no target and nothing is said about it in the JSON.
func TestPostWithoutATargetLeavesItOut(t *testing.T) {
	setup(t)
	for _, extra := range []string{``, `,"targetProfileId":""`, `,"targetProfileId":"   "`, `,"targetProfileId":null`} {
		id := createViaAPI(t, runBody+extra+`}`)
		if r := runOf(t, id); r.TargetProfileID != "" {
			t.Errorf("%q: %+v", extra, r)
		}
		one := decode(t, call(http.MethodGet, "/cv-import/runs?id="+id, ""))["run"].(map[string]any)
		if _, there := one["targetProfileId"]; there {
			t.Errorf("%q: the JSON names a target: %v", extra, one)
		}
		var raw any
		if err := db.CareerDB.QueryRow(`SELECT target_profile_id FROM cv_import_runs WHERE id = ?`, id).Scan(&raw); err != nil || raw != nil {
			t.Errorf("%q: stored %v (%v), want NULL", extra, raw, err)
		}
	}
}

// A profile that does not exist is refused and nothing is recorded.
func TestPostWithAnUnknownTargetProfileIsRefused(t *testing.T) {
	setup(t)
	newProfile(t, "p-1")
	for _, body := range []string{
		runBody + `,"targetProfileId":"nope"}`,
		runBody + `,"aiProposal":{"profile":{}},"targetProfileId":"nope"}`,
		runBody + `,"targetProfileId":"P-1"}`,
	} {
		rec := call(http.MethodPost, "/cv-import/runs", body)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "that profile does not exist") {
			t.Errorf("%s: %d %s", body, rec.Code, rec.Body.String())
		}
	}
	var n int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM cv_import_runs`).Scan(&n); err != nil || n != 0 {
		t.Errorf("runs recorded: %d (%v)", n, err)
	}
}

// The id is trimmed before it is checked and stored.
func TestPostTrimsTheTargetProfileID(t *testing.T) {
	setup(t)
	newProfile(t, "p-1")
	id := createViaAPI(t, runBody+`,"targetProfileId":"  p-1 "}`)
	if r := runOf(t, id); r.TargetProfileID != "p-1" {
		t.Errorf("%+v", r)
	}
}

// A profile deleted after the run was made is still named by the run: the page tells that apart from "no target".
func TestARunKeepsItsTargetWhenTheProfileIsDeleted(t *testing.T) {
	setup(t)
	newProfile(t, "p-1")
	id := createViaAPI(t, runBody+`,"targetProfileId":"p-1"}`)
	if _, err := db.CareerDB.Exec(`DELETE FROM profiles WHERE id = 'p-1'`); err != nil {
		t.Fatal(err)
	}
	if r := runOf(t, id); r.TargetProfileID != "p-1" {
		t.Errorf("%+v", r)
	}
}

// Finishing a run and recording its summary leave the target alone.
func TestTheTargetSurvivesFinishingAndTheSaveSummary(t *testing.T) {
	setup(t)
	newProfile(t, "p-1")
	id := createViaAPI(t, runBody+`,"aiProposal":{"profile":{}},"targetProfileId":"p-1"}`)
	if rec := call(http.MethodPut, "/cv-import/runs", `{"id":"`+id+`","saveSummary":{"ok":true}}`); rec.Code != http.StatusNoContent {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if r := runOf(t, id); r.TargetProfileID != "p-1" {
		t.Errorf("%+v", r)
	}
}
