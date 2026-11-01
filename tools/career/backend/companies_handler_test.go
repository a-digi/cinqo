package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/db"
)

type companiesAnswer struct {
	Companies []struct {
		ID, Name string
	} `json:"companies"`
	Total   *int `json:"total"`
	Summary *struct {
		Companies, WithJobs, WithRecruiters, Jobs, Recruiters int
	} `json:"summary"`
}

func companiesGet(t *testing.T, query string) (int, companiesAnswer, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	companiesHandler(rec, httptest.NewRequest(http.MethodGet, "/companies"+query, nil))
	var a companiesAnswer
	_ = json.Unmarshal(rec.Body.Bytes(), &a)
	return rec.Code, a, rec.Body.String()
}

func seedCompanies(t *testing.T, n int) {
	t.Helper()
	setupHandlerDB(t)
	for i := 0; i < n; i++ {
		if _, err := db.JobsDB.Exec(`INSERT INTO companies (id, name, description, created_at) VALUES (?, ?, '', ?)`,
			fmt.Sprintf("id%03d", i), fmt.Sprintf("Company %03d", i), fmt.Sprintf("2026-06-01 10:%02d:00", i%60)); err != nil {
			t.Fatal(err)
		}
	}
}

// Without parameters every company comes back, as it always did; the answer now also carries the total and the overview.
func TestCompaniesWithoutParametersAreAllOfThem(t *testing.T) {
	seedCompanies(t, 7)
	code, a, body := companiesGet(t, "")
	if code != 200 || len(a.Companies) != 7 || a.Total == nil || *a.Total != 7 || a.Summary == nil || a.Summary.Companies != 7 {
		t.Fatalf("%d %s", code, body)
	}
	for _, key := range []string{`"companies"`, `"total"`, `"summary"`, `"withJobs"`, `"withRecruiters"`, `"jobs"`, `"recruiters"`} {
		if !strings.Contains(body, key) {
			t.Errorf("missing %s in %s", key, body)
		}
	}
}

func TestCompaniesPagedAndSearched(t *testing.T) {
	seedCompanies(t, 12)
	_, a, _ := companiesGet(t, "?limit=5&offset=5")
	if len(a.Companies) != 5 || a.Companies[0].Name != "Company 005" || *a.Total != 12 {
		t.Errorf("page 2 of 5: %+v total %d", a.Companies, *a.Total)
	}
	_, a, _ = companiesGet(t, "?limit=5&offset=10")
	if len(a.Companies) != 2 {
		t.Errorf("last page: %d", len(a.Companies))
	}
	_, a, _ = companiesGet(t, "?limit=5&offset=100")
	if a.Companies == nil || len(a.Companies) != 0 || *a.Total != 12 {
		t.Errorf("past the end: %+v", a)
	}
	_, a, _ = companiesGet(t, "?query=company%2000&limit=3")
	if len(a.Companies) != 3 || *a.Total != 10 {
		t.Errorf("search: %d of %d", len(a.Companies), *a.Total)
	}
	_, a, _ = companiesGet(t, "?query=%25")
	if len(a.Companies) != 0 || *a.Total != 0 {
		t.Errorf("a percent sign is a character: %+v", a)
	}
	_, a, _ = companiesGet(t, "?query=")
	if len(a.Companies) != 12 {
		t.Errorf("empty search: %d", len(a.Companies))
	}
}

func TestCompaniesLimitIsClampedAndOffsetForgiven(t *testing.T) {
	seedCompanies(t, 5)
	for query, want := range map[string]int{"?limit=0": 1, "?limit=-4": 1, "?limit=1": 1, "?limit=3": 3, "?limit=500": 5} {
		if code, a, body := companiesGet(t, query); code != 200 || len(a.Companies) != want {
			t.Errorf("%s: %d, %d companies (want %d) %s", query, code, len(a.Companies), want, body)
		}
	}
	if _, a, _ := companiesGet(t, "?limit=2&offset=-7"); len(a.Companies) != 2 || a.Companies[0].Name != "Company 000" {
		t.Errorf("negative offset: %+v", a.Companies)
	}
	// an offset without a limit is ignored (no limit means all)
	if _, a, _ := companiesGet(t, "?offset=3"); len(a.Companies) != 5 {
		t.Errorf("offset without limit: %d", len(a.Companies))
	}
}

func TestCompaniesRefuseParametersThatAreNotNumbers(t *testing.T) {
	seedCompanies(t, 3)
	for _, q := range []string{"?limit=abc", "?limit=2.5", "?limit=1e3", "?offset=x", "?offset=1.5", "?limit=%20"} {
		if code, _, _ := companiesGet(t, q); code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", q, code)
		}
	}
}

// Creating and updating still answer with the whole list: other callers rely on it.
func TestCompaniesPostAndPutStillAnswerWithTheWholeList(t *testing.T) {
	seedCompanies(t, 4)
	rec := httptest.NewRecorder()
	companiesHandler(rec, httptest.NewRequest(http.MethodPost, "/companies", strings.NewReader(`{"name":"New one","description":"d"}`)))
	var created struct {
		ID        string `json:"id"`
		Companies []struct{ ID string }
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if rec.Code != 200 || created.ID == "" || len(created.Companies) != 5 {
		t.Fatalf("POST: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	companiesHandler(rec, httptest.NewRequest(http.MethodPut, "/companies", strings.NewReader(fmt.Sprintf(`{"id":%q,"name":"Renamed"}`, created.ID))))
	var updated struct{ Companies []struct{ Name string } }
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if rec.Code != 200 || len(updated.Companies) != 5 {
		t.Fatalf("PUT: %d %s", rec.Code, rec.Body.String())
	}
}
