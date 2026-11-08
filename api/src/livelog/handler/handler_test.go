package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/livelog"
	"github.com/a-digi/cinqo/src/livelog/livelogtest"
)

var (
	admin = identity{UserID: "root", Scopes: []string{livelog.AdminScope}}
	ada   = identity{UserID: "ada", Scopes: []string{"cinqo:conversation:use"}}
	tool  = identity{UserID: "tom", Scopes: []string{"tool:x:read"}}
)

type env struct {
	d     *deps
	trace *livelogtest.Provider
	log   *livelogtest.Log
	audit []string
	now   time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{now: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)}
	e.log = livelogtest.NewLog("one", "two", "three")
	e.trace = &livelogtest.Provider{IDValue: "ai-trace", TitleValue: "AI trace logs", ScopeList: []string{livelog.AdminScope}, Logs: map[string]*livelogtest.Log{"conv/turn": e.log}}
	other := &livelogtest.Provider{IDValue: "tool-log", TitleValue: "Tool log", ScopeList: []string{"tool:x:read"}, Logs: map[string]*livelogtest.Log{"a": livelogtest.NewLog("x")}}
	reg := livelog.NewRegistry()
	for _, p := range []livelog.Provider{e.trace, other} {
		if err := reg.Register(p); err != nil {
			t.Fatal(err)
		}
	}
	lim := livelog.DefaultLimits()
	lim.PollTimeout = 200 * time.Millisecond
	e.d = newDeps(reg, lim, livelog.NewFollowerCap(3, 15*time.Second), func(f string, a ...any) { e.audit = append(e.audit, fmt.Sprintf(f, a...)) })
	e.d.now = func() time.Time { return e.now }
	return e
}

type result struct {
	code   int
	header http.Header
	out    map[string]any
	raw    string
}

func (r result) message() map[string]any { m, _ := r.out["message"].(map[string]any); return m }

func (e *env) poll(who identity, provider, ref, query string) result {
	rec := httptest.NewRecorder()
	e.d.poll(rec, httptest.NewRequest(http.MethodGet, "/x?"+query, nil), who, provider, ref)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return result{code: rec.Code, header: rec.Header(), out: out, raw: rec.Body.String()}
}

func texts(r result) []string {
	out := []string{}
	for _, l := range r.message()["lines"].([]any) {
		out = append(out, l.(map[string]any)["text"].(string))
	}
	return out
}

func TestAdminFollowsALogFromTheEndAndThenOnlyWhatIsNew(t *testing.T) {
	e := newEnv(t)
	first := e.poll(admin, "ai-trace", "conv/turn", "lines=2")
	if first.code != 200 || fmt.Sprint(texts(first)) != "[two three]" || first.message()["eof"] != true || first.out["success"] != true {
		t.Fatalf("first poll: %d %s", first.code, first.raw)
	}
	cur := first.message()["cursor"].(string)
	e.log.Append("four", "five")
	next := e.poll(admin, "ai-trace", "conv/turn", "cursor="+cur)
	if fmt.Sprint(texts(next)) != "[four five]" || next.message()["reset"] != false {
		t.Fatalf("next poll: %s", next.raw)
	}
	quiet := e.poll(admin, "ai-trace", "conv/turn", "cursor="+next.message()["cursor"].(string))
	if len(texts(quiet)) != 0 || quiet.message()["lines"] == nil {
		t.Fatalf("a quiet poll has an empty list, never null: %s", quiet.raw)
	}
}

func TestTheScopeIsCheckedBeforeAnyReferenceIsOpenedSoNothingIsAnOracle(t *testing.T) {
	e := newEnv(t)
	real := e.poll(ada, "ai-trace", "conv/turn", "")
	fake := e.poll(ada, "ai-trace", "conv/missing", "")
	garbage := e.poll(ada, "ai-trace", "../../etc/passwd", "")
	empty := e.poll(ada, "ai-trace", "", "")
	for name, r := range map[string]result{"real": real, "fake": fake, "garbage": garbage, "empty": empty} {
		if r.code != http.StatusForbidden || r.raw != real.raw {
			t.Errorf("%s: %d %s (a caller without the scope must get the same 403 whatever the reference)", name, r.code, r.raw)
		}
	}
	if len(e.audit) != 0 {
		t.Errorf("a refused caller left an audit line: %v", e.audit)
	}
	// a scope for another provider does not open this one
	if r := e.poll(tool, "ai-trace", "conv/turn", ""); r.code != http.StatusForbidden {
		t.Errorf("tool scope on ai-trace: %d", r.code)
	}
	if r := e.poll(tool, "tool-log", "a", ""); r.code != 200 {
		t.Errorf("its own scope: %d %s", r.code, r.raw)
	}
}

