package cv

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func edu(institution string) EducationProposal {
	return EducationProposal{Action: "insert", Institution: institution, Degree: "B.Sc.", FieldOfStudy: "Physics", StartDate: "2010-09", EndDate: "2014"}
}

func withEducation(es ...EducationProposal) Proposal {
	p := okProposal()
	p.Education = es
	return p
}

func TestAnEducationProposalIsValidatedLikeAnEntryAddedByHand(t *testing.T) {
	got, err := validateProposal(withEducation(edu("MIT"), EducationProposal{Action: "insert", Institution: "Night school"}))
	if err != nil || len(got.Education) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	// no education at all is fine and is an empty list, never null
	got, err = validateProposal(okProposal())
	if err != nil || got.Education == nil || len(got.Education) != 0 {
		t.Errorf("no education: %+v %v", got.Education, err)
	}
	// a skipped entry may be incomplete (it is never inserted)
	if _, err := validateProposal(withEducation(EducationProposal{Action: "skip"})); err != nil {
		t.Errorf("a skipped, empty entry: %v", err)
	}
	long := strings.Repeat("x", MaxProposalTextRunes+1)
	dup := long
	bad := map[string]struct {
		mutate func(*EducationProposal)
		want   string
	}{
		"action":      {func(e *EducationProposal) { e.Action = "update" }, "education[1].action"},
		"institution": {func(e *EducationProposal) { e.Institution = "  " }, "education[1]"},
		"start":       {func(e *EducationProposal) { e.StartDate = "Sep 2010" }, "YYYY"},
		"end":         {func(e *EducationProposal) { e.EndDate = "2014-13" }, "YYYY"},
		"order":       {func(e *EducationProposal) { e.StartDate, e.EndDate = "2020", "2019" }, "before the start"},
		"long":        {func(e *EducationProposal) { e.Description = long }, "education[1].description"},
		"long dup":    {func(e *EducationProposal) { e.PossibleDuplicateOf = &dup }, "education[1].possibleDuplicateOf"},
	}
	for name, c := range bad {
		e := edu("Second")
		c.mutate(&e)
		t.Run(name, func(t *testing.T) { invalidFor(t, withEducation(edu("First"), e), c.want) })
	}
	var many []EducationProposal
	for i := 0; i <= MaxProposalEducation; i++ {
		many = append(many, edu(fmt.Sprintf("S%d", i)))
	}
	invalidFor(t, withEducation(many...), "education can have at most")
}

func TestEducationPiecesAreKeptInTheDraftAndReplacedByKey(t *testing.T) {
	setup(t)
	id := newRun(t)
	d, err := AddDraftEducation(id, edu("MIT"))
	if err != nil || len(d.Education) != 1 {
		t.Fatalf("%+v %v", d.Education, err)
	}
	// a list, as an object or as its JSON text
	text, _ := json.Marshal([]EducationProposal{edu("ETH"), edu("Oxford")})
	d, err = AddDraftEducation(id, string(text))
	if err != nil || len(d.Education) != 3 {
		t.Fatalf("list as text: %+v %v", d.Education, err)
	}
	// the same institution, degree, field and start (any case, trimmed) replaces
	again := edu("  mit ")
	again.Description = "second time"
	d, err = AddDraftEducation(id, asObject(t, again))
	if err != nil || len(d.Education) != 3 || d.Education[0].Description != "second time" {
		t.Errorf("replace: %+v %v", d.Education, err)
	}
	// another start date is another entry
	other := edu("MIT")
	other.StartDate = "2015"
	other.EndDate = ""
	if d, err = AddDraftEducation(id, other); err != nil || len(d.Education) != 4 {
		t.Errorf("another entry: %+v %v", d.Education, err)
	}
	// the summary the AI sees counts it, and education is never "still missing"
	if sum := d.summary(); sum["draft"].(map[string]any)["education"] != 4 {
		t.Errorf("summary = %v", sum)
	}
	for _, m := range d.missing() {
		if strings.Contains(m, "education") {
			t.Errorf("education must not be required: %v", d.missing())
		}
	}
}

