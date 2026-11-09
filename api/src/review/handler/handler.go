// Package handler serves the review engine's user-facing routes. All are authenticated with no static scope: which scope applies, and
// which scale the stars use, depends on the owner's entity type, so the access resolver decides it here (the same pattern as comments
// and media). The request never carries a scale, a half-star flag or an author: they come from the type's declaration and the token.
// See plan/ai/review/review-engine.md.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/review"
	"github.com/a-digi/cinqo/src/review/access"
)

// deps is what the handlers need: the per-owner stores and the permission rules.
type deps struct {
	stores   *review.Manager
	resolver *access.Resolver
	logf     func(format string, args ...any)
}

var (
	mu      sync.RWMutex
	current *deps
)

// Configure installs the engine the handlers use. Called once at boot, the same convention as the comments engine.
func Configure(stores *review.Manager, resolver *access.Resolver, logf func(format string, args ...any)) {
	mu.Lock()
	current = &deps{stores: stores, resolver: resolver, logf: logf}
	mu.Unlock()
}

func configured() *deps {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// maxBodyBytes bounds a request body (a 2000-character text is at most ~8 KB; 100 ids about 4 KB).
const maxBodyBytes = 64 * 1024

// reviewResponse is a review plus what the caller may do with it, so the screen never has to guess.
type reviewResponse struct {
	review.Review
	CanEdit   bool `json:"canEdit"`
	CanDelete bool `json:"canDelete"`
}

type listResponse struct {
	// Scale and HalfStars are the type's: how many stars there are and whether a rating may end in .5.
	Scale     int            `json:"scale"`
	HalfStars bool           `json:"halfStars"`
	Summary   review.Summary `json:"summary"`
	// Mine is the caller's own review of this entity, whichever page it would be on; null when they have none.
	Mine       *reviewResponse  `json:"mine"`
	Reviews    []reviewResponse `json:"reviews"`
	Total      int              `json:"total"`
	NextBefore string           `json:"nextBefore"`
	// CanCreate says whether the caller may give a review here, so the screen can show the stars as read-only for someone who may
	// only read.
	CanCreate bool `json:"canCreate"`
}

type summariesResponse struct {
	Scale     int                     `json:"scale"`
	HalfStars bool                    `json:"halfStars"`
	Summaries map[string]review.Brief `json:"summaries"`
}

// putRequest is what a put reads. Rating is a pointer so a missing one is told from 0. Anything else in the body (a scale, an
// author, a half-star flag) is ignored: those come from the type and the token.
type putRequest struct {
	Rating *float64 `json:"rating"`
	Body   string   `json:"body"`
}

type summariesRequest struct {
	IDs []string `json:"ids"`
}

func (d *deps) present(t access.EntityType, who identity, r review.Review) reviewResponse {
	return reviewResponse{Review: r, CanEdit: access.CanEdit(t, who.Caller, r), CanDelete: access.CanDelete(t, who.Caller, r)}
}

// fail answers an error with the status the design assigns it. Permission answers and "no such review" carry no detail; a validation
// problem says which field.
func (d *deps) fail(w http.ResponseWriter, err error) {
	var v *review.ValidationError
	switch {
	case errors.Is(err, access.ErrNotFound), errors.Is(err, review.ErrNotFound), errors.Is(err, review.ErrInvalidOwner):
		response.ErrorResponse(w, http.StatusNotFound, "not found")
	case errors.Is(err, access.ErrForbidden):
		response.ErrorResponse(w, http.StatusForbidden, "not allowed")
	case errors.As(err, &v):
		response.ErrorResponse(w, http.StatusBadRequest, v.Error())
	default:
		d.logf("reviews: request failed: %v", err)
		response.ErrorResponse(w, http.StatusInternalServerError, "request failed")
	}
}

// ---- the operations, free of the request framework so they can be tested ----

func (d *deps) get(w http.ResponseWriter, r *http.Request, who identity, owner, entityType, entityID string) {
	t, err := d.resolver.Authorize(who.Caller, owner, entityType, access.Read)
	if err != nil {
		d.fail(w, err)
		return
	}
	q := r.URL.Query()
	opts := review.ListOptions{Before: q.Get("before")}
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
	sum, err := store.Summary(entityType, entityID, t.Scale)
	if err != nil {
		d.fail(w, err)
		return
	}
	_, writeErr := d.resolver.Authorize(who.Caller, owner, entityType, access.Write)
	out := listResponse{
		Scale: t.Scale, HalfStars: t.HalfStars, Summary: sum,
		Reviews: make([]reviewResponse, 0, len(page.Reviews)), Total: page.Total, NextBefore: page.NextBefore, CanCreate: writeErr == nil,
	}
	mine, err := store.GetByAuthor(entityType, entityID, who.UserID)
	switch {
	case err == nil:
		m := d.present(t, who, mine)
		out.Mine = &m
	case errors.Is(err, review.ErrNotFound):
	default:
		d.fail(w, err)
		return
	}
	for _, rv := range page.Reviews {
		out.Reviews = append(out.Reviews, d.present(t, who, rv))
	}
	response.SuccessResponse(w, http.StatusOK, out)
}

func (d *deps) put(w http.ResponseWriter, r *http.Request, who identity, owner, entityType, entityID string) {
	t, err := d.resolver.Authorize(who.Caller, owner, entityType, access.Write)
	if err != nil {
		d.fail(w, err)
		return
	}
	in, ok := readPut(w, r)
	if !ok {
		return
	}
	store, err := d.stores.Store(owner)
	if err != nil {
		d.fail(w, err)
		return
	}
	saved, err := store.Put(review.PutReview{
		EntityType: entityType, EntityID: entityID, Body: in.Body, Rating: *in.Rating,
		// The scale and the half stars are the type's, the author is the token's: never anything in the request.
		Scale: t.Scale, HalfStars: t.HalfStars, AuthorID: who.UserID, AuthorName: who.Name,
	})
	if err != nil {
		d.fail(w, err)
		return
	}
	response.SuccessResponse(w, http.StatusOK, d.present(t, who, saved))
}

// remove deletes the caller's own review (idempotent: 204 whether or not there was one), or, with ?reviewId=, one review by id: the
// author's own or, for an administrator, anyone's.
func (d *deps) remove(w http.ResponseWriter, r *http.Request, who identity, owner, entityType, entityID string) {
	reviewID := r.URL.Query().Get("reviewId")
	action := access.Write
	if reviewID != "" {
		action = access.Read // the permission is decided on the review itself below
	}
	t, err := d.resolver.Authorize(who.Caller, owner, entityType, action)
	if err != nil {
		d.fail(w, err)
		return
	}
	store, err := d.stores.Store(owner)
	if err != nil {
		d.fail(w, err)
		return
	}
	if reviewID == "" {
		if _, err := store.DeleteMine(entityType, entityID, who.UserID); err != nil {
			d.fail(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	existing, err := store.Get(entityType, entityID, reviewID)
	if err != nil {
		d.fail(w, err)
		return
	}
	if !access.CanDelete(t, who.Caller, existing) {
		d.fail(w, access.ErrForbidden)
		return
	}
	if err := store.Delete(entityType, entityID, reviewID); err != nil {
		d.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d *deps) summaries(w http.ResponseWriter, r *http.Request, who identity, owner, entityType string) {
	t, err := d.resolver.Authorize(who.Caller, owner, entityType, access.Read)
	if err != nil {
		d.fail(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in summariesRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, `body must be {"ids": ["<uuid>", …]}`)
		return
	}
	store, err := d.stores.Store(owner)
	if err != nil {
		d.fail(w, err)
		return
	}
	briefs, err := store.Briefs(entityType, in.IDs, t.Scale)
	if err != nil {
		d.fail(w, err)
		return
	}
	response.SuccessResponse(w, http.StatusOK, summariesResponse{Scale: t.Scale, HalfStars: t.HalfStars, Summaries: briefs})
}

// readPut decodes {"rating": 3.5, "body": "…"}; it answers 400 itself and returns false on a bad one.
func readPut(w http.ResponseWriter, r *http.Request) (putRequest, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in putRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, `body must be {"rating": 3.5, "body": "…"}`)
		return putRequest{}, false
	}
	if in.Rating == nil {
		response.ErrorResponse(w, http.StatusBadRequest, "rating: is required")
		return putRequest{}, false
	}
	return in, true
}

// ---- the routes ----

// frame does what every route needs before its own work: method check, the engine being configured, and a validated caller.
func frame(reqCtx request.RequestContext, method string) (d *deps, who identity, ok bool) {
	w, r := reqCtx.GetWriter(), reqCtx.GetRequest()
	if r.Method != method {
		response.ErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return nil, identity{}, false
	}
	d = configured()
	if d == nil {
		response.ErrorResponse(w, http.StatusServiceUnavailable, "reviews are not available")
		return nil, identity{}, false
	}
	who, err := callerFrom(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return nil, identity{}, false
	}
	return d, who, true
}

func vars(reqCtx request.RequestContext) (owner, entityType, entityID string) {
	u := reqCtx.GetURI()
	return u.GetPathVariable("owner"), u.GetPathVariable("entityType"), u.GetPathVariable("entityId")
}

// GetHandler handles GET /api/v1/reviews/{owner}/{entityType}/{entityId}.
func GetHandler(reqCtx request.RequestContext) {
	if d, who, ok := frame(reqCtx, http.MethodGet); ok {
		owner, typ, id := vars(reqCtx)
		d.get(reqCtx.GetWriter(), reqCtx.GetRequest(), who, owner, typ, id)
	}
}

// PutHandler handles PUT /api/v1/reviews/{owner}/{entityType}/{entityId}.
func PutHandler(reqCtx request.RequestContext) {
	if d, who, ok := frame(reqCtx, http.MethodPut); ok {
		owner, typ, id := vars(reqCtx)
		d.put(reqCtx.GetWriter(), reqCtx.GetRequest(), who, owner, typ, id)
	}
}

// DeleteHandler handles DELETE /api/v1/reviews/{owner}/{entityType}/{entityId}.
func DeleteHandler(reqCtx request.RequestContext) {
	if d, who, ok := frame(reqCtx, http.MethodDelete); ok {
		owner, typ, id := vars(reqCtx)
		d.remove(reqCtx.GetWriter(), reqCtx.GetRequest(), who, owner, typ, id)
	}
}

// SummariesHandler handles POST /api/v1/review-summaries/{owner}/{entityType}.
func SummariesHandler(reqCtx request.RequestContext) {
	if d, who, ok := frame(reqCtx, http.MethodPost); ok {
		owner, typ, _ := vars(reqCtx)
		d.summaries(reqCtx.GetWriter(), reqCtx.GetRequest(), who, owner, typ)
	}
}
