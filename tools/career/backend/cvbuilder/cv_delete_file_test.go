package cvbuilder

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeMedia is the core's Media API for these tests: it records each delete (and whether the CV row still existed at that moment) and
// answers with whatever status the test scripts for it, in order (the last one repeats).
type fakeMedia struct {
	mu          sync.Mutex
	paths       []string
	rowExisted  []bool
	auth        []string
	cookies     []string
	statuses    []int
	docIDToSeek string
}

func newFakeMedia(t *testing.T, docID string, statuses ...int) *fakeMedia {
	t.Helper()
	f := &fakeMedia{statuses: statuses, docIDToSeek: docID}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Method == http.MethodDelete {
			f.paths = append(f.paths, r.URL.Path)
			f.rowExisted = append(f.rowExisted, docExists(f.docIDToSeek))
			f.auth = append(f.auth, r.Header.Get("Authorization"))
			f.cookies = append(f.cookies, r.Header.Get("Cookie"))
		}
		i := len(f.paths) - 1
		if i >= len(f.statuses) {
			i = len(f.statuses) - 1
		}
		if i < 0 {
			i = 0
		}
		code := f.statuses[i]
		if code == http.StatusInternalServerError {
			_, _ = w.Write([]byte("media internals: disk exploded"))
		}
		w.WriteHeader(code)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	return f
}

func TestTheCvFileIsDeletedBeforeTheRow(t *testing.T) {
	personaID := setupCvDB(t)
	UseJobLinks(&fakeLinks{})
	doc := insertDoc(t, personaID, "CV", "media-1", "")
	media := newFakeMedia(t, doc.ID, http.StatusNoContent)

	if rec := deleteDoc(doc.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if len(media.paths) != 1 || media.paths[0] != "/api/v1/media/media-1" {
		t.Fatalf("media deletes = %v", media.paths)
	}
	if !media.rowExisted[0] {
		t.Error("the file must be deleted while the CV row still exists, so a failed file delete can keep the CV")
	}
	if docExists(doc.ID) {
		t.Error("the row must be gone after a successful delete")
	}
}

func TestWhenTheFileCannotBeDeletedNothingIsRemovedAndTheCallerIsTold(t *testing.T) {
	personaID := setupCvDB(t)
	UseJobLinks(&fakeLinks{})
	doc := insertDoc(t, personaID, "CV", "media-1", "")
	newFakeMedia(t, doc.ID, http.StatusInternalServerError)

	rec := deleteDoc(doc.ID, "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d, want 502: %s", rec.Code, rec.Body.String())
	}
	if !docExists(doc.ID) {
		t.Fatal("the CV must still be there")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "The CV file could not be deleted, so nothing was removed") {
		t.Errorf("the message must say what happened: %q", body)
	}
	if strings.Contains(body, "disk exploded") || strings.Contains(body, "media delete failed") || strings.Contains(body, "500") {
		t.Errorf("the message must not carry the Media API's own text: %q", body)
	}
}

func TestAMissingFileCountsAsDeletedAndARetryAfterAFailureWorks(t *testing.T) {
	personaID := setupCvDB(t)
	UseJobLinks(&fakeLinks{})
	doc := insertDoc(t, personaID, "CV", "media-1", "")
	// first the outage, then the file is reported missing (a half-finished earlier attempt), which is as good as deleted
	media := newFakeMedia(t, doc.ID, http.StatusInternalServerError, http.StatusNotFound)

	if rec := deleteDoc(doc.ID, ""); rec.Code != http.StatusBadGateway || !docExists(doc.ID) {
		t.Fatalf("first try: %d exists=%v", rec.Code, docExists(doc.ID))
	}
	if rec := deleteDoc(doc.ID, ""); rec.Code != http.StatusNoContent || docExists(doc.ID) {
		t.Fatalf("retry: %d exists=%v", rec.Code, docExists(doc.ID))
	}
	if len(media.paths) != 2 {
		t.Errorf("media deletes = %v", media.paths)
	}
}

