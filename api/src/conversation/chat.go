package conversation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
	platform_crypto "github.com/a-digi/cinqo/src/platform/crypto"
	platform_query "github.com/a-digi/cinqo/src/platform/repository/query"
	tool_manager "github.com/a-digi/cinqo/src/tool/manager"
	tool_mcp "github.com/a-digi/cinqo/src/tool/mcp"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"

	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

// maxMessageContentLength matches the reference's own
// MaxMessageContentLength exactly — a proven value, not picked
// arbitrarily. See plan/ai/conversation/step-02-sending-a-message.md.
//
// No send-side timeout is imposed here (deliberately removed — see
// plan/ai/conversation/step-12-remove-send-timeout.md): a real LLM
// call, especially one running a multi-step tool-calling loop, can
// legitimately take a long time, and there is no server-level
// http.Server.WriteTimeout/ReadTimeout configured either (verified
// directly in coco-server's own server.go — the http.Server there
// sets only Addr/Handler), so the operation is bounded only by the
// incoming request's own context (cancelled if the caller's own
// connection goes away) and by each individual downstream call's own
// timeout (chatcompleter HTTP client, tool_mcp.Invoke's own
// invokeTimeout, etc.), not by an artificial ceiling on the whole
// send.
const maxMessageContentLength = 8000

// ContextTurns is how many recent turns are sent to the platform as
// context — the same tail-read step 1 recommends, not the full
// retained window. Exported: step 3's "get conversation" handler uses
// this exact same value for what it shows the frontend, per step 1's
// own "one read mechanism, two callers" design. See step 1's own open
// question 1 (3 vs 4).
const ContextTurns = 4

// maxToolIterations bounds the tool-calling loop (send -> tool call ->
// invoke -> send again) — a hard ceiling against a model that keeps
// calling tools without ever producing a final reply. Originally 5
// (plan/ai/tools/pdf-generator/step-04-ai-model-invocation.md), raised
// to 15 (step-29-batch-save-portal-jobs.md) — 5 was tuned for
// pdf-generator's own single-tool-call flows and structurally
// collided with career's own multi-step crawl workflow (get
// instructions, navigate, crawl_paginated, then one save call per job
// found), which a real job-listing page routinely needs more than 5
// round-trips for even after step 29's own save_portal_jobs batching
// cuts the per-job multiplier out entirely.
const maxToolIterations = 15

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
	// ErrToolIterationLimitReached fires when maxToolIterations is hit
	// without the model ever returning a plain final reply — a real,
	// visible failure mode (mapped to its own response at the HTTP
	// layer), not silently swallowed.
	ErrToolIterationLimitReached = errors.New("conversation: model kept calling tools without a final reply")
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
	callerScopes []string,
	corePort int,
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
		if t.Failed {
			// No real exchange happened — replaying the user's own
			// text with a fabricated empty assistant reply would be
			// wrong (and some providers reject an empty message
			// outright). Skip the whole turn rather than either.
			continue
		}
		messages = append(messages,
			chatcompleter.Message{Role: "user", Content: t.UserContent},
			chatcompleter.Message{Role: "assistant", Content: t.AssistantContent},
		)
	}
	userTimestamp := time.Now().UTC().Format(time.RFC3339)
	messages = append(messages, chatcompleter.Message{Role: "user", Content: content})

	tools, err := offerableTools(mainDB, callerScopes)
	if err != nil {
		return nil, fmt.Errorf("conversation: look up offerable tools: %w", err)
	}

	assistantContent, err := runToolLoop(ctx, httpClient, entry, plainKey, model, messages, tools, mainDB, callerScopes, corePort, nil)
	if err != nil {
		// Record the user's own message AND the real failure reason —
		// a real, recognized "## error —" block (step 8), not a
		// dangling fragment silently dropped on the next read. The
		// decrypted key never touches this path or any error message.
		failedTurn := Turn{
			UserTimestamp:  userTimestamp,
			UserContent:    content,
			Failed:         true,
			ErrorTimestamp: time.Now().UTC().Format(time.RFC3339),
			ErrorMessage:   truncateError(err.Error()),
		}
		if appendErr := AppendTurn(conv.FilePath, failedTurn); appendErr != nil {
			return nil, fmt.Errorf("conversation: chat completion failed (%v) and failed to record the failed turn: %w", err, appendErr)
		}
		if errors.Is(err, ErrToolIterationLimitReached) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", ErrProviderCallFailed, err)
	}

	turn := Turn{
		UserTimestamp:      userTimestamp,
		UserContent:        content,
		AssistantTimestamp: time.Now().UTC().Format(time.RFC3339),
		AssistantContent:   assistantContent,
	}
	if err := AppendTurn(conv.FilePath, turn); err != nil {
		return nil, fmt.Errorf("conversation: persist turn: %w", err)
	}

	return &turn, nil
}

