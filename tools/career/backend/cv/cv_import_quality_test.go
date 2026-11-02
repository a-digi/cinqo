package cv

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func bulletJob(company string, bullets, lines int) ExperienceProposal {
	var parts []string
	for i := 0; i < lines; i++ {
		parts = append(parts, fmt.Sprintf("- bullet %d of %s with the numbers and tools of the CV", i+1, company))
	}
	n := FlexCount(bullets)
	return ExperienceProposal{Action: "insert", Company: company, Title: "Engineer", StartDate: "2020", Description: strings.Join(parts, "\n"), BulletsInCv: &n}
}

func withJobs(jobs ...ExperienceProposal) Proposal {
	p := okProposal()
	p.Experience = jobs
	return p
}

func TestACountMayBeSentAsANumberOrATextOfDigits(t *testing.T) {
	for in, want := range map[string]int64{`9`: 9, `"9"`: 9, `" 12 "`: 12, `9.0`: 9, `0`: 0, `-1`: -1} {
		var c FlexCount
		if err := json.Unmarshal([]byte(in), &c); err != nil || int64(c) != want {
			t.Errorf("%s: %d %v, want %d", in, c, err, want)
		}
	}
	for _, bad := range []string{`"abc"`, `""`, `9.5`, `true`, `[1]`, `{}`} {
		var c FlexCount
		if err := json.Unmarshal([]byte(bad), &c); err == nil {
			t.Errorf("%s was accepted as %d", bad, c)
		}
	}
	// absent or null: the job has no count (a pointer stays nil)
	var e ExperienceProposal
	if err := json.Unmarshal([]byte(`{"company":"A","bulletsInCv":null}`), &e); err != nil || e.BulletsInCv != nil {
		t.Errorf("null: %+v %v", e.BulletsInCv, err)
	}
}

func TestTheDescriptionNeedsALineForAtLeast70PercentOfTheBullets(t *testing.T) {
	for bullets, want := range map[int64]int{0: 0, 1: 1, 2: 2, 3: 3, 4: 3, 5: 4, 9: 7, 10: 7, 38: 27} {
		if got := requiredLines(bullets); got != want {
			t.Errorf("requiredLines(%d) = %d, want %d", bullets, got, want)
		}
	}
	// exactly at the line passes, one line below is refused with the job's name and both numbers
	if _, err := validateProposal(withJobs(bulletJob("VERSO", 9, 7))); err != nil {
		t.Errorf("7 lines for 9 bullets: %v", err)
	}
	_, err := validateProposal(withJobs(bulletJob("VERSO", 9, 6)))
	if err == nil {
		t.Fatal("6 lines for 9 bullets were accepted")
	}
	for _, want := range []string{"experience[0]", "VERSO", "9 bullet points", "6 lines", "at least 7", "send the job again"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message %q lacks %q", err, want)
		}
	}
	// two lines for nine bullets (what the real runs did) is refused
	invalidFor(t, withJobs(bulletJob("Acme", 9, 2)), "at least 7")
	// blank lines and spaces are not lines
	padded := bulletJob("Acme", 4, 2)
	padded.Description = "- one\n\n   \n- two\n\n"
	invalidFor(t, withJobs(padded), "has 2 lines")
	// a job the CV describes in a sentence: bulletsInCv 0 needs no lines, even an empty description
	zero := bulletJob("MCN", 0, 1)
	if _, err := validateProposal(withJobs(zero)); err != nil {
		t.Errorf("0 bullets: %v", err)
	}
	zero.Description = ""
	if _, err := validateProposal(withJobs(zero)); err != nil {
		t.Errorf("0 bullets, empty description: %v", err)
	}
	// a negative count is a wrong claim; a skipped job is never inserted, so it is not checked against its bullets
	neg := bulletJob("Acme", 3, 3)
	n := FlexCount(-2)
	neg.BulletsInCv = &n
	invalidFor(t, withJobs(neg), "bulletsInCv must be 0 or more")
	skipped := bulletJob("Acme", 9, 1)
	skipped.Action = "skip"
	if _, err := validateProposal(withJobs(skipped)); err != nil {
		t.Errorf("a skipped job: %v", err)
	}
	// without a count the old rules apply: a one-line description is valid
	plain := ExperienceProposal{Action: "insert", Company: "Acme", Title: "E", Description: "short"}
	if _, err := validateProposal(withJobs(plain)); err != nil {
		t.Errorf("no count: %v", err)
	}
}

