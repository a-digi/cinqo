package handler

import (
	"encoding/json"
	"strings"
	"testing"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	conversation_persistent "github.com/a-digi/cinqo/src/conversation/repository/persistent"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

func micro(v int64) *int64 { return &v }

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestTheCostIsAnsweredInDollarsAndAbsentWhenUnknown(t *testing.T) {
	if e := usageExtrasOf(0, 0, 0, nil); e.CostUsd != nil {
		t.Errorf("an unknown cost must be absent, got %v", *e.CostUsd)
	}
	for in, want := range map[int64]float64{0: 0, 250000: 0.25, 123: 0.000123, 1: 0.000001, 12_000_000: 12} {
		e := usageExtrasOf(0, 0, 0, micro(in))
		if e.CostUsd == nil || *e.CostUsd != want {
			t.Errorf("%d micro-dollars = %v, want %v", in, e.CostUsd, want)
		}
	}
}

func TestNothingReportedAddsNoFieldsToAResponse(t *testing.T) {
	got := jsonOf(t, messageResponse{Role: "assistant", Content: "hi", CreatedAt: "t", usageExtrasResponse: usageExtrasOf(0, 0, 0, nil)})
	for _, key := range []string{"cacheReadTokens", "cacheWriteTokens", "reasoningTokens", "costUsd"} {
		if strings.Contains(got, key) {
			t.Errorf("%s must be absent when nothing was reported: %s", key, got)
		}
	}
	full := jsonOf(t, messageResponse{Role: "assistant", Content: "hi", CreatedAt: "t", usageExtrasResponse: usageExtrasOf(8000, 300, 600, micro(250000))})
	for _, want := range []string{`"cacheReadTokens":8000`, `"cacheWriteTokens":300`, `"reasoningTokens":600`, `"costUsd":0.25`} {
		if !strings.Contains(full, want) {
			t.Errorf("%s missing: %s", want, full)
		}
	}
	// a free call is a reported cost of 0: present, not omitted
	if free := jsonOf(t, messageResponse{usageExtrasResponse: usageExtrasOf(0, 0, 0, micro(0))}); !strings.Contains(free, `"costUsd":0`) {
		t.Errorf("a free call must say so: %s", free)
	}
}

func TestAnAttachedTurnsExtrasGoOnTheMessageThatCarriesItsCounts(t *testing.T) {
	msgs := []messageResponse{
		{Role: "user", Content: "q1", TurnRunID: ""},
		{Role: "assistant", Content: "a1", TurnRunID: "t1"},
		{Role: "user", Content: "q2 failed", Failed: true, TurnRunID: "t2"},
		{Role: "assistant", Content: "a3", TurnRunID: "t-unknown"},
		{Role: "assistant", Content: "a4", TurnRunID: "t4"},
		{Role: "assistant", Content: "a5 old turn without a run id"},
	}
	usage := map[string]conversation_entity.TurnUsage{
		"t1": {CacheReadTokens: 8000, ReasoningTokens: 600, CostMicroUSD: micro(250000)},
		"t2": {CacheWriteTokens: 5},
		"t4": {}, // the provider reported nothing
	}
	attachTurnUsage(msgs, usage)
	if got := msgs[1]; got.CacheReadTokens != 8000 || got.ReasoningTokens != 600 || got.CostUsd == nil || *got.CostUsd != 0.25 {
		t.Errorf("the assistant message of t1: %+v", got.usageExtrasResponse)
	}
	if got := msgs[2]; got.CacheWriteTokens != 5 || got.CostUsd != nil {
		t.Errorf("the failed turn's user message: %+v", got.usageExtrasResponse)
	}
	for _, i := range []int{0, 3, 4, 5} {
		if msgs[i].usageExtrasResponse != (usageExtrasResponse{}) {
			t.Errorf("message %d must have no extras: %+v", i, msgs[i].usageExtrasResponse)
		}
	}
	// the plain user message of a successful turn never carries the turn's numbers, even with a run id
	ok := []messageResponse{{Role: "user", Content: "q", TurnRunID: "t1"}}
	attachTurnUsage(ok, usage)
	if ok[0].usageExtrasResponse != (usageExtrasResponse{}) {
		t.Errorf("a user message of a successful turn: %+v", ok[0].usageExtrasResponse)
	}
}

func TestTheLiveTurnAndItsSubAgentsCarryWhatTheProviderReportedSoFar(t *testing.T) {
	run := &conversation_entity.TurnRun{ID: "t1", ConversationID: "c", Status: "running", PromptTokens: 100, CompletionTokens: 10, TotalTokens: 110,
		CacheReadTokens: 60, CacheWriteTokens: 5, ReasoningTokens: 4, CostMicroUSD: micro(500000)}
	sub := &conversation_entity.SubAgentRun{ID: "s1", Task: "x", Status: "running", PromptTokens: 50, CompletionTokens: 5, TotalTokens: 55, CacheReadTokens: 30}
	got := jsonOf(t, toActiveTurnResponse(run, []*conversation_entity.SubAgentRun{sub}, nil))
	for _, want := range []string{`"promptTokens":100`, `"cacheReadTokens":60`, `"cacheWriteTokens":5`, `"reasoningTokens":4`, `"costUsd":0.5`, `"cacheReadTokens":30`} {
		if !strings.Contains(got, want) {
			t.Errorf("%s missing: %s", want, got)
		}
	}
	// the sub-agent reported no cost: its entry has none (the turn's own cost is the only costUsd)
	if strings.Count(got, `"costUsd"`) != 1 {
		t.Errorf("only the turn has a cost: %s", got)
	}
	plain := jsonOf(t, toActiveTurnResponse(&conversation_entity.TurnRun{ID: "t2", Status: "running", PromptTokens: 1}, nil, nil))
	if strings.Contains(plain, "cache") || strings.Contains(plain, "reasoning") || strings.Contains(plain, "costUsd") {
		t.Errorf("a turn without extras must not carry any field: %s", plain)
	}
}

func TestTheUsageLookupReadsOnlyTheNumbersOfTheConversationsOwnTurns(t *testing.T) {
	db := openConversationDB(t)
	convs := conversation_persistent.NewConversationPersistentRepo(db)
	for _, id := range []string{"c1", "c2"} {
		if err := convs.Insert(&conversation_entity.Conversation{ID: id, UserID: "u1", Title: "t", FilePath: "/tmp/" + id, PlatformID: "p", Model: "m", OrchestrationMode: conversation_entity.OrchestrationModeDirect}); err != nil {
			t.Fatal(err)
		}
	}
	runs := conversation_persistent.NewTurnRunPersistentRepo(db)
	// a conversation has at most one running turn, so a finishes before b starts
	for _, r := range []struct{ id, conv string }{{"a", "c1"}, {"b", "c1"}, {"z", "c2"}} {
		if err := runs.Insert(&conversation_entity.TurnRun{ID: r.id, ConversationID: r.conv, UserContent: "hi", StartedAt: "2026-01-01T00:00:00Z"}); err != nil {
			t.Fatal(err)
		}
		if r.id == "a" {
			if err := runs.SetTerminalStatus("a", "completed", "2026-01-01T00:00:01Z"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := runs.AddUsage("a", conversation_entity.UsageAmounts{PromptTokens: 10, CacheReadTokens: 8, CacheWriteTokens: 2, ReasoningTokens: 3, CostMicroUSD: micro(1500)}); err != nil {
		t.Fatal(err)
	}
	if err := runs.AddUsage("z", conversation_entity.UsageAmounts{CacheReadTokens: 99, CostMicroUSD: micro(7)}); err != nil {
		t.Fatal(err)
	}
	got, err := conversation_query.NewTurnRunQueryRepo(db).FindUsageByConversationID("c1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("c1 has two turns, got %v", got)
	}
	if a := got["a"]; a.CacheReadTokens != 8 || a.CacheWriteTokens != 2 || a.ReasoningTokens != 3 || a.CostMicroUSD == nil || *a.CostMicroUSD != 1500 {
		t.Errorf("a = %+v", a)
	}
	if b := got["b"]; b.CacheReadTokens != 0 || b.CostMicroUSD != nil {
		t.Errorf("b reported nothing: %+v", b)
	}
	if _, ok := got["z"]; ok {
		t.Error("another conversation's turn leaked")
	}
	if none, err := conversation_query.NewTurnRunQueryRepo(db).FindUsageByConversationID("nope"); err != nil || len(none) != 0 {
		t.Errorf("an unknown conversation: %v %v", none, err)
	}
}

func TestTheMessagesOfAConversationGetTheirTurnsNumbersAndAFailedLookupLeavesThemAlone(t *testing.T) {
	db := openConversationDB(t)
	if err := conversation_persistent.NewConversationPersistentRepo(db).Insert(&conversation_entity.Conversation{ID: "c1", UserID: "u1", Title: "t", FilePath: "/tmp/c1", PlatformID: "p", Model: "m", OrchestrationMode: conversation_entity.OrchestrationModeDirect}); err != nil {
		t.Fatal(err)
	}
	runs := conversation_persistent.NewTurnRunPersistentRepo(db)
	if err := runs.Insert(&conversation_entity.TurnRun{ID: "t1", ConversationID: "c1", UserContent: "hi", StartedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := runs.AddUsage("t1", conversation_entity.UsageAmounts{CacheReadTokens: 8000, CostMicroUSD: micro(1500)}); err != nil {
		t.Fatal(err)
	}
	msgs := []messageResponse{{Role: "user", Content: "q"}, {Role: "assistant", Content: "a", TurnRunID: "t1"}}
	addTurnUsage(db, "c1", msgs)
	if msgs[1].CacheReadTokens != 8000 || msgs[1].CostUsd == nil || *msgs[1].CostUsd != 0.0015 || msgs[0].usageExtrasResponse != (usageExtrasResponse{}) {
		t.Errorf("messages = %+v / %+v", msgs[0].usageExtrasResponse, msgs[1].usageExtrasResponse)
	}
	// a database that cannot be read: the messages stay as they were, nothing panics
	closed := openConversationDB(t)
	closed.Close()
	again := []messageResponse{{Role: "assistant", Content: "a", TurnRunID: "t1"}}
	addTurnUsage(closed, "c1", again)
	if again[0].usageExtrasResponse != (usageExtrasResponse{}) {
		t.Errorf("a failed lookup changed a message: %+v", again[0].usageExtrasResponse)
	}
}

func TestTheActiveTurnAlwaysSaysHowManyToolCallsWereRefused(t *testing.T) {
	for n, want := range map[int]string{0: `"toolErrors":0`, 1: `"toolErrors":1`, 7: `"toolErrors":7`} {
		got := jsonOf(t, toActiveTurnResponse(&conversation_entity.TurnRun{ID: "t", Status: "completed", ToolErrors: n}, nil, nil))
		if !strings.Contains(got, want) {
			t.Errorf("%d: %s missing in %s", n, want, got)
		}
	}
}
