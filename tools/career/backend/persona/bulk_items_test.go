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

func ownSkills(t *testing.T, personaID string) []string {
	t.Helper()
	rows, err := db.CareerDB.Query(`SELECT skill FROM career_skills WHERE persona_id = ? ORDER BY rowid`, personaID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func newBulkPersona(t *testing.T) (profileID, personaID string) {
	t.Helper()
	profileID = setupRunsDB(t)
	id, err := CreatePersona(profileID, "Backend", "")
	if err != nil {
		t.Fatal(err)
	}
	return profileID, id
}

func TestSkillsAreAddedTrimmedInOrderAndAnnouncedOnce(t *testing.T) {
	pid, id := newBulkPersona(t)
	ev := capturePersonaEvents(t)
	res, err := AddCareerSkills(id, []string{"Go", "  Kubernetes  ", "SQL"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Added, []string{"Go", "Kubernetes", "SQL"}) || len(res.Skipped) != 0 || res.Skipped == nil {
		t.Errorf("result %+v (skipped must be an empty list, not null)", res)
	}
	if got := ownSkills(t, id); !reflect.DeepEqual(got, []string{"Go", "Kubernetes", "SQL"}) {
		t.Errorf("stored %v", got)
	}
	if got := ev.take(); !reflect.DeepEqual(got, []string{"skill_added:" + id + ":" + pid}) {
		t.Errorf("events %v: the call is one change", got)
	}
}

func TestASkillThePersonaHasAlreadyOrTheCallRepeatsIsSkippedWithoutCase(t *testing.T) {
	_, id := newBulkPersona(t)
	if err := AddCareerSkill(id, "Go"); err != nil {
		t.Fatal(err)
	}
	ev := capturePersonaEvents(t)
	res, err := AddCareerSkills(id, []string{"go", " GO ", "Rust", "rust", "Python"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Added, []string{"Rust", "Python"}) || !reflect.DeepEqual(res.Skipped, []string{"go", "GO", "rust"}) {
		t.Errorf("result %+v", res)
	}
	if got := ownSkills(t, id); !reflect.DeepEqual(got, []string{"Go", "Rust", "Python"}) {
		t.Errorf("stored %v: the first spelling stays", got)
	}
	if len(ev.take()) != 1 {
		t.Error("one event")
	}
}

func TestACallThatAddsNothingNewAnnouncesNothing(t *testing.T) {
	_, id := newBulkPersona(t)
	_ = AddCareerSkill(id, "Go")
	ev := capturePersonaEvents(t)
	res, err := AddCareerSkills(id, []string{"go", "GO"})
	if err != nil || len(res.Added) != 0 || len(res.Skipped) != 2 {
		t.Fatalf("%+v %v", res, err)
	}
	if got := ev.take(); len(got) != 0 {
		t.Errorf("events %v", got)
	}
}

func TestOneRefusedSkillRefusesTheWholeCallAndNamesEveryProblem(t *testing.T) {
	_, id := newBulkPersona(t)
	ev := capturePersonaEvents(t)
	_, err := AddCareerSkills(id, []string{"Go", "   ", strings.Repeat("x", 301), "<b>Bold</b>", "SQL"})
	if !errors.Is(err, ErrBulkRefused) {
		t.Fatalf("err = %v", err)
	}
	for _, want := range []string{"skills[1] is empty", "skills[2] is too long (max 300 characters)", "skills[3] must be plain text"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q is missing from %q", want, err.Error())
		}
	}
	if strings.Contains(err.Error(), "skills[0]") || strings.Contains(err.Error(), "skills[4]") {
		t.Errorf("the good entries must not be named: %q", err.Error())
	}
	if got := ownSkills(t, id); len(got) != 0 {
		t.Errorf("nothing may be added: %v", got)
	}
	if got := ev.take(); len(got) != 0 {
		t.Errorf("events %v", got)
	}
}

func TestTheMessageNamesAtMostEightProblemsAndCountsTheRest(t *testing.T) {
	_, id := newBulkPersona(t)
	blanks := make([]string, 12)
	_, err := AddCareerSkills(id, blanks)
	if err == nil || !strings.Contains(err.Error(), "(and 4 more)") || strings.Contains(err.Error(), "skills[8]") {
		t.Errorf("%v", err)
	}
}

func TestTheLimitIsFiftyAndAnEmptyListIsRefused(t *testing.T) {
	_, id := newBulkPersona(t)
	fifty := make([]string, 50)
	for i := range fifty {
		fifty[i] = fmt.Sprintf("Skill %d", i)
	}
	if res, err := AddCareerSkills(id, fifty); err != nil || len(res.Added) != 50 {
		t.Fatalf("50 must pass: %v %+v", err, res)
	}
	over := append(append([]string{}, fifty...), "One more")
	if _, err := AddCareerSkills(id, over); !errors.Is(err, ErrBulkRefused) || !strings.Contains(err.Error(), "max 50") {
		t.Errorf("51: %v", err)
	}
	if _, err := AddCareerSkills(id, nil); !errors.Is(err, ErrBulkRefused) || !strings.Contains(err.Error(), "skills is empty") {
		t.Errorf("empty: %v", err)
	}
	if got := len(ownSkills(t, id)); got != 50 {
		t.Errorf("stored %d", got)
	}
}

func TestAnUnknownPersonaIsRefusedBeforeAnythingElse(t *testing.T) {
	setupRunsDB(t)
	if _, err := AddCareerSkills("nope", []string{"Go"}); !errors.Is(err, ErrUnknownPersona) {
		t.Errorf("err = %v", err)
	}
	if _, err := AddCareerSkills("nope", nil); !errors.Is(err, ErrUnknownPersona) {
		t.Errorf("an unknown persona with an empty list: %v", err)
	}
}

func TestOnlyThePersonasOwnSkillsAreTouched(t *testing.T) {
	pid, a := newBulkPersona(t)
	b, err := CreatePersona(pid, "Data", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = AddCareerSkill(b, "Go")
	if res, err := AddCareerSkills(a, []string{"Go"}); err != nil || len(res.Added) != 1 {
		t.Fatalf("another persona's skill must not count: %v %+v", err, res)
	}
	if got := ownSkills(t, b); !reflect.DeepEqual(got, []string{"Go"}) {
		t.Errorf("the other persona changed: %v", got)
	}
}

// The tool through a real in-memory MCP session.
func TestAddCareerSkillsThroughMCP(t *testing.T) {
	_, id := newBulkPersona(t)
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1"}, nil)
	RegisterAddCareerSkills(server)
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
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "add_career_skills", Arguments: args})
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

	text, isErr := call(map[string]any{"personaId": id, "skills": []string{"Go", "SQL", "go"}})
	if isErr {
		t.Fatalf("%s", text)
	}
	var out BulkSkillsResult
	if err := json.Unmarshal([]byte(text), &out); err != nil || !reflect.DeepEqual(out.Added, []string{"Go", "SQL"}) || !reflect.DeepEqual(out.Skipped, []string{"go"}) {
		t.Errorf("%s %v", text, err)
	}
	// a missing argument is refused by the tool's own schema, an empty one by the handler
	if text, isErr := call(map[string]any{"skills": []string{"Go"}}); !isErr || !strings.Contains(text, "personaId") {
		t.Errorf("no persona: %v %s", isErr, text)
	}
	if text, isErr := call(map[string]any{"personaId": "", "skills": []string{"Go"}}); !isErr || !strings.Contains(text, "personaId is required") {
		t.Errorf("empty persona: %v %s", isErr, text)
	}
	if text, isErr := call(map[string]any{"personaId": "nope", "skills": []string{"Go"}}); !isErr || !strings.Contains(text, `unknown persona id "nope"`) {
		t.Errorf("unknown persona: %v %s", isErr, text)
	}
	if text, isErr := call(map[string]any{"personaId": id, "skills": []string{"Rust", " "}}); !isErr || !strings.Contains(text, "skills[1] is empty") || !strings.Contains(text, "nothing was added") {
		t.Errorf("refused: %v %s", isErr, text)
	}
	if got := ownSkills(t, id); !reflect.DeepEqual(got, []string{"Go", "SQL"}) {
		t.Errorf("a refused call must add nothing: %v", got)
	}
}

func TestASkillOfExactlyTheMaximumLengthIsAccepted(t *testing.T) {
	_, id := newBulkPersona(t)
	long := strings.Repeat("é", 300) // 300 characters, 600 bytes: the limit counts characters
	if res, err := AddCareerSkills(id, []string{long}); err != nil || len(res.Added) != 1 {
		t.Fatalf("%v %+v", err, res)
	}
}