// offerableTools reads every enabled tool's cached MCP schema and
// keeps only the ones callerScopes actually grants (cinqo:super:admin
// bypasses, matching this codebase's usual OR-matched scope
// convention) — the pre-request half of the "check twice" posture;
// runToolLoop/invokeToolCall re-check again immediately before
// actually spawning anything. See
// plan/ai/tools/pdf-generator/step-04-ai-model-invocation.md.
func offerableTools(mainDB *sql.DB, callerScopes []string) ([]chatcompleter.ToolDef, error) {
	mcpTools, err := tool_query.NewToolMCPToolQueryRepo(mainDB).FindAllEnabled()
	if err != nil {
		return nil, err
	}
	out := make([]chatcompleter.ToolDef, 0, len(mcpTools))
	for _, t := range mcpTools {
		if !hasScope(callerScopes, t.RequiredScope) {
			continue
		}
		out = append(out, chatcompleter.ToolDef{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: json.RawMessage(t.InputSchema),
		})
	}
	return out, nil
}

// runToolLoop is the send -> tool call -> invoke -> send-again loop,
// bounded by maxToolIterations. Tool calls within one turn are invoked
// sequentially, not concurrently — a model requesting the same
// Chrome-backed tool twice in parallel would otherwise spawn two
// headless Chrome instances at once.
//
// Every real file link any tool call produced this turn is collected
// (allLinks) and deterministically appended to the model's own final
// reply — never left to the model to reproduce unchanged. This closes
// a real, live-observed failure: asked to relay a tool's result, a
// model paraphrased a real relative link into a fabricated absolute
// URL pointing at an entirely unrelated domain. The underlying
// generation/storage was correct; only the model's own retelling of
// the link was wrong — so the host now states the real link itself
// rather than trusting that retelling. See
// plan/ai/tools/pdf-generator/step-04-ai-model-invocation.md.
// logStep, when non-nil, is called once per loop iteration (before
// the model call) and once per tool invocation within that iteration
// — a coarse, step-by-step progress trace a caller can persist
// somewhere a client can poll (see runner.go's own use of this, added
// by plan/ai/conversation/step-23-detach-turn-execution-from-request.md).
// Never receives raw model/tool content, only fixed, backend-authored
// strings — see that step's own Security considerations for why.
func runToolLoop(
	ctx context.Context,
	httpClient *http.Client,
	entry platform.Entry,
	apiKey, model string,
	messages []chatcompleter.Message,
	tools []chatcompleter.ToolDef,
	mainDB *sql.DB,
	callerScopes []string,
	corePort int,
	logStep func(step string),
) (string, error) {
	var allLinks []tool_mcp.ResourceLink

	for i := 0; i < maxToolIterations; i++ {
		if logStep != nil {
			logStep(fmt.Sprintf("iteration %d: calling model", i+1))
		}
		result, err := entry.Completer.ChatCompletion(ctx, httpClient, entry.DefaultBaseURL, apiKey, model, messages, tools)
		if err != nil {
			return "", err
		}
		if len(result.ToolCalls) == 0 {
			return appendResourceLinks(result.Message.Content, allLinks), nil
		}

		messages = append(messages, chatcompleter.Message{
			Role:      "assistant",
			Content:   result.Message.Content,
			ToolCalls: result.ToolCalls,
		})

		for _, call := range result.ToolCalls {
			if logStep != nil {
				logStep(fmt.Sprintf("iteration %d: invoking tool %s", i+1, call.Name))
			}
			text, links := invokeToolCall(ctx, mainDB, callerScopes, call, corePort)
			messages = append(messages, chatcompleter.Message{Role: "tool", ToolCallID: call.ID, Content: text})
			allLinks = append(allLinks, links...)
		}
	}
	return "", ErrToolIterationLimitReached
}

