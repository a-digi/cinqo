package handler

import (
	"database/sql"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

// usageExtrasResponse is what the provider reported for a turn (or a sub-agent) besides the prompt and completion counts (plan/ai/
// conversation/step-178, step 3), embedded in the responses that already carry the counts so its fields sit beside them. Every field is
// left out when the provider reported nothing, so a response for a provider that reports no extras is byte-identical to before. The
// cost is US dollars exactly as the provider reported it: absent when unknown, present as 0 when the calls were free.
type usageExtrasResponse struct {
	CacheReadTokens  int      `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int      `json:"cacheWriteTokens,omitempty"`
	ReasoningTokens  int      `json:"reasoningTokens,omitempty"`
	CostUsd          *float64 `json:"costUsd,omitempty"`
}

// usageExtrasOf builds the extras from a row's numbers; the cost is stored in whole micro-dollars and answered in dollars.
func usageExtrasOf(cacheRead, cacheWrite, reasoning int, costMicroUSD *int64) usageExtrasResponse {
	e := usageExtrasResponse{CacheReadTokens: cacheRead, CacheWriteTokens: cacheWrite, ReasoningTokens: reasoning}
	if costMicroUSD != nil {
		dollars := float64(*costMicroUSD) / 1e6
		e.CostUsd = &dollars
	}
	return e
}

// attachTurnUsage puts each finished turn's extras on its message(s): the assistant message of a successful turn and the user message
// of a failed one, the two places that already carry the turn's counts (those are the messages with a turn run id and counts). A turn
// the lookup does not know, or whose provider reported nothing, is left unchanged.
func attachTurnUsage(messages []messageResponse, usage map[string]conversation_entity.TurnUsage) {
	for i := range messages {
		m := &messages[i]
		if m.TurnRunID == "" || (m.Role != "assistant" && !m.Failed) {
			continue
		}
		if u, ok := usage[m.TurnRunID]; ok {
			m.usageExtrasResponse = usageExtrasOf(u.CacheReadTokens, u.CacheWriteTokens, u.ReasoningTokens, u.CostMicroUSD)
		}
	}
}

// addTurnUsage reads what the provider reported for the conversation's turns and puts it on the messages (plan/ai/conversation/
// step-178, step 3). Best effort: when the lookup fails the messages are left as they are, so a conversation is never refused for the
// sake of a number that only decorates it.
func addTurnUsage(db *sql.DB, conversationID string, messages []messageResponse) {
	usage, err := conversation_query.NewTurnRunQueryRepo(db).FindUsageByConversationID(conversationID)
	if err != nil {
		return
	}
	attachTurnUsage(messages, usage)
}
