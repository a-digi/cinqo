package persona

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
)

type storedRole struct{ Company, Title, Start, End, Description string }

func ownRoles(t *testing.T, personaID string) []storedRole {
	t.Helper()
	rows, err := db.CareerDB.Query(`SELECT company, title, COALESCE(start_date,''), COALESCE(end_date,''), COALESCE(description,'') FROM career_experience WHERE persona_id = ? ORDER BY rowid`, personaID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := []storedRole{}
	for rows.Next() {
		var r storedRole
		if err := rows.Scan(&r.Company, &r.Title, &r.Start, &r.End, &r.Description); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func role(company, title, start string) BulkExperience {
	return BulkExperience{Company: company, Title: title, StartDate: start}
}

func TestExperiencesAreAddedTrimmedInOrderWithTheirIdsAndAnnouncedOnce(t *testing.T) {
	pid, id := newBulkPersona(t)
	ev := capturePersonaEvents(t)
	res, err := AddCareerExperiences(id, []BulkExperience{
		{Company: "  Acme  ", Title: " Senior Engineer ", StartDate: " 2020-01 ", EndDate: "", Description: "  Built things.  "},
		{Company: "Beta", Title: "Engineer", StartDate: "2016", EndDate: "  2019-12  ", Description: "Ran things."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Added) != 2 || res.Skipped == nil || len(res.Skipped) != 0 {
		t.Fatalf("result %+v (skipped must be an empty list, not null)", res)
	}
	if res.Added[0].Index != 0 || res.Added[0].Company != "Acme" || res.Added[0].Title != "Senior Engineer" || res.Added[1].Index != 1 || res.Added[0].ID == "" || res.Added[0].ID == res.Added[1].ID {
		t.Errorf("added %+v", res.Added)
	}
	want := []storedRole{{"Acme", "Senior Engineer", "2020-01", "", "Built things."}, {"Beta", "Engineer", "2016", "2019-12", "Ran things."}}
	if got := ownRoles(t, id); !reflect.DeepEqual(got, want) {
		t.Errorf("stored %+v", got)
	}
	var id0 string
	if err := db.CareerDB.QueryRow(`SELECT id FROM career_experience WHERE persona_id = ? AND company = 'Acme'`, id).Scan(&id0); err != nil || id0 != res.Added[0].ID {
		t.Errorf("the returned id must be the stored one: %v %v", id0, err)
	}
	if got := ev.take(); !reflect.DeepEqual(got, []string{"experience_added:" + id + ":" + pid}) {
		t.Errorf("events %v: the call is one change", got)
	}
}

func TestABulkCallStoresWhatNSingleCallsWould(t *testing.T) {
	pid, a := newBulkPersona(t)
	b, err := CreatePersona(pid, "Data", "")
	if err != nil {
		t.Fatal(err)
	}
	entries := []BulkExperience{
		{Company: "Acme", Title: "Lead", StartDate: "2021", EndDate: "", Description: "Led."},
		{Company: "Beta", Title: "Dev", StartDate: "2018-03", EndDate: "2020-12", Description: ""},
		{Company: "Gamma", Title: "Intern"},
	}
	if _, err := AddCareerExperiences(a, entries); err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if _, err := AddCareerExperience(b, e.Company, e.Title, e.StartDate, e.EndDate, e.Description); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := ownRoles(t, a), ownRoles(t, b); !reflect.DeepEqual(got, want) {
		t.Errorf("bulk %+v, single calls %+v", got, want)
	}
}

func TestAnEntryWithoutACompanyOrATitleRefusesTheWholeCallAndNamesEveryProblem(t *testing.T) {
	_, id := newBulkPersona(t)
	ev := capturePersonaEvents(t)
	_, err := AddCareerExperiences(id, []BulkExperience{
		role("Acme", "Dev", "2020"),
		role("  ", "Dev", "2019"),
		role("Beta", "", "2018"),
		role("", "", "2017"),
	})
	if !errors.Is(err, ErrBulkRefused) {
		t.Fatalf("err = %v", err)
	}
	for _, want := range []string{"experiences[1].company is empty", "experiences[2].title is empty", "experiences[3].company is empty", "experiences[3].title is empty"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q is missing from %q", want, err.Error())
		}
	}
	if strings.Contains(err.Error(), "experiences[0]") {
		t.Errorf("the good entry must not be named: %q", err.Error())
	}
	if got := ownRoles(t, id); len(got) != 0 {
		t.Errorf("nothing may be added: %v", got)
	}
	if got := ev.take(); len(got) != 0 {
		t.Errorf("events %v", got)
	}
}

func TestLengthAndMarkupLimitsPerField(t *testing.T) {
	_, id := newBulkPersona(t)
	long := func(n int) string { return strings.Repeat("é", n) } // characters, not bytes
	// the limits themselves are accepted
	ok := BulkExperience{Company: long(300), Title: long(300), StartDate: long(300), EndDate: long(300), Description: long(4000)}
	if res, err := AddCareerExperiences(id, []BulkExperience{ok}); err != nil || len(res.Added) != 1 {
		t.Fatalf("at the limits: %v %+v", err, res)
	}
	for name, e := range map[string]BulkExperience{
		"company":     {Company: long(301), Title: "T"},
		"title":       {Company: "C", Title: long(301)},
		"startDate":   {Company: "C", Title: "T", StartDate: long(301)},
		"endDate":     {Company: "C", Title: "T", EndDate: long(301)},
		"description": {Company: "C", Title: "T", Description: long(4001)},
	} {
		_, err := AddCareerExperiences(id, []BulkExperience{e})
		if !errors.Is(err, ErrBulkRefused) || !strings.Contains(err.Error(), "experiences[0]."+name+" is too long") {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, e := range map[string]BulkExperience{
		"company":     {Company: "<b>C</b>", Title: "T"},
		"title":       {Company: "C", Title: "<script>x</script>"},
		"startDate":   {Company: "C", Title: "T", StartDate: "<i>2020</i>"},
		"endDate":     {Company: "C", Title: "T", EndDate: "<i>2020</i>"},
		"description": {Company: "C", Title: "T", Description: "Did <b>things</b>"},
	} {
		_, err := AddCareerExperiences(id, []BulkExperience{e})
		if !errors.Is(err, ErrBulkRefused) || !strings.Contains(err.Error(), "experiences[0]."+name+" must be plain text") {
			t.Errorf("markup in %s: %v", name, err)
		}
	}
	if got := len(ownRoles(t, id)); got != 1 {
		t.Errorf("only the first call may have added: %d", got)
	}
}

func TestAnEntryThePersonaHasAlreadyOrTheCallRepeatsIsSkippedAndReported(t *testing.T) {
	_, id := newBulkPersona(t)
	if _, err := AddCareerExperience(id, "Acme", "Dev", "2020", "", "first"); err != nil {
		t.Fatal(err)
	}
	// a stored entry without a start date (NULL) counts as an empty start date
	if _, err := db.CareerDB.Exec(`INSERT INTO career_experience (id, persona_id, company, title, start_date) VALUES ('x1', ?, 'Nodate', 'Dev', NULL)`, id); err != nil {
		t.Fatal(err)
	}
	ev := capturePersonaEvents(t)
	res, err := AddCareerExperiences(id, []BulkExperience{
		role("acme", " DEV ", "2020"), // 0: already there (case and spaces do not matter)
		role("Acme", "Dev", "2015"),   // 1: the same job title at the same employer in another stint: a new entry
		role("Beta", "Dev", "2018"),   // 2: new
		role("BETA", "dev", "2018"),   // 3: repeated in the call
		role("Nodate", "Dev", ""),     // 4: already there with no start date
		role("Beta", "Dev", ""),       // 5: a different (empty) start date: new
	})
	if err != nil {
		t.Fatal(err)
	}
	var added []int
	for _, a := range res.Added {
		added = append(added, a.Index)
	}
	var skipped []int
	for _, s := range res.Skipped {
		skipped = append(skipped, s.Index)
	}
	if !reflect.DeepEqual(added, []int{1, 2, 5}) || !reflect.DeepEqual(skipped, []int{0, 3, 4}) {
		t.Errorf("added %v skipped %v: %+v", added, skipped, res)
	}
	if res.Skipped[1].Company != "BETA" || res.Skipped[1].StartDate != "2018" {
		t.Errorf("a skipped entry is echoed as sent (trimmed): %+v", res.Skipped[1])
	}
	if got := ownRoles(t, id); len(got) != 2+3 {
		t.Errorf("stored %d: %+v", len(got), got)
	}
	if len(ev.take()) != 1 {
		t.Error("one event")
	}
}

func TestAnExperienceCallThatAddsNothingNewAnnouncesNothing(t *testing.T) {
	_, id := newBulkPersona(t)
	_, _ = AddCareerExperience(id, "Acme", "Dev", "2020", "", "")
	ev := capturePersonaEvents(t)
	res, err := AddCareerExperiences(id, []BulkExperience{role("Acme", "Dev", "2020")})
	if err != nil || len(res.Added) != 0 || len(res.Skipped) != 1 || res.Added == nil {
		t.Fatalf("%+v %v", res, err)
	}
	if got := ev.take(); len(got) != 0 {
		t.Errorf("events %v", got)
	}
}

func TestAnotherPersonasEntriesDoNotCountAsDuplicates(t *testing.T) {
	pid, a := newBulkPersona(t)
	b, err := CreatePersona(pid, "Data", "")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = AddCareerExperience(b, "Acme", "Dev", "2020", "", "")
	if res, err := AddCareerExperiences(a, []BulkExperience{role("Acme", "Dev", "2020")}); err != nil || len(res.Added) != 1 {
		t.Fatalf("%v %+v", err, res)
	}
	if got := len(ownRoles(t, b)); got != 1 {
		t.Errorf("the other persona changed: %d", got)
	}
}

func TestTheExperienceLimitIsTenAnEmptyListAndAnUnknownPersonaAreRefused(t *testing.T) {
	_, id := newBulkPersona(t)
	ten := make([]BulkExperience, 10)
	for i := range ten {
		ten[i] = role(fmt.Sprintf("Company %d", i), "Dev", "")
	}
	if res, err := AddCareerExperiences(id, ten); err != nil || len(res.Added) != 10 {
		t.Fatalf("10 must pass: %v %+v", err, res)
	}
	if _, err := AddCareerExperiences(id, append(append([]BulkExperience{}, ten...), role("Eleven", "Dev", ""))); !errors.Is(err, ErrBulkRefused) || !strings.Contains(err.Error(), "max 10") {
		t.Errorf("11: %v", err)
	}
	if _, err := AddCareerExperiences(id, nil); !errors.Is(err, ErrBulkRefused) || !strings.Contains(err.Error(), "experiences is empty") {
		t.Errorf("empty: %v", err)
	}
	if _, err := AddCareerExperiences("nope", ten[:1]); !errors.Is(err, ErrUnknownPersona) {
		t.Errorf("unknown persona: %v", err)
	}
	if _, err := AddCareerExperiences("nope", nil); !errors.Is(err, ErrUnknownPersona) {
		t.Errorf("unknown persona with an empty list: %v", err)
	}
	if got := len(ownRoles(t, id)); got != 10 {
		t.Errorf("stored %d", got)
	}
}

func TestTheExperienceMessageNamesAtMostEightProblemsAndCountsTheRest(t *testing.T) {
	_, id := newBulkPersona(t)
	_, err := AddCareerExperiences(id, make([]BulkExperience, 6)) // 12 problems: a company and a title each
	if err == nil || !strings.Contains(err.Error(), "(and 4 more)") || strings.Contains(err.Error(), "experiences[4]") {
		t.Errorf("%v", err)
	}
}

// The tool through a real in-memory MCP session.
func TestAddCareerExperiencesThroughMCP(t *testing.T) {
	_, id := newBulkPersona(t)
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1"}, nil)
	RegisterAddCareerExperiences(server)
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
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "add_career_experiences", Arguments: args})
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
	entry := func(company, title, start string) map[string]any {
		return map[string]any{"company": company, "title": title, "startDate": start}
	}

	text, isErr := call(map[string]any{"personaId": id, "experiences": []map[string]any{entry("Acme", "Dev", "2020"), entry("acme", "dev", "2020"), entry("Beta", "Lead", "2018")}})
	if isErr {
		t.Fatalf("%s", text)
	}
	var out BulkExperiencesResult
	if err := json.Unmarshal([]byte(text), &out); err != nil || len(out.Added) != 2 || len(out.Skipped) != 1 || out.Skipped[0].Index != 1 || out.Added[1].Index != 2 {
		t.Errorf("%s %v", text, err)
	}
	if text, isErr := call(map[string]any{"experiences": []map[string]any{entry("A", "B", "")}}); !isErr || !strings.Contains(text, "personaId") {
		t.Errorf("no persona: %v %s", isErr, text)
	}
	if text, isErr := call(map[string]any{"personaId": "", "experiences": []map[string]any{entry("A", "B", "")}}); !isErr || !strings.Contains(text, "personaId is required") {
		t.Errorf("empty persona: %v %s", isErr, text)
	}
	if text, isErr := call(map[string]any{"personaId": "nope", "experiences": []map[string]any{entry("A", "B", "")}}); !isErr || !strings.Contains(text, `unknown persona id "nope"`) {
		t.Errorf("unknown persona: %v %s", isErr, text)
	}
	if text, isErr := call(map[string]any{"personaId": id, "experiences": []map[string]any{entry("Gamma", "Dev", ""), entry("", "Dev", "")}}); !isErr || !strings.Contains(text, "experiences[1].company is empty") || !strings.Contains(text, "nothing was added") {
		t.Errorf("refused: %v %s", isErr, text)
	}
	if got := ownRoles(t, id); len(got) != 2 {
		t.Errorf("a refused call must add nothing: %+v", got)
	}
}
