package cvbuilder

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// deleteCore is the core as the CV delete sees it: Media's download (whose header carries the file's original name), Media's delete and
// the delete of a generated file in pdf_tools. It records the calls in order with the caller's credentials.
type deleteCore struct {
	mu          sync.Mutex
	calls       []string // "name:<media id>", "media-delete:<id>", "pdf-delete:<id>"
	auth        []string
	cookies     []string
	disposition string
	nameCode    int
	mediaCode   int
	pdfCode     int
	pdfWait     time.Duration
	nameWait    time.Duration
}

func newDeleteCore(t *testing.T, originalName string) *deleteCore {
	t.Helper()
	c := &deleteCore{nameCode: http.StatusOK, mediaCode: http.StatusNoContent, pdfCode: http.StatusNoContent}
	if originalName != "" {
		c.disposition = fmt.Sprintf("inline; filename=%q", originalName)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		record := func(call string) {
			c.calls = append(c.calls, call)
			c.auth = append(c.auth, r.Header.Get("Authorization"))
			c.cookies = append(c.cookies, r.Header.Get("Cookie"))
		}
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/media/") && strings.HasSuffix(r.URL.Path, "/download"):
			record("name:" + strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/media/"), "/download"))
			code, disp, wait := c.nameCode, c.disposition, c.nameWait
			c.mu.Unlock()
			time.Sleep(wait)
			if disp != "" {
				w.Header().Set("Content-Disposition", disp)
			}
			w.WriteHeader(code)
			fmt.Fprint(w, "%PDF-1.4")
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/media/"):
			record("media-delete:" + strings.TrimPrefix(r.URL.Path, "/api/v1/media/"))
			code := c.mediaCode
			c.mu.Unlock()
			w.WriteHeader(code)
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/proxy/files"):
			record("pdf-delete:" + r.URL.Query().Get("id"))
			code, wait := c.pdfCode, c.pdfWait
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

func (c *deleteCore) seen() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.calls...)
}

func deleteCv(id string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/cv-documents?id="+id, nil)
	req.Header.Set("Authorization", "Bearer caller-token")
	req.Header.Set("Cookie", "access_token=caller-cookie")
	rec := httptest.NewRecorder()
	DocumentsHandler(rec, req)
	return rec
}

const sourceUUID = "6270bc10-6dd9-4769-a6b5-cc60cae8fdfa"

func TestDeletingAnAiSavedCvAlsoDeletesTheGeneratedPdfItWasCopiedFrom(t *testing.T) {
	pid := setupCvDB(t)
	UseJobLinks(&fakeLinks{})
	core := newDeleteCore(t, sourceUUID+".pdf")
	doc := insertDoc(t, pid, "CV", "media-1", "")
	if rec := deleteCv(doc.ID); rec.Code != http.StatusNoContent {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	want := []string{"name:media-1", "media-delete:media-1", "pdf-delete:" + sourceUUID}
	if got := core.seen(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("calls %v, want %v (the name is read before the Media file goes, the source goes after it)", got, want)
	}
	for i := range core.auth {
		if core.auth[i] != "Bearer caller-token" || core.cookies[i] != "access_token=caller-cookie" {
			t.Errorf("call %d must carry the caller's own credentials: %q %q", i, core.auth[i], core.cookies[i])
		}
	}
	if docExists(doc.ID) {
		t.Error("the CV must be gone")
	}
}

func TestAWizardCvHasNoGeneratedFileToDelete(t *testing.T) {
	pid := setupCvDB(t)
	UseJobLinks(&fakeLinks{})
	core := newDeleteCore(t, "cv.pdf") // what the wizard uploads; its generated file was deleted when it was created
	doc := insertDoc(t, pid, "CV", "media-1", "")
	if rec := deleteCv(doc.ID); rec.Code != http.StatusNoContent {
		t.Fatalf("%d", rec.Code)
	}
	want := []string{"name:media-1", "media-delete:media-1"}
	if got := core.seen(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("calls %v, want %v", got, want)
	}
}

func TestOnlyTheExactNameOfAGeneratedFileIsTakenForOne(t *testing.T) {
	for name, want := range map[string]string{
		sourceUUID + ".pdf":                  sourceUUID,
		"":                                   "",
		"cv.pdf":                             "",
		"a.pdf":                              "",
		sourceUUID:                           "",
		sourceUUID + ".PDF":                  "",
		sourceUUID + ".pdf.exe":              "",
		sourceUUID + ".pdf ":                 "",
		" " + sourceUUID + ".pdf":            "",
		strings.ToUpper(sourceUUID) + ".pdf": "",
		strings.ReplaceAll(sourceUUID, "-", "") + ".pdf": "",
		"{" + sourceUUID + "}.pdf":                       "",
		"urn:uuid:" + sourceUUID + ".pdf":                "",
		"../" + sourceUUID + ".pdf":                      "",
		"x/" + sourceUUID + ".pdf":                       "",
		sourceUUID + "/.pdf":                             "",
		sourceUUID + sourceUUID + ".pdf":                 "",
		"6270bc10-6dd9-4769-a6b5-cc60cae8fdfg.pdf":       "", // not hexadecimal
		"6270bc10-6dd9-4769-a6b5-cc60cae8fd.pdf":         "", // too short
		sourceUUID + ".pdf\n":                            "",
		"6270BC10-6dd9-4769-a6b5-cc60cae8fdfa.pdf":       "", // capitals in the first group only
		"6270bc10-6DD9-4769-a6b5-cc60cae8fdfa.pdf":       "", // in a middle group only
		"6270bc10-6dd9-4769-a6b5-CC60CAE8FDFA.pdf":       "", // in the last group only
	} {
		if got := generatedIDFromName(name); got != want {
			t.Errorf("generatedIDFromName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestTheNameIsReadFromTheHeaderInItsUsualForms(t *testing.T) {
	pid := setupCvDB(t)
	UseJobLinks(&fakeLinks{})
	for name, disposition := range map[string]string{
		"inline with a quoted name":  `inline; filename="` + sourceUUID + `.pdf"`,
		"attachment":                 `attachment; filename="` + sourceUUID + `.pdf"`,
		"an unquoted name":           `inline; filename=` + sourceUUID + `.pdf`,
		"an extended name (RFC5987)": `inline; filename*=UTF-8''` + sourceUUID + `.pdf`,
		"extra parameters":           `inline; size=12; filename="` + sourceUUID + `.pdf"; creation-date="x"`,
	} {
		core := newDeleteCore(t, "")
		core.disposition = disposition
		doc := insertDoc(t, pid, "CV "+name, "media-1", "")
		if rec := deleteCv(doc.ID); rec.Code != http.StatusNoContent {
			t.Fatalf("%s: %d", name, rec.Code)
		}
		if got := core.seen(); len(got) != 3 || got[2] != "pdf-delete:"+sourceUUID {
			t.Errorf("%s: calls %v", name, got)
		}
	}
	for name, disposition := range map[string]string{
		"no header at all":     "",
		"an unreadable header": `inline; filename="`,
		"another file":         `inline; filename="notes.pdf"`,
		"no filename":          `inline`,
	} {
		core := newDeleteCore(t, "")
		core.disposition = disposition
		doc := insertDoc(t, pid, "CV "+name, "media-1", "")
		if rec := deleteCv(doc.ID); rec.Code != http.StatusNoContent {
			t.Fatalf("%s: %d", name, rec.Code)
		}
		if got := core.seen(); len(got) != 2 {
			t.Errorf("%s: nothing may be asked of pdf_tools: %v", name, got)
		}
	}
}

func TestACvIsStillDeletedWhenTheNameCannotBeRead(t *testing.T) {
	pid := setupCvDB(t)
	UseJobLinks(&fakeLinks{})
	for name, code := range map[string]int{"404": http.StatusNotFound, "403": http.StatusForbidden, "500": http.StatusInternalServerError} {
		core := newDeleteCore(t, sourceUUID+".pdf")
		core.nameCode = code
		doc := insertDoc(t, pid, "CV "+name, "media-1", "")
		if rec := deleteCv(doc.ID); rec.Code != http.StatusNoContent || docExists(doc.ID) {
			t.Errorf("%s: %d exists=%v", name, rec.Code, docExists(doc.ID))
		}
		if got := core.seen(); len(got) != 2 || got[1] != "media-delete:media-1" {
			t.Errorf("%s: calls %v: the file is still deleted, the unknown source is left alone", name, got)
		}
	}
	// an unreachable core: the name cannot be read, the Media delete fails too and keeps the CV (step 1), nothing else happens
	doc := insertDoc(t, pid, "CV down", "media-9", "")
	t.Setenv("CORE_API_URL", "http://127.0.0.1:1")
	if rec := deleteCv(doc.ID); rec.Code != http.StatusBadGateway || !docExists(doc.ID) {
		t.Errorf("unreachable: %d exists=%v", rec.Code, docExists(doc.ID))
	}
}

func TestAFailedMediaDeleteLeavesTheGeneratedPdfAloneSoTheRetryCanFindIt(t *testing.T) {
	pid := setupCvDB(t)
	UseJobLinks(&fakeLinks{})
	core := newDeleteCore(t, sourceUUID+".pdf")
	core.mediaCode = http.StatusInternalServerError
	doc := insertDoc(t, pid, "CV", "media-1", "")
	if rec := deleteCv(doc.ID); rec.Code != http.StatusBadGateway || !docExists(doc.ID) {
		t.Fatalf("%d exists=%v", rec.Code, docExists(doc.ID))
	}
	for _, c := range core.seen() {
		if strings.HasPrefix(c, "pdf-delete:") {
			t.Errorf("the generated file must stay while the CV stays: %v", core.seen())
		}
	}
	// the retry, with Media back, reads the name again and finishes the job
	core.mediaCode = http.StatusNoContent
	if rec := deleteCv(doc.ID); rec.Code != http.StatusNoContent || docExists(doc.ID) {
		t.Fatalf("retry: %d", rec.Code)
	}
	got := core.seen()
	if got[len(got)-1] != "pdf-delete:"+sourceUUID {
		t.Errorf("the retry deletes the generated file: %v", got)
	}
}

func TestACleanUpThatFailsNeverChangesTheDeleteOfTheCv(t *testing.T) {
	pid := setupCvDB(t)
	UseJobLinks(&fakeLinks{})
	for name, code := range map[string]int{"500": http.StatusInternalServerError, "404 (an old pdf_tools)": http.StatusNotFound, "400": http.StatusBadRequest} {
		core := newDeleteCore(t, sourceUUID+".pdf")
		core.pdfCode = code
		doc := insertDoc(t, pid, "CV "+name, "media-1", "")
		if rec := deleteCv(doc.ID); rec.Code != http.StatusNoContent || docExists(doc.ID) {
			t.Errorf("%s: %d exists=%v", name, rec.Code, docExists(doc.ID))
		}
		if got := core.seen(); len(got) != 3 || got[2] != "pdf-delete:"+sourceUUID {
			t.Errorf("%s: %v", name, got)
		}
	}
	// a clean-up that hangs is cut off by its timeout
	core := newDeleteCore(t, sourceUUID+".pdf")
	core.pdfWait = 2 * time.Second
	old := pdfSourceDeleteTimeout
	pdfSourceDeleteTimeout = 150 * time.Millisecond
	t.Cleanup(func() { pdfSourceDeleteTimeout = old })
	doc := insertDoc(t, pid, "CV slow", "media-1", "")
	start := time.Now()
	if rec := deleteCv(doc.ID); rec.Code != http.StatusNoContent || docExists(doc.ID) {
		t.Fatalf("slow: %d", rec.Code)
	}
	if took := time.Since(start); took > time.Second {
		t.Errorf("the delete waited %v for the clean-up", took)
	}
}

func TestACvWithoutAFileOrThatCannotBeDeletedAsksNothingAboutAGeneratedFile(t *testing.T) {
	pid := setupCvDB(t)
	core := newDeleteCore(t, sourceUUID+".pdf")
	// no PDF at all
	UseJobLinks(&fakeLinks{})
	noPdf := insertDoc(t, pid, "No PDF", "", "")
	if rec := deleteCv(noPdf.ID); rec.Code != http.StatusNoContent {
		t.Fatalf("%d", rec.Code)
	}
	// jobs use it and nobody confirmed
	UseJobLinks(&fakeLinks{using: []JobRef{{ID: "j1", Title: "A"}}})
	used := insertDoc(t, pid, "Used", "media-2", "")
	if rec := deleteCv(used.ID); rec.Code != http.StatusConflict {
		t.Fatalf("%d", rec.Code)
	}
	// an unknown CV
	if rec := deleteCv("nope"); rec.Code != http.StatusNotFound {
		t.Fatalf("%d", rec.Code)
	}
	if got := core.seen(); len(got) != 0 {
		t.Errorf("nothing may be asked of the core: %v", got)
	}
}

func TestAConfirmedCvInUseStillCleansUpAfterItsFileIsGone(t *testing.T) {
	pid := setupCvDB(t)
	core := newDeleteCore(t, sourceUUID+".pdf")
	links := &fakeLinks{using: []JobRef{{ID: "j1", Title: "A"}}}
	UseJobLinks(links)
	doc := insertDoc(t, pid, "CV", "media-1", "")
	req := httptest.NewRequest(http.MethodDelete, "/cv-documents?id="+doc.ID+"&unlinkJobs=true", nil)
	rec := httptest.NewRecorder()
	DocumentsHandler(rec, req)
	if rec.Code != http.StatusNoContent || len(links.unlinked) != 1 {
		t.Fatalf("%d %v", rec.Code, links.unlinked)
	}
	if got := core.seen(); len(got) != 3 || got[2] != "pdf-delete:"+sourceUUID {
		t.Errorf("%v", got)
	}
}

func TestAHangingNameLookupDoesNotBlockTheDeleteOfTheCv(t *testing.T) {
	pid := setupCvDB(t)
	UseJobLinks(&fakeLinks{})
	core := newDeleteCore(t, sourceUUID+".pdf")
	core.nameWait = 2 * time.Second
	old := pdfSourceDeleteTimeout
	pdfSourceDeleteTimeout = 150 * time.Millisecond
	t.Cleanup(func() { pdfSourceDeleteTimeout = old })
	doc := insertDoc(t, pid, "CV", "media-1", "")
	start := time.Now()
	if rec := deleteCv(doc.ID); rec.Code != http.StatusNoContent || docExists(doc.ID) {
		t.Fatalf("%d exists=%v", rec.Code, docExists(doc.ID))
	}
	if took := time.Since(start); took > time.Second {
		t.Errorf("the delete waited %v for the name: its timeout must apply", took)
	}
	for _, c := range core.seen() {
		if strings.HasPrefix(c, "pdf-delete:") {
			t.Errorf("a name that could not be read is not guessed: %v", core.seen())
		}
	}
}
