package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/a-digi/coco-logger/logger"
	"github.com/a-digi/coco-orm/orm"
	serverdi "github.com/a-digi/coco-server/server/di"
	"github.com/a-digi/coco-server/server/request"

	"github.com/a-digi/cinqo/src/download"
	"github.com/a-digi/cinqo/src/toolrepository/fetch"
)

type fakeDI struct{ items map[string]any }

func (f fakeDI) Get(key string) (any, bool)               { v, ok := f.items[key]; return v, ok }
func (f fakeDI) GetDatabaseManager() *orm.DatabaseManager { return nil }
func (f fakeDI) GetLogger() logger.Logger                 { return nil }

var _ serverdi.Context = fakeDI{}

type fakeRequest struct {
	request.RequestContext
	w   *httptest.ResponseRecorder
	di  serverdi.Context
	uri *request.URI
	r   *http.Request
}

func (f fakeRequest) GetWriter() http.ResponseWriter { return f.w }
func (f fakeRequest) GetDI() serverdi.Context        { return f.di }
func (f fakeRequest) GetURI() *request.URI           { return f.uri }
func (f fakeRequest) GetRequest() *http.Request      { return f.r }

// streamer serves a body that blocks after the first bytes until the job is cancelled or released.
type streamer struct{ release chan struct{} }

type gate struct {
	ctx     context.Context
	data    []byte
	pos     int
	release chan struct{}
}

func (g *gate) Read(p []byte) (int, error) {
	if g.pos < len(g.data)/2 {
		n := copy(p, g.data[g.pos:len(g.data)/2])
		g.pos += n
		return n, nil
	}
	select {
	case <-g.release:
		if g.pos >= len(g.data) {
			return 0, io.EOF
		}
		n := copy(p, g.data[g.pos:])
		g.pos += n
		return n, nil
	case <-g.ctx.Done():
		return 0, g.ctx.Err()
	}
}

func (s streamer) Stream(ctx context.Context, _ string, _ fetch.StreamOptions) (*fetch.Stream, error) {
	data := bytes.Repeat([]byte("x"), 2<<20)
	return &fetch.Stream{Status: 200, ContentLength: int64(len(data)), Header: http.Header{}, Body: io.NopCloser(&gate{ctx: ctx, data: data, release: s.release})}, nil
}

