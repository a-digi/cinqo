package cvbuilder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// wizardCore is the core as the wizard sees it: pdf_tools' generate and files through the proxy, Media's upload, and the new delete of a
// generated file. It records the calls in order, with the caller's credentials, and answers with what the test sets.
type wizardCore struct {
	mu         sync.Mutex
	calls      []string // "generate", "fetch", "upload", "delete:<id>"
	deleteAuth []string
	deleteCook []string
	genID      string
	genStatus  int
	fetchCode  int
	uploadCode int
	deleteCode int
	deleteWait time.Duration
}

func newWizardCore(t *testing.T) *wizardCore {
	t.Helper()
	c := &wizardCore{genID: "11111111-2222-4333-8444-555555555555", genStatus: http.StatusOK, fetchCode: http.StatusOK, uploadCode: http.StatusCreated, deleteCode: http.StatusNoContent}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/proxy/generate"):
			c.calls = append(c.calls, "generate")
			code, id := c.genStatus, c.genID
			c.mu.Unlock()
			_, _ = io.Copy(io.Discard, r.Body)
			if code != http.StatusOK {
				http.Error(w, "boom", code)
				return
			}
			fmt.Fprintf(w, `{"id":%q,"bytes":4}`, id)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/proxy/files"):
			c.calls = append(c.calls, "fetch")
			code := c.fetchCode
			c.mu.Unlock()
			if code != http.StatusOK {
				http.Error(w, "boom", code)
				return
			}
			fmt.Fprint(w, "%PDF-1.4")
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/media/upload":
			c.calls = append(c.calls, "upload")
			code := c.uploadCode
			c.mu.Unlock()
			_, _ = io.Copy(io.Discard, r.Body)
			if code != http.StatusCreated {
				http.Error(w, "boom", code)
				return
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"message":{"fileId":"cv-media-1"}}`)
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/proxy/files"):
			c.calls = append(c.calls, "delete:"+r.URL.Query().Get("id"))
			c.deleteAuth = append(c.deleteAuth, r.Header.Get("Authorization"))
			c.deleteCook = append(c.deleteCook, r.Header.Get("Cookie"))
			code, wait := c.deleteCode, c.deleteWait
			c.mu.Unlock()
			time.Sleep(wait)
			w.WriteHeader(code)
		default:
			c.mu.Unlock()
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusTeapot)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	return c
}

func (c *wizardCore) seen() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.calls...)
}

func createCv(personaID string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]any{"personaId": personaID, "templateId": "classic", "title": "Ada CV", "data": map[string]any{"fullName": "Ada Lovelace", "skills": []string{"Go"}}})
	req := httptest.NewRequest(http.MethodPost, "/cv-documents", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer caller-token")
	req.Header.Set("Cookie", "access_token=caller-cookie")
	rec := httptest.NewRecorder()
	DocumentsHandler(rec, req)
	return rec
}

func TestTheWizardDeletesTheGeneratedPdfAfterItIsInMediaAndTheCvIsStored(t *testing.T) {
	pid := setupCvDB(t)
	UseJobLinks(&fakeLinks{})
	core := newWizardCore(t)
	rec := createCv(pid)
	if rec.Code != http.StatusCreated {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	want := []string{"generate", "fetch", "upload", "delete:" + core.genID}
	if got := core.seen(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("calls %v, want %v (the file is deleted last, when nothing needs it any more)", got, want)
	}
	if len(core.deleteAuth) != 1 || core.deleteAuth[0] != "Bearer caller-token" || core.deleteCook[0] != "access_token=caller-cookie" {
		t.Errorf("the delete must carry the caller's own credentials: %v %v", core.deleteAuth, core.deleteCook)
	}
	var doc CvDocument
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil || doc.MediaFileID != "cv-media-1" || doc.PersonaID != pid {
		t.Errorf("the CV must be stored with its Media file: %+v %v", doc, err)
	}
	if cvRows(t) != 1 {
		t.Errorf("cv rows = %d", cvRows(t))
	}
}

func TestACleanUpThatFailsNeverChangesTheAnswerOrTheCv(t *testing.T) {
	for name, set := range map[string]func(*wizardCore){
		"pdf_tools answers 500":               func(c *wizardCore) { c.deleteCode = http.StatusInternalServerError },
		"an old pdf_tools: the proxy 404s":    func(c *wizardCore) { c.deleteCode = http.StatusNotFound },
		"the proxy refuses (403)":             func(c *wizardCore) { c.deleteCode = http.StatusForbidden },
		"a bad request (400)":                 func(c *wizardCore) { c.deleteCode = http.StatusBadRequest },
		"an answer that is not 204 but a 200": func(c *wizardCore) { c.deleteCode = http.StatusOK },
	} {
		pid := setupCvDB(t)
		UseJobLinks(&fakeLinks{})
		core := newWizardCore(t)
		set(core)
		rec := createCv(pid)
		if rec.Code != http.StatusCreated || cvRows(t) != 1 {
			t.Errorf("%s: %d rows=%d %s", name, rec.Code, cvRows(t), rec.Body.String())
		}
		if got := core.seen(); len(got) != 4 || !strings.HasPrefix(got[3], "delete:") {
			t.Errorf("%s: the delete was still tried: %v", name, got)
		}
	}
}

func TestACleanUpThatHangsDoesNotHoldTheRequestLonger(t *testing.T) {
	pid := setupCvDB(t)
	UseJobLinks(&fakeLinks{})
	core := newWizardCore(t)
	core.deleteWait = 2 * time.Second
	old := pdfSourceDeleteTimeout
	pdfSourceDeleteTimeout = 150 * time.Millisecond
	t.Cleanup(func() { pdfSourceDeleteTimeout = old })
	start := time.Now()
	rec := createCv(pid)
	if rec.Code != http.StatusCreated || cvRows(t) != 1 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if took := time.Since(start); took > time.Second {
		t.Errorf("the request waited %v for the clean-up: its timeout must apply", took)
	}
}

func TestTheGeneratedPdfIsDeletedWhateverEndsTheRequestAfterItWasGenerated(t *testing.T) {
	for name, set := range map[string]func(*wizardCore){
		"the fetch fails":  func(c *wizardCore) { c.fetchCode = http.StatusBadGateway },
		"the upload fails": func(c *wizardCore) { c.uploadCode = http.StatusInternalServerError },
	} {
		pid := setupCvDB(t)
		UseJobLinks(&fakeLinks{})
		core := newWizardCore(t)
		set(core)
		rec := createCv(pid)
		if rec.Code != http.StatusBadGateway || cvRows(t) != 0 {
			t.Errorf("%s: %d rows=%d", name, rec.Code, cvRows(t))
		}
		got := core.seen()
		if len(got) == 0 || got[len(got)-1] != "delete:"+core.genID {
			t.Errorf("%s: the file must still be deleted: %v", name, got)
		}
	}
}

func TestNothingIsDeletedWhenNothingWasGenerated(t *testing.T) {
	pid := setupCvDB(t)
	UseJobLinks(&fakeLinks{})
	core := newWizardCore(t)
	core.genStatus = http.StatusInternalServerError
	if rec := createCv(pid); rec.Code != http.StatusBadGateway {
		t.Fatalf("%d", rec.Code)
	}
	for _, c := range core.seen() {
		if strings.HasPrefix(c, "delete:") {
			t.Errorf("no file exists, so no delete is sent: %v", core.seen())
		}
	}
	// a refused request (before anything is generated) deletes nothing either
	core2 := newWizardCore(t)
	body := strings.NewReader(`{"personaId":"` + pid + `","templateId":"classic","title":"","data":{"fullName":"Ada"}}`)
	rec := httptest.NewRecorder()
	DocumentsHandler(rec, httptest.NewRequest(http.MethodPost, "/cv-documents", body))
	if rec.Code != http.StatusBadRequest || len(core2.seen()) != 0 {
		t.Errorf("%d %v", rec.Code, core2.seen())
	}
}

func TestTheIdOfTheFileIsEscapedInTheDeleteRequest(t *testing.T) {
	core := newWizardCore(t)
	// a real id is a UUID; the delete call must not depend on that: whatever it is given arrives whole in the query
	if err := forwardDeletePdfSource(httptest.NewRequest(http.MethodPost, "/x", nil), "a b&c=d?e#f"); err != nil {
		t.Fatal(err)
	}
	if got := core.seen(); len(got) != 1 || got[0] != "delete:a b&c=d?e#f" {
		t.Errorf("the id must arrive whole in the query: %v", got)
	}
}

func TestOnlyA204IsASuccessOfTheDeleteCall(t *testing.T) {
	core := newWizardCore(t)
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	if err := forwardDeletePdfSource(req, core.genID); err != nil {
		t.Errorf("204: %v", err)
	}
	for _, code := range []int{http.StatusOK, http.StatusNotFound, http.StatusForbidden, http.StatusBadRequest, http.StatusInternalServerError} {
		core.deleteCode = code
		err := forwardDeletePdfSource(req, core.genID)
		if err == nil || !strings.Contains(err.Error(), "pdf_tools answered") {
			t.Errorf("%d: err = %v", code, err)
		}
	}
	t.Setenv("CORE_API_URL", "http://127.0.0.1:1") // nothing listens there
	if err := forwardDeletePdfSource(req, core.genID); err == nil {
		t.Error("an unreachable core is a failure the caller logs")
	}
}

func TestThePreviewKeepsItsFileBecauseTheBrowserIsStillShowingIt(t *testing.T) {
	pid := setupCvDB(t)
	core := newWizardCore(t)
	body, _ := json.Marshal(map[string]any{"personaId": pid, "templateId": "classic", "data": map[string]any{"fullName": "Ada Lovelace"}})
	rec := httptest.NewRecorder()
	PreviewHandler(rec, httptest.NewRequest(http.MethodPost, "/cv-documents/preview", bytes.NewReader(body)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), core.genID) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	for _, c := range core.seen() {
		if strings.HasPrefix(c, "delete:") {
			t.Errorf("the preview's link points at the file: it must not be deleted here: %v", core.seen())
		}
	}
}

func TestAnEmptyIdIsNeverSentAsADelete(t *testing.T) {
	core := newWizardCore(t)
	deleteGeneratedPdf(httptest.NewRequest(http.MethodPost, "/x", nil), "")
	if len(core.seen()) != 0 {
		t.Errorf("%v", core.seen())
	}
}
