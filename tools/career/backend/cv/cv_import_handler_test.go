package cv

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func call(method, target, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	CVImportRunsHandler(rec, httptest.NewRequest(method, target, strings.NewReader(body)))
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("%d %q: %v", rec.Code, rec.Body.String(), err)
	}
	return m
}

func createViaAPI(t *testing.T, body string) string {
	t.Helper()
	rec := call(http.MethodPost, "/cv-import/runs", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	return decode(t, rec)["id"].(string)
}

// ---- POST ----

func TestPostWithoutAProposalStartsAGeneratingRun(t *testing.T) {
	setup(t)
	for _, body := range []string{
		`{"fileId":"m1","originalFilename":"cv.pdf","conversationId":"c1"}`,
		`{"fileId":"m1","originalFilename":"cv.pdf","conversationId":"c1","aiProposal":null}`,
		`{"fileId":"m1","originalFilename":"cv.pdf","conversationId":"c1","aiProposal":  null  }`,
	} {
		id := createViaAPI(t, body)
		r := runOf(t, id)
		if r.Status != "generating" || string(r.AIProposal) != "null" || r.MediaFileID != "m1" || r.ConversationID != "c1" || r.OriginalFilename != "cv.pdf" {
			t.Errorf("%s: %+v", body, r)
		}
	}
}

// The way the page recorded a finished analysis before still works, and gives a ready run.
func TestPostWithAProposalStillRecordsAReadyRun(t *testing.T) {
	setup(t)
	id := createViaAPI(t, `{"fileId":"m1","originalFilename":"cv.pdf","conversationId":"c1","aiProposal":{"profile":{"action":"insert"}}}`)
	if r := runOf(t, id); r.Status != "ready" || !strings.Contains(string(r.AIProposal), `"profile"`) {
		t.Errorf("%+v", r)
	}
}

func TestPostNeedsTheFileAndTheConversation(t *testing.T) {
	setup(t)
	for name, body := range map[string]string{
		"no file":         `{"conversationId":"c1"}`,
		"no conversation": `{"fileId":"m1"}`,
		"empty":           `{}`,
		"not json":        `nope`,
	} {
		if rec := call(http.MethodPost, "/cv-import/runs", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	if count(t, `SELECT COUNT(*) FROM cv_import_runs`) != 0 {
		t.Error("a refused request created a run")
	}
}

// ---- PUT: finishing ----

func finishBody(id, status, errText string) string {
	b, _ := json.Marshal(map[string]string{"id": id, "status": status, "error": errText})
	return string(b)
}

func TestPutFinishesARunWithItsProposal(t *testing.T) {
	setup(t)
	id := createViaAPI(t, `{"fileId":"m1","originalFilename":"cv.pdf","conversationId":"c1"}`)
	if err := StoreProposal(id, okProposal()); err != nil {
		t.Fatal(err)
	}
	rec := call(http.MethodPut, "/cv-import/runs", finishBody(id, "ready", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	run := decode(t, rec)["run"].(map[string]any)
	proposal, _ := run["aiProposal"].(map[string]any)
	if run["id"] != id || run["status"] != "ready" || proposal["profile"] == nil {
		t.Errorf("%v", run)
	}
}

func TestPutFinishesARunAsFailedWithItsReason(t *testing.T) {
	setup(t)
	id := createViaAPI(t, `{"fileId":"m1","originalFilename":"cv.pdf","conversationId":"c1"}`)
	rec := call(http.MethodPut, "/cv-import/runs", finishBody(id, "failed", "the AI did not answer"))
	run := decode(t, rec)["run"].(map[string]any)
	if rec.Code != http.StatusOK || run["status"] != "failed" || run["error"] != "the AI did not answer" {
		t.Errorf("%d %v", rec.Code, run)
	}
}

func TestPutRefusesAnImpossibleFinish(t *testing.T) {
	setup(t)
	noProposal := createViaAPI(t, `{"fileId":"m1","originalFilename":"cv.pdf","conversationId":"c1"}`)
	finished := createViaAPI(t, `{"fileId":"m2","originalFilename":"b.pdf","conversationId":"c2"}`)
	_ = StoreProposal(finished, okProposal())
	call(http.MethodPut, "/cv-import/runs", finishBody(finished, "ready", ""))
	for name, tc := range map[string]struct {
		body string
		code int
	}{
		"ready without a proposal": {finishBody(noProposal, "ready", ""), http.StatusBadRequest},
		"failed without a reason":  {finishBody(noProposal, "failed", ""), http.StatusBadRequest},
		"an unknown status":        {finishBody(noProposal, "done", "x"), http.StatusBadRequest},
		"an unknown run":           {finishBody("nope", "failed", "x"), http.StatusNotFound},
		"a finished run again":     {finishBody(finished, "failed", "x"), http.StatusConflict},
		"status and saveSummary":   {`{"id":"` + noProposal + `","status":"failed","error":"x","saveSummary":{"a":1}}`, http.StatusBadRequest},
		"no id":                    {`{"status":"failed","error":"x"}`, http.StatusBadRequest},
		"not json":                 {`nope`, http.StatusBadRequest},
	} {
		if rec := call(http.MethodPut, "/cv-import/runs", tc.body); rec.Code != tc.code {
			t.Errorf("%s: %d %q, want %d", name, rec.Code, strings.TrimSpace(rec.Body.String()), tc.code)
		}
	}
	if runOf(t, noProposal).Status != "generating" || runOf(t, finished).Status != "ready" {
		t.Error("a refused request changed a run")
	}
}

// ---- PUT: the review's outcome, as before ----

func TestPutStillRecordsTheReviewOutcome(t *testing.T) {
	setup(t)
	id, _ := createCVImportRun("m", "f", "c", []byte(`{}`))
	rec := call(http.MethodPut, "/cv-import/runs", `{"id":"`+id+`","saveSummary":{"inserted":3}}`)
	if rec.Code != http.StatusNoContent || string(runOf(t, id).SaveSummary) != `{"inserted":3}` {
		t.Errorf("%d %s", rec.Code, runOf(t, id).SaveSummary)
	}
	for name, tc := range map[string]struct {
		body string
		code int
	}{
		"unknown id": {`{"id":"nope","saveSummary":{"a":1}}`, http.StatusBadRequest},
		"no summary": {`{"id":"` + id + `"}`, http.StatusBadRequest},
		"no id":      {`{"saveSummary":{"a":1}}`, http.StatusBadRequest},
		"not json":   {`nope`, http.StatusBadRequest},
	} {
		if rec := call(http.MethodPut, "/cv-import/runs", tc.body); rec.Code != tc.code {
			t.Errorf("%s: %d, want %d", name, rec.Code, tc.code)
		}
	}
}

// ---- GET and DELETE ----

func TestGetListsTheRunsWithTheirStatus(t *testing.T) {
	setup(t)
	g := createViaAPI(t, `{"fileId":"m1","originalFilename":"a.pdf","conversationId":"c1"}`)
	r := createViaAPI(t, `{"fileId":"m2","originalFilename":"b.pdf","conversationId":"c2","aiProposal":{}}`)
	call(http.MethodPut, "/cv-import/runs", finishBody(g, "failed", "boom"))
	rec := call(http.MethodGet, "/cv-import/runs", "")
	runs := decode(t, rec)["runs"].([]any)
	byID := map[string]map[string]any{}
	for _, x := range runs {
		m := x.(map[string]any)
		byID[m["id"].(string)] = m
	}
	if rec.Code != http.StatusOK || len(runs) != 2 || byID[g]["status"] != "failed" || byID[g]["error"] != "boom" || byID[r]["status"] != "ready" {
		t.Errorf("%d %v", rec.Code, byID)
	}
}

func TestDeleteRemovesTheRunAndAsksMediaToDropTheFile(t *testing.T) {
	setup(t)
	var mu sync.Mutex
	var deleted []string
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		deleted = append(deleted, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer core.Close()
	t.Setenv("CORE_API_URL", core.URL)
	id := createViaAPI(t, `{"fileId":"media-9","originalFilename":"a.pdf","conversationId":"c1"}`)
	if rec := call(http.MethodDelete, "/cv-import/runs?id="+id, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("%d", rec.Code)
	}
	mu.Lock()
	defer mu.Unlock()
	if count(t, `SELECT COUNT(*) FROM cv_import_runs`) != 0 || len(deleted) != 1 || deleted[0] != "DELETE /api/v1/media/media-9" {
		t.Errorf("%v", deleted)
	}
	if rec := call(http.MethodDelete, "/cv-import/runs?id="+id, ""); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown: %d", rec.Code)
	}
	if rec := call(http.MethodDelete, "/cv-import/runs", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("no id: %d", rec.Code)
	}
}

func TestOtherMethodsAreRefused(t *testing.T) {
	setup(t)
	if rec := call(http.MethodPatch, "/cv-import/runs", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("%d", rec.Code)
	}
}

// ---- GET one run (plan 166) ----

func TestGetByIdReturnsThatRunAndTheListIsUnchanged(t *testing.T) {
	setup(t)
	first := createViaAPI(t, `{"fileId":"m1","originalFilename":"a.pdf","conversationId":"c1","aiProposal":{"profile":{"action":"insert"}}}`)
	second := createViaAPI(t, `{"fileId":"m2","originalFilename":"b.pdf","conversationId":"c2"}`)

	rec := call(http.MethodGet, "/cv-import/runs?id="+first, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	run := decode(t, rec)["run"].(map[string]any)
	if run["id"] != first || run["originalFilename"] != "a.pdf" || run["status"] != "ready" || run["aiProposal"] == nil {
		t.Errorf("run %v", run)
	}
	if _, has := decode(t, call(http.MethodGet, "/cv-import/runs?id="+first, ""))["runs"]; has {
		t.Error("a read by id answered with the list")
	}
	other := decode(t, call(http.MethodGet, "/cv-import/runs?id="+second, ""))["run"].(map[string]any)
	if other["id"] != second || other["status"] != "generating" {
		t.Errorf("second run %v", other)
	}

	list := decode(t, call(http.MethodGet, "/cv-import/runs", ""))["runs"].([]any)
	if len(list) != 2 {
		t.Errorf("the list has %d runs, want 2", len(list))
	}
}

func TestGetByIdRefusesAnUnknownOrMalformedId(t *testing.T) {
	setup(t)
	createViaAPI(t, `{"fileId":"m1","originalFilename":"a.pdf","conversationId":"c1"}`)
	for target, want := range map[string]int{
		"/cv-import/runs?id=" + "5d1c9a0e-3f43-4b64-9a63-0d3f3b1c9a11": http.StatusNotFound,
		"/cv-import/runs?id=not-a-uuid":                                http.StatusBadRequest,
		"/cv-import/runs?id=%27%20OR%201%3D1%20--":                     http.StatusBadRequest,
	} {
		if rec := call(http.MethodGet, target, ""); rec.Code != want {
			t.Errorf("%s: %d %q, want %d", target, rec.Code, rec.Body.String(), want)
		}
	}
}

func TestGetByIdSeesADeletedRunAsUnknown(t *testing.T) {
	setup(t)
	id := createViaAPI(t, `{"fileId":"m1","originalFilename":"a.pdf","conversationId":"c1"}`)
	if _, err := deleteCVImportRun(id); err != nil {
		t.Fatal(err)
	}
	if rec := call(http.MethodGet, "/cv-import/runs?id="+id, ""); rec.Code != http.StatusNotFound {
		t.Errorf("%d", rec.Code)
	}
}