func startJob(t *testing.T, m *download.Manager, scope string) string {
	t.Helper()
	data := bytes.Repeat([]byte("x"), 2<<20)
	h := sha256.Sum256(data)
	id, err := m.Start(download.Spec{
		Label: "A model", URL: "https://models.example.com/o/r/resolve/78eb92a46fc37e6b524df991ed9aca9bc6aa7b80/m.gguf",
		Dest: filepath.Join(t.TempDir(), "m.gguf"), SizeBytes: int64(len(data)), SHA256: hex.EncodeToString(h[:]),
		AllowHost: func(host string) bool { return host == "models.example.com" }, Scope: scope,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func waitDownloading(t *testing.T, m *download.Manager, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if v, ok := m.Get(id); ok && v.State == download.StateDownloading && v.BytesDone > 0 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("the job did not start")
}

func with(scopes []string, err error) func() {
	old := scopesOf
	scopesOf = func(request.RequestContext) ([]string, error) { return scopes, err }
	return func() { scopesOf = old }
}

func call(h func(request.RequestContext), m *download.Manager, id string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	items := map[string]any{}
	if m != nil {
		items["download_manager"] = m
	}
	h(fakeRequest{w: rec, di: fakeDI{items: items}, uri: &request.URI{PathVariables: map[string]string{"id": id}}, r: httptest.NewRequest("GET", "/", nil)})
	return rec
}

type listBody struct {
	Message []download.Progress `json:"message"`
}

func TestAnUnauthenticatedCallerGetsNothing(t *testing.T) {
	defer with(nil, errNoToken)()
	m := download.New(download.Options{Fetcher: streamer{release: make(chan struct{})}})
	for name, h := range map[string]func(request.RequestContext){"list": ListHandler, "get": GetHandler, "cancel": CancelHandler} {
		if rec := call(h, m, "x"); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", name, rec.Code)
		}
	}
}

func TestACallerSeesOnlyTheJobsOfTheirScopes(t *testing.T) {
	rel := make(chan struct{})
	m := download.New(download.Options{Fetcher: streamer{release: rel}})
	mine := startJob(t, m, "cinqo:platform:manage")
	other := startJob(t, m, "cinqo:tool:manage")
	waitDownloading(t, m, mine)
	waitDownloading(t, m, other)
	defer func() { m.Cancel(mine); m.Cancel(other) }()

	defer with([]string{"cinqo:platform:manage"}, nil)()
	var body listBody
	rec := call(ListHandler, m, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != 200 {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	if len(body.Message) != 1 || body.Message[0].ID != mine || body.Message[0].State != download.StateDownloading || body.Message[0].BytesTotal != 2<<20 {
		t.Errorf("list = %+v, want only my job", body.Message)
	}
	if rec := call(GetHandler, m, mine); rec.Code != 200 {
		t.Errorf("my own job: status %d", rec.Code)
	}
	invisible := call(GetHandler, m, other)
	unknown := call(GetHandler, m, "no-such-id")
	if invisible.Code != 404 || unknown.Code != 404 || invisible.Body.String() != unknown.Body.String() {
		t.Errorf("a job of another scope must look exactly like an unknown one: %d %q vs %d %q", invisible.Code, invisible.Body.String(), unknown.Code, unknown.Body.String())
	}
}

func TestSuperAdminSeesEveryJob(t *testing.T) {
	m := download.New(download.Options{Fetcher: streamer{release: make(chan struct{})}})
	a := startJob(t, m, "cinqo:platform:manage")
	b := startJob(t, m, "cinqo:tool:manage")
	waitDownloading(t, m, a)
	waitDownloading(t, m, b)
	defer func() { m.Cancel(a); m.Cancel(b) }()
	defer with([]string{"cinqo:super:admin"}, nil)()
	var body listBody
	_ = json.Unmarshal(call(ListHandler, m, "").Body.Bytes(), &body)
	if len(body.Message) != 2 {
		t.Errorf("%d jobs, want 2", len(body.Message))
	}
}

func TestAnEmptyListIsAnEmptyArrayNotNull(t *testing.T) {
	defer with([]string{"cinqo:super:admin"}, nil)()
	rec := call(ListHandler, download.New(download.Options{}), "")
	if got := rec.Body.String(); !bytes.Contains([]byte(got), []byte(`"message":[]`)) {
		t.Errorf("body = %s", got)
	}
}

func TestCancelStopsAJobAndSaysSoWhenItIsAlreadyOver(t *testing.T) {
	m := download.New(download.Options{Fetcher: streamer{release: make(chan struct{})}})
	mine := startJob(t, m, "cinqo:platform:manage")
	other := startJob(t, m, "cinqo:tool:manage")
	waitDownloading(t, m, mine)
	waitDownloading(t, m, other)
	defer m.Cancel(other)
	defer with([]string{"cinqo:platform:manage"}, nil)()

	if rec := call(CancelHandler, m, other); rec.Code != 404 {
		t.Errorf("cancelling a job of another scope: status %d, want 404", rec.Code)
	}
	if v, _ := m.Get(other); v.State != download.StateDownloading {
		t.Error("a job of another scope must not be touched")
	}
	if rec := call(CancelHandler, m, mine); rec.Code != 204 || rec.Body.Len() != 0 {
		t.Errorf("cancel: status %d body %q, want 204 and nothing", rec.Code, rec.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if v, _ := m.Get(mine); v.State == download.StateCancelled {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if rec := call(CancelHandler, m, mine); rec.Code != 409 {
		t.Errorf("cancelling a finished job: status %d, want 409", rec.Code)
	}
	if rec := call(CancelHandler, m, "no-such-id"); rec.Code != 404 {
		t.Errorf("unknown job: status %d, want 404", rec.Code)
	}
}

func TestWithoutAManagerTheAnswerIsAServerError(t *testing.T) {
	defer with([]string{"cinqo:super:admin"}, nil)()
	for name, h := range map[string]func(request.RequestContext){"list": ListHandler, "get": GetHandler, "cancel": CancelHandler} {
		if rec := call(h, nil, "x"); rec.Code != 500 {
			t.Errorf("%s: status %d, want 500", name, rec.Code)
		}
	}
}

func TestTheTokenIsReadFromTheCookieOrTheBearerHeader(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "bearer abc")
	if got := tokenFromRequest(r); got != "abc" {
		t.Errorf("header token = %q", got)
	}
	r = httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: "access_token", Value: "cookie-token"})
	r.Header.Set("Authorization", "Bearer other")
	if got := tokenFromRequest(r); got != "cookie-token" {
		t.Errorf("cookie must win: %q", got)
	}
	for _, h := range []string{"", "Basic abc", "Bearer", "abc"} {
		r = httptest.NewRequest("GET", "/", nil)
		if h != "" {
			r.Header.Set("Authorization", h)
		}
		if got := tokenFromRequest(r); got != "" {
			t.Errorf("%q: token %q, want none", h, got)
		}
	}
	if _, err := callerScopes(fakeRequest{di: fakeDI{items: map[string]any{}}, r: httptest.NewRequest("GET", "/", nil)}); !errors.Is(err, errNoToken) {
		t.Errorf("no token: err = %v", err)
	}
}

// The three routes of config/routes/route-download.yaml, the handler names in routes.go and these handlers must agree.
func TestTheRouteFileAndTheRouterAgree(t *testing.T) {
	yaml, err := os.ReadFile("../../../config/routes/route-download.yaml")
	if err != nil {
		t.Fatal(err)
	}
	routes, err := os.ReadFile("../../../config/routes/routes.go")
	if err != nil {
		t.Fatal(err)
	}
	for executor, handlerFunc := range map[string]string{
		"DownloadList":   "download_handler.ListHandler",
		"DownloadGet":    "download_handler.GetHandler",
		"DownloadCancel": "download_handler.CancelHandler",
	} {
		if !bytes.Contains(yaml, []byte("executor: "+executor+"\n")) {
			t.Errorf("route file has no %s", executor)
		}
		if !bytes.Contains(routes, []byte(`"`+executor+`":`)) || !bytes.Contains(routes, []byte(handlerFunc)) {
			t.Errorf("routes.go does not register %s with %s", executor, handlerFunc)
		}
	}
	if bytes.Contains(yaml, []byte("scopes:\n      -")) {
		t.Error("these routes check the scope of each job inside the handler: a static scope here would hide jobs from the people who may see them")
	}
	if bytes.Count(yaml, []byte("security: authenticated")) != 3 || bytes.Contains(yaml, []byte("security: public")) {
		t.Error("every route must be authenticated")
	}
}
