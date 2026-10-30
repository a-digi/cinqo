package cvbuilder

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"career-tool-backend/cvbuilder/templates"
	"career-tool-backend/db"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

func setupCvDB(t *testing.T) (personaID string) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatalf("InitDatabases: %v", err)
	}
	t.Cleanup(func() {
		UseJobLinks(nil)
		db.JobsDB.Close()
		db.CareerDB.Close()
	})
	profileID, err := profile.CreateProfile("Ada", "Lovelace")
	if err != nil {
		t.Fatal(err)
	}
	personaID, err = persona.CreatePersona(profileID, "Backend", "")
	if err != nil {
		t.Fatal(err)
	}
	return personaID
}

func insertDoc(t *testing.T, personaID, title, mediaFileID, createdAt string) CvDocument {
	t.Helper()
	doc, err := InsertCvDocument(personaID, "classic", title, templates.CvData{}, mediaFileID, "")
	if err != nil {
		t.Fatal(err)
	}
	if createdAt != "" {
		if _, err := db.CareerDB.Exec(`UPDATE cv_documents SET created_at = ? WHERE id = ?`, createdAt, doc.ID); err != nil {
			t.Fatal(err)
		}
	}
	return doc
}

func TestListAllCvDocumentsAcrossPersonasNewestFirstWithPersonaNames(t *testing.T) {
	first := setupCvDB(t)
	profileID, _ := profile.CreateProfile("Grace", "Hopper")
	second, err := persona.CreatePersona(profileID, "Data", "")
	if err != nil {
		t.Fatal(err)
	}
	insertDoc(t, first, "Old", "m1", "2026-01-01 10:00:00")
	insertDoc(t, second, "Newest", "m2", "2026-03-01 10:00:00")
	insertDoc(t, first, "Middle", "m3", "2026-02-01 10:00:00")

	docs, err := ListAllCvDocuments()
	if err != nil || len(docs) != 3 {
		t.Fatalf("%d %v", len(docs), err)
	}
	if docs[0].Title != "Newest" || docs[1].Title != "Middle" || docs[2].Title != "Old" {
		t.Errorf("order: %s %s %s", docs[0].Title, docs[1].Title, docs[2].Title)
	}
	if docs[0].PersonaName != "Data" || docs[1].PersonaName != "Backend" {
		t.Errorf("persona names: %q %q", docs[0].PersonaName, docs[1].PersonaName)
	}
	// the per-persona list is unchanged and has no persona name
	own, err := ListCvDocuments(first)
	if err != nil || len(own) != 2 || own[0].PersonaName != "" {
		t.Errorf("%+v %v", own, err)
	}
}

func TestListAllCvDocumentsIsCapped(t *testing.T) {
	personaID := setupCvDB(t)
	for i := 0; i < MaxAllCvDocuments+5; i++ {
		insertDoc(t, personaID, fmt.Sprintf("CV %03d", i), "m", "")
	}
	docs, err := ListAllCvDocuments()
	if err != nil || len(docs) != MaxAllCvDocuments {
		t.Errorf("%d %v", len(docs), err)
	}
}