func TestAShortDescriptionWithoutACountIsOnlyWarnedAbout(t *testing.T) {
	short := ExperienceProposal{Action: "insert", Company: "Acme", Title: "E", Description: "Go services."}
	long := ExperienceProposal{Action: "insert", Company: "Beta", Title: "E", Description: strings.Repeat("x", shortDescriptionRunes)}
	multi := ExperienceProposal{Action: "insert", Company: "Gamma", Title: "E", Description: "- a\n- b"}
	skipped := ExperienceProposal{Action: "skip", Company: "Delta", Title: "E", Description: "x"}
	n := FlexCount(1)
	counted := ExperienceProposal{Action: "insert", Company: "Eps", Title: "E", Description: "x", BulletsInCv: &n}
	got := shortDescriptionWarnings([]ExperienceProposal{short, long, multi, skipped, counted})
	if len(got) != 1 || !strings.Contains(got[0], `"Acme"`) || !strings.Contains(got[0], "12 characters") || !strings.Contains(got[0], "never cut it") {
		t.Fatalf("warnings = %v", got)
	}
	// 199 characters is short, 200 is not
	edge := long
	edge.Description = strings.Repeat("é", 199) // written out, so the limit itself is pinned
	if len(shortDescriptionWarnings([]ExperienceProposal{edge})) != 1 {
		t.Error("199 characters must warn")
	}
	edge.Description = strings.Repeat("é", 200)
	if len(shortDescriptionWarnings([]ExperienceProposal{edge})) != 0 {
		t.Error("200 characters must not warn")
	}
}

func TestTheDraftSaysTheSizeAndWarnsOnlyInTheCallThatMadeTheWarning(t *testing.T) {
	setup(t)
	id := newRun(t)
	short := ExperienceProposal{Action: "insert", Company: "Acme", Title: "E", StartDate: "2020", Description: "Go services."}
	d, err := AddDraftExperience(id, short)
	if err != nil || len(d.warnings) != 1 {
		t.Fatalf("warnings = %v, err %v", d.warnings, err)
	}
	if sum := d.summary(); sum["warnings"] == nil || sum["draft"].(map[string]any)["experienceChars"] != utf8.RuneCountInString("Go services.") {
		t.Errorf("summary = %v", sum)
	}
	// the same job sent again with its text replaces it: no warning, the size grows, still one job
	better := bulletJob("Acme", 9, 8)
	better.Title, better.StartDate = "E", "2020"
	d, err = AddDraftExperience(id, better)
	if err != nil || len(d.Experience) != 1 || len(d.warnings) != 0 {
		t.Fatalf("after the resend: %d jobs, warnings %v, err %v", len(d.Experience), d.warnings, err)
	}
	if _, has := d.summary()["warnings"]; has {
		t.Error("an empty warnings list must not be in the answer")
	}
	if d.experienceChars() != utf8.RuneCountInString(better.Description) {
		t.Errorf("experienceChars = %d", d.experienceChars())
	}
	// the warning is not stored with the draft
	var stored *string
	if err := dbDraft(id, &stored); err != nil || stored == nil || strings.Contains(*stored, "warning") {
		t.Errorf("stored draft = %v, %v", stored, err)
	}
}

func TestARefusedBulletCountKeepsNothingOfItsCallAndTheJobCanBeSentAgain(t *testing.T) {
	setup(t)
	id := newRun(t)
	good := bulletJob("Good", 3, 3)
	bad := bulletJob("Bad", 9, 2)
	text := asText(t, []ExperienceProposal{good, bad})
	if _, err := AddDraftExperience(id, text); err == nil || !strings.Contains(err.Error(), "experience[1] (Bad)") || !strings.Contains(err.Error(), "counting from 0 in this call") {
		t.Fatalf("err = %v", err)
	}
	d, _ := AddDraftExperience(id, bulletJob("Other", 1, 1))
	if len(d.Experience) != 1 {
		t.Errorf("the refused call kept %d jobs", len(d.Experience))
	}
	fixed := bulletJob("Bad", 9, 8)
	if d, err := AddDraftExperience(id, []ExperienceProposal{good, fixed}); err != nil || len(d.Experience) != 3 {
		t.Errorf("the corrected call: %d jobs, %v", len(d.Experience), err)
	}
	// the count may arrive as text inside the JSON of a job
	if _, err := AddDraftExperience(id, `{"action":"insert","company":"T","title":"E","startDate":"2001","description":"- a\n- b","bulletsInCv":"2"}`); err != nil {
		t.Errorf("a text count: %v", err)
	}
	if _, err := AddDraftExperience(id, `{"action":"insert","company":"U","title":"E","startDate":"2002","description":"- a","bulletsInCv":"six"}`); err == nil {
		t.Error("a count that is not a number was accepted")
	}
}

