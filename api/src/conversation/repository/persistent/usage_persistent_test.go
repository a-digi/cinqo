package persistent_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	conversation_persistent "github.com/a-digi/cinqo/src/conversation/repository/persistent"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

func i64(v int64) *int64 { return &v }

func seedRuns(t *testing.T) *sql.DB {
	t.Helper()
	db := openConversationDB(t)
	if err := conversation_persistent.NewConversationPersistentRepo(db).Insert(newConversation("c1", "direct")); err != nil {
		t.Fatal(err)
	}
	if err := conversation_persistent.NewTurnRunPersistentRepo(db).Insert(&conversation_entity.TurnRun{ID: "t1", ConversationID: "c1", UserContent: "hi", StartedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := conversation_persistent.NewSubAgentRunPersistentRepo(db).Insert(&conversation_entity.SubAgentRun{ID: "s1", ParentTurnRunID: "t1", ConversationID: "c1", Task: "x", StartedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	return db
}

type totals struct {
	prompt, completion, total, cacheRead, cacheWrite, reasoning int
	cost                                                        *int64
}

func turnTotals(t *testing.T, db *sql.DB, id string) totals {
	t.Helper()
	r, err := conversation_query.NewTurnRunQueryRepo(db).FindByID(id)
	if err != nil {
		t.Fatal(err)
	}
	return totals{r.PromptTokens, r.CompletionTokens, r.TotalTokens, r.CacheReadTokens, r.CacheWriteTokens, r.ReasoningTokens, r.CostMicroUSD}
}

func subTotals(t *testing.T, db *sql.DB, id string) totals {
	t.Helper()
	r, err := conversation_query.NewSubAgentRunQueryRepo(db).FindByID(id)
	if err != nil {
		t.Fatal(err)
	}
	return totals{r.PromptTokens, r.CompletionTokens, r.TotalTokens, r.CacheReadTokens, r.CacheWriteTokens, r.ReasoningTokens, r.CostMicroUSD}
}

func sameTotals(a, b totals) bool {
	if a.prompt != b.prompt || a.completion != b.completion || a.total != b.total || a.cacheRead != b.cacheRead || a.cacheWrite != b.cacheWrite || a.reasoning != b.reasoning {
		return false
	}
	if (a.cost == nil) != (b.cost == nil) {
		return false
	}
	return a.cost == nil || *a.cost == *b.cost
}

type adder interface {
	AddUsage(id string, u conversation_entity.UsageAmounts) error
}

func exerciseUsage(t *testing.T, name string, repo adder, id string, read func() totals) {
	t.Helper()
	check := func(step string, want totals) {
		t.Helper()
		if got := read(); !sameTotals(got, want) {
			t.Errorf("%s, %s: %+v (cost %v), want %+v (cost %v)", name, step, got, derefOrNil(got.cost), want, derefOrNil(want.cost))
		}
	}
	// a fresh run has nothing, and no cost is unknown (NULL), not zero
	check("fresh", totals{})
	// a response without a cost adds the counts and leaves the cost unknown
	if err := repo.AddUsage(id, conversation_entity.UsageAmounts{PromptTokens: 100, CompletionTokens: 10, CacheReadTokens: 60, CacheWriteTokens: 5, ReasoningTokens: 4}); err != nil {
		t.Fatal(err)
	}
	check("no cost yet", totals{prompt: 100, completion: 10, total: 110, cacheRead: 60, cacheWrite: 5, reasoning: 4})
	// the first reported cost makes it known
	if err := repo.AddUsage(id, conversation_entity.UsageAmounts{PromptTokens: 50, CompletionTokens: 5, CacheReadTokens: 10, CostMicroUSD: i64(250000)}); err != nil {
		t.Fatal(err)
	}
	check("first cost", totals{prompt: 150, completion: 15, total: 165, cacheRead: 70, cacheWrite: 5, reasoning: 4, cost: i64(250000)})
	// a response without a cost keeps the sum; one with a cost adds to it (the same ?7 is used twice in the SQL)
	if err := repo.AddUsage(id, conversation_entity.UsageAmounts{PromptTokens: 1, CompletionTokens: 1}); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddUsage(id, conversation_entity.UsageAmounts{PromptTokens: 2, CompletionTokens: 0, CostMicroUSD: i64(123)}); err != nil {
		t.Fatal(err)
	}
	check("more", totals{prompt: 153, completion: 16, total: 169, cacheRead: 70, cacheWrite: 5, reasoning: 4, cost: i64(250123)})
	// an unknown id changes nothing and is no error
	if err := repo.AddUsage("nope", conversation_entity.UsageAmounts{PromptTokens: 9, CostMicroUSD: i64(9)}); err != nil {
		t.Errorf("%s: an unknown id: %v", name, err)
	}
	check("unknown id", totals{prompt: 153, completion: 16, total: 169, cacheRead: 70, cacheWrite: 5, reasoning: 4, cost: i64(250123)})
}

func derefOrNil(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestUsageIsAddedToATurnRunWithTheCostUnknownUntilAProviderReportsOne(t *testing.T) {
	db := seedRuns(t)
	exerciseUsage(t, "turn run", conversation_persistent.NewTurnRunPersistentRepo(db), "t1", func() totals { return turnTotals(t, db, "t1") })
}

func TestUsageIsAddedToASubAgentRunTheSameWay(t *testing.T) {
	db := seedRuns(t)
	exerciseUsage(t, "sub-agent run", conversation_persistent.NewSubAgentRunPersistentRepo(db), "s1", func() totals { return subTotals(t, db, "s1") })
}

func TestAFreeCallIsAKnownCostOfZeroNotAnUnknownOne(t *testing.T) {
	db := seedRuns(t)
	repo := conversation_persistent.NewTurnRunPersistentRepo(db)
	if err := repo.AddUsage("t1", conversation_entity.UsageAmounts{PromptTokens: 10, CompletionTokens: 1, CostMicroUSD: i64(0)}); err != nil {
		t.Fatal(err)
	}
	if got := turnTotals(t, db, "t1"); got.cost == nil || *got.cost != 0 {
		t.Errorf("a free call: cost = %v, want 0", derefOrNil(got.cost))
	}
	// and the runs of other turns are untouched
	if got := subTotals(t, db, "s1"); got.prompt != 0 || got.cost != nil {
		t.Errorf("another run changed: %+v", got)
	}
}

// A database from before the migration keeps every row, with the new counts at 0 and the cost unknown, and the new file applies cleanly.
func TestTheMigrationAddsTheColumnsToAnOlderDatabaseAndKeepsItsRows(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "config", "db", "migrations-conversation")
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations (err %v)", err)
	}
	sort.Strings(files)
	last := files[len(files)-1]
	if filepath.Base(last) != "31_10_2026_13_00_00.sql" {
		t.Fatalf("the usage migration must be the last file by name (it needs the turn_runs table), the last is %s", filepath.Base(last))
	}
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	apply := func(f string) {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		// The app (coco-orm) cuts a migration file at every semicolon, comments included, so the tests do exactly that.
		for _, stmt := range strings.Split(string(raw), ";") {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("%s: %v", filepath.Base(f), err)
			}
		}
	}
	for _, f := range files[:len(files)-1] {
		apply(f)
	}
	if err := conversation_persistent.NewConversationPersistentRepo(db).Insert(newConversation("c1", "direct")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO turn_runs (id, conversation_id, user_content, status, started_at, log, cancel_requested, prompt_tokens, completion_tokens, total_tokens) VALUES ('old','c1','hi','completed','2026-01-01T00:00:00Z','',0,500,50,550)`); err != nil {
		t.Fatal(err)
	}
	apply(last)
	got := turnTotals(t, db, "old")
	if !sameTotals(got, totals{prompt: 500, completion: 50, total: 550}) {
		t.Errorf("the old row after the migration: %+v cost %v", got, derefOrNil(got.cost))
	}
	// applying the file a second time is an error (the columns exist): the runner records applied files, so it never does
	if _, err := db.Exec(`ALTER TABLE turn_runs ADD COLUMN cost_microusd INTEGER`); err == nil {
		t.Error("the column must exist after the migration")
	}
}
