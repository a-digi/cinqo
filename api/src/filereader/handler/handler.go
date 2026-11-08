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
	"github.com/a-digi/cinqo/src/filereader/jsonview"
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

// The views of a file a request may ask for (the "view" query parameter): the file as it is, or (plan 202) its JSON beautified.
const (
	viewRaw  = "raw"
	viewJSON = "json"
)

type deps struct {
	engine  *filereader.Engine
	sources *source.Registry
	logf    func(format string, args ...any)
	now     func() time.Time
	// jsonView makes the indented copies behind view=json; nil when the boot could not set it up (view=json then answers 503).
	jsonView *jsonview.Cache

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

// SetJSONView installs the cache behind view=json (plan 202). Called once at boot, after Configure; without it view=json is unavailable
// and everything else works as before.
func SetJSONView(cache *jsonview.Cache) {
	mu.Lock()
	if current != nil {
		current.jsonView = cache
	}
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
func (d *deps) audit(who identity, sourceID, ref, view string) {
	key := who.UserID + "\x00" + sourceID + "\x00" + ref + "\x00" + view
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
	if view == viewJSON {
		d.logf("file-reader: user=%s opened source=%s ref=%s view=%s", who.UserID, sourceID, ref, view)
		return
	}
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

// target is open plus the view: the path the engine reads. For the raw view that is the file; for view=json it is the indented copy of the
// file, made (or found) only AFTER the source's scope and the reference have passed, so the cache is never a way around either. The
// view parameter is checked here, after the scope, like every other parameter.
func (d *deps) target(w http.ResponseWriter, r *http.Request, who identity, sourceID, ref string) (path, view string, derived jsonview.Derived, ok bool) {
	path, ok = d.open(w, who, sourceID, ref)
	if !ok {
		return "", "", jsonview.Derived{}, false
	}
	switch v := r.URL.Query().Get("view"); v {
	case "", viewRaw:
		return path, viewRaw, jsonview.Derived{}, true
	case viewJSON:
		view = viewJSON
	default:
		response.ErrorResponse(w, http.StatusBadRequest, "view must be raw or json")
		return "", "", jsonview.Derived{}, false
	}
	if d.jsonView == nil {
		response.ErrorResponse(w, http.StatusServiceUnavailable, "JSON formatting is not available")
		return "", "", jsonview.Derived{}, false
	}
	derived, err := d.jsonView.Ensure(r.Context(), sourceID+"|"+ref, path)
	if err != nil {
		d.fail(w, r, err)
		return "", "", jsonview.Derived{}, false
	}
	return derived.Path, view, derived, true
}

// fail answers an engine error with the status the design assigns it. Messages never carry file content or
// paths.
func (d *deps) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, context.Canceled), r.Context().Err() != nil:
		// the client went away: nobody is listening
	case errors.Is(err, filereader.ErrSlow):
		response.ErrorResponse(w, http.StatusGatewayTimeout, "the file took too long to read, try again")
	case errors.Is(err, filereader.ErrBadQuery):
		response.ErrorResponse(w, http.StatusBadRequest, "q must be 1 to 200 characters of plain text on one line")
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
	case errors.Is(err, filereader.ErrNotRegular), errors.Is(err, os.ErrNotExist), errors.Is(err, jsonview.ErrNotRegular):
		response.ErrorResponse(w, http.StatusNotFound, "not found")
	case errors.Is(err, jsonview.ErrTooLarge):
		response.ErrorResponse(w, http.StatusRequestEntityTooLarge, "file is too large to format as JSON")
	case errors.Is(err, jsonview.ErrTooDeep):
		response.ErrorResponse(w, http.StatusUnprocessableEntity, "the JSON is nested too deeply to format")
	case errors.Is(err, jsonview.ErrBusy):
		response.ErrorResponse(w, http.StatusServiceUnavailable, "too many files are being formatted, try again")
	case errors.Is(err, jsonview.ErrSlow):
		response.ErrorResponse(w, http.StatusGatewayTimeout, "formatting the file took too long, try again")
	case errors.Is(err, jsonview.ErrChanged):
		// Not 409: the reader's clients read a 409 as "the index has not got that far yet" and wait for it. This one is a plain try-again.
		response.ErrorResponse(w, http.StatusServiceUnavailable, "the file changed while it was formatted, try again")
	case isSyntaxError(err):
		// The message is built from fixed phrases and the byte number, never from the file's own text.
		response.ErrorResponse(w, http.StatusUnprocessableEntity, err.Error())
	default:
		d.logf("file-reader: request failed: %v", err)
		response.ErrorResponse(w, http.StatusInternalServerError, "request failed")
	}
}

