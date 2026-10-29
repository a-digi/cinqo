package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func postJob(body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/jobs", strings.NewReader(body))
	rec := httptest.NewRecorder()
	jobsHandler(rec, req)
	return rec
}

func TestPostJobsCreatesAManualJob(t *testing.T) {
	setupHandlerDB(t)
	rec := postJob(`{"title":"Backend Engineer","company":"","location":"Hamburg","description":"Build things","postedAt":"2026-10-29","sourceUrl":"https://example.com/jobs/1"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Job struct {
			ID        string `json:"id"`
			Title     string `json:"title"`
			Source    string `json:"source"`
			SourceURL string `json:"sourceUrl"`
			Location  string `json:"location"`
		} `json:"job"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Job.ID == "" || resp.Job.Title != "Backend Engineer" || resp.Job.Source != "manual" || resp.Job.SourceURL != "https://example.com/jobs/1" || resp.Job.Location != "Hamburg" {
		t.Errorf("%+v", resp.Job)
	}

	// It is there for the list the Jobs page reads.
	list := httptest.NewRecorder()
	jobsHandler(list, httptest.NewRequest(http.MethodGet, "/jobs", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"source":"manual"`) || !strings.Contains(list.Body.String(), "Backend Engineer") {
		t.Errorf("list: %d %s", list.Code, list.Body.String())
	}
}

func TestPostJobsWithoutAURLNeverShowsThePlaceholder(t *testing.T) {
	setupHandlerDB(t)
	rec := postJob(`{"title":"Heard about it from a friend"}`)
	if rec.Code != http.StatusCreated || strings.Contains(rec.Body.String(), "manual:") || !strings.Contains(rec.Body.String(), `"sourceUrl":""`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestPostJobsAnswers400WithTheReason(t *testing.T) {
	setupHandlerDB(t)
	for name, c := range map[string]struct{ body, contains string }{
		"no title":             {`{"company":"Acme"}`, "title: is required"},
		"bad url":              {`{"title":"t","sourceUrl":"javascript:alert(1)"}`, "sourceUrl: must be an absolute http or https URL"},
		"relative url":         {`{"title":"t","sourceUrl":"/jobs/1"}`, "sourceUrl"},
		"title too long":       {`{"title":"` + strings.Repeat("a", 301) + `"}`, "title: must be at most 300 characters"},
		"not json":             {`nope`, "invalid request body"},
		"wrong field type":     {`{"title": 5}`, "invalid request body"},
		"body over a megabyte": {`{"title":"t","description":"` + strings.Repeat("a", 1<<20) + `"}`, "invalid request body"},
	} {
		rec := postJob(c.body)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), c.contains) {
			t.Errorf("%s: %d %q (want 400 containing %q)", name, rec.Code, rec.Body.String(), c.contains)
		}
	}
	list := httptest.NewRecorder()
	jobsHandler(list, httptest.NewRequest(http.MethodGet, "/jobs", nil))
	if strings.Contains(list.Body.String(), `"title"`) {
		t.Errorf("a rejected request must store nothing: %s", list.Body.String())
	}
}

func TestPostJobsAnswers409WithTheExistingJob(t *testing.T) {
	setupHandlerDB(t)
	first := postJob(`{"title":"Backend Engineer","company":"Acme","sourceUrl":"https://example.com/jobs/1"}`)
	var created struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
	}
	_ = json.Unmarshal(first.Body.Bytes(), &created)

	for name, body := range map[string]string{
		"same URL":           `{"title":"Another title","sourceUrl":"https://example.com/jobs/1"}`,
		"same title+company": `{"title":"backend engineer","company":"ACME"}`,
	} {
		rec := postJob(body)
		if rec.Code != http.StatusConflict {
			t.Fatalf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
		var dup map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &dup)
		if dup["error"] != "duplicate" || dup["existingJobId"] != created.Job.ID || dup["message"] == "" {
			t.Errorf("%s: %v", name, dup)
		}
	}
}

func TestOtherJobsMethodsAreUnchanged(t *testing.T) {
	setupHandlerDB(t)
	rec := postJob(`{"title":"To delete"}`)
	var created struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	del := httptest.NewRecorder()
	jobsHandler(del, httptest.NewRequest(http.MethodDelete, "/jobs?id="+created.Job.ID, nil))
	if del.Code != http.StatusNoContent {
		t.Errorf("DELETE: %d", del.Code)
	}
	if put := httptest.NewRecorder(); func() int {
		jobsHandler(put, httptest.NewRequest(http.MethodPut, "/jobs", strings.NewReader(`{}`)))
		return put.Code
	}() != http.StatusBadRequest {
		t.Errorf("PUT without an id is still a 400: %d", put.Code)
	}
}

func TestManifestDeclaresPostJobsWithTheJobsScope(t *testing.T) {
	raw, err := readManifestRoutes()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range raw {
		if r["method"] == "POST" && r["path_suffix"] == "jobs" {
			if r["required_scope"] != "tool:career:jobs" {
				t.Errorf("scope = %q", r["required_scope"])
			}
			return
		}
	}
	t.Error("manifest.json does not declare POST jobs")
}

func readManifestRoutes() ([]map[string]string, error) {
	raw, err := os.ReadFile("../manifest.json")
	if err != nil {
		return nil, err
	}
	var m struct {
		Routes []map[string]string `json:"routes"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m.Routes, nil
}
