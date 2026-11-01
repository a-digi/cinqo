package cv

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func invalidFor(t *testing.T, p Proposal, wantInMessage string) {
	t.Helper()
	_, err := validateProposal(p)
	if err == nil {
		t.Errorf("accepted, want an error mentioning %q", wantInMessage)
		return
	}
	var bad *proposalProblem
	if !asProblem(err, &bad) || !strings.Contains(bad.msg, wantInMessage) {
		t.Errorf("error %q, want it to mention %q", err, wantInMessage)
	}
}

func asProblem(err error, target **proposalProblem) bool {
	p, ok := err.(*proposalProblem)
	if ok {
		*target = p
	}
	return ok
}

func TestAValidProposalPasses(t *testing.T) {
	got, err := validateProposal(okProposal())
	if err != nil || got.Profile.Name != "Ada Lovelace" {
		t.Fatalf("%+v %v", got, err)
	}
	// the smallest valid proposal: every item skipped, nothing in the lists
	empty := Proposal{Profile: ProfileProposal{Action: "skip"}, Persona: PersonaProposal{Action: "skip"}, PersonalDetails: DetailsProposal{Action: "skip"}}
	got, err = validateProposal(empty)
	if err != nil || got.Skills == nil || got.Experience == nil || len(got.Skills) != 0 || len(got.Experience) != 0 {
		t.Errorf("%+v %v", got, err)
	}
}

func TestEveryItemNeedsAnInsertOrSkipAction(t *testing.T) {
	for _, bad := range []string{"", "INSERT", "update", "delete", " insert", "insert "} {
		cases := map[string]func(*Proposal){
			"profile.action":         func(p *Proposal) { p.Profile.Action = bad },
			"persona.action":         func(p *Proposal) { p.Persona.Action = bad },
			"personalDetails.action": func(p *Proposal) { p.PersonalDetails.Action = bad },
			"skills[1].action":       func(p *Proposal) { p.Skills[1].Action = bad },
			"experience[0].action":   func(p *Proposal) { p.Experience[0].Action = bad },
		}
		for path, mutate := range cases {
			p := okProposal()
			mutate(&p)
			t.Run(fmt.Sprintf("%s=%q", path, bad), func(t *testing.T) { invalidFor(t, p, path) })
		}
	}
}

func TestTheSizesAreBounded(t *testing.T) {
	skills := func(n int) []SkillProposal {
		out := make([]SkillProposal, n)
		for i := range out {
			out[i] = SkillProposal{Action: "insert", Value: fmt.Sprintf("skill %d", i)}
		}
		return out
	}
	experience := func(n int) []ExperienceProposal {
		out := make([]ExperienceProposal, n)
		for i := range out {
			out[i] = ExperienceProposal{Action: "insert", Company: "c", Title: "t"}
		}
		return out
	}
	p := okProposal()
	p.Skills = skills(MaxProposalSkills)
	p.Experience = experience(MaxProposalExperience)
	if _, err := validateProposal(p); err != nil {
		t.Errorf("at the limit: %v", err)
	}
	p.Skills = skills(MaxProposalSkills + 1)
	invalidFor(t, p, "skills can have at most")
	p = okProposal()
	p.Experience = experience(MaxProposalExperience + 1)
	invalidFor(t, p, "experience can have at most")
	if MaxProposalSkills != 200 || MaxProposalExperience != 50 || MaxProposalTextRunes != 4000 {
		t.Errorf("limits %d %d %d", MaxProposalSkills, MaxProposalExperience, MaxProposalTextRunes)
	}
}

