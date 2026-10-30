// Package handler serves the comments engine's user-facing routes. All four are
// authenticated with no static scope: which scope applies depends on the owner's
// entity type, so the access resolver decides it here (the same pattern as media).
// See plan/ai/comments/comments-engine.md.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/comments"
	"github.com/a-digi/cinqo/src/comments/access"
)

// deps is what the handlers need: the per-owner stores and the permission rules.
type deps struct {
	stores   *comments.Manager
	resolver *access.Resolver
	logf     func(format string, args ...any)
}

var (
	mu      sync.RWMutex
	current *deps
)

// Configure installs the engine the handlers use. Called once at boot, the same
// convention as dashboard.SetDefault.
func Configure(stores *comments.Manager, resolver *access.Resolver, logf func(format string, args ...any)) {
	mu.Lock()
	current = &deps{stores: stores, resolver: resolver, logf: logf}
	mu.Unlock()
}

func configured() *deps {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// maxBodyBytes bounds a request body (a 4000-character comment is at most ~16 KB).
const maxBodyBytes = 64 * 1024

// commentResponse is a comment plus what the caller may do with it, so the screen
// never has to guess.
type commentResponse struct {
	comments.Comment
	CanEdit   bool `json:"canEdit"`
	CanDelete bool `json:"canDelete"`
}

type listResponse struct {
	Comments   []commentResponse `json:"comments"`
	Total      int               `json:"total"`
	NextBefore string            `json:"nextBefore"`
	// CanCreate says whether the caller may add a comment here, so the screen can
	// leave the box out for someone who may only read.
	CanCreate bool `json:"canCreate"`
}

// bodyRequest is what create and edit read. ParentID only means something to create
// (a reply's parent, empty or null for a top-level comment); an edit ignores it, since
// a comment can never move to another parent.
type bodyRequest struct {
	Body     string `json:"body"`
	ParentID string `json:"parentId"`
}

func (d *deps) present(t access.EntityType, who identity, c comments.Comment) commentResponse {
	return commentResponse{Comment: c, CanEdit: access.CanEdit(t, who.Caller, c), CanDelete: access.CanDelete(t, who.Caller, c)}
}

// fail answers an error with the status the design assigns it. Permission answers
// and "no such comment" carry no detail; a validation problem says which field.
func (d *deps) fail(w http.ResponseWriter, err error) {
	var v *comments.ValidationError
	switch {
	case errors.Is(err, access.ErrNotFound), errors.Is(err, comments.ErrNotFound), errors.Is(err, comments.ErrInvalidOwner):
		response.ErrorResponse(w, http.StatusNotFound, "not found")
	case errors.Is(err, access.ErrForbidden):
		response.ErrorResponse(w, http.StatusForbidden, "not allowed")
	case errors.As(err, &v):
		response.ErrorResponse(w, http.StatusBadRequest, v.Error())
	default:
		d.logf("comments: request failed: %v", err)
		response.ErrorResponse(w, http.StatusInternalServerError, "request failed")
	}
}

// ---- the four operations, free of the request framework so they can be tested ----

func (d *deps) list(w http.ResponseWriter, r *http.Request, who identity, owner, entityType, entityID string) {
	t, err := d.resolver.Authorize(who.Caller, owner, entityType, access.Read)
	if err != nil {
		d.fail(w, err)
		return
	}
	q := r.URL.Query()
	opts := comments.ListOptions{
		Before: q.Get("before"), AuthorID: q.Get("authorId"),
		CreatedFrom: q.Get("createdFrom"), CreatedTo: q.Get("createdTo"),
	}
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			response.ErrorResponse(w, http.StatusBadRequest, "limit: must be a positive number")
			return
		}
		opts.Limit = n
	}
	store, err := d.stores.Store(owner)
	if err != nil {
		d.fail(w, err)
		return
	}
	page, err := store.List(entityType, entityID, opts)
	if err != nil {
		d.fail(w, err)
		return
	}
	_, createErr := d.resolver.Authorize(who.Caller, owner, entityType, access.Create)
	out := listResponse{Comments: make([]commentResponse, 0, len(page.Comments)), Total: page.Total, NextBefore: page.NextBefore, CanCreate: createErr == nil}
	for _, c := range page.Comments {
		out.Comments = append(out.Comments, d.present(t, who, c))
	}
	response.SuccessResponse(w, http.StatusOK, out)
}

func (d *deps) create(w http.ResponseWriter, r *http.Request, who identity, owner, entityType, entityID string) {
	t, err := d.resolver.Authorize(who.Caller, owner, entityType, access.Create)
	if err != nil {
		d.fail(w, err)
		return
	}
	in, ok := readBody(w, r)
	if !ok {
		return
	}
	store, err := d.stores.Store(owner)
	if err != nil {
		d.fail(w, err)
		return
	}
	c, err := store.Create(comments.NewComment{
		EntityType: entityType, EntityID: entityID, Body: in.Body, ParentID: in.ParentID,
		// The author is always the token's, never anything in the request.
		AuthorID: who.UserID, AuthorName: who.Name,
	})
	if err != nil {
		d.fail(w, err)
		return
	}
	response.SuccessResponse(w, http.StatusCreated, d.present(t, who, c))
}

