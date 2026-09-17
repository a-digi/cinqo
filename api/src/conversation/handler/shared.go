package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	dbmanager "github.com/a-digi/coco-orm/orm"
	"github.com/a-digi/coco-server/server/request"

	auth_service "github.com/a-digi/cinqo/src/auth/service"
	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
)

// diStore mirrors src/auth/handler.diStore, src/tool/handler.diStore,
// and src/platform/handler.diStore — Get isn't part of
// serverdi.Context, so the concrete ContextBag is re-asserted to this
// local interface to reach it. Same per-package idiom already
// established repeatedly in this codebase.
type diStore interface {
	Get(key string) (any, bool)
}

var errNoToken = errors.New("no token")

// tokenFromRequest matches auth_handler.MeHandler's own established
// approach.
func tokenFromRequest(r *http.Request) string {
	if c, err := r.Cookie("access_token"); err == nil && c.Value != "" {
		return c.Value
	}
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return ""
}

// callerUserID resolves the authenticated caller's own user ID (JWT
// subject) — every handler in this feature scopes its read/write to
// this value, and never bypasses it for cinqo:super:admin (step 3's
// own deliberate exception to this codebase's usual admin-bypass
// pattern).
func callerUserID(reqCtx request.RequestContext) (string, error) {
	token := tokenFromRequest(reqCtx.GetRequest())
	if token == "" {
		return "", errNoToken
	}

	storeCtx, ok := reqCtx.GetDI().(diStore)
	if !ok {
		return "", errNoToken
	}
	raw, ok := storeCtx.Get("jwks_service")
	if !ok {
		return "", errNoToken
	}
	jwksSvc, ok := raw.(*auth_service.JwksService)
	if !ok {
		return "", errNoToken
	}

	sub, _, _, err := jwksSvc.Validate(token)
	if err != nil || sub == "" {
		return "", errNoToken
	}
	return sub, nil
}

// callerScopes resolves the authenticated caller's own scopes — used
// to scope-filter which MCP tools get offered to the model (see
// plan/ai/tools/pdf-generator/step-04-ai-model-invocation.md). Same
// token-validation path as callerUserID, kept as its own function
// since most callers only need one or the other.
func callerScopes(reqCtx request.RequestContext) ([]string, error) {
	token := tokenFromRequest(reqCtx.GetRequest())
	if token == "" {
		return nil, errNoToken
	}

	storeCtx, ok := reqCtx.GetDI().(diStore)
	if !ok {
		return nil, errNoToken
	}
	raw, ok := storeCtx.Get("jwks_service")
	if !ok {
		return nil, errNoToken
	}
	jwksSvc, ok := raw.(*auth_service.JwksService)
	if !ok {
		return nil, errNoToken
	}

	_, scopes, _, err := jwksSvc.Validate(token)
	if err != nil {
		return nil, errNoToken
	}
	return scopes, nil
}

// conversationDB resolves the conversation feature's own separate
// database (cinqo.Start registers it into DI as
// "conversation_db_manager" — plan/ai/conversation/step-01).
func conversationDB(reqCtx request.RequestContext) (*sql.DB, error) {
	storeCtx, ok := reqCtx.GetDI().(diStore)
	if !ok {
		return nil, errors.New("conversation database not configured")
	}
	raw, ok := storeCtx.Get("conversation_db_manager")
	if !ok {
		return nil, errors.New("conversation database not configured")
	}
	manager, ok := raw.(*dbmanager.DatabaseManager)
	if !ok {
		return nil, errors.New("conversation database not configured")
	}
	return manager.Connector.DB, nil
}

// encryptionKey resolves the platform API-key encryption key
// (plan/ai/platform/step-02's platform_crypto.LoadOrGenerateKey,
// registered into DI by cinqo.Start under the same
// "platform_encryption_key" key plan/ai/platform/step-05's handlers
// use).
func encryptionKey(reqCtx request.RequestContext) ([]byte, error) {
	storeCtx, ok := reqCtx.GetDI().(diStore)
	if !ok {
		return nil, errors.New("encryption key not configured")
	}
	raw, ok := storeCtx.Get("platform_encryption_key")
	if !ok {
		return nil, errors.New("encryption key not configured")
	}
	key, ok := raw.([]byte)
	if !ok {
		return nil, errors.New("encryption key not configured")
	}
	return key, nil
}

