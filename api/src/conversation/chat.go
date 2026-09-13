package conversation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
	platform_crypto "github.com/a-digi/cinqo/src/platform/crypto"
	platform_query "github.com/a-digi/cinqo/src/platform/repository/query"

	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

// maxMessageContentLength/sendTimeout match the reference's own
// MaxMessageContentLength/chatRequestTimeout exactly — proven values,
// not picked arbitrarily. See
// plan/ai/conversation/step-02-sending-a-message.md.
const (
	maxMessageContentLength = 8000
	sendTimeout             = 60 * time.Second
)

// ContextTurns is how many recent turns are sent to the platform as
// context — the same tail-read step 1 recommends, not the full
// retained window. Exported: step 3's "get conversation" handler uses
// this exact same value for what it shows the frontend, per step 1's
// own "one read mechanism, two callers" design. See step 1's own open
// question 1 (3 vs 4).
const ContextTurns = 4

var (
	ErrEmptyContent         = errors.New("conversation: content is empty")
	ErrContentTooLong       = errors.New("conversation: content exceeds maximum length")
	ErrPlatformNotFound     = errors.New("conversation: platform is not registered")
	ErrPlatformUnsupported  = errors.New("conversation: platform has no chat completion support yet")
	ErrNoKeyForPlatform     = errors.New("conversation: no API key registered for this platform")
	ErrConversationNotFound = errors.New("conversation: conversation not found")
	// ErrProviderCallFailed distinguishes an upstream platform failure
	// (map to 502 at the HTTP layer, step 3) from every other error
	// this function can return. The user's own message is still
	// recorded when this happens — see AppendUserOnly.
	ErrProviderCallFailed = errors.New("conversation: provider call failed")
)

// SendMessage resolves the conversation's own fixed platform+model
// (set once at creation, never mutated — see
// plan/ai/conversation/step-07-fixed-platform-and-model-per-conversation.md)
// and its most recently created key, sends the conversation's recent
// history plus the new user message to that platform, and persists
// both sides as one turn. No conversation-ownership check happens
// here — that's the HTTP handler layer's concern (step 4); this
// trusts the conversationID it's given. See
// plan/ai/conversation/step-02-sending-a-message.md.
func SendMessage(
	ctx context.Context,
	httpClient *http.Client,
	mainDB *sql.DB,
	conversationDB *sql.DB,
	encryptionKey []byte,
	conversationID, content string,
) (*Turn, error) {
	if content == "" {
		return nil, ErrEmptyContent
	}
	if len(content) > maxMessageContentLength {
		return nil, ErrContentTooLong
	}

	conv, err := conversation_query.NewConversationQueryRepo(conversationDB).FindByID(conversationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrConversationNotFound
		}
		return nil, fmt.Errorf("conversation: look up conversation: %w", err)
	}

	entry, ok := platform.Lookup(conv.PlatformID)
	if !ok {
		return nil, ErrPlatformNotFound
	}
	if entry.Completer == nil {
		return nil, ErrPlatformUnsupported
	}
	model := conv.Model

	key, err := platform_query.NewKeyQueryRepo(mainDB).FindMostRecentForPlatform(conv.PlatformID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoKeyForPlatform
		}
		return nil, fmt.Errorf("conversation: look up platform key: %w", err)
	}
	plainKey, err := platform_crypto.Decrypt(key.EncryptedKey, encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("conversation: decrypt platform key: %w", err)
	}

	recent, err := ReadRecentTurns(conv.FilePath, ContextTurns)
	if err != nil {
		return nil, fmt.Errorf("conversation: read recent turns: %w", err)
	}

	messages := make([]chatcompleter.Message, 0, len(recent)*2+1)
	for _, t := range recent {
		messages = append(messages,
			chatcompleter.Message{Role: "user", Content: t.UserContent},
			chatcompleter.Message{Role: "assistant", Content: t.AssistantContent},
		)
	}
	userTimestamp := time.Now().UTC().Format(time.RFC3339)
	messages = append(messages, chatcompleter.Message{Role: "user", Content: content})

	sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	result, err := entry.Completer.ChatCompletion(sendCtx, httpClient, entry.DefaultBaseURL, plainKey, model, messages)
	if err != nil {
		// Record the user's own message even though the reply failed —
		// see step 2's open question 4. The decrypted key never
		// touches this path or any error message.
		if appendErr := AppendUserOnly(conv.FilePath, userTimestamp, content); appendErr != nil {
			return nil, fmt.Errorf("conversation: chat completion failed (%v) and failed to record the user message: %w", err, appendErr)
		}
		return nil, fmt.Errorf("%w: %v", ErrProviderCallFailed, err)
	}

	turn := Turn{
		UserTimestamp:      userTimestamp,
		UserContent:        content,
		AssistantTimestamp: time.Now().UTC().Format(time.RFC3339),
		AssistantContent:   result.Message.Content,
	}
	if err := AppendTurn(conv.FilePath, turn); err != nil {
		return nil, fmt.Errorf("conversation: persist turn: %w", err)
	}

	return &turn, nil
}
