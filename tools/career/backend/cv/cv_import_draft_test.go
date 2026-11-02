package cv

import (
	"career-tool-backend/db"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func asObject(t *testing.T, v any) any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func asText(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// sendAll hands the whole okProposal over piece by piece, each in the given form (an object or its JSON text).
func sendAll(t *testing.T, runID string, form func(*testing.T, any) any) {
	t.Helper()
	p := okProposal()
	for section, v := range map[string]any{sectionProfile: p.Profile, sectionPersona: p.Persona, sectionPersonalDetails: p.PersonalDetails} {
		if _, err := SetDraftSection(runID, section, form(t, v)); err != nil {
			t.Fatalf("%s: %v", section, err)
		}
	}
	if _, err := AddDraftSkills(runID, form(t, p.Skills)); err != nil {
		t.Fatal(err)
	}
	if _, err := AddDraftExperience(runID, form(t, p.Experience[0])); err != nil {
		t.Fatal(err)
	}
}

func TestPiecesBecomeTheSameProposalAsOneCall(t *testing.T) {
	for name, form := range map[string]func(*testing.T, any) any{"objects": asObject, "json text": func(t *testing.T, v any) any { return asText(t, v) }} {
		t.Run(name, func(t *testing.T) {
			setup(t)
			id := newRun(t)
			sendAll(t, id, form)
			if err := FinishDraft(id); err != nil {
				t.Fatal(err)
			}
			whole := newRun(t)
			if err := StoreProposal(whole, okProposal()); err != nil {
				t.Fatal(err)
			}
			got, want := string(runOf(t, id).AIProposal), string(runOf(t, whole).AIProposal)
			if got != want {
				t.Errorf("pieces gave\n%s\nwant\n%s", got, want)
			}
			var draft *string
			if err := dbDraft(id, &draft); err != nil || draft != nil {
				t.Errorf("the draft was not dropped: %v %v", draft, err)
			}
		})
	}
}

func TestAWrongPieceIsRefusedAloneAndTheRestStays(t *testing.T) {
	setup(t)
	id := newRun(t)
	sendAll(t, id, asObject)
	bad := okProposal().Experience[0]
	bad.Action = "maybe"
	if _, err := AddDraftExperience(id, asObject(t, bad)); err == nil || !strings.Contains(err.Error(), "experience[0].action") {
		t.Fatalf("a wrong job was accepted: %v", err)
	}
	if _, err := SetDraftSection(id, sectionPersona, asObject(t, PersonaProposal{Action: "insert", Name: strings.Repeat("x", MaxProposalTextRunes+1)})); err == nil {
		t.Fatal("a too long name was accepted")
	}
	if _, err := AddDraftSkills(id, "not json at all"); err == nil {
		t.Fatal("text that is not JSON was accepted")
	}
	if err := FinishDraft(id); err != nil {
		t.Fatalf("the refused pieces damaged the draft: %v", err)
	}
	if got := runOf(t, id); !strings.Contains(string(got.AIProposal), "Backend Engineer") || strings.Contains(string(got.AIProposal), "maybe") {
		t.Errorf("stored %s", got.AIProposal)
	}
}

func TestAPieceSentAgainReplacesTheEarlierOne(t *testing.T) {
	setup(t)
	id := newRun(t)
	sendAll(t, id, asObject)
	sendAll(t, id, asObject)
	if _, err := AddDraftSkills(id, []SkillProposal{{Action: "insert", Value: " go "}}); err != nil {
		t.Fatal(err)
	}
	if _, err := SetDraftSection(id, sectionProfile, ProfileProposal{Action: "insert", Name: "Second name"}); err != nil {
		t.Fatal(err)
	}
	d, err := AddDraftExperience(id, okProposal().Experience[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Skills) != 2 || len(d.Experience) != 1 || d.Profile.Name != "Second name" || d.Skills[0].Value != " go " {
		t.Errorf("draft %+v", d)
	}
}

func TestFinishNamesWhatIsMissing(t *testing.T) {
	setup(t)
	id := newRun(t)
	if _, err := SetDraftSection(id, sectionProfile, ProfileProposal{Action: "insert", Name: "A"}); err != nil {
		t.Fatal(err)
	}
	err := FinishDraft(id)
	for _, want := range []string{"persona", "personalDetails", "skills or experience"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("finish error %v lacks %q", err, want)
		}
	}
	if string(runOf(t, id).AIProposal) != noProposalJSON {
		t.Error("an incomplete draft was stored as the proposal")
	}
}

func TestPiecesOfAnEndedOrUnknownRunAreRefused(t *testing.T) {
	setup(t)
	id := newRun(t)
	if _, err := finishCVImportRun(id, "failed", "boom"); err != nil {
		t.Fatal(err)
	}
	res, _, _ := draftResult(id, importDraft{}, errOf(SetDraftSection(id, sectionProfile, ProfileProposal{Action: "skip"})))
	if !res.IsError || !strings.Contains(textOf(res), "start a new import") {
		t.Errorf("ended run: %q", textOf(res))
	}
	if _, err := AddDraftSkills("nope", []SkillProposal{{Action: "skip"}}); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Errorf("unknown run: %v", err)
	}
	if err := FinishDraft(id); err == nil {
		t.Error("an ended run was finished")
	}
}

func errOf(_ importDraft, err error) error { return err }

func TestACallWithoutARunIdBelongsToTheOnlyGeneratingRun(t *testing.T) {
	setup(t)
	if _, err := resolveRunID(""); err != errNoGeneratingRun {
		t.Fatalf("no run: %v", err)
	}
	first := newRun(t)
	if got, err := resolveRunID(" "); err != nil || got != first {
		t.Fatalf("one run: %q %v", got, err)
	}
	newRun(t)
	if _, err := resolveRunID(""); err != errSeveralGeneratingRuns {
		t.Fatalf("two runs: %v", err)
	}
	if got, _ := resolveRunID(first); got != first {
		t.Errorf("a named run: %q", got)
	}
}

func TestTheProposalMayArriveAsTextToo(t *testing.T) {
	setup(t)
	id := newRun(t)
	for name, raw := range map[string]string{
		"object": asText(t, okProposal()),
		"text":   asText(t, asText(t, okProposal())),
	} {
		var args suggestCVImportArgs
		if err := json.Unmarshal([]byte(`{"proposal":`+raw+`}`), &args); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if res, _, _ := suggestCVImport(args); res.IsError {
			t.Errorf("%s refused: %s", name, textOf(res))
		}
	}
	_ = id
	var bad suggestCVImportArgs
	if err := json.Unmarshal([]byte(`{"proposal":"oops"}`), &bad); err == nil {
		t.Error("text that is not a JSON object was accepted")
	}
}

func TestTheSchemaAllowsTheProposalAsTextAndNeedsNoRunId(t *testing.T) {
	s := suggestSchema()
	prop := s.Properties["proposal"]
	if prop == nil || len(prop.Types) != 2 {
		t.Fatalf("proposal schema %+v", prop)
	}
	for _, r := range s.Required {
		if r == "runId" {
			t.Error("runId is still required")
		}
	}
}

func TestTheFourPieceToolsAreRegisteredThroughTheServer(t *testing.T) {
	setup(t)
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	RegisterSuggestCVImport(server)
	RegisterCVImportDraftTools(server)
}

func dbDraft(runID string, into **string) error {
	var s *string
	err := db.CareerDB.QueryRow(`SELECT draft_json FROM cv_import_runs WHERE id = ?`, runID).Scan(&s)
	*into = s
	return err
}

// Through a real MCP client the schema check runs before the tool: the text forms and a missing runId must get past it.
func TestThroughTheProtocolTheTextFormsAndAMissingRunIdPassTheSchema(t *testing.T) {
	setup(t)
	id := newRun(t)
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	RegisterSuggestCVImport(server)
	RegisterCVImportDraftTools(server)
	ct, st := mcp.NewInMemoryTransports()
	ctx := t.Context()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	call := func(name string, args map[string]any) string {
		t.Helper()
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s rejected before running: %v", name, err)
		}
		if res.IsError {
			t.Fatalf("%s: %s", name, textOf(res))
		}
		return textOf(res)
	}
	p := okProposal()
	call("cv_import_set", map[string]any{"section": "profile", "value": asText(t, p.Profile)})
	call("cv_import_set", map[string]any{"runId": id, "section": "persona", "value": asObject(t, p.Persona)})
	call("cv_import_set", map[string]any{"section": "personalDetails", "value": asObject(t, p.PersonalDetails)})
	call("cv_import_add_skills", map[string]any{"items": asText(t, p.Skills)})
	call("cv_import_add_experience", map[string]any{"item": asText(t, p.Experience[0])})
	call("cv_import_finish", map[string]any{})
	if string(runOf(t, id).AIProposal) == noProposalJSON {
		t.Fatal("nothing stored")
	}
	if _, err := finishCVImportRun(id, "ready", ""); err != nil { // the page ends the run
		t.Fatal(err)
	}
	second := newRun(t)
	call("suggest_cv_import", map[string]any{"proposal": asText(t, p)})
	if string(runOf(t, second).AIProposal) == noProposalJSON {
		t.Error("the text form of the proposal was not stored")
	}
}

// The core only offers a tool the manifest declares, with the scope the user's token must hold.
func TestTheManifestDeclaresEveryImportTool(t *testing.T) {
	raw, err := os.ReadFile("../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		MCPTools []struct {
			Name          string `json:"name"`
			RequiredScope string `json:"required_scope"`
		} `json:"mcp_tools"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	scopes := map[string]string{}
	for _, tool := range m.MCPTools {
		scopes[tool.Name] = tool.RequiredScope
	}
	for _, name := range []string{"suggest_cv_import", "cv_import_set", "cv_import_add_skills", "cv_import_add_experience", "cv_import_finish"} {
		if scopes[name] != "tool:career:cv_import" {
			t.Errorf("manifest tool %s has scope %q, want tool:career:cv_import", name, scopes[name])
		}
	}
}

// main.go must register the piece tools next to suggest_cv_import, or the manifest declares tools the server does not serve.
func TestMainRegistersThePieceTools(t *testing.T) {
	raw, err := os.ReadFile("../main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "cv.RegisterCVImportDraftTools(server)") {
		t.Error("main.go does not register the CV import piece tools")
	}
}
