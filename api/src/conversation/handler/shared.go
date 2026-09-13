package handler

import (
	"database/sql"
	"errors"
	"net/http"
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

// conversationDB resolves the conversation feature's own separate
// database (backendapp.Start registers it into DI as
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
// registered into DI by backendapp.Start under the same
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