func TestEveryTextFieldIsBoundedInCharacters(t *testing.T) {
	fit, over := strings.Repeat("x", MaxProposalTextRunes), strings.Repeat("x", MaxProposalTextRunes+1)
	fitMulti, overMulti := strings.Repeat("é", MaxProposalTextRunes), strings.Repeat("é", MaxProposalTextRunes+1)
	cases := map[string]func(*Proposal, string){
		"profile.name":                        func(p *Proposal, v string) { p.Profile.Name = v },
		"profile.possibleDuplicateOf":         func(p *Proposal, v string) { p.Profile.PossibleDuplicateOf = &v },
		"persona.name":                        func(p *Proposal, v string) { p.Persona.Name = v },
		"persona.title":                       func(p *Proposal, v string) { p.Persona.Title = v },
		"persona.possibleDuplicateOf":         func(p *Proposal, v string) { p.Persona.PossibleDuplicateOf = &v },
		"personalDetails.headline":            func(p *Proposal, v string) { p.PersonalDetails.Headline = v },
		"personalDetails.summary":             func(p *Proposal, v string) { p.PersonalDetails.Summary = v },
		"personalDetails.location":            func(p *Proposal, v string) { p.PersonalDetails.Location = v },
		"personalDetails.desiredTitles":       func(p *Proposal, v string) { p.PersonalDetails.DesiredTitles = v },
		"personalDetails.desiredLocations":    func(p *Proposal, v string) { p.PersonalDetails.DesiredLocations = v },
		"personalDetails.possibleDuplicateOf": func(p *Proposal, v string) { p.PersonalDetails.PossibleDuplicateOf = &v },
		"skills[0].value":                     func(p *Proposal, v string) { p.Skills[0].Value = v },
		"skills[0].possibleDuplicateOf":       func(p *Proposal, v string) { p.Skills[0].PossibleDuplicateOf = &v },
		"experience[0].company":               func(p *Proposal, v string) { p.Experience[0].Company = v },
		"experience[0].title":                 func(p *Proposal, v string) { p.Experience[0].Title = v },
		"experience[0].startDate":             func(p *Proposal, v string) { p.Experience[0].StartDate = v },
		"experience[0].endDate":               func(p *Proposal, v string) { p.Experience[0].EndDate = v },
		"experience[0].description":           func(p *Proposal, v string) { p.Experience[0].Description = v },
		"experience[0].possibleDuplicateOf":   func(p *Proposal, v string) { p.Experience[0].PossibleDuplicateOf = &v },
	}
	for path, set := range cases {
		for _, v := range []string{fit, fitMulti} {
			p := okProposal()
			set(&p, v)
			if _, err := validateProposal(p); err != nil {
				t.Errorf("%s at the limit: %v", path, err)
			}
		}
		for _, v := range []string{over, overMulti} {
			p := okProposal()
			set(&p, v)
			t.Run(path, func(t *testing.T) { invalidFor(t, p, path) })
		}
	}
}

func TestTheMinimumSalaryCannotBeNegative(t *testing.T) {
	p := okProposal()
	p.PersonalDetails.MinSalary = 0
	if _, err := validateProposal(p); err != nil {
		t.Errorf("0: %v", err)
	}
	p.PersonalDetails.MinSalary = -1
	invalidFor(t, p, "personalDetails.minSalary")
}

// What is stored is the shape the review page reads (parseProposal.ts): these keys, these types, arrays and null never missing.
func TestTheStoredProposalHasTheShapeTheReviewPageReads(t *testing.T) {
	setup(t)
	id := newRun(t)
	p := okProposal()
	p.Skills = nil
	p.Persona.PossibleDuplicateOf = str("persona-id-1")
	if err := StoreProposal(id, p); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(runOf(t, id).AIProposal, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"profile", "persona", "personalDetails", "skills", "experience"} {
		if _, ok := m[k]; !ok {
			t.Errorf("key %q missing", k)
		}
	}
	if skills, ok := m["skills"].([]any); !ok || len(skills) != 0 {
		t.Errorf("skills should be [] and not null: %#v", m["skills"])
	}
	profile := m["profile"].(map[string]any)
	if v, ok := profile["possibleDuplicateOf"]; !ok || v != nil || profile["name"] != "Ada Lovelace" || profile["action"] != "insert" {
		t.Errorf("profile %#v (possibleDuplicateOf must be present and null)", profile)
	}
	persona := m["persona"].(map[string]any)
	if persona["possibleDuplicateOf"] != "persona-id-1" || persona["title"] != "Senior Backend Engineer" {
		t.Errorf("persona %#v", persona)
	}
	details := m["personalDetails"].(map[string]any)
	if _, isNumber := details["minSalary"].(float64); !isNumber || details["minSalary"] != float64(70000) || details["desiredLocations"] != "Hamburg, Remote" {
		t.Errorf("details %#v", details)
	}
	// the skills' keys too (the second skill names a duplicate)
	p2 := okProposal()
	id2 := newRun(t)
	if err := StoreProposal(id2, p2); err != nil {
		t.Fatal(err)
	}
	var m2 map[string]any
	_ = json.Unmarshal(runOf(t, id2).AIProposal, &m2)
	skills := m2["skills"].([]any)
	first, second := skills[0].(map[string]any), skills[1].(map[string]any)
	if first["value"] != "Go" || first["action"] != "insert" || second["possibleDuplicateOf"] != "Docker" || second["action"] != "skip" {
		t.Errorf("skills %#v", skills)
	}
	if v, ok := first["possibleDuplicateOf"]; !ok || v != nil {
		t.Errorf("a skill without a duplicate must carry possibleDuplicateOf null: %#v", first)
	}
	exp := m["experience"].([]any)[0].(map[string]any)
	for _, k := range []string{"company", "title", "startDate", "endDate", "description", "action", "possibleDuplicateOf"} {
		if _, ok := exp[k]; !ok {
			t.Errorf("experience key %q missing", k)
		}
	}
}

