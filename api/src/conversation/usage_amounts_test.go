package conversation

import (
	"context"
	"database/sql"
	"net/http"
	"testing"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	conversation_persistent "github.com/a-digi/cinqo/src/conversation/repository/persistent"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

func TestTheCostIsStoredInWholeMicroDollarsRoundedToTheNearest(t *testing.T) {
	cases := []struct {
		dollars float64
		want    int64
	}{
		{0, 0}, {0.25, 250000}, {0.000123, 123}, {0.0000004, 0}, {0.0000005, 1}, {0.0000006, 1}, {1.5, 1500000}, {12, 12000000}, {0.1234567891, 123457},
		{1e12, 1_000_000_000_000_000_000}, {1e13, 9_223_372_036_854_775_807}, {1e18, 9_223_372_036_854_775_807}, // a cost too large for an int64 is capped, never wrapped
	}
	for _, c := range cases {
		d := c.dollars
		got := usageAmounts(chatcompleter.Usage{Cost: &d}).CostMicroUSD
		if got == nil || *got != c.want {
			t.Errorf("%v dollars = %v, want %d", c.dollars, derefOr(got), c.want)
		}
	}
	if got := usageAmounts(chatcompleter.Usage{PromptTokens: 5}).CostMicroUSD; got != nil {
		t.Errorf("no cost reported must stay unknown, got %v", *got)
	}
}

func derefOr(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestTheCountsAreMappedOneToOne(t *testing.T) {
	got := usageAmounts(chatcompleter.Usage{PromptTokens: 100, CompletionTokens: 10, TotalTokens: 999, CacheReadTokens: 60, CacheCreationTokens: 5, ReasoningTokens: 4})
	want := conversation_entity.UsageAmounts{PromptTokens: 100, CompletionTokens: 10, CacheReadTokens: 60, CacheWriteTokens: 5, ReasoningTokens: 4}
	if got != want {
		t.Errorf("%+v, want %+v", got, want)
	}
}

type recordingAdder struct {
	id  string
	got []conversation_entity.UsageAmounts
}

func (r *recordingAdder) AddUsage(id string, u conversation_entity.UsageAmounts) error {
	r.id = id
	r.got = append(r.got, u)
	return nil
}

func TestTheReporterAddsEachResponsesWholeUsageToTheRunsRow(t *testing.T) {
	rec := &recordingAdder{}
	report := usageReporter(rec, "run-7")
	c := 0.5
	report(chatcompleter.Usage{PromptTokens: 100, CompletionTokens: 10, CacheReadTokens: 60, ReasoningTokens: 4, Cost: &c})
	report(chatcompleter.Usage{PromptTokens: 1})
	if rec.id != "run-7" || len(rec.got) != 2 || rec.got[0].CacheReadTokens != 60 || rec.got[0].ReasoningTokens != 4 || rec.got[0].CostMicroUSD == nil || *rec.got[0].CostMicroUSD != 500000 || rec.got[1].CostMicroUSD != nil {
		t.Errorf("%+v", rec)
	}
}

func seedTurn(t *testing.T) *sql.DB {
	t.Helper()
	db := openConversationDB(t)
	if err := conversation_persistent.NewConversationPersistentRepo(db).Insert(&conversation_entity.Conversation{ID: "conv", UserID: "u1", Title: "t", FilePath: "/tmp/conv", PlatformID: "p", Model: "m", OrchestrationMode: conversation_entity.OrchestrationModeDirect}); err != nil {
		t.Fatal(err)
	}
	if err := conversation_persistent.NewTurnRunPersistentRepo(db).Insert(&conversation_entity.TurnRun{ID: "turn1", ConversationID: "conv", UserContent: "go", StartedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	return db
}

// The whole path: a model response with the full usage goes through the tool loop's report callback into the run's row, so the
// provider's cache, reasoning and cost numbers are stored per turn and readable live.
func TestAModelResponsesFullUsageEndsUpInTheTurnRunsRow(t *testing.T) {
	db := seedTurn(t)
	calls := 0
	rich := func() (*chatcompleter.ChatCompletionResult, error) {
		calls++
		cost := 0.25
		return &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant", Content: "done"}, FinishReason: "stop",
			Usage: chatcompleter.Usage{PromptTokens: 12345, CompletionTokens: 900, TotalTokens: 13245, CacheReadTokens: 8000, CacheCreationTokens: 300, ReasoningTokens: 600, Cost: &cost}}, nil
	}
	c := &stepCompleter{steps: []func() (*chatcompleter.ChatCompletionResult, error){rich}}
	report := usageReporter(conversation_persistent.NewTurnRunPersistentRepo(db), "turn1")
	got, err := runToolLoop(context.Background(), http.DefaultClient, platform.Entry{Completer: c}, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: "hi"}}, nil, nil, nil, "u1", "", 0, "conv", db, "turn1", 0, "direct", nil, report, nil, 0)
	if err != nil || got != "done" {
		t.Fatalf("got %q, %v", got, err)
	}
	row, err := conversation_query.NewTurnRunQueryRepo(db).FindByID("turn1")
	if err != nil {
		t.Fatal(err)
	}
	if row.PromptTokens != 12345 || row.CompletionTokens != 900 || row.TotalTokens != 13245 || row.CacheReadTokens != 8000 || row.CacheWriteTokens != 300 || row.ReasoningTokens != 600 {
		t.Errorf("row = %+v", row)
	}
	if row.CostMicroUSD == nil || *row.CostMicroUSD != 250000 {
		t.Errorf("cost = %v, want 250000", derefOr(row.CostMicroUSD))
	}
}

// A provider that reports nothing extra leaves the extras at 0 and the cost unknown: exactly the old behaviour.
func TestAProviderWithoutExtrasLeavesTheTurnRunsExtrasEmpty(t *testing.T) {
	db := seedTurn(t)
	plain := answerWith(chatcompleter.Usage{PromptTokens: 100, CompletionTokens: 10, TotalTokens: 110})
	c := &stepCompleter{steps: []func() (*chatcompleter.ChatCompletionResult, error){plain}}
	report := usageReporter(conversation_persistent.NewTurnRunPersistentRepo(db), "turn1")
	if _, err := runToolLoop(context.Background(), http.DefaultClient, platform.Entry{Completer: c}, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: "hi"}}, nil, nil, nil, "u1", "", 0, "conv", db, "turn1", 0, "direct", nil, report, nil, 0); err != nil {
		t.Fatal(err)
	}
	row, _ := conversation_query.NewTurnRunQueryRepo(db).FindByID("turn1")
	if row.PromptTokens != 100 || row.CompletionTokens != 10 || row.TotalTokens != 110 || row.CacheReadTokens != 0 || row.CacheWriteTokens != 0 || row.ReasoningTokens != 0 || row.CostMicroUSD != nil {
		t.Errorf("row = %+v cost %v", row, derefOr(row.CostMicroUSD))
	}
}
