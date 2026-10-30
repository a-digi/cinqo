package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/cvbuilder"
	"career-tool-backend/cvbuilder/templates"
	"career-tool-backend/jobs"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

func seedLinkFixture(t *testing.T) (profileID, docID, noPdfDocID string) {
	t.Helper()
	setupHandlerDB(t)
	profileID, err := profile.CreateProfile("Ada", "Lovelace")
	if err != nil {
		t.Fatal(err)
	}
	personaID, err := persona.CreatePersona(profileID, "Backend", "")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := cvbuilder.InsertCvDocument(personaID, "classic", "CV", templates.CvData{}, "media-1", "")
	if err != nil {
		t.Fatal(err)
	}
	noPdf, err := cvbuilder.InsertCvDocument(personaID, "classic", "No PDF", templates.CvData{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return profileID, doc.ID, noPdf.ID
}

func addJob(t *testing.T, title, company string) string {
	t.Helper()
	rec := postJob(`{"title":"` + title + `","company":"` + company + `"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Job.ID
}

func link(method, target, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	cvLinkHandler(rec, httptest.NewRequest(method, target, strings.NewReader(body)))
	return rec
}

func jobJSON(t *testing.T, id string) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	jobsHandler(rec, httptest.NewRequest(http.MethodGet, "/jobs?id="+id, nil))
	var out struct {
		Job map[string]any `json:"job"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Job == nil {
		t.Fatalf("%v: %s", err, rec.Body.String())
	}
	return out.Job
}

func TestPutCvLinkAttachesTheCvAndTheJobJSONShowsIt(t *testing.T) {
	_, docID, _ := seedLinkFixture(t)
	jobID := addJob(t, "Backend Engineer", "Acme")

	rec := link(http.MethodPut, "/jobs/cv-link", `{"jobId":"`+jobID+`","cvDocumentId":"`+docID+`","source":"existing"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	j := jobJSON(t, jobID)
	if j["cvStatus"] != "completed" || j["cvMediaFileId"] != "media-1" || j["cvSource"] != "existing" || j["cvDocumentId"] != docID {
		t.Errorf("%v", j)
	}
	// again: idempotent
	if rec := link(http.MethodPut, "/jobs/cv-link", `{"jobId":"`+jobID+`","cvDocumentId":"`+docID+`","source":"existing"}`); rec.Code != http.StatusNoContent {
		t.Errorf("%d", rec.Code)
	}
}

func TestPutCvLinkStatuses(t *testing.T) {
	profileID, docID, noPdfID := seedLinkFixture(t)
	jobID := addJob(t, "Dev", "")
	body := func(job, doc, source string) string {
		return `{"jobId":"` + job + `","cvDocumentId":"` + doc + `","source":"` + source + `"}`
	}
	cases := []struct {
		name string
		body string
		want int
	}{
		{"not json", `{`, http.StatusBadRequest},
		{"no job id", body("", docID, "existing"), http.StatusBadRequest},
		{"no document id", body(jobID, "", "existing"), http.StatusBadRequest},
		{"bad source", body(jobID, docID, "ai"), http.StatusBadRequest},
		{"unknown job", body("nope", docID, "existing"), http.StatusNotFound},
		{"unknown document", body(jobID, "nope", "existing"), http.StatusNotFound},
		{"document without a PDF", body(jobID, noPdfID, "existing"), http.StatusUnprocessableEntity},
	}
	for _, c := range cases {
		if rec := link(http.MethodPut, "/jobs/cv-link", c.body); rec.Code != c.want {
			t.Errorf("%s: %d %s", c.name, rec.Code, rec.Body.String())
		}
	}
	if rec := link(http.MethodPost, "/jobs/cv-link", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", rec.Code)
	}

	if err := jobs.StartCvGeneration(jobID, profileID, "conv"); err != nil {
		t.Fatal(err)
	}
	if rec := link(http.MethodPut, "/jobs/cv-link", body(jobID, docID, "existing")); rec.Code != http.StatusConflict {
		t.Errorf("while generating: %d", rec.Code)
	}
	if rec := link(http.MethodDelete, "/jobs/cv-link?jobId="+jobID, ""); rec.Code != http.StatusConflict {
		t.Errorf("DELETE while generating: %d", rec.Code)
	}
}

func TestDeleteCvLinkRemovesTheCvFromTheJobOnly(t *testing.T) {
	_, docID, _ := seedLinkFixture(t)
	jobID := addJob(t, "Dev", "")
	link(http.MethodPut, "/jobs/cv-link", `{"jobId":"`+jobID+`","cvDocumentId":"`+docID+`","source":"manual"}`)

	if rec := link(http.MethodDelete, "/jobs/cv-link?jobId="+jobID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	j := jobJSON(t, jobID)
	if _, has := j["cvStatus"]; has {
		t.Errorf("the job must have no CV any more: %v", j)
	}
	if _, err := cvbuilder.GetCvDocument(docID); err != nil {
		t.Errorf("the CV document must stay: %v", err)
	}
	// idempotent, and the error cases
	if rec := link(http.MethodDelete, "/jobs/cv-link?jobId="+jobID, ""); rec.Code != http.StatusNoContent {
		t.Errorf("again: %d", rec.Code)
	}
	if rec := link(http.MethodDelete, "/jobs/cv-link?jobId=nope", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown job: %d", rec.Code)
	}
	if rec := link(http.MethodDelete, "/jobs/cv-link", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("no jobId: %d", rec.Code)
	}
}

// The whole delete-a-shared-CV flow, wired the way main.go wires it: the jobs
// implementation behind cvbuilder's JobLinks.
func TestDeletingASharedCvListsEveryJobThenUnlinksThemAll(t *testing.T) {
	_, docID, _ := seedLinkFixture(t)
	cvbuilder.UseJobLinks(jobs.DocumentLinks{})
	t.Cleanup(func() { cvbuilder.UseJobLinks(nil) })
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer core.Close()
	t.Setenv("CORE_API_URL", core.URL)

	a, b, c := addJob(t, "Backend Engineer", "Acme"), addJob(t, "Platform Engineer", "Globex"), addJob(t, "Unrelated", "")
	link(http.MethodPut, "/jobs/cv-link", `{"jobId":"`+a+`","cvDocumentId":"`+docID+`","source":"manual"}`)
	link(http.MethodPut, "/jobs/cv-link", `{"jobId":"`+b+`","cvDocumentId":"`+docID+`","source":"existing"}`)

	del := func(q string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		cvbuilder.DocumentsHandler(rec, httptest.NewRequest(http.MethodDelete, "/cv-documents?id="+docID+q, nil))
		return rec
	}
	rec := del("")
	if rec.Code != http.StatusConflict {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Jobs []cvbuilder.JobRef `json:"jobs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Jobs) != 2 ||
		body.Jobs[0].Title != "Backend Engineer" || body.Jobs[0].Company != "Acme" || body.Jobs[1].Title != "Platform Engineer" {
		t.Fatalf("%v %+v", err, body)
	}
	if _, err := cvbuilder.GetCvDocument(docID); err != nil {
		t.Fatal("nothing may be deleted before the confirmation")
	}

	if rec := del("&unlinkJobs=true"); rec.Code != http.StatusNoContent {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	for _, id := range []string{a, b} {
		if _, has := jobJSON(t, id)["cvStatus"]; has {
			t.Errorf("job %s still has the CV", id)
		}
	}
	if _, has := jobJSON(t, c)["cvStatus"]; has {
		t.Error("the unrelated job must be untouched")
	}
	if _, err := cvbuilder.GetCvDocument(docID); err == nil {
		t.Error("the document must be deleted")
	}
}
