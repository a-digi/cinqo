package cv

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// A real run of the CV import with a small local model (2026-11-03) sent cv_import_add_skills the wrapper of cv_import_set:
// {"runId": ..., "value": {"action":"insert","possibleDuplicateOf":null,"value":"Project Management"}}. The schema refused the unknown
// argument four times and the turn failed. The three add tools now accept "value" for one entry (or a list), like cv_import_set, and
// the skills tool accepts one entry where it says "a list".

func session(t *testing.T) (*mcp.ClientSession, func(name string, args map[string]any) (string, bool)) {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	RegisterSuggestCVImport(server)
	RegisterCVImportDraftTools(server)
	ct, st := mcp.NewInMemoryTransports()
	ctx := t.Context()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	s, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	call := func(name string, args map[string]any) (string, bool) {
		res, err := s.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			return err.Error(), false // refused by the schema before the tool ran
		}
		return textOf(res), !res.IsError
	}
	return s, call
}

func draftOf(t *testing.T, id string) importDraft {
	t.Helper()
	var raw *string
	if err := dbDraft(id, &raw); err != nil {
		t.Fatalf("no draft: %v", err)
	}
	var d importDraft
	if raw == nil { // nothing was ever stored for this run
		return d
	}
	if err := json.Unmarshal([]byte(*raw), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestTheSkillsCallThatFailedTheRealRunIsAccepted(t *testing.T) {
	setup(t)
	id := newRun(t)
	_, call := session(t)
	out, ok := call("cv_import_add_skills", map[string]any{"runId": id, "value": map[string]any{"action": "insert", "possibleDuplicateOf": nil, "value": "Project Management"}})
	if !ok {
		t.Fatalf("the call of the real run is refused: %s", out)
	}
	d := draftOf(t, id)
	if len(d.Skills) != 1 || d.Skills[0].Value != "Project Management" || d.Skills[0].Action != "insert" {
		t.Errorf("skills = %+v", d.Skills)
	}
}

func TestEveryAddToolAcceptsValueForOneEntryOrAList(t *testing.T) {
	setup(t)
	id := newRun(t)
	_, call := session(t)
	job := map[string]any{"action": "insert", "company": "Acme", "title": "Engineer", "startDate": "2019-03", "endDate": "2023-01", "description": "Built Go services.", "bulletsInCv": 0}
	edu := map[string]any{"action": "insert", "institution": "TU Berlin", "degree": "BSc", "startDate": "2012", "endDate": "2015"}
	for name, args := range map[string]map[string]any{
		"skills, one entry":        {"value": map[string]any{"action": "insert", "value": "Go"}},
		"skills, a list":           {"value": []any{map[string]any{"action": "insert", "value": "SQL"}, map[string]any{"action": "insert", "value": "Docker"}}},
		"skills, the text of one":  {"value": `{"action":"insert","value":"Kubernetes"}`},
		"skills, the text of many": {"value": `[{"action":"insert","value":"gRPC"}]`},
	} {
		args["runId"] = id
		if out, ok := call("cv_import_add_skills", args); !ok {
			t.Errorf("%s: %s", name, out)
		}
	}
	if got := len(draftOf(t, id).Skills); got != 5 {
		t.Errorf("skills stored = %d, want 5", got)
	}
	if out, ok := call("cv_import_add_experience", map[string]any{"runId": id, "value": job}); !ok {
		t.Errorf("experience, one job: %s", out)
	}
	if out, ok := call("cv_import_add_experience", map[string]any{"runId": id, "value": []any{map[string]any{"action": "insert", "company": "Globex", "title": "Dev", "startDate": "2016-01", "endDate": "2019-02", "description": "Wrote tools.", "bulletsInCv": 0}}}); !ok {
		t.Errorf("experience, a list: %s", out)
	}
	if got := len(draftOf(t, id).Experience); got != 2 {
		t.Errorf("jobs stored = %d, want 2", got)
	}
	if out, ok := call("cv_import_add_education", map[string]any{"runId": id, "value": edu}); !ok {
		t.Errorf("education, one entry: %s", out)
	}
	if got := len(draftOf(t, id).Education); got != 1 {
		t.Errorf("education stored = %d, want 1", got)
	}
}

func TestTheNamesTheToolsAlreadyHadStillWorkAndComeFirst(t *testing.T) {
	setup(t)
	id := newRun(t)
	_, call := session(t)
	one := func(v string) map[string]any { return map[string]any{"action": "insert", "value": v} }
	// items and skills as before
	if out, ok := call("cv_import_add_skills", map[string]any{"runId": id, "items": []any{one("A")}}); !ok {
		t.Errorf("items: %s", out)
	}
	if out, ok := call("cv_import_add_skills", map[string]any{"runId": id, "skills": []any{one("B")}}); !ok {
		t.Errorf("skills: %s", out)
	}
	// When more than one name is sent, the earlier name wins (items, then skills, then value): nothing is merged or guessed.
	if out, ok := call("cv_import_add_skills", map[string]any{"runId": id, "items": []any{one("C")}, "value": one("IGNORED")}); !ok {
		t.Errorf("both: %s", out)
	}
	got := map[string]bool{}
	for _, s := range draftOf(t, id).Skills {
		got[s.Value] = true
	}
	if !got["A"] || !got["B"] || !got["C"] || got["IGNORED"] || len(got) != 3 {
		t.Errorf("skills stored = %v", got)
	}
}

func TestAWrongShapeIsStillRefusedWithAClearMessage(t *testing.T) {
	setup(t)
	id := newRun(t)
	_, call := session(t)
	for name, args := range map[string]map[string]any{
		"nothing at all":      {"runId": id},
		"a number":            {"runId": id, "value": 7},
		"a string of no json": {"runId": id, "value": "Go, SQL"},
		"a list of numbers":   {"runId": id, "value": []any{1, 2}},
	} {
		out, ok := call("cv_import_add_skills", args)
		if ok {
			t.Errorf("%s: must be refused", name)
			continue
		}
		if !strings.Contains(out, "items") && !strings.Contains(out, "shape") && !strings.Contains(out, "right shape") && !strings.Contains(out, "required") {
			t.Errorf("%s: the refusal must say what is wanted: %q", name, out)
		}
	}
	if len(draftOf(t, id).Skills) != 0 {
		t.Error("a refused call must store nothing")
	}
}

func TestTheSchemasOfTheThreeToolsNameValueAndStillRequireNothing(t *testing.T) {
	setup(t)
	s, _ := session(t)
	res, err := s.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		if tool.Name != "cv_import_add_skills" && tool.Name != "cv_import_add_experience" && tool.Name != "cv_import_add_education" {
			continue
		}
		raw, _ := json.Marshal(tool.InputSchema)
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		if _, ok := schema.Properties["value"]; !ok {
			t.Errorf("%s: the schema does not name value: %s", tool.Name, raw)
		}
		if len(schema.Required) != 0 {
			t.Errorf("%s: nothing may be required (a weak model names it differently from call to call): %v", tool.Name, schema.Required)
		}
	}
}
