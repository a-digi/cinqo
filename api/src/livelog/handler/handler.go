// Package handler serves the live log routes. It knows only the module's interfaces (the registry, Provider, Follower): it never learns
// what a file, a table or a buffer is. Both routes are authenticated with no static scope, because the scope depends on the provider
// being read, so each request checks the provider's own, deny by default (the same pattern as the file reader and the comments routes).
// See plan/ai/file-reader/live-follow.md.
package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/livelog"
)

const (
	maxRefBytes = 200
	// auditEvery is how long an "opened" line is held back for the same person and log, so following one log for an hour leaves one
	// line, not thousands.
	auditEvery = 5 * time.Minute
	maxAudited = 1000
)

type deps struct {
	reg  *livelog.Registry
	lim  livelog.Limits
	caps *livelog.FollowerCap
	logf func(format string, args ...any)
	now  func() time.Time

	mu      sync.Mutex
	audited map[string]time.Time
}

var (
	mu      sync.RWMutex
	current *deps
)

// Configure installs the registry the handlers use. Called once at boot by the composition root.
func Configure(reg *livelog.Registry, logf func(format string, args ...any)) {
	mu.Lock()
	current = newDeps(reg, livelog.DefaultLimits(), livelog.NewFollowerCap(0, 0), logf)
	mu.Unlock()
}

func newDeps(reg *livelog.Registry, lim livelog.Limits, caps *livelog.FollowerCap, logf func(format string, args ...any)) *deps {
	return &deps{reg: reg, lim: lim, caps: caps, logf: logf, now: time.Now, audited: map[string]time.Time{}}
}

func configured() *deps {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// audit writes "who opened what" (never any content), at most once per auditEvery for the same person, provider and reference.
func (d *deps) audit(who identity, providerID, ref string) {
	key := who.UserID + "\x00" + providerID + "\x00" + ref
	now := d.now()
	d.mu.Lock()
	if last, ok := d.audited[key]; ok && now.Sub(last) < auditEvery {
		d.mu.Unlock()
		return
	}
	if len(d.audited) >= maxAudited {
		for k, t := range d.audited {
			if now.Sub(t) >= auditEvery {
				delete(d.audited, k)
			}
		}
		if len(d.audited) >= maxAudited {
			d.audited = map[string]time.Time{}
		}
	}
	d.audited[key] = now
	d.mu.Unlock()
	d.logf("live-logs: user=%s followed provider=%s ref=%s", who.UserID, providerID, ref)
}

// fail answers a poll error with the status the design assigns it. Messages never carry log content or paths.
func (d *deps) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, context.Canceled), r.Context().Err() != nil:
		// the client went away: nobody is listening
	case errors.Is(err, livelog.ErrNotFound):
		response.ErrorResponse(w, http.StatusNotFound, "not found")
	case errors.Is(err, livelog.ErrBadCursor):
		response.ErrorResponse(w, http.StatusBadRequest, "cursor is not valid")
	case errors.Is(err, livelog.ErrNotText):
		response.ErrorResponse(w, http.StatusUnsupportedMediaType, "the log is not text")
	case errors.Is(err, livelog.ErrBusy):
		w.Header().Set("Retry-After", "5")
		response.ErrorResponse(w, http.StatusServiceUnavailable, "the log is busy, try again")
	case errors.Is(err, livelog.ErrTimeout):
		response.ErrorResponse(w, http.StatusGatewayTimeout, "the log took too long to read, try again")
	default:
		d.logf("live-logs: request failed: %v", err)
		response.ErrorResponse(w, http.StatusInternalServerError, "request failed")
	}
}

// positive reads an optional positive integer query parameter.
func positive(r *http.Request, name string) (int, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// ---- the operations, free of the request framework so they can be tested ----

// poll runs the checks in this order: the provider must exist, the caller must hold its scope, and only then is the reference opened, so
// a 403 or a 404 never says whether a log exists; then the follower cap, then the poll.
func (d *deps) poll(w http.ResponseWriter, r *http.Request, who identity, providerID, ref string) {
	p, found := d.reg.Get(providerID)
	if !found {
		response.ErrorResponse(w, http.StatusNotFound, "not found")
		return
	}
	if !livelog.Allowed(p, who.Scopes) {
		response.ErrorResponse(w, http.StatusForbidden, "missing required scope")
		return
	}
	if ref == "" || len(ref) > maxRefBytes {
		response.ErrorResponse(w, http.StatusBadRequest, "ref: required, at most 200 characters")
		return
	}
	lines, ok := positive(r, "lines")
	if !ok {
		response.ErrorResponse(w, http.StatusBadRequest, "lines must be a positive number")
		return
	}
	f, err := p.Open(ref)
	if err != nil {
		d.fail(w, r, err)
		return
	}
	if !d.caps.Acquire(who.UserID, providerID+"\x00"+ref, d.now()) {
		w.Header().Set("Retry-After", "15")
		response.ErrorResponse(w, http.StatusTooManyRequests, "too many logs are followed at once: close one and try again")
		return
	}
	batch, err := livelog.Poll(r.Context(), f, r.URL.Query().Get("cursor"), lines, d.lim)
	if err != nil {
		d.fail(w, r, err)
		return
	}
	d.audit(who, providerID, ref)
	response.SuccessResponse(w, http.StatusOK, batch)
}

// providers lists what the caller may follow, so the UI shows "Live" only where it works.
func (d *deps) providers(w http.ResponseWriter, who identity) {
	response.SuccessResponse(w, http.StatusOK, d.reg.List(who.Scopes))
}

// ---- the routes ----

func frame(reqCtx request.RequestContext) (d *deps, who identity, ok bool) {
	w, r := reqCtx.GetWriter(), reqCtx.GetRequest()
	if r.Method != http.MethodGet {
		response.ErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return nil, identity{}, false
	}
	d = configured()
	if d == nil {
		response.ErrorResponse(w, http.StatusServiceUnavailable, "live logs are not available")
		return nil, identity{}, false
	}
	who, err := callerFrom(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return nil, identity{}, false
	}
	return d, who, true
}

// PollHandler handles GET /api/v1/live-logs/{provider}/poll?ref=&cursor=&lines=.
func PollHandler(reqCtx request.RequestContext) {
	if d, who, ok := frame(reqCtx); ok {
		r := reqCtx.GetRequest()
		d.poll(reqCtx.GetWriter(), r, who, reqCtx.GetURI().GetPathVariable("provider"), r.URL.Query().Get("ref"))
	}
}

// ProvidersHandler handles GET /api/v1/live-logs/providers.
func ProvidersHandler(reqCtx request.RequestContext) {
	if d, who, ok := frame(reqCtx); ok {
		d.providers(reqCtx.GetWriter(), who)
	}
}