func TestARefusedEducationPieceKeepsNothingOfItsCall(t *testing.T) {
	setup(t)
	id := newRun(t)
	if _, err := AddDraftEducation(id, edu("MIT")); err != nil {
		t.Fatal(err)
	}
	bad := edu("Bad")
	bad.StartDate = "Sep 2010"
	for name, item := range map[string]any{
		"one bad entry":     bad,
		"good then bad":     []EducationProposal{edu("Good"), bad},
		"empty list":        []EducationProposal{},
		"nothing":           nil,
		"not an entry":      "just text",
		"wrong field types": `{"action":"insert","institution":5}`,
	} {
		if _, err := AddDraftEducation(id, item); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	var eleven []EducationProposal
	for i := 0; i < MaxEducationPerCall+1; i++ {
		eleven = append(eleven, edu(fmt.Sprintf("S%d", i)))
	}
	if _, err := AddDraftEducation(id, eleven); err == nil || !strings.Contains(err.Error(), "at most") {
		t.Errorf("too many in one call: %v", err)
	}
	d, err := AddDraftEducation(id, edu("MIT")) // still just the first one
	if err != nil || len(d.Education) != 1 {
		t.Errorf("a refused call changed the draft: %+v %v", d.Education, err)
	}
	// the total is bounded over several calls too, and the refused call changes nothing
	for from := 0; len(d.Education) < MaxProposalEducation; from += 5 {
		var batch []EducationProposal
		for i := 0; i < 5 && len(d.Education)+len(batch) < MaxProposalEducation; i++ {
			batch = append(batch, edu(fmt.Sprintf("T%d-%d", from, i)))
		}
		if d, err = AddDraftEducation(id, batch); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := AddDraftEducation(id, edu("One too many")); err == nil || !strings.Contains(err.Error(), "at most") {
		t.Errorf("over the total: %v", err)
	}
	if d, _ = AddDraftEducation(id, edu("MIT")); len(d.Education) != MaxProposalEducation {
		t.Errorf("draft has %d entries", len(d.Education))
	}
}

func TestFinishCarriesTheEducationAndWorksWithoutIt(t *testing.T) {
	setup(t)
	with := newRun(t)
	sendAll(t, with, asObject)
	if _, err := AddDraftEducation(with, []EducationProposal{edu("MIT"), edu("ETH")}); err != nil {
		t.Fatal(err)
	}
	if err := FinishDraft(with); err != nil {
		t.Fatal(err)
	}
	var stored Proposal
	if err := json.Unmarshal(runOf(t, with).AIProposal, &stored); err != nil || len(stored.Education) != 2 || stored.Education[0].Institution != "MIT" {
		t.Fatalf("stored education = %+v %v", stored.Education, err)
	}
	without := newRun(t)
	sendAll(t, without, asObject)
	if err := FinishDraft(without); err != nil {
		t.Fatalf("a CV without education must finish: %v", err)
	}
	if string(runOf(t, without).AIProposal) == noProposalJSON {
		t.Fatal("nothing stored")
	}
	var plain map[string]json.RawMessage
	if err := json.Unmarshal(runOf(t, without).AIProposal, &plain); err != nil {
		t.Fatal(err)
	}
	if _, has := plain["education"]; has {
		t.Errorf("an empty education list must not be written: %s", runOf(t, without).AIProposal)
	}
}

func TestSuggestCVImportTakesEducationAndDoesNotRequireIt(t *testing.T) {
	setup(t)
	id := newRun(t)
	res, _, err := suggestCVImport(suggestCVImportArgs{RunID: id, Proposal: withEducation(edu("MIT"))})
	if err != nil || res.IsError {
		t.Fatalf("%v %v", err, res)
	}
	var stored Proposal
	if err := json.Unmarshal(runOf(t, id).AIProposal, &stored); err != nil || len(stored.Education) != 1 {
		t.Fatalf("%+v %v", stored.Education, err)
	}
	if res, _, _ := suggestCVImport(suggestCVImportArgs{RunID: newRun(t), Proposal: withEducation(EducationProposal{Action: "insert", Institution: "X", StartDate: "later"})}); !res.IsError {
		t.Error("a bad date in a one-call proposal must be refused")
	}
	// the tool's schema does not require the field, so a model that does not know it is not refused before the tool runs
	schema := suggestSchema()
	proposal := schema.Properties["proposal"]
	if proposal == nil {
		t.Fatal("no proposal property")
	}
	raw, _ := json.Marshal(schema)
	if !strings.Contains(string(raw), `"education"`) {
		t.Error("the schema does not describe education")
	}
	for _, r := range schema.Required {
		if r == "education" {
			t.Error("education is required")
		}
	}
	for _, r := range proposal.Required {
		if r == "education" {
			t.Error("education is required in the proposal")
		}
	}
}

func TestTheEducationToolWorksThroughTheProtocolAndNamesTheCorrection(t *testing.T) {
	setup(t)
	id := newRun(t)
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
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
	call := func(args map[string]any) (string, bool) {
		t.Helper()
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "cv_import_add_education", Arguments: args})
		if err != nil {
			t.Fatalf("rejected before running: %v", err)
		}
		return textOf(res), res.IsError
	}
	count := func(text string) float64 {
		var out struct {
			Draft map[string]any `json:"draft"`
		}
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			t.Fatalf("%v: %s", err, text)
		}
		n, _ := out.Draft["education"].(float64)
		return n
	}
	if text, isErr := call(map[string]any{"item": asObject(t, edu("MIT"))}); isErr || count(text) != 1 {
		t.Fatalf("item as object: %s", text)
	}
	if text, isErr := call(map[string]any{"items": asText(t, []EducationProposal{edu("ETH")})}); isErr || count(text) != 2 {
		t.Fatalf("items as text, no runId: %s", text)
	}
	bad := edu("Late")
	bad.EndDate = "2000"
	if text, isErr := call(map[string]any{"runId": id, "item": asObject(t, bad)}); !isErr || !strings.Contains(text, "education[0]") || !strings.Contains(text, "before the start") {
		t.Errorf("a wrong entry must be refused with its place and reason: %v %s", isErr, text)
	}
	// the run that ended takes nothing more
	if _, err := finishCVImportRun(id, "failed", "boom"); err != nil {
		t.Fatal(err)
	}
	if text, isErr := call(map[string]any{"runId": id, "item": asObject(t, edu("Late"))}); !isErr || !strings.Contains(text, "start a new import") {
		t.Errorf("an ended run: %v %s", isErr, text)
	}
}

// The limit is 30 entries: exactly 30 pass and 31 do not (the numbers are written out so the limit itself is pinned).
func TestAProposalMayHaveExactly30EducationEntries(t *testing.T) {
	mk := func(n int) []EducationProposal {
		var out []EducationProposal
		for i := 0; i < n; i++ {
			out = append(out, edu(fmt.Sprintf("S%d", i)))
		}
		return out
	}
	if _, err := validateProposal(withEducation(mk(30)...)); err != nil {
		t.Errorf("30 entries: %v", err)
	}
	invalidFor(t, withEducation(mk(31)...), "education can have at most 30")
}