func TestGetAllOverHTTP(t *testing.T) {
	personaID := setupCvDB(t)
	insertDoc(t, personaID, "CV", "m1", "")
	rec := httptest.NewRecorder()
	DocumentsHandler(rec, httptest.NewRequest(http.MethodGet, "/cv-documents?all=1", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"personaName":"Backend"`) {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
	// without personaId and without all it is still a 400
	rec = httptest.NewRecorder()
	DocumentsHandler(rec, httptest.NewRequest(http.MethodGet, "/cv-documents", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("%d", rec.Code)
	}
}

// fakeLinks records what the delete flow asks of the jobs side.
type fakeLinks struct {
	mu       sync.Mutex
	using    []JobRef
	unlinked []string
	failWith error
}

func (f *fakeLinks) JobsUsing(docID, mediaFileID string) ([]JobRef, error) {
	return f.using, f.failWith
}
func (f *fakeLinks) UnlinkDocument(docID, mediaFileID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unlinked = append(f.unlinked, docID)
	return nil
}

// mediaDeletes points the core at a local server and records Media deletes.
func mediaDeletes(t *testing.T) *[]string {
	t.Helper()
	var mu sync.Mutex
	got := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			mu.Lock()
			got = append(got, r.URL.Path)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	return &got
}

func deleteDoc(id, query string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	DocumentsHandler(rec, httptest.NewRequest(http.MethodDelete, "/cv-documents?id="+id+query, nil))
	return rec
}

func docExists(id string) bool {
	_, err := GetCvDocument(id)
	return err == nil
}

func TestDeletingAnUnusedCvWorksAsBefore(t *testing.T) {
	personaID := setupCvDB(t)
	deletes := mediaDeletes(t)
	UseJobLinks(&fakeLinks{})
	doc := insertDoc(t, personaID, "CV", "media-1", "")
	if rec := deleteDoc(doc.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if docExists(doc.ID) || len(*deletes) != 1 || (*deletes)[0] != "/api/v1/media/media-1" {
		t.Errorf("exists=%v deletes=%v", docExists(doc.ID), *deletes)
	}
}

func TestDeletingACvThatJobsUseAsksFirstAndListsEveryJob(t *testing.T) {
	personaID := setupCvDB(t)
	deletes := mediaDeletes(t)
	var refs []JobRef
	for i := 0; i < 5; i++ {
		refs = append(refs, JobRef{ID: fmt.Sprintf("j%d", i), Title: fmt.Sprintf("Job %d", i), Company: "Co"})
	}
	links := &fakeLinks{using: refs}
	UseJobLinks(links)
	doc := insertDoc(t, personaID, "CV", "media-1", "")

	rec := deleteDoc(doc.ID, "")
	if rec.Code != http.StatusConflict || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Error string   `json:"error"`
		Jobs  []JobRef `json:"jobs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "in_use" || len(body.Jobs) != 5 || body.Jobs[4].Title != "Job 4" {
		t.Errorf("every job must be listed: %+v", body)
	}
	if !docExists(doc.ID) || len(links.unlinked) != 0 || len(*deletes) != 0 {
		t.Errorf("nothing may change before the confirmation: exists=%v unlinked=%v deletes=%v", docExists(doc.ID), links.unlinked, *deletes)
	}
	// anything but unlinkJobs=true is not a confirmation
	if rec := deleteDoc(doc.ID, "&unlinkJobs=1"); rec.Code != http.StatusConflict {
		t.Errorf("unlinkJobs=1: %d", rec.Code)
	}
	if rec := deleteDoc(doc.ID, "&unlinkJobs=false"); rec.Code != http.StatusConflict {
		t.Errorf("unlinkJobs=false: %d", rec.Code)
	}
}

func TestConfirmingUnlinksTheJobsThenDeletesTheCvAndItsFile(t *testing.T) {
	personaID := setupCvDB(t)
	deletes := mediaDeletes(t)
	links := &fakeLinks{using: []JobRef{{ID: "j1", Title: "A"}, {ID: "j2", Title: "B"}}}
	UseJobLinks(links)
	doc := insertDoc(t, personaID, "CV", "media-1", "")

	if rec := deleteDoc(doc.ID, "&unlinkJobs=true"); rec.Code != http.StatusNoContent {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if len(links.unlinked) != 1 || links.unlinked[0] != doc.ID || docExists(doc.ID) || len(*deletes) != 1 {
		t.Errorf("unlinked=%v exists=%v deletes=%v", links.unlinked, docExists(doc.ID), *deletes)
	}
}

func TestDeleteErrorsAndTheNilLinksCase(t *testing.T) {
	personaID := setupCvDB(t)
	mediaDeletes(t)

	// no JobLinks registered: deletes as before
	doc := insertDoc(t, personaID, "CV", "media-1", "")
	if rec := deleteDoc(doc.ID, ""); rec.Code != http.StatusNoContent {
		t.Errorf("nil links: %d", rec.Code)
	}

	UseJobLinks(&fakeLinks{})
	if rec := deleteDoc("nope", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown id: %d", rec.Code)
	}
	if rec := deleteDoc("", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("no id: %d", rec.Code)
	}

	// a failing usage check must not delete anything
	doc2 := insertDoc(t, personaID, "CV 2", "media-2", "")
	UseJobLinks(&fakeLinks{failWith: fmt.Errorf("db down")})
	if rec := deleteDoc(doc2.ID, "&unlinkJobs=true"); rec.Code != http.StatusInternalServerError || !docExists(doc2.ID) {
		t.Errorf("usage check failed: %d exists=%v", rec.Code, docExists(doc2.ID))
	}
}
