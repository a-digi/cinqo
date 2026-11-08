// Package handler holds domainevent's own HTTP surface — currently
// just PublishHandler (POST /api/v1/events/publish), letting a tool's
// own backend process publish an event into the bus it authored. Split
// from domainevent's own core package the same way media/handler is
// split from media's own core logic. See
// plan/ai/domain-events/step-05-tool-publish-endpoint.md.
package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/domainevent"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

// toolServiceAuthScheme is this route's own deliberately distinct
// Authorization scheme name — never "Bearer" — so a tool's own
// persistent service_token is never visually confused with an end
// user's session JWT at a glance, even though both travel in the same
// header.
const toolServiceAuthScheme = "ToolService "

type publishRequest struct {
	Topic   string          `json:"topic"`
	Payload json.RawMessage `json:"payload"`
}

type publishResponse struct {
	ID string `json:"id"`
}

// PublishHandler handles POST /api/v1/events/publish — deliberately
// NOT gated by the normal scope/RBAC system (its own route declares
// "security: public"): a tool's own service_token, checked here, is a
// machine-identity credential, not an end-user permission. The only
// caller with a legitimate reason to reach this route is a loopback
// tool subprocess holding a secret only core itself ever generated and
// handed to it (tool/manager.go's own TOOL_SERVICE_TOKEN env var).
func PublishHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	r := reqCtx.GetRequest()

	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), toolServiceAuthScheme)
	if !ok {
		response.ErrorResponse(w, http.StatusUnauthorized, "missing or malformed Authorization header")
		return
	}

	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	tool, err := tool_query.NewToolQueryRepo(db).FindByServiceToken(token)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusInternalServerError, "failed to authenticate")
			return
		}
		response.ErrorResponse(w, http.StatusUnauthorized, "invalid service token")
		return
	}

	var body publishRequest
	if err := reqCtx.BindJSON(&body); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(body.Topic) == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "topic is required")
		return
	}

	// The one runtime enforcement point in the whole Domain Events
	// engine that stops a tool from publishing under another tool's own
	// namespace or forging a core-native "cinqo."-prefixed event — see
	// this step's own design doc.
	requiredPrefix := tool.Slug + "."
	if !strings.HasPrefix(body.Topic, requiredPrefix) {
		response.ErrorResponse(w, http.StatusBadRequest, `topic must start with "`+requiredPrefix+`"`)
		return
	}

	evt, err := domainevent.PublishFromTool(body.Topic, body.Payload, tool.Slug)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to publish event")
		return
	}

	response.SuccessResponse(w, http.StatusAccepted, publishResponse{ID: evt.ID})
}
