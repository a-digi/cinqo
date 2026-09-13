// Package conversation manages each conversation's own Markdown log
// file — message content lives here, never in a SQL table. See
// plan/ai/conversation/step-01-data-model-and-separate-database.md.
package conversation

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// maxTurns/maxFileSize are the two independent retention caps —
// whichever is tighter wins; older turns are pruned from the front
// until both hold (step 1's own "Writing a turn" section).
const (
	maxTurns    = 20
	maxFileSize = 5 * 1024 * 1024 // 5MB
)

// Turn is one user+assistant exchange — the unit both retention caps
// operate on. A turn is never split across the size/count boundary.
type Turn struct {
	UserTimestamp      string
	UserContent        string
	AssistantTimestamp string
	AssistantContent   string
}

// locks serializes the append-then-prune-then-rewrite sequence (and
// reads, so a read never observes a rewrite mid-flight) per
// conversation. Keyed by the conversation's own log file path rather
// than a separately-passed conversation ID — the path already encodes
// the ID 1:1 (LogPath below), so this avoids threading an extra
// parameter through every call site that already has the path.
var locks sync.Map // filePath -> *sync.Mutex

func lockFor(filePath string) *sync.Mutex {
	actual, _ := locks.LoadOrStore(filePath, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

// LogPath returns the on-disk path for a conversation's own Markdown
// log file. The one place that ever produces this value — callers
// (step 3's create-conversation handler) write this same value into
// both Conversation.FilePath and use it as the file's real path, so a
// stored file_path is trusted precisely because nothing else can
// produce it.
func LogPath(logsRoot, conversationID string) string {
	return filepath.Join(logsRoot, conversationID+"_conversation.md")
}

var (
	userHeaderRe      = regexp.MustCompile(`(?m)^## user — (.*)$`)
	assistantHeaderRe = regexp.MustCompile(`(?m)^## assistant — (.*)$`)
)

func formatTurn(t Turn) string {
	return fmt.Sprintf("## user — %s\n\n%s\n\n## assistant — %s\n\n%s\n\n",
		t.UserTimestamp, t.UserContent, t.AssistantTimestamp, t.AssistantContent)
}

func renderTurns(turns []Turn) string {
	var sb strings.Builder
	for _, t := range turns {
		sb.WriteString(formatTurn(t))
	}
	return sb.String()
}

// parseTurns splits content on "## user — " headers, then matches each
// resulting block's own "## assistant — " header to recover both
// halves. A block missing its assistant header (a truncated/malformed
// file) is skipped rather than failing the whole parse — a partial
// turn is useless as context either way.
func parseTurns(content string) []Turn {
	starts := userHeaderRe.FindAllStringIndex(content, -1)
	turns := make([]Turn, 0, len(starts))

	for i, s := range starts {
		blockStart := s[0]
		blockEnd := len(content)
		if i+1 < len(starts) {
			blockEnd = starts[i+1][0]
		}
		block := content[blockStart:blockEnd]

		userHeader := userHeaderRe.FindStringSubmatchIndex(block)
		assistantHeader := assistantHeaderRe.FindStringSubmatchIndex(block)
		if userHeader == nil || assistantHeader == nil {
			continue
		}

		turns = append(turns, Turn{
			UserTimestamp:      block[userHeader[2]:userHeader[3]],
			UserContent:        strings.TrimSpace(block[userHeader[1]:assistantHeader[0]]),
			AssistantTimestamp: block[assistantHeader[2]:assistantHeader[3]],
			AssistantContent:   strings.TrimSpace(block[assistantHeader[1]:]),
		})
	}
	return turns
}

// AppendTurn appends one already-complete turn to filePath (creating
// it if needed), then enforces both retention caps in the same locked
// section — a single rewrite handles both constraints together, since
// dropping a turn for either reason produces the same result.
func AppendTurn(filePath string, turn Turn) error {
	mu := lockFor(filePath)
	mu.Lock()
	defer mu.Unlock()

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("conversation: open log file: %w", err)
	}
	if _, err := f.WriteString(formatTurn(turn)); err != nil {
		f.Close()
		return fmt.Errorf("conversation: append turn: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("conversation: close log file: %w", err)
	}

	return enforceRetention(filePath)
}

// enforceRetention drops the oldest turn(s) until the file holds at
// most maxTurns turns AND is at most maxFileSize bytes, rewriting the
// file once with whatever survives. A no-op (no rewrite) when both
// caps already hold.
func enforceRetention(filePath string) error {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("conversation: read log file for retention check: %w", err)
	}

	turns := parseTurns(string(raw))
	rendered := renderTurns(turns)
	if len(turns) <= maxTurns && len(rendered) <= maxFileSize {
		return nil
	}

	for len(turns) > 0 && (len(turns) > maxTurns || len(rendered) > maxFileSize) {
		turns = turns[1:]
		rendered = renderTurns(turns)
	}

	if err := os.WriteFile(filePath, []byte(rendered), 0o644); err != nil {
		return fmt.Errorf("conversation: rewrite log file after pruning: %w", err)
	}
	return nil
}

// ReadRecentTurns parses the conversation's own Markdown file and
// returns its last n turns, oldest of the n first. Used identically by
// step 2 (building the platform's context) and step 3 (the "get
// conversation" response) — one read mechanism, two callers. Reads
// directly off the file, never from any in-memory or database cache.
func ReadRecentTurns(filePath string, n int) ([]Turn, error) {
	mu := lockFor(filePath)
	mu.Lock()
	defer mu.Unlock()

	raw, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("conversation: read log file: %w", err)
	}

	turns := parseTurns(string(raw))
	if len(turns) <= n {
		return turns, nil
	}
	return turns[len(turns)-n:], nil
}