func TestAnUnknownProviderIsNotFound(t *testing.T) {
	e := newEnv(t)
	for _, id := range []string{"nope", "", "AI-TRACE", "ai-trace/"} {
		if r := e.poll(admin, id, "conv/turn", ""); r.code != http.StatusNotFound {
			t.Errorf("provider %q: %d", id, r.code)
		}
	}
}

func TestBadInputIsABadRequestAndAnUnknownReferenceIsNotFound(t *testing.T) {
	e := newEnv(t)
	for name, c := range map[string]struct{ ref, q string }{
		"no ref":          {"", ""},
		"ref too long":    {strings.Repeat("a", 201), ""},
		"lines zero":      {"conv/turn", "lines=0"},
		"lines negative":  {"conv/turn", "lines=-3"},
		"lines text":      {"conv/turn", "lines=many"},
		"cursor garbage":  {"conv/turn", "cursor=not-a-cursor"},
		"cursor too long": {"conv/turn", "cursor=" + strings.Repeat("A", 600)},
	} {
		if r := e.poll(admin, "ai-trace", c.ref, c.q); r.code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, r.code, r.raw)
		}
	}
	if r := e.poll(admin, "ai-trace", "conv/missing", ""); r.code != http.StatusNotFound {
		t.Errorf("unknown reference: %d", r.code)
	}
	if r := e.poll(admin, "ai-trace", strings.Repeat("a", 200), ""); r.code != http.StatusNotFound {
		t.Errorf("a reference of the longest allowed size: %d", r.code)
	}
}

func TestEveryErrorOfAFollowerHasItsOwnStatusAndNeverLeaksADetail(t *testing.T) {
	for _, c := range []struct {
		err    error
		code   int
		header string
	}{
		{livelog.ErrNotFound, 404, ""},
		{livelog.ErrNotText, 415, ""},
		{livelog.ErrBusy, 503, "5"},
		{errors.New("open /secret/path/app.log: permission denied"), 500, ""},
		{livelog.ErrBadBatch, 500, ""},
	} {
		e := newEnv(t)
		e.log.Err = c.err
		r := e.poll(admin, "ai-trace", "conv/turn", "")
		if r.code != c.code || r.header.Get("Retry-After") != c.header {
			t.Errorf("%v: %d (Retry-After %q), want %d (%q)", c.err, r.code, r.header.Get("Retry-After"), c.code, c.header)
		}
		if strings.Contains(r.raw, "secret") || strings.Contains(r.raw, "permission") {
			t.Errorf("%v leaked into the answer: %s", c.err, r.raw)
		}
	}
	// the unexpected error is logged for the operator, with no log content
	e := newEnv(t)
	e.log.Err = errors.New("disk failure")
	e.poll(admin, "ai-trace", "conv/turn", "")
	if len(e.audit) != 1 || !strings.Contains(e.audit[0], "disk failure") {
		t.Errorf("the failure was not logged: %v", e.audit)
	}
}

func TestASlowFollowerAnswersGatewayTimeout(t *testing.T) {
	e := newEnv(t)
	e.log.Delay, e.log.IgnoreContext = 2*time.Second, true
	start := time.Now()
	r := e.poll(admin, "ai-trace", "conv/turn", "")
	if r.code != http.StatusGatewayTimeout || time.Since(start) > time.Second {
		t.Errorf("%d after %v", r.code, time.Since(start))
	}
}

func TestACallerWhoHasGoneAwayGetsNoAnswerWritten(t *testing.T) {
	e := newEnv(t)
	e.log.Delay = 2 * time.Second
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	ctx, cancel := contextWithCancel(req)
	cancel()
	rec := httptest.NewRecorder()
	e.d.poll(rec, req.WithContext(ctx), admin, "ai-trace", "conv/turn")
	if rec.Body.Len() != 0 {
		t.Errorf("an answer was written to a client that left: %q", rec.Body.String())
	}
}

