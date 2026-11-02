package cv

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// A scripted weak model works through a long CV over a real MCP client, the way the logs of plan 165 show it: pieces as text, a
// piece with a wrong value, a premature finish, a missing run id, a job sent twice, and a resend after the page ended the run. The
// stored proposal must hold exactly the corrected pieces, and every refusal must tell the model what to do (plan 165, step 5).
func TestAScriptedWeakModelFinishesALongCVPieceByPiece(t *testing.T) {
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
	rounds := 0
	call := func(name string, args map[string]any) (string, bool) {
		t.Helper()
		rounds++
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s rejected by the schema before running: %v", name, err)
		}
		return textOf(res), res.IsError
	}
	mustOK := func(name string, args map[string]any) {
		t.Helper()
		if msg, isErr := call(name, args); isErr {
			t.Fatalf("%s refused: %s", name, msg)
		}
	}
	mustRefuse := func(name string, args map[string]any, want string) {
		t.Helper()
		msg, isErr := call(name, args)
		if !isErr || !strings.Contains(msg, want) {
			t.Fatalf("%s: error %v %q, want a refusal mentioning %q", name, isErr, msg, want)
		}
	}
	p := okProposal()

	mustRefuse("cv_import_finish", map[string]any{}, "still missing: profile, persona, personalDetails")
	mustOK("cv_import_set", map[string]any{"section": "profile", "value": asText(t, p.Profile)}) // text, no run id
	badPersona := p.Persona
	badPersona.Action = "yes"
	mustRefuse("cv_import_set", map[string]any{"section": "persona", "value": asObject(t, badPersona)}, `persona.action must be "insert" or "skip"`)
	mustOK("cv_import_set", map[string]any{"runId": id, "section": "persona", "value": asObject(t, p.Persona)})
	mustRefuse("cv_import_set", map[string]any{"section": "skills", "value": "[]"}, "cv_import_add_skills")
	mustOK("cv_import_set", map[string]any{"section": "personalDetails", "value": asText(t, p.PersonalDetails)})
	mustRefuse("cv_import_finish", map[string]any{}, "skills or experience")

	var skills []SkillProposal
	for i := 0; i < 40; i++ {
		skills = append(skills, SkillProposal{Action: "insert", Value: fmt.Sprintf("Skill %d", i)})
	}
	mustOK("cv_import_add_skills", map[string]any{"items": asObject(t, skills[:30])})
	mustOK("cv_import_add_skills", map[string]any{"items": asText(t, skills[20:])}) // overlaps: 10 are the same skills

	jobs := make([]ExperienceProposal, 15)
	for i := range jobs {
		jobs[i] = ExperienceProposal{Action: "insert", Company: fmt.Sprintf("Company %d", i), Title: "Engineer", StartDate: "2010-01", Description: strings.Repeat("Did work. ", 50)}
	}
	bad := jobs[7]
	bad.Action = ""
	mustRefuse("cv_import_add_experience", map[string]any{"item": asObject(t, []ExperienceProposal{jobs[5], bad})}, "counting from 0 in this call")
	mustRefuse("cv_import_add_experience", map[string]any{"item": asObject(t, jobs)}, "at most 10 jobs per call")
	mustRefuse("cv_import_add_experience", map[string]any{"item": "[]"}, "no job")
	mustOK("cv_import_add_experience", map[string]any{"item": asObject(t, jobs[:5])})
	mustOK("cv_import_add_experience", map[string]any{"item": asText(t, jobs[5:10])})
	mustOK("cv_import_add_experience", map[string]any{"item": asObject(t, jobs[10])}) // one job, as an object
	mustOK("cv_import_add_experience", map[string]any{"item": asText(t, jobs[10:])})  // overlaps by one: still 15 jobs
	// the AI must say how many jobs the CV lists: fewer sent than stated is refused, the right number (also as text) is accepted
	mustRefuse("cv_import_finish", map[string]any{}, "needs jobsInCv")
	mustRefuse("cv_import_finish", map[string]any{"jobsInCv": 16}, "you said the CV lists 16 jobs but only 15 were sent")
	mustOK("cv_import_finish", map[string]any{"jobsInCv": "15"})

	var stored Proposal
	if err := json.Unmarshal(runOf(t, id).AIProposal, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored.Skills) != 40 || len(stored.Experience) != 15 || stored.Profile.Name != p.Profile.Name || stored.Persona.Title != p.Persona.Title {
		t.Fatalf("stored %d skills, %d jobs, profile %q, persona %q", len(stored.Skills), len(stored.Experience), stored.Profile.Name, stored.Persona.Title)
	}
	for _, e := range stored.Experience {
		if e.Action != "insert" {
			t.Errorf("a refused job slipped in: %+v", e)
		}
	}
	if rounds > 20 {
		t.Errorf("%d calls for one long CV", rounds)
	}

	// The page ends the run; a resend from the chat window then gets a clear stop, and nothing changes.
	if _, err := finishCVImportRun(id, "ready", ""); err != nil {
		t.Fatal(err)
	}
	before := string(runOf(t, id).AIProposal)
	mustRefuse("cv_import_add_skills", map[string]any{"runId": id, "items": asText(t, skills[:1])}, "start a new import")
	mustRefuse("cv_import_finish", map[string]any{"runId": id}, "do not say the analysis is done")
	mustRefuse("suggest_cv_import", map[string]any{"runId": id, "proposal": asText(t, p)}, "no longer generating")
	mustRefuse("cv_import_finish", map[string]any{}, "no import run is generating")
	if string(runOf(t, id).AIProposal) != before {
		t.Error("an ended run changed")
	}
	t.Logf("a 40-skill, 15-job CV took %d tool calls", rounds)
}