func TestFinishNeedsTheNumberOfJobsAndRefusesAShortfall(t *testing.T) {
	setup(t)
	id := newRun(t)
	sendAll(t, id, asObject) // 1 job
	if _, err := AddDraftExperience(id, []ExperienceProposal{{Action: "insert", Company: "B", Title: "E", StartDate: "2010", Description: "x"}, {Action: "insert", Company: "C", Title: "E", StartDate: "2005", Description: "y"}}); err != nil {
		t.Fatal(err)
	}
	// 3 jobs are in the draft
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"missing", nil, "needs jobsInCv"},
		{"shortfall", 4, "you said the CV lists 4 jobs but only 3 were sent"},
		{"shortfall as text", "5", "lists 5 jobs but only 3"},
		{"negative", -1, "whole number of 0 or more"},
		{"not a number", "six", "whole number of 0 or more"},
		{"negative as text", "-1", "whole number of 0 or more"},
		{"fraction", 2.5, "whole number of 0 or more"},
		{"a list", []any{3}, "whole number of 0 or more"},
	}
	for _, c := range cases {
		err := FinishDraftChecked(id, c.in)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
	}
	// a refused finish stores nothing and leaves the draft: the AI adds the missing job and finishes again
	if string(runOf(t, id).AIProposal) != noProposalJSON {
		t.Fatal("a refused finish stored a proposal")
	}
	if _, err := AddDraftExperience(id, ExperienceProposal{Action: "insert", Company: "D", Title: "E", StartDate: "2000", Description: "z"}); err != nil {
		t.Fatal(err)
	}
	if err := FinishDraftChecked(id, 4); err != nil {
		t.Fatalf("the right number: %v", err)
	}
	var stored Proposal
	if err := json.Unmarshal(runOf(t, id).AIProposal, &stored); err != nil || len(stored.Experience) != 4 {
		t.Errorf("stored %d jobs, %v", len(stored.Experience), err)
	}
}

func TestMoreJobsThanStatedAreAcceptedAndAnExactCountToo(t *testing.T) {
	setup(t)
	for _, in := range []any{3, "3", float64(2), 0} {
		id := newRun(t)
		sendAll(t, id, asObject)
		if _, err := AddDraftExperience(id, []ExperienceProposal{{Action: "insert", Company: "B", Title: "E", StartDate: "2010", Description: "x"}, {Action: "insert", Company: "C", Title: "E", StartDate: "2005", Description: "y"}}); err != nil {
			t.Fatal(err)
		}
		if err := FinishDraftChecked(id, in); err != nil {
			t.Errorf("jobsInCv %v with 3 jobs: %v", in, err)
		}
	}
}

