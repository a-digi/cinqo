package cvbuilder

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/cvbuilder/templates"
	"career-tool-backend/db"
	"career-tool-backend/persona"
)

// fakeTitleCore stands in for the core's Media "set title" route: it records every PATCH and answers with the status the test sets.
type fakeTitleCore struct {
	mu     sync.Mutex
	paths  []string
	titles []string
	status int
}

func newFakeTitleCore(t *testing.T, status int) *fakeTitleCore {
	t.Helper()
	c := &fakeTitleCore{status: status}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Title string `json:"title"`
		}
		_ = json.Unmarshal(raw, &body)
		c.mu.Lock()
		defer c.mu.Unlock()
		if r.Method == http.MethodPatch {
			c.paths = append(c.paths, r.URL.Path)
			c.titles = append(c.titles, body.Title)
		}
		w.WriteHeader(c.status)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "tok")
	return c
}

func cvRows(t *testing.T) int {
	t.Helper()
	var n int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM cv_documents`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestACvIsSavedOnTheNamedPersonaAndItsFileGetsItsTitle(t *testing.T) {
	pid := setupCvDB(t)
	core := newFakeTitleCore(t, http.StatusOK)
	doc, err := SaveCvForPersona(pid, "classic", "  Ada - Backend  ", goodData(), "media-1")
	if err != nil {
		t.Fatal(err)
	}
	if doc.PersonaID != pid || doc.Title != "Ada - Backend" || doc.MediaFileID != "media-1" || doc.JobID != "" || doc.TemplateID != "classic" {
		t.Fatalf("doc = %+v", doc)
	}
	if len(doc.Personas) != 1 || doc.Personas[0].ID != pid || !doc.Personas[0].BuiltFrom {
		t.Fatalf("not linked to the persona it was built from: %+v", doc.Personas)
	}
	if doc.Data.FullName != "Ada Lovelace" {
		t.Errorf("the data must be stored: %+v", doc.Data)
	}
	if len(core.paths) != 1 || core.paths[0] != "/api/v1/media/media-1/title" || core.titles[0] != "Ada - Backend.pdf" {
		t.Errorf("Media was told %v %v", core.paths, core.titles)
	}
	if cvRows(t) != 1 {
		t.Errorf("cv rows = %d", cvRows(t))
	}
}

func TestAPersonaCanHaveSeveralCvsEachWithItsOwnFile(t *testing.T) {
	pid := setupCvDB(t)
	newFakeTitleCore(t, http.StatusOK)
	for i, media := range []string{"media-1", "media-2", "media-3"} {
		if _, err := SaveCvForPersona(pid, "classic", "CV "+media, goodData(), media); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}
	if cvRows(t) != 3 {
		t.Errorf("cv rows = %d", cvRows(t))
	}
}

func TestARefusedSaveStoresNothingAndNeverTouchesMedia(t *testing.T) {
	pid := setupCvDB(t)
	core := newFakeTitleCore(t, http.StatusOK)
	cases := []struct {
		name    string
		persona string
		tpl     string
		title   string
		data    templates.CvData
		media   string
		want    error
	}{
		{"unknown persona", "nope", "classic", "T", goodData(), "m", persona.ErrUnknownPersona},
		{"unknown template", pid, "no-such-template", "T", goodData(), "m", nil},
		{"blank title", pid, "classic", "  ", goodData(), "m", nil},
		{"long title", pid, "classic", strings.Repeat("x", maxCvAiTitle+1), goodData(), "m", nil},
		{"no full name", pid, "classic", "T", templates.CvData{}, "m", nil},
		{"markup in data", pid, "classic", "T", templates.CvData{FullName: "Ada", Summary: "<strong>hi</strong>"}, "m", nil},
		{"raw resource link", pid, "classic", "T", goodData(), "file:///x/y.pdf", nil},
		{"leading slash", pid, "classic", "T", goodData(), "/x.pdf", nil},
		{"no pdf", pid, "classic", "T", goodData(), "", nil},
		{"blank pdf", pid, "classic", "T", goodData(), "   ", nil},
	}
	for _, c := range cases {
		_, err := SaveCvForPersona(c.persona, c.tpl, c.title, c.data, c.media)
		if err == nil || (c.want != nil && !errors.Is(err, c.want)) {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
	if cvRows(t) != 0 {
		t.Errorf("a refused save left %d CVs", cvRows(t))
	}
	if len(core.paths) != 0 {
		t.Errorf("a refused save must not change any file in Media: %v", core.paths)
	}
}

func TestAPdfThatAlreadyBelongsToAnotherCvIsRefusedAndThatFileIsLeftAlone(t *testing.T) {
	pid := setupCvDB(t)
	core := newFakeTitleCore(t, http.StatusOK)
	first, err := SaveCvForPersona(pid, "classic", "First", goodData(), "media-1")
	if err != nil {
		t.Fatal(err)
	}
	other, err := persona.CreatePersona(profileOfPersona(t, pid), "Other", "")
	if err != nil {
		t.Fatal(err)
	}
	// the same persona and another persona both get the refusal
	for _, p := range []string{pid, other} {
		if _, err := SaveCvForPersona(p, "classic", "Second", goodData(), "media-1"); !errors.Is(err, ErrCvPdfInUse) {
			t.Errorf("persona %s: err = %v", p, err)
		}
	}
	if cvRows(t) != 1 {
		t.Errorf("cv rows = %d", cvRows(t))
	}
	if len(core.paths) != 1 {
		t.Errorf("the first CV's file must not be retitled by the refused saves: %v %v", core.paths, core.titles)
	}
	got, err := GetCvDocument(first.ID)
	if err != nil || got.Title != "First" {
		t.Errorf("the first CV changed: %+v %v", got, err)
	}
}

func TestAFileMediaDoesNotKnowIsRefusedAndNothingIsStored(t *testing.T) {
	pid := setupCvDB(t)
	for _, status := range []int{http.StatusNotFound, http.StatusForbidden} {
		newFakeTitleCore(t, status)
		_, err := SaveCvForPersona(pid, "classic", "T", goodData(), "media-x")
		if err == nil || !strings.Contains(err.Error(), "does not correspond to a real, already-promoted Media file") {
			t.Errorf("status %d: err = %v", status, err)
		}
	}
	if cvRows(t) != 0 {
		t.Errorf("cv rows = %d", cvRows(t))
	}
}

func TestAnUnreachableOrFailingMediaDoesNotBlockTheSaveAsForTheRunsSave(t *testing.T) {
	pid := setupCvDB(t)
	newFakeTitleCore(t, http.StatusInternalServerError)
	if _, err := SaveCvForPersona(pid, "classic", "T", goodData(), "media-1"); err != nil {
		t.Errorf("a 500 from Media says nothing about the file: %v", err)
	}
	t.Setenv("CORE_API_URL", "")
	if _, err := SaveCvForPersona(pid, "classic", "T2", goodData(), "media-2"); err != nil {
		t.Errorf("no core configured: %v", err)
	}
}

func TestTheRunsSaveStillChecksTheSameThingsAndRenamesItsFile(t *testing.T) {
	pid := setupCvDB(t)
	core := newFakeTitleCore(t, http.StatusOK)
	run := newCvRun(t, pid)
	if _, err := SaveCvAiDocument(run, "classic", " Run CV ", goodData(), "media-run"); err != nil {
		t.Fatal(err)
	}
	if len(core.titles) != 1 || core.titles[0] != "Run CV.pdf" {
		t.Errorf("the run's save names its file too: %v", core.titles)
	}
	other, err := persona.CreatePersona(profileOfPersona(t, pid), "Other", "") // one run at a time per persona
	if err != nil {
		t.Fatal(err)
	}
	run2 := newCvRun(t, other)
	newFakeTitleCore(t, http.StatusNotFound)
	if _, err := SaveCvAiDocument(run2, "classic", "T", goodData(), "media-y"); err == nil || !strings.Contains(err.Error(), "does not correspond to a real") {
		t.Errorf("the run's save refuses a file Media does not know: %v", err)
	}
}

// The tool through a real in-memory MCP session.
func TestSaveCvForPersonaThroughMCP(t *testing.T) {
	pid := setupCvDB(t)
	newFakeTitleCore(t, http.StatusOK)
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1"}, nil)
	RegisterSaveCvForPersona(server)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	call := func(args map[string]any) (string, bool) {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "save_cv_for_persona", Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		text := ""
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				text += tc.Text
			}
		}
		return text, res.IsError
	}
	args := func(over map[string]any) map[string]any {
		a := map[string]any{"personaId": pid, "templateId": "classic", "title": "Ada - Backend", "data": map[string]any{"fullName": "Ada Lovelace", "skills": []string{"Go"}}, "pdfResource": "media-1"}
		for k, v := range over {
			a[k] = v
		}
		return a
	}

	text, isErr := call(args(nil))
	if isErr {
		t.Fatalf("%s", text)
	}
	var out struct{ CvDocumentID, Status string }
	var raw map[string]string
	if err := json.Unmarshal([]byte(text), &raw); err != nil || raw["status"] != "saved" || raw["cvDocumentId"] == "" {
		t.Fatalf("%s %v", text, err)
	}
	out.CvDocumentID = raw["cvDocumentId"]
	if doc, err := GetCvDocument(out.CvDocumentID); err != nil || doc.PersonaID != pid || doc.Data.FullName != "Ada Lovelace" || len(doc.Data.Skills) != 1 {
		t.Errorf("stored %+v %v", doc, err)
	}
	if text, isErr := call(args(map[string]any{"personaId": ""})); !isErr || !strings.Contains(text, "personaId is required") {
		t.Errorf("empty persona: %v %s", isErr, text)
	}
	if text, isErr := call(args(map[string]any{"personaId": "nope", "pdfResource": "media-2"})); !isErr || !strings.Contains(text, `unknown persona id "nope"`) {
		t.Errorf("unknown persona: %v %s", isErr, text)
	}
	if text, isErr := call(args(map[string]any{"title": "  ", "pdfResource": "media-3"})); !isErr || !strings.Contains(text, "title is required") {
		t.Errorf("blank title: %v %s", isErr, text)
	}
	if text, isErr := call(args(map[string]any{"title": "Again"})); !isErr || !strings.Contains(text, "already belongs to another CV") {
		t.Errorf("the same file again: %v %s", isErr, text)
	}
	if cvRows(t) != 1 {
		t.Errorf("only the first call may have saved: %d", cvRows(t))
	}
}
