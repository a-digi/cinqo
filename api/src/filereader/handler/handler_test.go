package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/filereader"
	"github.com/a-digi/cinqo/src/filereader/source"
)

const (
	conv = "11111111-2222-4333-8444-555555555555"
	turn = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	ref  = conv + "/" + turn
)

var (
	admin = identity{UserID: "root", Scopes: []string{source.AdminScope}}
	user  = identity{UserID: "ada", Scopes: []string{"cinqo:conversation:use"}}
)

type env struct {
	d     *deps
	root  string
	audit []string
}

func newEnv(t *testing.T, lines int) *env {
	t.Helper()
	root := filepath.Join(t.TempDir(), "conversations")
	os.MkdirAll(filepath.Join(root, conv), 0o755)
	var b strings.Builder
	for i := 1; i <= lines; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	os.WriteFile(filepath.Join(root, conv, turn+".txt"), []byte(b.String()), 0o600)
	reg := source.NewRegistry()
	if err := reg.Register(source.NewAITrace(root)); err != nil {
		t.Fatal(err)
	}
	e := &env{root: root}
	e.d = newDeps(filereader.New(filereader.DefaultLimits()), reg, func(f string, a ...any) { e.audit = append(e.audit, fmt.Sprintf(f, a...)) })
	return e
}

type result struct {
	code int
	out  map[string]any
}

func (r result) message() map[string]any { m, _ := r.out["message"].(map[string]any); return m }

func call(f func(w http.ResponseWriter, r *http.Request), query string) result {
	rec := httptest.NewRecorder()
	f(rec, httptest.NewRequest(http.MethodGet, "/x?"+query, nil))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return result{code: rec.Code, out: out}
}

func (e *env) info(who identity, src, ref string) result {
	return call(func(w http.ResponseWriter, r *http.Request) { e.d.info(w, r, who, src, ref) }, "")
}

func (e *env) lines(who identity, src, ref, query string) result {
	return call(func(w http.ResponseWriter, r *http.Request) { e.d.lines(w, r, who, src, ref) }, query)
}

func TestAdminReadsAWindow(t *testing.T) {
	e := newEnv(t, 1200)
	res := e.lines(admin, source.AITraceID, ref, "from=1&count=500")
	if res.code != 200 {
		t.Fatalf("code %d: %v", res.code, res.out)
	}
	m := res.message()
	lines := m["lines"].([]any)
	if len(lines) != 500 || m["hasMore"] != true || m["nextFrom"].(float64) != 501 {
		t.Fatalf("window: %d lines, %v", len(lines), m)
	}
	if first := lines[0].(map[string]any); first["text"] != "line 1" || first["n"].(float64) != 1 {
		t.Fatalf("first line: %v", first)
	}
}

func TestInfoAndIndexedJump(t *testing.T) {
	e := newEnv(t, 5000)
	res := e.info(admin, source.AITraceID, ref)
	m := res.message()
	if res.code != 200 || m["complete"] != true || m["totalLines"].(float64) != 5000 {
		t.Fatalf("info: %d %v", res.code, res.out)
	}
	res = e.lines(admin, source.AITraceID, ref, "from=4001&count=3")
	lines := res.message()["lines"].([]any)
	if res.code != 200 || len(lines) != 3 || lines[0].(map[string]any)["text"] != "line 4001" {
		t.Fatalf("jump: %d %v", res.code, res.out)
	}
}

func TestNotIndexedIsAConflict(t *testing.T) {
	e := newEnv(t, 5000)
	if res := e.lines(admin, source.AITraceID, ref, "from=4001"); res.code != http.StatusConflict {
		t.Fatalf("a jump before indexing: %d", res.code)
	}
}

