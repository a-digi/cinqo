package handler

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"gopkg.in/yaml.v3"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	conversation_persistent "github.com/a-digi/cinqo/src/conversation/repository/persistent"
)

func openConversationDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	dir := filepath.Join("..", "..", "..", "config", "db", "migrations-conversation")
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations in %s (err %v)", dir, err)
	}
	sort.Strings(files)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, stmt := range strings.Split(string(raw), "/***Statement***/") {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("%s: %v", filepath.Base(f), err)
			}
		}
	}
	return db
}

func seedTurn(t *testing.T, db *sql.DB) *conversation_entity.TurnRun {
	t.Helper()
	if err := conversation_persistent.NewConversationPersistentRepo(db).Insert(&conversation_entity.Conversation{
		ID: "conv", UserID: "u1", Title: "t", FilePath: "/tmp/conv", PlatformID: "p", Model: "m",
		OrchestrationMode: conversation_entity.OrchestrationModeComposer,
	}); err != nil {
		t.Fatal(err)
	}
	run := &conversation_entity.TurnRun{ID: "turn1", ConversationID: "conv", UserContent: "go", StartedAt: "2026-01-01T00:00:00Z", Status: "running"}
	if err := conversation_persistent.NewTurnRunPersistentRepo(db).Insert(run); err != nil {
		t.Fatal(err)
	}
	return run
}

func TestLoadPlanAndResponseShape(t *testing.T) {
	db := openConversationDB(t)
	run := seedTurn(t, db)

	// A direct-style turn: no plan at all.
	plan, err := loadPlan(db, run.ID)
	if err != nil || plan != nil {
		t.Fatalf("a turn without a plan must load as nil: %v, %v", plan, err)
	}
	bare, _ := json.Marshal(toActiveTurnResponse(run, nil, plan))
	if strings.Contains(string(bare), `"plan"`) {
		t.Fatalf("the plan field must be omitted for a turn without one: %s", bare)
	}

	// A composer turn: two plans, one worker linked, one step skipped.
	if _, err := db.Exec(`INSERT INTO sub_agent_runs (id, parent_turn_run_id, conversation_id, task, started_at) VALUES ('w1', 'turn1', 'conv', 't', 'now')`); err != nil {
		t.Fatal(err)
	}
	repo := conversation_persistent.NewTurnPlanStepPersistentRepo(db)
	if err := repo.InsertAll([]*conversation_entity.TurnPlanStep{
		{TurnRunID: "turn1", PlanNo: 1, StepID: "a", Task: "fetch", DependsOn: "[]"},
		{TurnRunID: "turn1", PlanNo: 1, StepID: "b", Task: "report", DependsOn: `["a"]`},
		{TurnRunID: "turn1", PlanNo: 2, StepID: "a2", Task: "retry", DependsOn: "[]"},
	}); err != nil {
		t.Fatal(err)
	}
	w1 := "w1"
	if err := repo.UpdateStatus("turn1", 1, "a", "completed", &w1); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateStatus("turn1", 1, "b", "skipped", nil); err != nil {
		t.Fatal(err)
	}

	plan, err = loadPlan(db, run.ID)
	if err != nil || len(plan) != 3 {
		t.Fatalf("plan: %+v, %v", plan, err)
	}
	if plan[0].ID != "a" || plan[0].PlanNo != 1 || plan[0].Status != "completed" || plan[0].SubAgentID != "w1" || len(plan[0].DependsOn) != 0 {
		t.Errorf("step a: %+v", plan[0])
	}
	if plan[1].ID != "b" || plan[1].Status != "skipped" || plan[1].SubAgentID != "" || len(plan[1].DependsOn) != 1 || plan[1].DependsOn[0] != "a" {
		t.Errorf("step b: %+v", plan[1])
	}
	if plan[2].PlanNo != 2 || plan[2].ID != "a2" || plan[2].Status != "pending" {
		t.Errorf("step a2 (the re-plan) must come last: %+v", plan[2])
	}

	raw, _ := json.Marshal(toActiveTurnResponse(run, nil, plan))
	var decoded struct {
		Plan []map[string]any `json:"plan"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil || len(decoded.Plan) != 3 {
		t.Fatalf("response JSON: %s (%v)", raw, err)
	}
	for _, key := range []string{"planNo", "id", "task", "dependsOn", "status"} {
		if _, ok := decoded.Plan[0][key]; !ok {
			t.Errorf("plan step JSON is missing %q: %v", key, decoded.Plan[0])
		}
	}
	if _, ok := decoded.Plan[1]["subAgentId"]; ok {
		t.Errorf("a step without a worker must omit subAgentId: %v", decoded.Plan[1])
	}
}

func TestPlanStepWithCorruptDependenciesStillRenders(t *testing.T) {
	got := toPlanStepResponse(&conversation_entity.TurnPlanStep{PlanNo: 1, StepID: "a", Task: "t", DependsOn: "not json", Status: "pending"})
	if got.DependsOn == nil || len(got.DependsOn) != 0 {
		t.Errorf("dependsOn must be an empty array, never null: %#v", got.DependsOn)
	}
}

func TestConversationResponseExposesTheOrchestrationMode(t *testing.T) {
	raw, _ := json.Marshal(toConversationResponse(&conversation_entity.Conversation{ID: "c", OrchestrationMode: conversation_entity.OrchestrationModeComposer}))
	if !strings.Contains(string(raw), `"orchestrationMode":"composer"`) {
		t.Errorf("response: %s", raw)
	}
}

// TestPlanRouteIsDeclaredWithScopesAndRegistered guards the rule that
// every non-public route declares scopes, and that its executor exists.
func TestPlanRouteIsDeclaredWithScopesAndRegistered(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "routes", "route-conversation.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Routes []struct {
			Path     string   `yaml:"path"`
			Method   string   `yaml:"method"`
			Executor string   `yaml:"executor"`
			Security string   `yaml:"security"`
			Scopes   []string `yaml:"scopes"`
		} `yaml:"routes"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range doc.Routes {
		if r.Path != "/api/v1/conversations/{id}/turns/{turnRunId}/plan" {
			continue
		}
		found = true
		if r.Method != "GET" || r.Executor != "ConversationGetTurnPlan" || r.Security != "authenticated" {
			t.Errorf("route: %+v", r)
		}
		if strings.Join(r.Scopes, ",") != "cinqo:conversation:use,cinqo:super:admin" {
			t.Errorf("the plan route must carry the same scopes as the sub-agents route, got %v", r.Scopes)
		}
	}
	if !found {
		t.Fatal("route-conversation.yaml does not declare the plan route")
	}

	routesGo, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "routes", "routes.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(routesGo), `"ConversationGetTurnPlan"`) {
		t.Error("the ConversationGetTurnPlan executor is not registered in routes.go")
	}
}
