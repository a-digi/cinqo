// Package handler serves the file reader's routes. Both are authenticated with no static scope: which
// scope applies depends on the source being read, so each request checks the source's own (the same
// pattern as the tools proxy and the comments routes). See plan/ai/file-reader/file-reader-engine.md.
package handler

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/filereader"
	"github.com/a-digi/cinqo/src/filereader/source"
)

const (
	maxRefBytes  = 200
	defaultCount = 200
	// auditEvery is how long an "opened" line is held back for the same person and file, so paging
	// through one file leaves one line, not hundreds.
	auditEvery = 5 * time.Minute
	maxAudited = 1000
)

type deps struct {
	engine  *filereader.Engine
	sources *source.Registry
	logf    func(format string, args ...any)
	now     func() time.Time

	mu      sync.Mutex
	audited map[string]time.Time
}

var (
	mu      sync.RWMutex
	current *deps
)

// Configure installs the engine and the sources the handlers use. Called once at boot.
func Configure(engine *filereader.Engine, sources *source.Registry, logf func(format string, args ...any)) {
	mu.Lock()
	current = newDeps(engine, sources, logf)
	mu.Unlock()
}

func newDeps(engine *filereader.Engine, sources *source.Registry, logf func(format string, args ...any)) *deps {
	return &deps{engine: engine, sources: sources, logf: logf, now: time.Now, audited: map[string]time.Time{}}
}

func configured() *deps {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// audit writes "who opened what" (never any content), at most once per auditEvery for the same
// person, source and file.
func (d *deps) audit(who identity, sourceID, ref string) {
	key := who.UserID + "\x00" + sourceID + "\x00" + ref
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
	d.logf("file-reader: user=%s opened source=%s ref=%s", who.UserID, sourceID, ref)
}

// open runs the checks every request shares, in this order: the source must exist, the caller must hold
// its scope, and only then is the reference resolved, so a 403 or a 404 never says whether a file exists.
func (d *deps) open(w http.ResponseWriter, who identity, sourceID, ref string) (path string, ok bool) {
	src, found := d.sources.Get(sourceID)
	if !found {
		response.ErrorResponse(w, http.StatusNotFound, "not found")
		return "", false
	}
	if !source.Allowed(src, who.Scopes) {
		response.ErrorResponse(w, http.StatusForbidden, "missing required scope")
		return "", false
	}
	if ref == "" || len(ref) > maxRefBytes {
		response.ErrorResponse(w, http.StatusBadRequest, "ref: required, at most 200 characters")
		return "", false
	}
	path, err := src.Resolve(ref)
	if err != nil {
		response.ErrorResponse(w, http.StatusNotFound, "not found")
		return "", false
	}
	return path, true
}

// fail answers an engine error with the status the design assigns it. Messages never carry file content or
// paths.
func (d *deps) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, context.Canceled), r.Context().Err() != nil:
		// the client went away: nobody is listening
	case errors.Is(err, filereader.ErrBadQuery):
		response.ErrorResponse(w, http.StatusBadRequest, "q must be 1 to 200 characters of plain text")
	case errors.Is(err, filereader.ErrBadRange):
		response.ErrorResponse(w, http.StatusBadRequest, "from and count must be positive numbers")
	case errors.Is(err, filereader.ErrNotIndexed):
		response.ErrorResponse(w, http.StatusConflict, "that line is not indexed yet")
	case errors.Is(err, filereader.ErrBusy):
		response.ErrorResponse(w, http.StatusServiceUnavailable, "the file reader is busy, try again")
	case errors.Is(err, filereader.ErrTooLarge):
		response.ErrorResponse(w, http.StatusRequestEntityTooLarge, "file is too large to read here")
	case errors.Is(err, filereader.ErrBinary):
		response.ErrorResponse(w, http.StatusUnsupportedMediaType, "file is not text")
	case errors.Is(err, filereader.ErrNotRegular), errors.Is(err, os.ErrNotExist):
		response.ErrorResponse(w, http.StatusNotFound, "not found")
	default:
		d.logf("file-reader: request failed: %v", err)
		response.ErrorResponse(w, http.StatusInternalServerError, "request failed")
	}
}