func (d *deps) edit(w http.ResponseWriter, r *http.Request, who identity, owner, entityType, entityID, id string) {
	t, err := d.resolver.Authorize(who.Caller, owner, entityType, access.Read)
	if err != nil {
		d.fail(w, err)
		return
	}
	in, ok := readBody(w, r)
	if !ok {
		return
	}
	store, err := d.stores.Store(owner)
	if err != nil {
		d.fail(w, err)
		return
	}
	existing, err := store.Get(entityType, entityID, id)
	if err != nil {
		d.fail(w, err)
		return
	}
	if !access.CanEdit(t, who.Caller, existing) {
		d.fail(w, access.ErrForbidden)
		return
	}
	updated, err := store.Update(entityType, entityID, id, in.Body)
	if err != nil {
		d.fail(w, err)
		return
	}
	response.SuccessResponse(w, http.StatusOK, d.present(t, who, updated))
}

func (d *deps) remove(w http.ResponseWriter, who identity, owner, entityType, entityID, id string) {
	t, err := d.resolver.Authorize(who.Caller, owner, entityType, access.Read)
	if err != nil {
		d.fail(w, err)
		return
	}
	store, err := d.stores.Store(owner)
	if err != nil {
		d.fail(w, err)
		return
	}
	existing, err := store.Get(entityType, entityID, id)
	if err != nil {
		d.fail(w, err)
		return
	}
	if !access.CanDelete(t, who.Caller, existing) {
		d.fail(w, access.ErrForbidden)
		return
	}
	if err := store.Delete(entityType, entityID, id); err != nil {
		d.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// readBody decodes {"body": "…", "parentId": "…"}; it answers 400 itself and returns
// false on a bad one.
func readBody(w http.ResponseWriter, r *http.Request) (bodyRequest, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in bodyRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, `body must be {"body": "…"}`)
		return bodyRequest{}, false
	}
	return in, true
}

// ---- the routes ----

// frame does what every route needs before its own work: method check, the engine
// being configured, and a validated caller.
func frame(reqCtx request.RequestContext, method string) (d *deps, who identity, ok bool) {
	w, r := reqCtx.GetWriter(), reqCtx.GetRequest()
	if r.Method != method {
		response.ErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return nil, identity{}, false
	}
	d = configured()
	if d == nil {
		response.ErrorResponse(w, http.StatusServiceUnavailable, "comments are not available")
		return nil, identity{}, false
	}
	who, err := callerFrom(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return nil, identity{}, false
	}
	return d, who, true
}

func vars(reqCtx request.RequestContext) (owner, entityType, entityID, commentID string) {
	u := reqCtx.GetURI()
	return u.GetPathVariable("owner"), u.GetPathVariable("entityType"), u.GetPathVariable("entityId"), u.GetPathVariable("commentId")
}

// ListHandler handles GET /api/v1/comments/{owner}/{entityType}/{entityId}.
func ListHandler(reqCtx request.RequestContext) {
	if d, who, ok := frame(reqCtx, http.MethodGet); ok {
		owner, typ, id, _ := vars(reqCtx)
		d.list(reqCtx.GetWriter(), reqCtx.GetRequest(), who, owner, typ, id)
	}
}

// CreateHandler handles POST /api/v1/comments/{owner}/{entityType}/{entityId}.
func CreateHandler(reqCtx request.RequestContext) {
	if d, who, ok := frame(reqCtx, http.MethodPost); ok {
		owner, typ, id, _ := vars(reqCtx)
		d.create(reqCtx.GetWriter(), reqCtx.GetRequest(), who, owner, typ, id)
	}
}

// EditHandler handles PATCH /api/v1/comments/{owner}/{entityType}/{entityId}/{commentId}.
func EditHandler(reqCtx request.RequestContext) {
	if d, who, ok := frame(reqCtx, http.MethodPatch); ok {
		owner, typ, id, commentID := vars(reqCtx)
		d.edit(reqCtx.GetWriter(), reqCtx.GetRequest(), who, owner, typ, id, commentID)
	}
}

// DeleteHandler handles DELETE /api/v1/comments/{owner}/{entityType}/{entityId}/{commentId}.
func DeleteHandler(reqCtx request.RequestContext) {
	if d, who, ok := frame(reqCtx, http.MethodDelete); ok {
		owner, typ, id, commentID := vars(reqCtx)
		d.remove(reqCtx.GetWriter(), who, owner, typ, id, commentID)
	}
}
