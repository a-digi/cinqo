package cvbuilder

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/cv/cvbuilder/templates"
	"career-tool-backend/db"
)

func goodPtr() *templates.CvData { d := goodData(); return &d }

// hand_over_cv (plan 256, step 4): a model-driven run hands over its design, title and data; the page makes the CV (with the photo) from them.

func choiceOf(t *testing.T, personaID, runID string) *CvAiChoice {
	t.Helper()
	runs, err := ListCvAiRuns(personaID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range runs {
		if r.ID == runID {
			return r.Choice
		}
	}
	t.Fatalf("run %s is not in the list", runID)
	return nil
}

func TestStoreCvAiChoice_TheRunKeepsExactlyWhatWasHandedOver(t *testing.T) {
	pid := setupCvDB(t)
	run := newCvRun(t, pid)
	if choiceOf(t, pid, run) != nil {
		t.Fatal("a new run has no choice")
	}
	data := richCv()
	if err := StoreCvAiChoice(run, "modern-sidebar", "  Ada - Backend  ", &data); err != nil {
		t.Fatal(err)
	}
	got := choiceOf(t, pid, run)
	if got == nil || got.TemplateID != "modern-sidebar" || got.Title != "Ada - Backend" || got.Data == nil || !reflect.DeepEqual(*got.Data, data) {
		t.Fatalf("choice = %+v", got)
	}
	// it is only a choice: the run still works and has no CV, so it cannot be finished as ready
	if status, cv, _ := runRow(t, run); status != "generating" || cv != "" {
		t.Errorf("run = %q %q", status, cv)
	}
	if err := FinishCvAiRun(run, cvAiReady, ""); !errors.Is(err, ErrCvAiNothingSaved) {
		t.Errorf("finishing ready on a choice alone: %v", err)
	}
	var n int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM cv_documents`).Scan(&n); err != nil || n != 0 {
		t.Errorf("a choice makes no CV: %d rows (%v)", n, err)
	}
}

func TestStoreCvAiChoice_RefusesBadContentAndLeavesTheRunWithoutAChoice(t *testing.T) {
	pid := setupCvDB(t)
	run := newCvRun(t, pid)
	good := goodData()
	for _, c := range []struct {
		name, template, title string
		data                  templates.CvData
	}{
		{"an empty title", "classic", "  ", good},
		{"a title that is too long", "classic", strings.Repeat("x", maxCvAiTitle+1), good},
		{"no name", "classic", "T", templates.CvData{Skills: []string{"Go"}}},
		{"an unknown design", "no-such-design", "T", good},
		{"no design", "", "T", good},
		{"markup in the data", "classic", "T", templates.CvData{FullName: "Ada", Summary: "<p>Hello</p>", Skills: []string{"Go"}}},
	} {
		if err := StoreCvAiChoice(run, c.template, c.title, &c.data); err == nil {
			t.Errorf("%s: accepted", c.name)
		}
		if choiceOf(t, pid, run) != nil {
			t.Fatalf("%s: the run kept a refused choice", c.name)
		}
	}
	if err := StoreCvAiChoice(run, "classic", "T", &good); err != nil {
		t.Fatalf("a good choice after the refusals: %v", err)
	}
}

func TestStoreCvAiChoice_RefusesWhatTheRunCannotTake(t *testing.T) {
	pid := setupCvDB(t)
	// an unknown run
	if err := StoreCvAiChoice("nope", "classic", "T", goodPtr()); !errors.Is(err, ErrUnknownCvAiRun) {
		t.Errorf("unknown run: %v", err)
	}
	// a second choice: the first stays
	run := newCvRun(t, pid)
	if err := StoreCvAiChoice(run, "classic", "First", goodPtr()); err != nil {
		t.Fatal(err)
	}
	if err := StoreCvAiChoice(run, "ats-clean", "Second", goodPtr()); !errors.Is(err, ErrCvAiChoiceAlreadyHandedOver) {
		t.Errorf("second choice: %v", err)
	}
	if c := choiceOf(t, pid, run); c == nil || c.TemplateID != "classic" || c.Title != "First" {
		t.Errorf("the first choice must stay: %+v", c)
	}
	// a run that already has a CV (made by the tool path or attached)
	doc := cvFor(t, pid, "m1")
	if err := FinishCvAiRun(run, cvAiFailed, "x"); err != nil {
		t.Fatal(err)
	}
	run2 := newCvRun(t, pid)
	if err := AttachCvToRun(run2, doc.ID); err != nil {
		t.Fatal(err)
	}
	if err := StoreCvAiChoice(run2, "classic", "T", goodPtr()); !errors.Is(err, ErrCvAiAlreadySaved) {
		t.Errorf("a run with a CV: %v", err)
	}
	if choiceOf(t, pid, run2) != nil {
		t.Error("a run with a CV took a choice")
	}
	// a finished run
	if err := FinishCvAiRun(run2, cvAiReady, ""); err != nil {
		t.Fatal(err)
	}
	if err := StoreCvAiChoice(run2, "classic", "T", goodPtr()); !errors.Is(err, ErrCvAiRunNotGenerating) {
		t.Errorf("a finished run: %v", err)
	}
	run3 := newCvRun(t, pid)
	if err := FinishCvAiRun(run3, cvAiFailed, "boom"); err != nil {
		t.Fatal(err)
	}
	if err := StoreCvAiChoice(run3, "classic", "T", goodPtr()); !errors.Is(err, ErrCvAiRunNotGenerating) {
		t.Errorf("a failed run: %v", err)
	}
	if choiceOf(t, pid, run3) != nil {
		t.Error("a failed run took a choice")
	}
}

func TestStoreCvAiChoice_OfTwoCallsAtOnceOnlyOneWins(t *testing.T) {
	pid := setupCvDB(t)
	run := newCvRun(t, pid)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, tpl := range []string{"classic", "ats-clean"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = StoreCvAiChoice(run, tpl, "T"+tpl, goodPtr())
		}()
	}
	wg.Wait()
	wins := 0
	for _, e := range errs {
		if e == nil {
			wins++
		} else if !errors.Is(e, ErrCvAiChoiceAlreadyHandedOver) {
			t.Errorf("the loser got %v", e)
		}
	}
	if wins != 1 {
		t.Fatalf("%d calls won, want 1 (%v)", wins, errs)
	}
	if c := choiceOf(t, pid, run); c == nil || (c.TemplateID != "classic" && c.TemplateID != "ats-clean") {
		t.Errorf("choice = %+v", c)
	}
}

func TestAnUnreadableStoredChoiceIsLeftOutOfTheListNotAFailure(t *testing.T) {
	pid := setupCvDB(t)
	run := newCvRun(t, pid)
	if _, err := db.CareerDB.Exec(`UPDATE cv_ai_runs SET choice_json = '{not json' WHERE id = ?`, run); err != nil {
		t.Fatal(err)
	}
	runs, err := ListCvAiRuns(pid)
	if err != nil || len(runs) != 1 || runs[0].Choice != nil {
		t.Errorf("runs = %+v, %v", runs, err)
	}
}

func TestTheRunListSaysChoiceOnlyWhereThereIsOne(t *testing.T) {
	pid := setupCvDB(t)
	a := newCvRun(t, pid)
	if err := StoreCvAiChoice(a, "classic", "A", goodPtr()); err != nil {
		t.Fatal(err)
	}
	if err := FinishCvAiRun(a, cvAiFailed, "x"); err != nil {
		t.Fatal(err)
	}
	b := newCvRun(t, pid)
	if choiceOf(t, pid, a) == nil || choiceOf(t, pid, b) != nil {
		t.Error("the choice belongs to the run that handed it over only")
	}
}

// The tool, called through a real in-memory MCP session.
func TestHandOverCvTool(t *testing.T) {
	pid := setupCvDB(t)
	run := newCvRun(t, pid)
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1"}, nil)
	RegisterHandOverCv(server)
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
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "hand_over_cv", Arguments: args})
		if err != nil {
			t.Fatalf("hand_over_cv: %v", err)
		}
		text := ""
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				text += tc.Text
			}
		}
		return text, res.IsError
	}
	data := map[string]any{"fullName": "Ada Lovelace", "headline": "Engineer", "skills": []string{"Go"}}
	if text, isErr := call(map[string]any{"runId": "", "templateId": "classic", "title": "T", "data": data}); !isErr || !strings.Contains(text, "runId is required") {
		t.Errorf("no run id: %q %v", text, isErr)
	}
	if text, isErr := call(map[string]any{"runId": "nope", "templateId": "classic", "title": "T", "data": data}); !isErr || !strings.Contains(text, `unknown run id "nope"`) {
		t.Errorf("unknown run: %q %v", text, isErr)
	}
	if text, isErr := call(map[string]any{"runId": run, "templateId": "no-such", "title": "T", "data": data}); !isErr || !strings.Contains(text, "unknown template") || !strings.Contains(text, "list_cv_templates") {
		t.Errorf("unknown design: %q %v", text, isErr)
	}
	if choiceOf(t, pid, run) != nil {
		t.Fatal("a refused call stored a choice")
	}
	if text, isErr := call(map[string]any{"runId": run, "templateId": "classic", "title": "Ada - CV", "data": data}); isErr || !strings.Contains(text, "handed over") {
		t.Errorf("good call: %q %v", text, isErr)
	}
	c := choiceOf(t, pid, run)
	if c == nil || c.TemplateID != "classic" || c.Title != "Ada - CV" || c.Data == nil || c.Data.FullName != "Ada Lovelace" || c.Data.Headline != "Engineer" || len(c.Data.Skills) != 1 {
		t.Errorf("choice = %+v", c)
	}
	if text, isErr := call(map[string]any{"runId": run, "templateId": "classic", "title": "Again", "data": data}); !isErr || !strings.Contains(text, "already handed over") {
		t.Errorf("second call: %q %v", text, isErr)
	}
}

// plan 256, step 6: data is optional. Without it the CV is made from the persona's own data, so the choice needs only a design and a title.
func TestStoreCvAiChoice_WithoutDataTheChoiceIsADesignAndATitle(t *testing.T) {
	pid := setupCvDB(t)
	run := newCvRun(t, pid)
	if err := StoreCvAiChoice(run, "side-navy", "  Ada - CV  ", nil); err != nil {
		t.Fatal(err)
	}
	got := choiceOf(t, pid, run)
	if got == nil || got.TemplateID != "side-navy" || got.Title != "Ada - CV" || got.Data != nil {
		t.Fatalf("choice = %+v", got)
	}
	// the data key is left out of the stored JSON and of the list's answer
	var raw string
	if err := db.CareerDB.QueryRow(`SELECT choice_json FROM cv_ai_runs WHERE id = ?`, run).Scan(&raw); err != nil || strings.Contains(raw, `"data"`) {
		t.Errorf("stored %q (%v)", raw, err)
	}
	if err := StoreCvAiChoice(run, "classic", "Again", nil); !errors.Is(err, ErrCvAiChoiceAlreadyHandedOver) {
		t.Errorf("a second choice: %v", err)
	}
}

func TestStoreCvAiChoice_WithoutDataTheDesignAndTheTitleAreStillChecked(t *testing.T) {
	pid := setupCvDB(t)
	run := newCvRun(t, pid)
	for _, c := range []struct{ name, template, title string }{
		{"an unknown design", "no-such-design", "T"},
		{"no design", "", "T"},
		{"an empty title", "classic", "   "},
		{"a title that is too long", "classic", strings.Repeat("x", maxCvAiTitle+1)},
	} {
		err := StoreCvAiChoice(run, c.template, c.title, nil)
		if err == nil {
			t.Errorf("%s: accepted", c.name)
		} else if strings.Contains(c.name, "design") && !strings.Contains(err.Error(), "list_cv_templates") {
			t.Errorf("%s: the refusal must say where to find the designs: %v", c.name, err)
		}
		if choiceOf(t, pid, run) != nil {
			t.Fatalf("%s: the run kept a refused choice", c.name)
		}
	}
	if err := StoreCvAiChoice("nope", "classic", "T", nil); !errors.Is(err, ErrUnknownCvAiRun) {
		t.Errorf("unknown run: %v", err)
	}
}

func TestStoreCvAiChoice_DataThatIsGivenIsStillChecked(t *testing.T) {
	pid := setupCvDB(t)
	run := newCvRun(t, pid)
	for name, d := range map[string]templates.CvData{
		"an empty object (no name)": {},
		"markup":                    {FullName: "Ada", Summary: "<p>Hello</p>"},
	} {
		if err := StoreCvAiChoice(run, "classic", "T", &d); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if choiceOf(t, pid, run) != nil {
		t.Fatal("a refused choice was kept")
	}
}

func TestHandOverCvToolTakesNoData(t *testing.T) {
	pid := setupCvDB(t)
	run := newCvRun(t, pid)
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1"}, nil)
	RegisterHandOverCv(server)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "hand_over_cv", Arguments: map[string]any{"runId": run, "templateId": "classic", "title": "Ada - CV"}})
	if err != nil || res.IsError {
		t.Fatalf("a call without data: %v %+v", err, res)
	}
	if c := choiceOf(t, pid, run); c == nil || c.TemplateID != "classic" || c.Data != nil {
		t.Errorf("choice = %+v", c)
	}
	// the tool says that data is optional: it is not among the required arguments
	tools, err := cs.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 1 {
		t.Fatalf("tools: %v %v", tools, err)
	}
	schema, _ := json.Marshal(tools.Tools[0].InputSchema)
	var sc struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(schema, &sc); err != nil {
		t.Fatal(err)
	}
	for _, r := range sc.Required {
		if r == "data" {
			t.Errorf("data must not be required: %v", sc.Required)
		}
	}
	if !strings.Contains(tools.Tools[0].Description, "Leave data out otherwise") {
		t.Error("the description must say when to leave data out")
	}
}