// positive reads an optional positive integer query parameter.
func positive(r *http.Request, name string, def int64) (int64, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def, true
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// ---- the operations, free of the request framework so they can be tested ----

func (d *deps) info(w http.ResponseWriter, r *http.Request, who identity, sourceID, ref string) {
	path, ok := d.open(w, who, sourceID, ref)
	if !ok {
		return
	}
	info, err := d.engine.Info(r.Context(), path)
	if err != nil {
		d.fail(w, r, err)
		return
	}
	d.audit(who, sourceID, ref)
	response.SuccessResponse(w, http.StatusOK, info)
}

func (d *deps) lines(w http.ResponseWriter, r *http.Request, who identity, sourceID, ref string) {
	path, ok := d.open(w, who, sourceID, ref)
	if !ok {
		return
	}
	from, okFrom := positive(r, "from", 1)
	count, okCount := positive(r, "count", defaultCount)
	if !okFrom || !okCount {
		response.ErrorResponse(w, http.StatusBadRequest, "from and count must be positive numbers")
		return
	}
	if count > 1<<20 {
		count = 1 << 20 // the engine clamps to its own maximum; this only keeps the int in range
	}
	win, err := d.engine.ReadLines(r.Context(), path, from, int(count))
	if err != nil {
		d.fail(w, r, err)
		return
	}
	d.audit(who, sourceID, ref)
	response.SuccessResponse(w, http.StatusOK, win)
}

func (d *deps) search(w http.ResponseWriter, r *http.Request, who identity, sourceID, ref string) {
	path, ok := d.open(w, who, sourceID, ref)
	if !ok {
		return
	}
	q := r.URL.Query().Get("q")
	from, okFrom := positive(r, "from", 1)
	limit, okLimit := positive(r, "limit", 50)
	if !okFrom || !okLimit {
		response.ErrorResponse(w, http.StatusBadRequest, "from and limit must be positive numbers")
		return
	}
	if limit > 1<<20 {
		limit = 1 << 20 // the engine clamps to its own maximum
	}
	res, err := d.engine.Search(r.Context(), path, q, from, int(limit), r.URL.Query().Get("case") == "1")
	if err != nil {
		d.fail(w, r, err)
		return
	}
	d.audit(who, sourceID, ref)
	response.SuccessResponse(w, http.StatusOK, res)
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
		response.ErrorResponse(w, http.StatusServiceUnavailable, "the file reader is not available")
		return nil, identity{}, false
	}
	who, err := callerFrom(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return nil, identity{}, false
	}
	return d, who, true
}

// InfoHandler handles GET /api/v1/file-reader/{source}/info?ref=.
func InfoHandler(reqCtx request.RequestContext) {
	if d, who, ok := frame(reqCtx); ok {
		r := reqCtx.GetRequest()
		d.info(reqCtx.GetWriter(), r, who, reqCtx.GetURI().GetPathVariable("source"), r.URL.Query().Get("ref"))
	}
}

// LinesHandler handles GET /api/v1/file-reader/{source}/lines?ref=&from=&count=.
func LinesHandler(reqCtx request.RequestContext) {
	if d, who, ok := frame(reqCtx); ok {
		r := reqCtx.GetRequest()
		d.lines(reqCtx.GetWriter(), r, who, reqCtx.GetURI().GetPathVariable("source"), r.URL.Query().Get("ref"))
	}
}

// SearchHandler handles GET /api/v1/file-reader/{source}/search?ref=&q=&from=&limit=&case=.
func SearchHandler(reqCtx request.RequestContext) {
	if d, who, ok := frame(reqCtx); ok {
		r := reqCtx.GetRequest()
		d.search(reqCtx.GetWriter(), r, who, reqCtx.GetURI().GetPathVariable("source"), r.URL.Query().Get("ref"))
	}
}