// ---- the tool ----

func TestTheToolStoresTheProposalAndSaysSo(t *testing.T) {
	setup(t)
	id := newRun(t)
	res, _, err := suggestCVImport(suggestCVImportArgs{RunID: id, Proposal: okProposal()})
	if err != nil || res.IsError || !strings.Contains(textOf(res), `"stored": true`) {
		t.Fatalf("%+v %v", res, err)
	}
	if string(runOf(t, id).AIProposal) == "null" {
		t.Error("nothing was stored")
	}
}

func TestTheToolRefusesWhatItCannotStoreWithAMessage(t *testing.T) {
	setup(t)
	id := newRun(t)
	done := newRun(t)
	_ = StoreProposal(done, okProposal())
	_, _ = finishCVImportRun(done, "ready", "")
	bad := okProposal()
	bad.Skills[0].Action = "maybe"
	for name, tc := range map[string]struct {
		args suggestCVImportArgs
		want string
	}{
		"no run id":    {suggestCVImportArgs{Proposal: okProposal()}, "runId is required"},
		"unknown run":  {suggestCVImportArgs{RunID: "nope", Proposal: okProposal()}, `unknown run id "nope"`},
		"finished run": {suggestCVImportArgs{RunID: done, Proposal: okProposal()}, "no longer generating"},
		"invalid":      {suggestCVImportArgs{RunID: id, Proposal: bad}, "the proposal is not valid: skills[0].action"},
	} {
		res, _, err := suggestCVImport(tc.args)
		if err != nil || res == nil || !res.IsError || !strings.Contains(textOf(res), tc.want) {
			t.Errorf("%s: %q (error result %v, %v), want %q", name, textOf(res), res != nil && res.IsError, err, tc.want)
		}
	}
	if string(runOf(t, id).AIProposal) != "null" {
		t.Error("a refused proposal was stored")
	}
}

// A refused proposal can be corrected and sent again within the same run.
func TestARefusedProposalCanBeCorrected(t *testing.T) {
	setup(t)
	id := newRun(t)
	bad := okProposal()
	bad.Persona.Action = ""
	if res, _, _ := suggestCVImport(suggestCVImportArgs{RunID: id, Proposal: bad}); !res.IsError {
		t.Fatal("accepted")
	}
	if res, _, _ := suggestCVImport(suggestCVImportArgs{RunID: id, Proposal: okProposal()}); res.IsError {
		t.Errorf("the corrected proposal was refused: %s", textOf(res))
	}
	if _, err := finishCVImportRun(id, "ready", ""); err != nil {
		t.Errorf("%v", err)
	}
}

func textOf(res *mcp.CallToolResult) string {
	if res == nil {
		return ""
	}
	var b strings.Builder
	for _, c := range res.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			b.WriteString(t.Text)
		}
	}
	return b.String()
}
