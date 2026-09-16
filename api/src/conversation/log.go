// Package conversation manages each conversation's own Markdown log
// file — message content lives here, never in a SQL table. See
// plan/ai/conversation/step-01-data-model-and-separate-database.md.
package conversation

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
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
//
// Failed/ErrorTimestamp/ErrorMessage are set instead of the two
// Assistant* fields when this turn's own provider call failed —
// mutually exclusive with a successful assistant reply, never both
// populated. See
// plan/ai/conversation/step-08-failed-message-handling.md.
type Turn struct {
	UserTimestamp      string
	UserContent        string
	AssistantTimestamp string
	AssistantContent   string
	Failed             bool
	ErrorTimestamp     string
	ErrorMessage       string
	// PromptTokens/CompletionTokens (step 35) are this turn's own real,
	// provider-reported token usage, accumulated across its whole
	// tool-calling loop (runner.go's runDetachedTurn) — set for both a
	// successful turn (accumulated up to the final reply) and a failed
	// one (whatever was accumulated up to the point of failure). Zero
	// for every turn logged before this field existed. See
	// plan/ai/conversation/step-35-persist-per-turn-token-usage.md.
	PromptTokens     int
	CompletionTokens int
}

// DurationMs returns how long this turn took to resolve — from the
// user's own message to the assistant's reply (or, for a failed
// turn, to the recorded error) — computed from the two already-
// persisted RFC3339 timestamps every turn already carries, never a
// separately stored value (so it can never drift from the timestamps
// it's derived from). Returns nil only if a timestamp fails to parse
// — should never happen in practice, since both are always written
// by SendMessage in this exact format; defensive, not expected to
// fire. See plan/ai/conversation/step-14-turn-duration-backend.md.
func (t Turn) DurationMs() *int64 {
	start, err := time.Parse(time.RFC3339, t.UserTimestamp)
	if err != nil {
		return nil
	}
	endStr := t.AssistantTimestamp
	if t.Failed {
		endStr = t.ErrorTimestamp
	}
	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return nil
	}
	ms := end.Sub(start).Milliseconds()
	return &ms
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
	errorHeaderRe     = regexp.MustCompile(`(?m)^## error — (.*)$`)
)

// formatTurn serializes a normal successful turn as "## user — / ##
// assistant —", or a failed one (Turn.Failed) as "## user — / ##
// error —" instead — the two shapes are mutually exclusive on one
// Turn, so a single branch here is enough for both AppendTurn (the
// only writer, reused unchanged for a failed turn too) and
// renderTurns' own retention-pruning rewrite to round-trip a failed
// turn correctly.
func formatTurn(t Turn) string {
	if t.Failed {
		return fmt.Sprintf("## user — %s\n\n%s\n\n## error — %s%s\n\n%s\n\n",
			t.UserTimestamp, t.UserContent, t.ErrorTimestamp, tokenSuffix(t.PromptTokens, t.CompletionTokens), t.ErrorMessage)
	}
	return fmt.Sprintf("## user — %s\n\n%s\n\n## assistant — %s%s\n\n%s\n\n",
		t.UserTimestamp, t.UserContent, t.AssistantTimestamp, tokenSuffix(t.PromptTokens, t.CompletionTokens), t.AssistantContent)
}

// tokenSuffix (step 35) appends " prompt=N completion=N" to an
// "## assistant —"/"## error —" header line — omitted entirely
// (returns "") when both are 0, so an old turn logged before this
// field existed, or one whose provider never reported usage, renders
// byte-identical to today. See parseHeaderTimestampAndTokens, below,
// for the matching read side.
func tokenSuffix(promptTokens, completionTokens int) string {
	if promptTokens == 0 && completionTokens == 0 {
		return ""
	}
	return fmt.Sprintf(" prompt=%d completion=%d", promptTokens, completionTokens)
}

// parseHeaderTimestampAndTokens splits an "## assistant —"/"## error —"
// header's own already-captured rest-of-line text into its timestamp
// plus an optional " prompt=N completion=N" suffix (step 35) — never
// applied to the "## user —" header, which never carries tokens. An
// old entry with only a bare timestamp (no suffix) parses identically
// to before this step: both token fields default to 0. No regex
// change needed anywhere — userHeaderRe/assistantHeaderRe/errorHeaderRe
// already capture the whole rest of the line via `(.*)$`.
func parseHeaderTimestampAndTokens(raw string) (timestamp string, promptTokens, completionTokens int) {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return "", 0, 0
	}
	timestamp = fields[0]
	for _, f := range fields[1:] {
		if v, ok := strings.CutPrefix(f, "prompt="); ok {
			promptTokens, _ = strconv.Atoi(v)
		}
		if v, ok := strings.CutPrefix(f, "completion="); ok {
			completionTokens, _ = strconv.Atoi(v)
		}
	}
	return timestamp, promptTokens, completionTokens
}

func renderTurns(turns []Turn) string {
	var sb strings.Builder
	for _, t := range turns {
		sb.WriteString(formatTurn(t))
	}
	return sb.String()
}

// parseTurns splits content on "## user — " headers, then matches each
// resulting block's own second header to recover the other half — a
// normal "## assistant — " header (a successful turn) or a "## error
// — " header (a failed one, Turn.Failed — see
// plan/ai/conversation/step-08-failed-message-handling.md). A block
// with NEITHER header (a truncated/malformed file, or a dangling
// fragment written before this step's own format existed) is skipped
// rather than failing the whole parse — a partial turn is useless as
// context either way, and there's no way to retroactively know what
// error (if any) an already-silently-dropped old entry represents.
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
		if userHeader == nil {
			continue
		}

		if assistantHeader := assistantHeaderRe.FindStringSubmatchIndex(block); assistantHeader != nil {
			ts, promptTokens, completionTokens := parseHeaderTimestampAndTokens(block[assistantHeader[2]:assistantHeader[3]])
			turns = append(turns, Turn{
				UserTimestamp:      block[userHeader[2]:userHeader[3]],
				UserContent:        strings.TrimSpace(block[userHeader[1]:assistantHeader[0]]),
				AssistantTimestamp: ts,
				AssistantContent:   strings.TrimSpace(block[assistantHeader[1]:]),
				PromptTokens:       promptTokens,
				CompletionTokens:   completionTokens,
			})
			continue
		}

		if errorHeader := errorHeaderRe.FindStringSubmatchIndex(block); errorHeader != nil {
			ts, promptTokens, completionTokens := parseHeaderTimestampAndTokens(block[errorHeader[2]:errorHeader[3]])
			turns = append(turns, Turn{
				UserTimestamp:    block[userHeader[2]:userHeader[3]],
				UserContent:      strings.TrimSpace(block[userHeader[1]:errorHeader[0]]),
				Failed:           true,
				ErrorTimestamp:   ts,
				ErrorMessage:     strings.TrimSpace(block[errorHeader[1]:]),
				PromptTokens:     promptTokens,
				CompletionTokens: completionTokens,
			})
			continue
		}
		// neither header — old-style dangling fragment, skipped.
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
// A conversation's log file doesn't exist yet until its first turn is
// ever appended (AppendTurn creates it lazily) — that's zero turns,
// not an error, so a not-yet-created file returns (nil, nil) rather
// than failing.
func ReadRecentTurns(filePath string, n int) ([]Turn, error) {
	mu := lockFor(filePath)
	mu.Lock()
	defer mu.Unlock()

	raw, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("conversation: read log file: %w", err)
	}

	turns := parseTurns(string(raw))
	if len(turns) <= n {
		return turns, nil
	}
	return turns[len(turns)-n:], nil
}