func isSyntaxError(err error) bool {
	var se *jsonview.SyntaxError
	return errors.As(err, &se)
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

// nonNegative reads an optional integer query parameter that may be zero (default 0).
func nonNegative(r *http.Request, name string) (int64, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, true
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// ---- the operations, free of the request framework so they can be tested ----

// infoView is the answer of info for a derived view: the file's own figures, which view it is, and the size of the original.
type infoView struct {
	filereader.Info
	View        string `json:"view"`
	SourceBytes int64  `json:"sourceBytes"`
}

func (d *deps) info(w http.ResponseWriter, r *http.Request, who identity, sourceID, ref string) {
	path, view, derived, ok := d.target(w, r, who, sourceID, ref)
	if !ok {
		return
	}
	info, err := d.engine.Info(r.Context(), path)
	if err != nil {
		d.fail(w, r, err)
		return
	}
	d.audit(who, sourceID, ref, view)
	if view == viewJSON {
		response.SuccessResponse(w, http.StatusOK, infoView{Info: info, View: viewJSON, SourceBytes: derived.SourceBytes})
		return
	}
	response.SuccessResponse(w, http.StatusOK, info)
}

func (d *deps) lines(w http.ResponseWriter, r *http.Request, who identity, sourceID, ref string) {
	path, view, _, ok := d.target(w, r, who, sourceID, ref)
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
	d.audit(who, sourceID, ref, view)
	response.SuccessResponse(w, http.StatusOK, win)
}

func (d *deps) search(w http.ResponseWriter, r *http.Request, who identity, sourceID, ref string) {
	path, view, _, ok := d.target(w, r, who, sourceID, ref)
	if !ok {
		return
	}
	q := r.URL.Query().Get("q")
	from, okFrom := positive(r, "from", 1)
	limit, okLimit := positive(r, "limit", 50)
	offset, okOffset := nonNegative(r, "offset")
	if !okFrom || !okLimit || !okOffset {
		response.ErrorResponse(w, http.StatusBadRequest, "from and limit must be positive numbers, offset zero or more")
		return
	}
	if limit > 1<<20 {
		limit = 1 << 20 // the engine clamps to its own maximum
	}
	res, err := d.engine.Search(r.Context(), path, q, from, offset, int(limit), r.URL.Query().Get("case") == "1")
	if err != nil {
		d.fail(w, r, err)
		return
	}
	d.audit(who, sourceID, ref, view)
	response.SuccessResponse(w, http.StatusOK, res)
}

func (d *deps) segment(w http.ResponseWriter, r *http.Request, who identity, sourceID, ref string) {
	path, view, _, ok := d.target(w, r, who, sourceID, ref)
	if !ok {
		return
	}
	line, okLine := positive(r, "line", 0)
	offset, okOffset := nonNegative(r, "offset")
	length, okLength := positive(r, "length", 16<<10)
	if !okLine || line < 1 || !okOffset || !okLength {
		response.ErrorResponse(w, http.StatusBadRequest, "line and length must be positive numbers, offset zero or more")
		return
	}
	if length > 1<<20 {
		length = 1 << 20 // the engine clamps to its own maximum
	}
	seg, err := d.engine.ReadSegment(r.Context(), path, line, offset, int(length))
	if err != nil {
		d.fail(w, r, err)
		return
	}
	d.audit(who, sourceID, ref, view)
	response.SuccessResponse(w, http.StatusOK, seg)
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

// SegmentHandler handles GET /api/v1/file-reader/{source}/segment?ref=&line=&offset=&length=.
func SegmentHandler(reqCtx request.RequestContext) {
	if d, who, ok := frame(reqCtx); ok {
		r := reqCtx.GetRequest()
		d.segment(reqCtx.GetWriter(), r, who, reqCtx.GetURI().GetPathVariable("source"), r.URL.Query().Get("ref"))
	}
}
