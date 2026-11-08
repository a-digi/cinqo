package conversation

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	conversation_persistent "github.com/a-digi/cinqo/src/conversation/repository/persistent"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

// maxUnconfirmedRetries is how many retries of one failed turn fail before another needs the caller's confirmation
// (plan/ai/conversation/retry-last-step.md): the third failed retry means it "seems to be failing all the time".
const maxUnconfirmedRetries = 3

var (
	// ErrTurnRunNotFound: no turn run with this id belongs to the conversation.
	ErrTurnRunNotFound = errors.New("conversation: turn run not found")
	// ErrTurnNotRetryable: the turn run is not the conversation's last, has not failed, or has no saved state to resume from.
	ErrTurnNotRetryable = errors.New("conversation: this turn cannot be retried")
)

// RetryConfirmationRequiredError is returned when the turn has already failed maxUnconfirmedRetries retries and the caller has not
// confirmed another. RetryCount is how many retries have failed so far.
type RetryConfirmationRequiredError struct{ RetryCount int }

func (e *RetryConfirmationRequiredError) Error() string {
	return fmt.Sprintf("conversation: this turn has failed %d retries in a row, confirm to try again", e.RetryCount)
}

// retryResume is what a retried run carries into runDetachedTurn.
type retryResume struct {
	checkpoint Checkpoint
}

// StartRetryRun redoes the last step of a failed turn: a new turn run that resumes from the failed run's saved state, so the work the
// failed run had already done (its tool rounds) is not repeated. Only the conversation's last run can be retried, and only while it is
// failed and has a checkpoint. A turn that has failed maxUnconfirmedRetries retries needs confirmed = true. The caller has already
// checked that the conversation belongs to the user; the turn run is checked here against the conversation.
func StartRetryRun(
	httpClient *http.Client,
	mainDB *sql.DB,
	conversationDB *sql.DB,
	encryptionKey []byte,
	conversationID, failedTurnRunID string,
	confirmed bool,
	callerScopes []string,
	dataDir string,
	corePort int,
) (*conversation_entity.TurnRun, error) {
	runsQ := conversation_query.NewTurnRunQueryRepo(conversationDB)

	failed, err := runsQ.FindByID(failedTurnRunID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTurnRunNotFound
		}
		return nil, fmt.Errorf("conversation: look up turn run: %w", err)
	}
	if failed.ConversationID != conversationID {
		return nil, ErrTurnRunNotFound // another conversation's run reads as not found, like a wrong owner
	}
	if active, err := runsQ.FindActiveByConversationID(conversationID); err == nil && active != nil {
		return nil, ErrTurnAlreadyRunning
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("conversation: look up the running turn: %w", err)
	}
	last, err := runsQ.FindMostRecentByConversationID(conversationID)
	if err != nil {
		return nil, fmt.Errorf("conversation: look up the last turn run: %w", err)
	}
	if last.ID != failed.ID || failed.Status != "failed" {
		return nil, ErrTurnNotRetryable
	}
	text, err := runsQ.FindCheckpoint(failed.ID)
	if err != nil {
		return nil, fmt.Errorf("conversation: read the checkpoint: %w", err)
	}
	cp, err := DecodeCheckpoint(text)
	if err != nil {
		return nil, ErrTurnNotRetryable // no state (an empty text) or unreadable state: the user starts again
	}
	if failed.RetryCount >= maxUnconfirmedRetries && !confirmed {
		return nil, &RetryConfirmationRequiredError{RetryCount: failed.RetryCount}
	}

	run := &conversation_entity.TurnRun{
		ID:             uuid.NewString(),
		ConversationID: conversationID,
		UserContent:    failed.UserContent,
		Status:         "running",
		StartedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	runs := conversation_persistent.NewTurnRunPersistentRepo(conversationDB)
	if err := runs.Insert(run); err != nil {
		if isUniqueConstraintErr(err) {
			return nil, ErrTurnAlreadyRunning
		}
		return nil, fmt.Errorf("conversation: start retry run: %w", err)
	}
	if err := runs.CopyOptions(failed.ID, run.ID); err != nil {
		finishTurnRun(runs, run.ID, "failed")
		return nil, fmt.Errorf("conversation: carry the turn options over: %w", err)
	}
	if err := runs.SetRetry(run.ID, failed.ID, failed.RetryCount+1); err != nil {
		finishTurnRun(runs, run.ID, "failed") // never leave a running row that nothing runs
		return nil, fmt.Errorf("conversation: record the retry: %w", err)
	}
	// The saved state moves to the new run (saving clears the older run's): a retry that fails before it can save one of its own
	// (the platform is down, say) is then still retryable, and a successful one clears it.
	if err := runs.SaveCheckpoint(run.ID, text); err != nil {
		finishTurnRun(runs, run.ID, "failed")
		return nil, fmt.Errorf("conversation: carry the checkpoint over: %w", err)
	}
	retryOf := failed.ID
	run.RetryOf, run.RetryCount = &retryOf, failed.RetryCount+1

	launchTurnRun(run.ID, httpClient, mainDB, conversationDB, encryptionKey, conversationID, failed.UserContent, callerScopes, dataDir, corePort,
		&retryResume{checkpoint: cp})
	return run, nil
}