func TestTheFollowerCapAnswersTooManyRequestsForANewLogButNeverForOneAlreadyFollowed(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 3; i++ {
		ref := fmt.Sprintf("log-%d", i)
		e.trace.Logs[ref] = livelogtest.NewLog("x")
		if r := e.poll(admin, "ai-trace", ref, ""); r.code != 200 {
			t.Fatalf("%s: %d", ref, r.code)
		}
	}
	e.trace.Logs["log-3"] = livelogtest.NewLog("x")
	r := e.poll(admin, "ai-trace", "log-3", "")
	if r.code != http.StatusTooManyRequests || r.header.Get("Retry-After") != "15" {
		t.Fatalf("the fourth log: %d %v", r.code, r.header)
	}
	for i := 0; i < 3; i++ { // the three already followed keep working
		if r := e.poll(admin, "ai-trace", fmt.Sprintf("log-%d", i), ""); r.code != 200 {
			t.Errorf("log-%d refused while followed: %d", i, r.code)
		}
	}
	if r := e.poll(identity{UserID: "other", Scopes: admin.Scopes}, "ai-trace", "log-3", ""); r.code != 200 {
		t.Errorf("the cap is per person: %d", r.code)
	}
	e.now = e.now.Add(16 * time.Second) // nobody polled for longer than the window
	if r := e.poll(admin, "ai-trace", "log-3", ""); r.code != 200 {
		t.Errorf("a place must free up when the polling stops: %d", r.code)
	}
	// a reference that does not exist never takes a place
	e2 := newEnv(t)
	for i := 0; i < 10; i++ {
		e2.poll(admin, "ai-trace", fmt.Sprintf("nothing-%d", i), "")
	}
	if got := e2.d.caps.Active("root", e2.now); got != 0 {
		t.Errorf("unknown references took %d places", got)
	}
}

func TestTheOpenedLineIsWrittenOncePerPersonAndLogAndNeverHoldsContent(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 30; i++ {
		e.poll(admin, "ai-trace", "conv/turn", "")
		e.now = e.now.Add(time.Second)
	}
	if len(e.audit) != 1 || !strings.Contains(e.audit[0], "user=root") || !strings.Contains(e.audit[0], "provider=ai-trace") || !strings.Contains(e.audit[0], "ref=conv/turn") {
		t.Fatalf("audit lines: %v", e.audit)
	}
	if strings.Contains(e.audit[0], "one") || strings.Contains(e.audit[0], "three") {
		t.Errorf("log content in the audit line: %s", e.audit[0])
	}
	e.now = e.now.Add(6 * time.Minute)
	e.poll(admin, "ai-trace", "conv/turn", "")
	if len(e.audit) != 2 {
		t.Errorf("a new line after the quiet period: %v", e.audit)
	}
	e.poll(identity{UserID: "other", Scopes: admin.Scopes}, "ai-trace", "conv/turn", "")
	if len(e.audit) != 3 {
		t.Errorf("another person is audited on their own: %v", e.audit)
	}
}

func TestTheProvidersListShowsOnlyWhatTheCallerMayFollow(t *testing.T) {
	e := newEnv(t)
	list := func(who identity) []string {
		rec := httptest.NewRecorder()
		e.d.providers(rec, who)
		var out struct {
			Message []map[string]string `json:"message"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != 200 {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
		ids := []string{}
		for _, m := range out.Message {
			ids = append(ids, m["id"]+"="+m["title"])
		}
		return ids
	}
	if got := fmt.Sprint(list(admin)); got != "[ai-trace=AI trace logs tool-log=Tool log]" {
		t.Errorf("admin: %s", got)
	}
	if got := fmt.Sprint(list(tool)); got != "[tool-log=Tool log]" {
		t.Errorf("tool: %s", got)
	}
	if got := list(ada); len(got) != 0 {
		t.Errorf("no scope: %v", got)
	}
}

func contextWithCancel(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithCancel(r.Context())
}