// defaultDataDir mirrors cinqo.ResolveDataDir's own fallback
// ("data") for the case "data_dir" isn't in DI at all — should never
// happen once cinqo.Start has run, but this package staying
// self-contained (not importing cinqo just for one string
// constant) is worth a one-line duplicated literal.
const defaultDataDir = "data"

// errCorePortUnavailable / corePort mirror tool/handler/paths.go's own
// identical helpers exactly — same "resolved once at boot, via DI"
// fix for the same class of bug: conversation/chat.go's own
// readCorePort used to call a bare server.LoadConfig("config.json"),
// CWD-relative with no fallback, the exact bug already fixed in
// tool/handler/paths.go's own corePort (step 20) but never carried
// over to this package's own intentionally-duplicated twin — found
// live, not assumed, when a real AI conversation's tool call ("please
// call list_portals") failed with "tool invocation failed: could not
// open config file: open config.json: no such file or directory" on
// an install running via apphome (CWD without its own config.json).
// See plan/ai/build/app/step-20-app-version-via-di.md and
// plan/ai/conversation/step-13-conversation-core-port-via-di.md.
var errCorePortUnavailable = errors.New("core port unavailable")

func corePort(reqCtx request.RequestContext) (int, error) {
	storeCtx, ok := reqCtx.GetDI().(diStore)
	if !ok {
		return 0, errCorePortUnavailable
	}
	raw, ok := storeCtx.Get("core_port")
	if !ok {
		return 0, errCorePortUnavailable
	}
	p, ok := raw.(int)
	if !ok || p == 0 {
		return 0, errCorePortUnavailable
	}
	return p, nil
}

// resolvedDataDir resolves whatever cinqo.Start registered as
// "data_dir" (defaults to "data", next to the running executable,
// unless overridden via --data) — the same root logsRoot's own
// "conversations" subdirectory and every tool subprocess's own
// TOOL_DB_DIR/TOOL_UPLOADS_DIR/TOOL_TMP_DIR (manager.ToolEnvVars, via
// runner.go's runDetachedTurn -> chat.go's invokeToolCall) derive
// from — extracted out of logsRoot so send_message_handler.go can pass
// the bare data directory itself into StartTurnRun without duplicating
// this same DI lookup. See
// plan/ai/build/app/step-17-configurable-data-directory.md and
// plan/ai/build/app/step-22-data-dir-always-executable-relative.md.
func resolvedDataDir(reqCtx request.RequestContext) string {
	dataDir := defaultDataDir
	if storeCtx, ok := reqCtx.GetDI().(diStore); ok {
		if raw, ok := storeCtx.Get("data_dir"); ok {
			if s, ok := raw.(string); ok && s != "" {
				dataDir = s
			}
		}
	}
	return dataDir
}

// logsRoot resolves the shared root directory every conversation's
// own log file lives under — the "conversations" subdirectory of the
// resolved data directory. Was a plain exported conversation.LogsRoot
// constant before --data existed; now resolved per-request since the
// value isn't known until Start runs. See
// plan/ai/build/app/step-17-configurable-data-directory.md.
func logsRoot(reqCtx request.RequestContext) string {
	return filepath.Join(resolvedDataDir(reqCtx), "conversations")
}

// conversationResponse is the shape every conversation-metadata
// response uses (create/list/get/rename) — never FilePath or UserID
// (both json:"-" on the entity anyway, but this DTO makes that
// explicit at the response-shape level too). PlatformID/Model ARE
// included — fixed at creation and never mutated afterward, but the
// frontend still needs to know what they are to display them (step 7).
type conversationResponse struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	StartedAt  string `json:"startedAt"`
	PlatformID string `json:"platformId"`
	Model      string `json:"model"`
}

func toConversationResponse(c *conversation_entity.Conversation) conversationResponse {
	return conversationResponse{
		ID:         c.ID,
		Title:      c.Title,
		StartedAt:  c.StartedAt,
		PlatformID: c.PlatformID,
		Model:      c.Model,
	}
}
