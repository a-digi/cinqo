// Package handler is the operator's API for organisations. Every route needs cinqo:organisations:admin (or the super admin), declared in
// config/routes/route-organisation.yaml.
package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	auth_service "github.com/a-digi/cinqo/src/auth/service"
	"github.com/a-digi/cinqo/src/organisation/entity"
	"github.com/a-digi/cinqo/src/organisation/repository/persistent"
	"github.com/a-digi/cinqo/src/organisation/repository/query"
)

// maxUserIDLength bounds the user id taken from the address (the identity provider's sub).
const maxUserIDLength = 200

type diStore interface {
	Get(key string) (any, bool)
}

func dbOf(reqCtx request.RequestContext) *sql.DB {
	return reqCtx.GetDI().GetDatabaseManager().Connector.DB
}

// callerUserID is the validated token's own user id.
func callerUserID(reqCtx request.RequestContext) string {
	r := reqCtx.GetRequest()
	token := ""
	if c, err := r.Cookie("access_token"); err == nil && c.Value != "" {
		token = c.Value
	} else if parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2); len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		token = parts[1]
	}
	if token == "" {
		return ""
	}
	store, ok := reqCtx.GetDI().(diStore)
	if !ok {
		return ""
	}
	raw, ok := store.Get("jwks_service")
	if !ok {
		return ""
	}
	svc, ok := raw.(*auth_service.JwksService)
	if !ok {
		return ""
	}
	sub, _, _, err := svc.Validate(token)
	if err != nil {
		return ""
	}
	return sub
}

type listItem struct {
	entity.Organisation
	MemberCount int `json:"memberCount"`
	// BackgroundEnabled: tools the administrator approved may work for this organisation without a signed-in person (off until the operator switches it on).
	BackgroundEnabled bool `json:"backgroundEnabled"`
}

type nameStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type assignBody struct {
	OrgID string `json:"orgId"`
}

func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, persistent.ErrInvalidName), errors.Is(err, persistent.ErrInvalidStatus), errors.Is(err, persistent.ErrInvalidUser):
		response.ErrorResponse(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, persistent.ErrNoSuchOrg), errors.Is(err, sql.ErrNoRows):
		response.ErrorResponse(w, http.StatusNotFound, "organisation not found")
	default:
		response.ErrorResponse(w, http.StatusInternalServerError, "organisation request failed")
	}
}

// ---- GET /organisations ----

func ListHandler(reqCtx request.RequestContext) { list(dbOf(reqCtx), reqCtx) }

func list(db *sql.DB, reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	q := query.NewOrganisationQueryRepo(db)
	orgs, err := q.List()
	if err != nil {
		fail(w, err)
		return
	}
	counts, err := q.MemberCounts()
	if err != nil {
		fail(w, err)
		return
	}
	states, err := q.BackgroundStates()
	if err != nil {
		fail(w, err)
		return
	}
	items := make([]listItem, 0, len(orgs))
	for _, o := range orgs {
		items = append(items, listItem{Organisation: o, MemberCount: counts[o.ID], BackgroundEnabled: states[o.ID]})
	}
	response.SuccessResponse(w, http.StatusOK, map[string]any{"items": items})
}

// ---- POST /organisations ----

func CreateHandler(reqCtx request.RequestContext) { create(dbOf(reqCtx), reqCtx) }

func create(db *sql.DB, reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	var body nameStatus
	if err := reqCtx.BindJSON(&body); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	o, err := persistent.NewOrganisationPersistentRepo(db).Create(body.Name)
	if err != nil {
		fail(w, err)
		return
	}
	response.SuccessResponse(w, http.StatusCreated, o)
}

// ---- PUT /organisations/{id} ----

func UpdateHandler(reqCtx request.RequestContext) { update(dbOf(reqCtx), callerUserID(reqCtx), reqCtx) }

// update renames an organisation and sets its status. The operator cannot suspend the organisation they are in themselves: that would lock them out of every
// tool, with no way back.
func update(db *sql.DB, caller string, reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	id := reqCtx.GetURI().GetPathVariable("id")
	var body nameStatus
	if err := reqCtx.BindJSON(&body); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	q := query.NewOrganisationQueryRepo(db)
	if body.Status == entity.StatusSuspended && caller != "" {
		if mine, err := q.FindByUser(caller); err == nil && mine.ID == id {
			response.ErrorResponse(w, http.StatusConflict, "you cannot suspend your own organisation")
			return
		}
	}
	if err := persistent.NewOrganisationPersistentRepo(db).Update(id, body.Name, body.Status); err != nil {
		fail(w, err)
		return
	}
	o, err := q.FindByID(id)
	if err != nil {
		fail(w, err)
		return
	}
	response.SuccessResponse(w, http.StatusOK, o)
}

// ---- GET /organisations/{id}/members ----

func MembersHandler(reqCtx request.RequestContext) { members(dbOf(reqCtx), reqCtx) }

func members(db *sql.DB, reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	id := reqCtx.GetURI().GetPathVariable("id")
	q := query.NewOrganisationQueryRepo(db)
	if _, err := q.FindByID(id); err != nil {
		fail(w, err)
		return
	}
	m, err := q.Members(id)
	if err != nil {
		fail(w, err)
		return
	}
	response.SuccessResponse(w, http.StatusOK, map[string]any{"items": m})
}

// ---- PUT /users/{sub}/organisation ----

func AssignHandler(reqCtx request.RequestContext) { assign(dbOf(reqCtx), callerUserID(reqCtx), reqCtx) }

// assign puts a user into an organisation (a move when they were in another). The operator cannot move themselves into a suspended organisation (same lock-out).
func assign(db *sql.DB, caller string, reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	user := reqCtx.GetURI().GetPathVariable("sub")
	if strings.TrimSpace(user) == "" || len(user) > maxUserIDLength {
		response.ErrorResponse(w, http.StatusBadRequest, "the user id must be 1 to 200 characters")
		return
	}
	var body assignBody
	if err := reqCtx.BindJSON(&body); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	q := query.NewOrganisationQueryRepo(db)
	target, err := q.FindByID(body.OrgID)
	if err != nil {
		fail(w, err)
		return
	}
	if user == caller && target.Status == entity.StatusSuspended {
		response.ErrorResponse(w, http.StatusConflict, "you cannot move yourself into a suspended organisation")
		return
	}
	if err := persistent.NewOrganisationPersistentRepo(db).Assign(user, target.ID); err != nil {
		fail(w, err)
		return
	}
	response.SuccessResponse(w, http.StatusOK, entity.Member{UserID: user, OrgID: target.ID})
}
