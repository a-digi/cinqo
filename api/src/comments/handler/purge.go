package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/comments"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

// toolServiceAuthScheme is the Authorization scheme a tool's own backend uses to
// call core as itself — deliberately not "Bearer", so a tool's service token is
// never confused with a user's session token. Same scheme as POST /api/v1/events/publish.
const toolServiceAuthScheme = "ToolService "

var (
	errNoServiceToken      = errors.New("missing or malformed Authorization header")
	errBadServiceToken     = errors.New("invalid service token")
	errToolHasNoSuchType   = errors.New("this tool did not declare that entity type")
	errSystemOwnerReserved = errors.New("not allowed")
)

// findToolByToken is how the purge route turns a service token into the tool's slug.
type findToolByToken func(token string) (slug string, err error)

// toolFinder is the production lookup: the tool whose own service_token this is.
func toolFinder(db *sql.DB) findToolByToken {
	return func(token string) (string, error) {
		tool, err := tool_query.NewToolQueryRepo(db).FindByServiceToken(token)
		if errors.Is(err, sql.ErrNoRows) {
			return "", errBadServiceToken
		}
		if err != nil {
			return "", err
		}
		return tool.Slug, nil
	}
}

// toolFromRequest authenticates the calling tool: its slug is the owner of whatever
// this request is allowed to touch. The owner is never taken from the request.
func toolFromRequest(r *http.Request, find findToolByToken) (string, error) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), toolServiceAuthScheme)
	if !ok || strings.TrimSpace(token) == "" {
		return "", errNoServiceToken
	}
	return find(strings.TrimSpace(token))
}

type purgeRequest struct {
	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId"`
}

type purgeResponse struct {
	Deleted int `json:"deleted"`
}

// purge removes every comment of one entity of the calling tool. Only for an entity
// type that tool declared (and is enabled), and only in the tool's own database.
func (d *deps) purge(w http.ResponseWriter, r *http.Request, tool string) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in purgeRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, `body must be {"entityType": "…", "entityId": "…"}`)
		return
	}
	// A tool whose slug is the system's reserved owner name must never reach the
	// system's comments, whatever types exist.
	if tool == comments.SystemOwner {
		response.ErrorResponse(w, http.StatusForbidden, errSystemOwnerReserved.Error())
		return
	}
	if _, ok, err := d.resolver.Find(tool, in.EntityType); err != nil {
		d.fail(w, err)
		return
	} else if !ok {
		response.ErrorResponse(w, http.StatusForbidden, errToolHasNoSuchType.Error())
		return
	}
	store, err := d.stores.Store(tool)
	if err != nil {
		d.fail(w, err)
		return
	}
	n, err := store.PurgeEntity(in.EntityType, in.EntityID)
	if err != nil {
		d.fail(w, err)
		return
	}
	response.SuccessResponse(w, http.StatusOK, purgeResponse{Deleted: n})
}

// PurgeHandler handles POST /api/v1/comments/purge — for a tool's own backend, to
// remove the comments of an entity it has deleted. Not behind the user/scope layer
// (its YAML says "security: public"): the credential is the tool's own service
// token, checked here, and the owner is the tool that owns it.
func PurgeHandler(reqCtx request.RequestContext) {
	w, r := reqCtx.GetWriter(), reqCtx.GetRequest()
	if r.Method != http.MethodPost {
		response.ErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	d := configured()
	if d == nil {
		response.ErrorResponse(w, http.StatusServiceUnavailable, "comments are not available")
		return
	}
	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	tool, err := toolFromRequest(r, toolFinder(db))
	switch {
	case errors.Is(err, errNoServiceToken), errors.Is(err, errBadServiceToken):
		response.ErrorResponse(w, http.StatusUnauthorized, err.Error())
		return
	case err != nil:
		d.logf("comments: authenticating a tool failed: %v", err)
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to authenticate")
		return
	}
	d.purge(w, r, tool)
}
