package handler

import (
	"database/sql"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

// attachRetryInfo marks the failed message of the conversation's last turn run as retryable (plan/ai/conversation/retry-last-step.md,
// step 4) and tells how many retries of it have failed already. Only that one message gets it: an older failed turn can only be started
// again. last is the conversation's most recent turn run, hasCheckpoint says whether it kept a saved state to resume from. Nothing else
// about the checkpoint ever leaves the server.
func attachRetryInfo(messages []messageResponse, last *conversation_entity.TurnRun, hasCheckpoint bool) {
	if last == nil || last.Status != "failed" || !hasCheckpoint {
		return
	}
	for i := range messages {
		m := &messages[i]
		if m.Failed && m.TurnRunID == last.ID {
			m.Retryable = true
			m.RetryCount = tokenPtr(last.RetryCount)
			return
		}
	}
}

// addRetryInfo reads the conversation's last turn run and attaches the retry information. Best effort like the usage numbers: the
// conversation is shown without a Retry button when the lookup fails, never refused for it.
func addRetryInfo(db *sql.DB, conversationID string, messages []messageResponse) {
	runs := conversation_query.NewTurnRunQueryRepo(db)
	last, err := runs.FindMostRecentByConversationID(conversationID)
	if err != nil || last.Status != "failed" {
		return
	}
	text, err := runs.FindCheckpoint(last.ID)
	if err != nil {
		return
	}
	attachRetryInfo(messages, last, text != "")
}
