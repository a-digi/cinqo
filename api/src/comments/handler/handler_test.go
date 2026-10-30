package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/a-digi/cinqo/src/comments"
	"github.com/a-digi/cinqo/src/comments/access"
	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
	tool_persistent "github.com/a-digi/cinqo/src/tool/repository/persistent"
)

const (
	job1 = "1fbd3006-0831-448c-8962-2851188e9de9"
	job2 = "2aaaaaaa-0831-448c-8962-2851188e9de9"
)

func coreDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	files, _ := filepath.Glob(filepath.Join("..", "..", "..", "config", "db", "migrations", "*.sql"))
	sort.Strings(files)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, stmt := range strings.Split(string(raw), "/***Statement***/") {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("%s: %v", filepath.Base(f), err)
			}
		}
	}
	return db
}

// env is a real engine: temp-dir stores, a resolver over a "career" tool that
// declares a "job" type and a system "note" type.
type env struct {
	d  *deps
	db *sql.DB
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := coreDB(t)
	if _, err := db.Exec(
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, created_at)
		 VALUES ('t1', 'career', 'Career', '1.0.0', 'frontend_only', 1, 'running', '/x', '2026-10-30T00:00:00Z')`,
	); err != nil {
		t.Fatal(err)
	}
	if err := tool_persistent.NewToolCommentTypePersistentRepo(db).ReplaceAll("t1", []tool_entity.ToolCommentType{
		{EntityType: "job", ReadScope: "tool:career:read", WriteScope: "tool:career:write"},
	}); err != nil {
		t.Fatal(err)
	}
	sys := access.NewSystemTypes()
	if err := sys.Register("note", "cinqo:note:read", "cinqo:note:write"); err != nil {
		t.Fatal(err)
	}
	stores := comments.NewManager(t.TempDir())
	t.Cleanup(func() { stores.Close() })
	return &env{
		d:  &deps{stores: stores, resolver: access.NewResolver(sys, access.NewToolTypes(db)), logf: func(string, ...any) {}},
		db: db,
	}
}

func who(id, name string, scopes ...string) identity {
	return identity{Caller: access.Caller{UserID: id, Scopes: scopes}, Name: name}
}

var (
	ada    = who("ada-id", "Ada", "tool:career:read", "tool:career:write")
	grace  = who("grace-id", "Grace", "tool:career:read", "tool:career:write")
	reader = who("reader-id", "Rita", "tool:career:read")
	admin  = who("root-id", "Root", access.AdminScope)
	nobody = who("nobody-id", "Nobody")
)

type result struct {
	code int
	out  map[string]any
}

func (r result) message() map[string]any { m, _ := r.out["message"].(map[string]any); return m }

func (r result) errText() string { s, _ := r.out["message"].(string); return s }

func run(f func(w http.ResponseWriter)) result {
	rec := httptest.NewRecorder()
	f(rec)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return result{code: rec.Code, out: out}
}

func (e *env) list(w identity, owner, typ, id, query string) result {
	return run(func(rec http.ResponseWriter) {
		e.d.list(rec, httptest.NewRequest(http.MethodGet, "/x?"+query, nil), w, owner, typ, id)
	})
}

func (e *env) create(w identity, owner, typ, id, body string) result {
	return run(func(rec http.ResponseWriter) {
		e.d.create(rec, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body)), w, owner, typ, id)
	})
}

func (e *env) edit(w identity, owner, typ, id, commentID, body string) result {
	return run(func(rec http.ResponseWriter) {
		e.d.edit(rec, httptest.NewRequest(http.MethodPatch, "/x", strings.NewReader(body)), w, owner, typ, id, commentID)
	})
}

func (e *env) remove(w identity, owner, typ, id, commentID string) result {
	return run(func(rec http.ResponseWriter) { e.d.remove(rec, w, owner, typ, id, commentID) })
}

func (e *env) add(t *testing.T, w identity, text string) string {
	t.Helper()
	r := e.create(w, "career", "job", job1, `{"body":"`+text+`"}`)
	if r.code != http.StatusCreated {
		t.Fatalf("create %q: %d %v", text, r.code, r.out)
	}
	return r.message()["id"].(string)
}

func TestCreateTakesTheAuthorFromTheTokenAndReportsWhatTheyMayDo(t *testing.T) {
	e := newEnv(t)
	r := e.create(ada, "career", "job", job1, `{"body":"  Looks good  ","authorId":"someone-else","authorName":"Mallory"}`)
	if r.code != http.StatusCreated {
		t.Fatalf("%d %v", r.code, r.out)
	}
	c := r.message()
	if c["body"] != "Looks good" || c["authorId"] != "ada-id" || c["authorName"] != "Ada" || c["entityType"] != "job" || c["entityId"] != job1 {
		t.Errorf("comment = %v", c)
	}
	if c["canEdit"] != true || c["canDelete"] != true || c["updatedAt"] != "" {
		t.Errorf("flags = %v", c)
	}
	if id, _ := c["id"].(string); len(id) != 36 {
		t.Errorf("id = %v", c["id"])
	}
}

func TestListShapeFlagsAndFilters(t *testing.T) {
	e := newEnv(t)
	e.add(t, ada, "by ada")
	e.add(t, grace, "by grace")
	e.add(t, ada, "by ada again")

	r := e.list(reader, "career", "job", job1, "")
	m := r.message()
	list := m["comments"].([]any)
	if r.code != 200 || len(list) != 3 || m["total"] != float64(3) || m["nextBefore"] != "" {
		t.Fatalf("%d %v", r.code, r.out)
	}
	if first := list[0].(map[string]any); first["body"] != "by ada again" {
		t.Errorf("not newest first: %v", first["body"])
	}
	for _, x := range list {
		if c := x.(map[string]any); c["canEdit"] != false || c["canDelete"] != false {
			t.Errorf("a reader can do nothing to anyone's comment: %v", c)
		}
	}

	// The same comments, as one of the authors and as an admin.
	own := e.list(ada, "career", "job", job1, "").message()["comments"].([]any)
	for _, x := range own {
		c := x.(map[string]any)
		mine := c["authorId"] == "ada-id"
		if c["canEdit"] != mine || c["canDelete"] != mine {
			t.Errorf("ada on %v: edit=%v delete=%v", c["authorId"], c["canEdit"], c["canDelete"])
		}
	}
	for _, x := range e.list(admin, "career", "job", job1, "").message()["comments"].([]any) {
		if c := x.(map[string]any); c["canEdit"] != false || c["canDelete"] != true {
			t.Errorf("admin: edit=%v delete=%v", c["canEdit"], c["canDelete"])
		}
	}

	// Whether the caller may add a comment is part of the answer.
	for name, tc := range map[string]struct {
		caller identity
		want   bool
	}{"reader": {reader, false}, "writer": {ada, true}, "admin": {admin, true}} {
		if got := e.list(tc.caller, "career", "job", job1, "").message()["canCreate"]; got != tc.want {
			t.Errorf("%s: canCreate = %v, want %v", name, got, tc.want)
		}
	}

	// Filters and paging pass through to the store.
	f := e.list(reader, "career", "job", job1, "authorId=ada-id&limit=1").message()
	if len(f["comments"].([]any)) != 1 || f["total"] != float64(2) || f["nextBefore"] == "" {
		t.Errorf("author filter + limit: %v", f)
	}
	if none := e.list(reader, "career", "job", job1, "createdFrom=2999-01-01").message(); none["total"] != float64(0) || len(none["comments"].([]any)) != 0 {
		t.Errorf("a range in the future: %v", none)
	}
	if empty := e.list(reader, "career", "job", job2, "").message(); empty["comments"] == nil || len(empty["comments"].([]any)) != 0 {
		t.Errorf("an entity with no comments must give [] not null: %v", empty)
	}
}

func TestPermissionsAndNotFound(t *testing.T) {
	e := newEnv(t)
	id := e.add(t, ada, "hello")

	// No read scope: 404 on every operation, indistinguishable from a type that does not exist.
	for name, r := range map[string]result{
		"list":                        e.list(nobody, "career", "job", job1, ""),
		"create":                      e.create(nobody, "career", "job", job1, `{"body":"x"}`),
		"edit":                        e.edit(nobody, "career", "job", job1, id, `{"body":"x"}`),
		"delete":                      e.remove(nobody, "career", "job", job1, id),
		"unknown type":                e.list(admin, "career", "nope", job1, ""),
		"unknown owner":               e.list(admin, "ghost", "job", job1, ""),
		"foreign type":                e.list(admin, "cinqo", "job", job1, ""),
		"tool scope on a system type": e.list(ada, "cinqo", "note", job1, ""),
	} {
		if r.code != http.StatusNotFound || r.errText() != "not found" {
			t.Errorf("%s: %d %v", name, r.code, r.out)
		}
	}

	// Read but not write: 403 to add.
	if r := e.create(reader, "career", "job", job1, `{"body":"x"}`); r.code != http.StatusForbidden {
		t.Errorf("reader create: %d %v", r.code, r.out)
	}
	// And a writer can use a system type only with its own scopes.
	sysWriter := who("s", "S", "cinqo:note:read", "cinqo:note:write")
	if r := e.create(sysWriter, "cinqo", "note", job1, `{"body":"system comment"}`); r.code != http.StatusCreated {
		t.Errorf("system create: %d %v", r.code, r.out)
	}
}

func TestEdit(t *testing.T) {
	e := newEnv(t)
	id := e.add(t, ada, "original")

	r := e.edit(ada, "career", "job", job1, id, `{"body":"  edited "}`)
	if c := r.message(); r.code != http.StatusOK || c["body"] != "edited" || c["updatedAt"] == "" || c["authorId"] != "ada-id" {
		t.Fatalf("author edit: %d %v", r.code, r.out)
	}
	if r := e.edit(grace, "career", "job", job1, id, `{"body":"hijack"}`); r.code != http.StatusForbidden {
		t.Errorf("another writer: %d", r.code)
	}
	if r := e.edit(admin, "career", "job", job1, id, `{"body":"hijack"}`); r.code != http.StatusForbidden {
		t.Errorf("an admin may delete but not rewrite someone's words: %d", r.code)
	}
	if r := e.edit(reader, "career", "job", job1, id, `{"body":"x"}`); r.code != http.StatusForbidden {
		t.Errorf("reader: %d", r.code)
	}
	// Through another entity, or an unknown comment: 404.
	if r := e.edit(ada, "career", "job", job2, id, `{"body":"x"}`); r.code != http.StatusNotFound {
		t.Errorf("wrong entity: %d", r.code)
	}
	if r := e.edit(ada, "career", "job", job1, "00000000-0000-4000-8000-000000000000", `{"body":"x"}`); r.code != http.StatusNotFound {
		t.Errorf("unknown comment: %d", r.code)
	}
	if r := e.edit(ada, "career", "job", job1, id, `{"body":"   "}`); r.code != http.StatusBadRequest {
		t.Errorf("blank edit: %d", r.code)
	}
	if got := e.list(ada, "career", "job", job1, "").message()["comments"].([]any)[0].(map[string]any)["body"]; got != "edited" {
		t.Errorf("a refused edit changed the comment: %v", got)
	}
}

func TestDelete(t *testing.T) {
	e := newEnv(t)
	a, b, c := e.add(t, ada, "a"), e.add(t, ada, "b"), e.add(t, ada, "c")

	if r := e.remove(grace, "career", "job", job1, a); r.code != http.StatusForbidden {
		t.Errorf("another writer: %d", r.code)
	}
	if r := e.remove(reader, "career", "job", job1, a); r.code != http.StatusForbidden {
		t.Errorf("reader: %d", r.code)
	}
	if r := e.remove(ada, "career", "job", job2, a); r.code != http.StatusNotFound {
		t.Errorf("wrong entity: %d", r.code)
	}
	if r := e.remove(ada, "career", "job", job1, a); r.code != http.StatusNoContent {
		t.Errorf("author: %d", r.code)
	}
	if r := e.remove(admin, "career", "job", job1, b); r.code != http.StatusNoContent {
		t.Errorf("admin: %d", r.code)
	}
	if r := e.remove(ada, "career", "job", job1, a); r.code != http.StatusNotFound {
		t.Errorf("second delete: %d", r.code)
	}
	if got := e.list(ada, "career", "job", job1, "").message(); got["total"] != float64(1) || got["comments"].([]any)[0].(map[string]any)["id"] != c {
		t.Errorf("remaining = %v", got)
	}
}

func TestBadRequests(t *testing.T) {
	e := newEnv(t)
	for name, r := range map[string]result{
		"not a uuid (list)":   e.list(ada, "career", "job", "123", ""),
		"not a uuid (create)": e.create(ada, "career", "job", "123", `{"body":"x"}`),
		"limit zero":          e.list(ada, "career", "job", job1, "limit=0"),
		"limit text":          e.list(ada, "career", "job", job1, "limit=ten"),
		"bad date":            e.list(ada, "career", "job", job1, "createdFrom=yesterday"),
		"inverted range":      e.list(ada, "career", "job", job1, "createdFrom=2026-10-05&createdTo=2026-10-01"),
		"foreign cursor":      e.list(ada, "career", "job", job1, "before=00000000-0000-4000-8000-000000000000"),
		"not json":            e.create(ada, "career", "job", job1, `nope`),
		"empty body":          e.create(ada, "career", "job", job1, ``),
		"wrong shape":         e.create(ada, "career", "job", job1, `{"body":5}`),
		"blank text":          e.create(ada, "career", "job", job1, `{"body":"  "}`),
		"too long":            e.create(ada, "career", "job", job1, `{"body":"`+strings.Repeat("x", comments.MaxBodyRunes+1)+`"}`),
		"oversized request":   e.create(ada, "career", "job", job1, `{"body":"`+strings.Repeat("x", maxBodyBytes)+`"}`),
	} {
		if r.code != http.StatusBadRequest {
			t.Errorf("%s: %d %v", name, r.code, r.out)
		}
	}
	if got := e.list(ada, "career", "job", job1, "").message()["total"]; got != float64(0) {
		t.Errorf("a rejected request stored something: total %v", got)
	}
}

func TestOwnersAreIsolatedThroughTheAPI(t *testing.T) {
	e := newEnv(t)
	e.add(t, ada, "career comment")
	sysWriter := who("s", "S", "cinqo:note:read", "cinqo:note:write")
	if r := e.create(sysWriter, "cinqo", "note", job1, `{"body":"system comment"}`); r.code != http.StatusCreated {
		t.Fatal(r.out)
	}
	if got := e.list(ada, "career", "job", job1, "").message(); got["total"] != float64(1) || got["comments"].([]any)[0].(map[string]any)["body"] != "career comment" {
		t.Errorf("career sees %v", got)
	}
	if got := e.list(sysWriter, "cinqo", "note", job1, "").message(); got["total"] != float64(1) || got["comments"].([]any)[0].(map[string]any)["body"] != "system comment" {
		t.Errorf("system sees %v", got)
	}
}

func TestDisabledToolIsUnreachableAndComesBack(t *testing.T) {
	e := newEnv(t)
	id := e.add(t, ada, "kept")
	if _, err := e.db.Exec(`UPDATE tools SET enabled = 0 WHERE id = 't1'`); err != nil {
		t.Fatal(err)
	}
	for name, r := range map[string]result{
		"list":   e.list(admin, "career", "job", job1, ""),
		"create": e.create(admin, "career", "job", job1, `{"body":"x"}`),
		"edit":   e.edit(ada, "career", "job", job1, id, `{"body":"x"}`),
		"delete": e.remove(admin, "career", "job", job1, id),
	} {
		if r.code != http.StatusNotFound {
			t.Errorf("%s on a disabled tool: %d", name, r.code)
		}
	}
	if _, err := e.db.Exec(`UPDATE tools SET enabled = 1 WHERE id = 't1'`); err != nil {
		t.Fatal(err)
	}
	if got := e.list(ada, "career", "job", job1, "").message(); got["total"] != float64(1) {
		t.Errorf("the comments must survive a disable: %v", got)
	}
}

type brokenSource struct{}

func (brokenSource) Lookup(string, string) (access.EntityType, bool, error) {
	return access.EntityType{}, false, errors.New("database is down")
}

func TestInternalFailuresAre500NotAPermissionAnswer(t *testing.T) {
	var logged []string
	d := &deps{stores: comments.NewManager(t.TempDir()), resolver: access.NewResolver(brokenSource{}), logf: func(f string, a ...any) { logged = append(logged, f) }}
	r := run(func(w http.ResponseWriter) {
		d.list(w, httptest.NewRequest(http.MethodGet, "/x", nil), admin, "career", "job", job1)
	})
	if r.code != http.StatusInternalServerError || r.errText() != "request failed" {
		t.Errorf("%d %v", r.code, r.out)
	}
	if len(logged) == 0 {
		t.Error("the failure was not logged")
	}
}