// appendResourceLinks deterministically adds every real file link
// produced this turn to the model's own final text — always, whether
// or not the model's own prose already mentioned one, since detecting
// "did the model already say this correctly" would mean parsing its
// freeform text (fragile) — a little possible redundancy is a small
// price for never showing a wrong link. A no-op when no tool this
// turn produced a file (the common, non-tool-calling case).
func appendResourceLinks(content string, links []tool_mcp.ResourceLink) string {
	if len(links) == 0 {
		return content
	}
	var sb strings.Builder
	sb.WriteString(content)
	if len(links) == 1 {
		sb.WriteString(fmt.Sprintf("\n\nGenerated file: [%s](%s)", links[0].Name, links[0].URI))
	} else {
		sb.WriteString("\n\nGenerated files:")
		for _, l := range links {
			sb.WriteString(fmt.Sprintf("\n- [%s](%s)", l.Name, l.URI))
		}
	}
	return sb.String()
}

// invokeToolCall never returns a bare error — MCP's own two-tier
// error model (protocol errors vs. tool-reported isError:true) and
// cinqo's own defense-in-depth scope re-check are both collapsed into
// plain result text here, so the model always receives a tool-result
// message it can react to instead of an unanswered tool_call
// stranding the conversation. See
// plan/ai/tools/pdf-generator/step-04-ai-model-invocation.md's
// "Invocation, concretely".
func invokeToolCall(ctx context.Context, mainDB *sql.DB, callerScopes []string, call chatcompleter.ToolCall, corePort int) (string, []tool_mcp.ResourceLink) {
	mcpTool, err := tool_query.NewToolMCPToolQueryRepo(mainDB).FindMCPToolByName(call.Name)
	if err != nil {
		return "tool unavailable", nil
	}
	if !hasScope(callerScopes, mcpTool.RequiredScope) {
		return "tool unavailable", nil
	}

	tool, err := tool_query.NewToolQueryRepo(mainDB).FindByID(mcpTool.ToolID)
	if err != nil || !tool.Enabled {
		return "tool unavailable", nil
	}

	envVars, err := tool_manager.ToolEnvVars(tool.Slug, corePort)
	if err != nil {
		return fmt.Sprintf("tool invocation failed: %v", err), nil
	}
	// See install_handler.go's identical addition and
	// plan/ai/tools/browser/step-02-shared-browser-session.md — lets a
	// tool's own --mcp subprocess reach its already-running HTTP-mode
	// sibling for state that must persist across separate calls.
	if port, ok := tool_manager.Port(tool.ID); ok {
		envVars = append(envVars, fmt.Sprintf("TOOL_OWN_PORT=%d", port))
	}
	execPath, err := filepath.Abs(filepath.Join(tool.InstallPath, tool.BackendExecutableRelpath))
	if err != nil {
		return fmt.Sprintf("tool invocation failed: %v", err), nil
	}

	text, _, links := tool_mcp.Invoke(ctx, execPath, call.Name, call.Arguments, envVars)
	return text, links
}

// hasScope matches proxy_handler.go's own established
// cinqo:super:admin-bypasses-everything convention — duplicated here
// rather than shared, matching this codebase's existing per-package
// idiom for this exact small helper.
func hasScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == want || s == "cinqo:super:admin" {
			return true
		}
	}
	return false
}

// maxStoredErrorLength caps a failed turn's own stored error text — a
// provider's raw error body is untrusted-length text; without a cap, a
// single pathological response could bloat the log file
// disproportionately. See
// plan/ai/conversation/step-08-failed-message-handling.md.
const maxStoredErrorLength = 2000

func truncateError(s string) string {
	if len(s) <= maxStoredErrorLength {
		return s
	}
	return s[:maxStoredErrorLength] + "… (truncated)"
}