func TestACvWithoutAPdfIsDeletedWithoutCallingMedia(t *testing.T) {
	personaID := setupCvDB(t)
	UseJobLinks(&fakeLinks{})
	doc := insertDoc(t, personaID, "No PDF", "", "")
	media := newFakeMedia(t, doc.ID, http.StatusInternalServerError) // would fail if it were called

	if rec := deleteDoc(doc.ID, ""); rec.Code != http.StatusNoContent || docExists(doc.ID) {
		t.Fatalf("%d exists=%v", rec.Code, docExists(doc.ID))
	}
	if len(media.paths) != 0 {
		t.Errorf("there is no file: media was called %v", media.paths)
	}
}

func TestAnUnknownCvDoesNotTouchMedia(t *testing.T) {
	setupCvDB(t)
	UseJobLinks(nil)
	media := newFakeMedia(t, "none", http.StatusNoContent)
	if rec := deleteDoc("nope", ""); rec.Code != http.StatusNotFound {
		t.Errorf("status %d", rec.Code)
	}
	if len(media.paths) != 0 {
		t.Errorf("media was called %v", media.paths)
	}
}

func TestAFailedFileDeleteAfterConfirmingTheJobsSaysTheJobsWereUnlinked(t *testing.T) {
	personaID := setupCvDB(t)
	links := &fakeLinks{using: []JobRef{{ID: "j1", Title: "A"}}}
	UseJobLinks(links)
	doc := insertDoc(t, personaID, "CV", "media-1", "")
	newFakeMedia(t, doc.ID, http.StatusInternalServerError)

	rec := deleteDoc(doc.ID, "&unlinkJobs=true")
	if rec.Code != http.StatusBadGateway || !docExists(doc.ID) {
		t.Fatalf("%d exists=%v", rec.Code, docExists(doc.ID))
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Its jobs were already unlinked") || strings.Contains(body, "nothing was removed") {
		t.Errorf("the message must not claim nothing changed: %q", body)
	}
	if len(links.unlinked) != 1 {
		t.Errorf("unlinked = %v", links.unlinked)
	}
}

func TestAnUnconfirmedCvInUseNeverCallsMedia(t *testing.T) {
	personaID := setupCvDB(t)
	UseJobLinks(&fakeLinks{using: []JobRef{{ID: "j1", Title: "A"}}})
	doc := insertDoc(t, personaID, "CV", "media-1", "")
	media := newFakeMedia(t, doc.ID, http.StatusNoContent)
	if rec := deleteDoc(doc.ID, ""); rec.Code != http.StatusConflict {
		t.Fatalf("%d", rec.Code)
	}
	if len(media.paths) != 0 || !docExists(doc.ID) {
		t.Errorf("nothing may happen before the confirmation: media %v exists %v", media.paths, docExists(doc.ID))
	}
}

func TestTheCallersCredentialsAreForwardedToTheMediaDelete(t *testing.T) {
	personaID := setupCvDB(t)
	UseJobLinks(&fakeLinks{})
	doc := insertDoc(t, personaID, "CV", "media-1", "")
	media := newFakeMedia(t, doc.ID, http.StatusNoContent)

	req := httptest.NewRequest(http.MethodDelete, "/cv-documents?id="+doc.ID, nil)
	req.Header.Set("Authorization", "Bearer caller-token")
	req.Header.Set("Cookie", "access_token=caller-cookie")
	rec := httptest.NewRecorder()
	DocumentsHandler(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if len(media.auth) != 1 || media.auth[0] != "Bearer caller-token" || media.cookies[0] != "access_token=caller-cookie" {
		t.Errorf("Media must see the caller's own credentials (its ownership check): auth %v cookie %v", media.auth, media.cookies)
	}
}

func TestAnUnknownCvIsRefusedBeforeAnyJobCheck(t *testing.T) {
	setupCvDB(t)
	// a failing usage check would answer 500 if it were reached: an unknown id must be a 404 first
	UseJobLinks(&fakeLinks{failWith: http.ErrAbortHandler})
	newFakeMedia(t, "none", http.StatusNoContent)
	if rec := deleteDoc("nope", "&unlinkJobs=true"); rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", rec.Code)
	}
}