func TestADraftWithoutJobsNeedsNoNumberButAStatedNumberMustBeMet(t *testing.T) {
	setup(t)
	id := newRun(t)
	p := okProposal()
	for section, v := range map[string]any{sectionProfile: p.Profile, sectionPersona: p.Persona, sectionPersonalDetails: p.PersonalDetails} {
		if _, err := SetDraftSection(id, section, asObject(t, v)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := AddDraftSkills(id, asObject(t, p.Skills)); err != nil {
		t.Fatal(err)
	}
	// skills only: stating 3 jobs with none sent is a shortfall; stating nothing is fine
	if err := FinishDraftChecked(id, 3); err == nil || !strings.Contains(err.Error(), "only 0 were sent") {
		t.Errorf("stated 3, sent 0: %v", err)
	}
	if err := FinishDraftChecked(id, nil); err != nil {
		t.Errorf("no jobs, no number: %v", err)
	}
	// a document that is not a CV: three skipped sections finish without any number
	notCV := newRun(t)
	for _, section := range []string{sectionProfile, sectionPersona, sectionPersonalDetails} {
		if _, err := SetDraftSection(notCV, section, map[string]any{"action": "skip"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := FinishDraftChecked(notCV, nil); err != nil {
		t.Errorf("not a CV: %v", err)
	}
}

// The page that ends a run finishes a complete draft the AI never finished, and has no count to give.
func TestThePageEndingARunStillFinishesADraftWithoutACount(t *testing.T) {
	setup(t)
	id := newRun(t)
	sendAll(t, id, asObject)
	if err := FinishDraft(id); err != nil {
		t.Fatalf("FinishDraft without a count: %v", err)
	}
}

func TestTheToolsReportWarningsSizesAndTheRefusalsOnTheWire(t *testing.T) {
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
	call := func(name string, args map[string]any) (map[string]any, string, bool) {
		t.Helper()
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s rejected before running: %v", name, err)
		}
		text := textOf(res)
		var out map[string]any
		_ = json.Unmarshal([]byte(text), &out)
		return out, text, res.IsError
	}
	p := okProposal()
	call("cv_import_set", map[string]any{"section": "profile", "value": asObject(t, p.Profile)})
	call("cv_import_set", map[string]any{"section": "persona", "value": asObject(t, p.Persona)})
	call("cv_import_set", map[string]any{"section": "personalDetails", "value": asObject(t, p.PersonalDetails)})
	call("cv_import_add_skills", map[string]any{"items": asObject(t, p.Skills)})

	// a short job without a count: accepted with a warning and the size
	out, text, isErr := call("cv_import_add_experience", map[string]any{"item": asObject(t, ExperienceProposal{Action: "insert", Company: "Acme", Title: "E", StartDate: "2020", Description: "Go services."})})
	if isErr {
		t.Fatalf("%s", text)
	}
	warnings, _ := out["warnings"].([]any)
	draft, _ := out["draft"].(map[string]any)
	if len(warnings) != 1 || !strings.Contains(warnings[0].(string), "Acme") || draft["experienceChars"] != float64(12) || draft["experience"] != float64(1) {
		t.Errorf("answer = %s", text)
	}
	// a job with a count the text does not meet: refused with the numbers
	if _, text, isErr := call("cv_import_add_experience", map[string]any{"item": asObject(t, ExperienceProposal{Action: "insert", Company: "VERSO", Title: "E", StartDate: "2024", Description: "- one\n- two", BulletsInCv: func() *FlexCount { n := FlexCount(9); return &n }()})}); !isErr || !strings.Contains(text, "VERSO") || !strings.Contains(text, "9 bullet points") || !strings.Contains(text, "at least 7") {
		t.Errorf("refusal = %v %s", isErr, text)
	}
	// finishing: the number is needed, then checked
	if _, text, isErr := call("cv_import_finish", map[string]any{}); !isErr || !strings.Contains(text, "jobsInCv") {
		t.Errorf("no number: %v %s", isErr, text)
	}
	if _, text, isErr := call("cv_import_finish", map[string]any{"jobsInCv": 2}); !isErr || !strings.Contains(text, "lists 2 jobs but only 1") {
		t.Errorf("shortfall: %v %s", isErr, text)
	}
	if _, text, isErr := call("cv_import_finish", map[string]any{"jobsInCv": "1"}); isErr {
		t.Errorf("right number as text: %s", text)
	}
	if string(runOf(t, id).AIProposal) == noProposalJSON {
		t.Error("nothing stored")
	}
}

// The one-call path checks a stated bullet count too, so both ways of handing over a proposal keep the same rule.
func TestSuggestCVImportChecksAStatedBulletCountToo(t *testing.T) {
	setup(t)
	bad := withJobs(bulletJob("VERSO", 9, 2))
	if res, _, _ := suggestCVImport(suggestCVImportArgs{RunID: newRun(t), Proposal: bad}); !res.IsError || !strings.Contains(textOf(res), "VERSO") {
		t.Errorf("a short job in a one-call proposal: %s", textOf(res))
	}
	id := newRun(t)
	if res, _, _ := suggestCVImport(suggestCVImportArgs{RunID: id, Proposal: withJobs(bulletJob("VERSO", 9, 8))}); res.IsError {
		t.Fatalf("%s", textOf(res))
	}
	var stored Proposal
	if err := json.Unmarshal(runOf(t, id).AIProposal, &stored); err != nil || stored.Experience[0].BulletsInCv == nil || *stored.Experience[0].BulletsInCv != 9 {
		t.Errorf("the stated count is kept with the job: %+v %v", stored.Experience, err)
	}
}