func TestScopeIsCheckedBeforeAnythingIsResolved(t *testing.T) {
	e := newEnv(t, 10)
	// a user without the scope gets 403 for a real file AND for a file that does not exist: no oracle
	real := e.lines(user, source.AITraceID, ref, "")
	fake := e.lines(user, source.AITraceID, conv+"/bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "")
	garbage := e.lines(user, source.AITraceID, "../../etc/passwd", "")
	for name, r := range map[string]result{"real": real, "fake": fake, "garbage": garbage} {
		if r.code != http.StatusForbidden {
			t.Errorf("%s: code %d", name, r.code)
		}
	}
	if res := e.info(user, source.AITraceID, ref); res.code != http.StatusForbidden {
		t.Errorf("info: %d", res.code)
	}
	if res := e.lines(identity{}, source.AITraceID, ref, ""); res.code != http.StatusForbidden {
		t.Errorf("no scopes at all: %d", res.code)
	}
	if len(e.audit) != 0 {
		t.Errorf("a refused request was audited: %v", e.audit)
	}
}

func TestUnknownSourceAndBadReferences(t *testing.T) {
	e := newEnv(t, 10)
	if res := e.lines(admin, "nope", ref, ""); res.code != http.StatusNotFound {
		t.Errorf("unknown source: %d", res.code)
	}
	for _, bad := range []string{"../../../etc/passwd", conv, conv + "/../" + turn, "/etc/passwd", ref + ".txt", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb/" + turn} {
		if res := e.lines(admin, source.AITraceID, bad, ""); res.code != http.StatusNotFound {
			t.Errorf("ref %q: %d", bad, res.code)
		}
	}
	if res := e.lines(admin, source.AITraceID, "", ""); res.code != http.StatusBadRequest {
		t.Errorf("empty ref: %d", res.code)
	}
	if res := e.lines(admin, source.AITraceID, strings.Repeat("a", 500), ""); res.code != http.StatusBadRequest {
		t.Errorf("long ref: %d", res.code)
	}
}

func TestParameterValidation(t *testing.T) {
	e := newEnv(t, 10)
	for _, q := range []string{"from=0", "from=-1", "from=abc", "count=0", "count=x", "from=1&count=-5", "from=99999999999999999999"} {
		if res := e.lines(admin, source.AITraceID, ref, q); res.code != http.StatusBadRequest {
			t.Errorf("query %q: %d", q, res.code)
		}
	}
	// a huge count is clamped by the engine, not an error
	if res := e.lines(admin, source.AITraceID, ref, "count=999999999"); res.code != 200 {
		t.Errorf("huge count: %d", res.code)
	}
}

func TestFileProblemsGetTheirOwnStatus(t *testing.T) {
	e := newEnv(t, 1)
	bin := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	os.WriteFile(filepath.Join(e.root, conv, bin+".txt"), []byte("a\x00b\n"), 0o600)
	if res := e.lines(admin, source.AITraceID, conv+"/"+bin, ""); res.code != http.StatusUnsupportedMediaType {
		t.Errorf("binary: %d", res.code)
	}
}

func TestResponsesNeverCarryAPath(t *testing.T) {
	e := newEnv(t, 3)
	for _, res := range []result{
		e.info(admin, source.AITraceID, ref),
		e.lines(admin, source.AITraceID, ref, ""),
		e.lines(admin, source.AITraceID, conv+"/bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", ""),
	} {
		raw, _ := json.Marshal(res.out)
		if strings.Contains(string(raw), e.root) {
			t.Errorf("a response leaked the server path: %s", raw)
		}
	}
}

func TestAuditIsOncePerWindowOfTime(t *testing.T) {
	e := newEnv(t, 10)
	clock := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	e.d.now = func() time.Time { return clock }

	e.lines(admin, source.AITraceID, ref, "")
	e.lines(admin, source.AITraceID, ref, "from=2")
	e.info(admin, source.AITraceID, ref)
	if len(e.audit) != 1 || !strings.Contains(e.audit[0], "user=root") || !strings.Contains(e.audit[0], ref) {
		t.Fatalf("audit = %v", e.audit)
	}
	if strings.Contains(e.audit[0], "line 1") {
		t.Fatal("the audit line carries content")
	}
	clock = clock.Add(auditEvery + time.Second)
	e.lines(admin, source.AITraceID, ref, "")
	other := identity{UserID: "root2", Scopes: admin.Scopes}
	e.lines(other, source.AITraceID, ref, "")
	if len(e.audit) != 3 {
		t.Fatalf("after the window and for another person: %v", e.audit)
	}
}

func TestAuditMemoryIsBounded(t *testing.T) {
	e := newEnv(t, 1)
	for i := 0; i < 3*maxAudited; i++ {
		e.d.audit(identity{UserID: fmt.Sprint("u", i)}, "s", "r")
	}
	if len(e.d.audited) > maxAudited {
		t.Fatalf("audit map grew to %d", len(e.d.audited))
	}
}

func (e *env) search(who identity, src, ref, query string) result {
	return call(func(w http.ResponseWriter, r *http.Request) { e.d.search(w, r, who, src, ref) }, query)
}

func TestSearchFindsLines(t *testing.T) {
	e := newEnv(t, 300)
	res := e.search(admin, source.AITraceID, ref, "q=LINE+15&limit=3")
	if res.code != 200 {
		t.Fatalf("code %d: %v", res.code, res.out)
	}
	m := res.message()
	matches := m["matches"].([]any)
	if len(matches) != 3 || matches[0].(map[string]any)["n"].(float64) != 15 || m["done"] != false || m["nextFrom"].(float64) != 152 {
		t.Fatalf("matches: %v", m)
	}
	if first := matches[0].(map[string]any); first["preview"] != "line 15" {
		t.Fatalf("preview: %v", first)
	}
	// case sensitive: "LINE" matches nothing
	res = e.search(admin, source.AITraceID, ref, "q=LINE&case=1")
	if got := res.message()["matches"].([]any); len(got) != 0 || res.message()["done"] != true {
		t.Fatalf("case sensitive: %v", res.out)
	}
}

func TestSearchChecksScopeFirstAndValidatesInput(t *testing.T) {
	e := newEnv(t, 10)
	if res := e.search(user, source.AITraceID, ref, "q=line"); res.code != http.StatusForbidden {
		t.Errorf("no scope: %d", res.code)
	}
	if res := e.search(admin, "nope", ref, "q=line"); res.code != http.StatusNotFound {
		t.Errorf("unknown source: %d", res.code)
	}
	if res := e.search(admin, source.AITraceID, "../../etc/passwd", "q=line"); res.code != http.StatusNotFound {
		t.Errorf("bad ref: %d", res.code)
	}
	for _, q := range []string{"", "q=", "q=" + strings.Repeat("a", 201), "q=a%00b", "q=%ff"} {
		if res := e.search(admin, source.AITraceID, ref, q); res.code != http.StatusBadRequest {
			t.Errorf("query %q: %d", q, res.code)
		}
	}
	for _, q := range []string{"q=a&from=0", "q=a&from=x", "q=a&limit=0", "q=a&limit=-2"} {
		if res := e.search(admin, source.AITraceID, ref, q); res.code != http.StatusBadRequest {
			t.Errorf("query %q: %d", q, res.code)
		}
	}
	// the search text is never echoed back, and no path leaks
	res := e.search(admin, source.AITraceID, ref, "q=line+1")
	raw, _ := json.Marshal(res.out)
	if strings.Contains(string(raw), e.root) || strings.Contains(string(raw), "\"q\"") {
		t.Errorf("response leaks: %s", raw)
	}
}

func TestSearchIsAuditedOncePerWindow(t *testing.T) {
	e := newEnv(t, 10)
	e.search(admin, source.AITraceID, ref, "q=line")
	e.search(admin, source.AITraceID, ref, "q=line&from=3")
	if len(e.audit) != 1 {
		t.Fatalf("audit = %v", e.audit)
	}
	if strings.Contains(e.audit[0], "line") && strings.Contains(e.audit[0], "q=") {
		t.Fatalf("the search text must not be logged: %v", e.audit)
	}
}
