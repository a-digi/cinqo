package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/a-digi/coco-logger/logger"
	"github.com/a-digi/coco-orm/orm"
	serverdi "github.com/a-digi/coco-server/server/di"
	"github.com/a-digi/coco-server/server/request"

	_ "github.com/mattn/go-sqlite3"

	"github.com/a-digi/cinqo/src/review"
	"github.com/a-digi/cinqo/src/review/access"
	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
	tool_persistent "github.com/a-digi/cinqo/src/tool/repository/persistent"
)

const (
	company1 = "1fbd3006-0831-448c-8962-2851188e9de9"
	company2 = "2aaaaaaa-0831-448c-8962-2851188e9de9"
	company3 = "3bbbbbbb-0831-448c-8962-2851188e9de9"
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

// env is a real engine: temp-dir stores, a resolver over a "career" tool that declares a "company" type (5 stars, halves), a "job"
// type (10 stars, whole), and a system "note" type (3 stars, halves).
type env struct {
	d  *deps
	db *sql.DB
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := coreDB(t)
	if _, err := db.Exec(
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, created_at)
		 VALUES ('t1', 'career', 'Career', '1.0.0', 'frontend_only', 1, 'running', '/x', '2026-11-09T00:00:00Z')`,
	); err != nil {
		t.Fatal(err)
	}
	if err := tool_persistent.NewToolReviewTypePersistentRepo(db).ReplaceAll("t1", []tool_entity.ToolReviewType{
		{EntityType: "company", Scale: 5, HalfStars: true, ReadScope: "tool:career:read", WriteScope: "tool:career:write"},
		{EntityType: "job", Scale: 10, HalfStars: false, ReadScope: "tool:career:read", WriteScope: "tool:career:write"},
	}); err != nil {
		t.Fatal(err)
	}
	sys := access.NewSystemTypes()
	if err := sys.Register("note", 3, true, "cinqo:note:read", "cinqo:note:write"); err != nil {
		t.Fatal(err)
	}
	stores := review.NewManager(t.TempDir())
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

func (e *env) get(w identity, owner, typ, id, query string) result {
	return run(func(rec http.ResponseWriter) {
		e.d.get(rec, httptest.NewRequest(http.MethodGet, "/x?"+query, nil), w, owner, typ, id)
	})
}

func (e *env) put(w identity, owner, typ, id, body string) result {
	return run(func(rec http.ResponseWriter) {
		e.d.put(rec, httptest.NewRequest(http.MethodPut, "/x", strings.NewReader(body)), w, owner, typ, id)
	})
}

func (e *env) remove(w identity, owner, typ, id, query string) result {
	return run(func(rec http.ResponseWriter) {
		e.d.remove(rec, httptest.NewRequest(http.MethodDelete, "/x?"+query, nil), w, owner, typ, id)
	})
}

func (e *env) summaries(w identity, owner, typ, body string) result {
	return run(func(rec http.ResponseWriter) {
		e.d.summaries(rec, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body)), w, owner, typ)
	})
}

func (e *env) rate(t *testing.T, w identity, id string, rating float64, text string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"rating": rating, "body": text})
	r := e.put(w, "career", "company", id, string(body))
	if r.code != http.StatusOK {
		t.Fatalf("put %v: %d %v", rating, r.code, r.out)
	}
	return r.message()["id"].(string)
}

func TestPutTakesTheAuthorAndTheScaleFromTheServer(t *testing.T) {
	e := newEnv(t)
	r := e.put(ada, "career", "company", company1,
		`{"rating":3.5,"body":"  Good place  ","authorId":"someone-else","authorName":"Mallory","scale":10,"halfStars":false,"entityType":"job"}`)
	if r.code != http.StatusOK {
		t.Fatalf("%d %v", r.code, r.out)
	}
	m := r.message()
	if m["rating"] != 3.5 || m["scale"] != float64(5) || m["body"] != "Good place" || m["authorId"] != "ada-id" || m["authorName"] != "Ada" ||
		m["entityType"] != "company" || m["entityId"] != company1 {
		t.Errorf("review = %v", m)
	}
	if m["canEdit"] != true || m["canDelete"] != true || m["updatedAt"] != "" {
		t.Errorf("flags = %v", m)
	}
	if id, _ := m["id"].(string); len(id) != 36 {
		t.Errorf("id = %v", m["id"])
	}
}

func TestPutAppliesTheTypesOwnRatingRules(t *testing.T) {
	e := newEnv(t)
	// company: 5 stars with halves.
	for body, code := range map[string]int{
		`{"rating":0.5}`: 200, `{"rating":5}`: 200, `{"rating":5.5}`: 400, `{"rating":0}`: 400, `{"rating":3.25}`: 400, `{"rating":-1}`: 400,
		`{"rating":"4"}`: 400, `{}`: 400, `{"rating":null}`: 400, `not json`: 400, ``: 400, `[]`: 400,
	} {
		if r := e.put(ada, "career", "company", company1, body); r.code != code {
			t.Errorf("company %s: %d %v", body, r.code, r.out)
		}
	}
	// job: 10 stars, whole only.
	for body, code := range map[string]int{`{"rating":10}`: 200, `{"rating":7}`: 200, `{"rating":7.5}`: 400, `{"rating":11}`: 400, `{"rating":0.5}`: 400} {
		if r := e.put(grace, "career", "job", company1, body); r.code != code {
			t.Errorf("job %s: %d %v", body, r.code, r.out)
		}
	}
	// system note: 3 stars with halves.
	for body, code := range map[string]int{`{"rating":2.5}`: 200, `{"rating":3.5}`: 400} {
		if r := e.put(admin, "cinqo", "note", company1, body); r.code != code {
			t.Errorf("note %s: %d %v", body, r.code, r.out)
		}
	}
	if r := e.put(ada, "career", "company", company1, `{"rating":4,"body":"`+strings.Repeat("x", 2001)+`"}`); r.code != 400 || !strings.HasPrefix(r.errText(), "body:") {
		t.Errorf("long text: %d %v", r.code, r.out)
	}
	if r := e.put(ada, "career", "company", "not-a-uuid", `{"rating":4}`); r.code != 400 || !strings.HasPrefix(r.errText(), "entityId:") {
		t.Errorf("bad id: %d %v", r.code, r.out)
	}
	big := `{"rating":4,"body":"` + strings.Repeat("x", 70*1024) + `"}`
	if r := e.put(ada, "career", "company", company2, big); r.code != 400 || !strings.HasPrefix(r.errText(), "body must be") {
		t.Errorf("a body over the size limit is refused before it is read: %d %v", r.code, r.out)
	}
	ids := `{"ids":["` + strings.Repeat("x", 70*1024) + `"]}`
	if r := e.summaries(ada, "career", "company", ids); r.code != 400 || !strings.HasPrefix(r.errText(), "body must be") {
		t.Errorf("summaries over the size limit: %d %v", r.code, r.out)
	}
}

func TestAnotherPutReplacesAndGetShowsTheSummary(t *testing.T) {
	e := newEnv(t)
	first := e.rate(t, ada, company1, 2, "meh")
	again := e.rate(t, ada, company1, 4.5, "better now")
	if first != again {
		t.Errorf("a second put by the same author must keep the review: %s vs %s", first, again)
	}
	e.rate(t, grace, company1, 3, "")
	m := e.get(reader, "career", "company", company1, "").message()
	sum := m["summary"].(map[string]any)
	if m["scale"] != float64(5) || m["halfStars"] != true || m["total"] != float64(2) || sum["count"] != float64(2) || sum["average"] != 3.75 || sum["otherScale"] != float64(0) {
		t.Errorf("%v", m)
	}
	if dist := sum["distribution"].([]any); len(dist) != 10 || dist[8] != float64(1) || dist[5] != float64(1) {
		t.Errorf("distribution %v", dist)
	}
	list := m["reviews"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["authorId"] != "grace-id" {
		t.Errorf("newest first: %v", list)
	}
	if edited := list[1].(map[string]any); edited["rating"] != 4.5 || edited["body"] != "better now" || edited["updatedAt"] == "" {
		t.Errorf("%v", edited)
	}
	if m["mine"] != nil {
		t.Errorf("a reader has no review of their own: %v", m["mine"])
	}
}

func TestGetReportsMineAndWhatTheCallerMayDo(t *testing.T) {
	e := newEnv(t)
	e.rate(t, ada, company1, 5, "a")
	e.rate(t, grace, company1, 1, "g")

	mine := e.get(ada, "career", "company", company1, "").message()["mine"].(map[string]any)
	if mine["authorId"] != "ada-id" || mine["rating"] != float64(5) || mine["canEdit"] != true {
		t.Errorf("%v", mine)
	}
	for _, x := range e.get(reader, "career", "company", company1, "").message()["reviews"].([]any) {
		if c := x.(map[string]any); c["canEdit"] != false || c["canDelete"] != false {
			t.Errorf("a reader can do nothing to anyone's review: %v", c)
		}
	}
	for _, x := range e.get(ada, "career", "company", company1, "").message()["reviews"].([]any) {
		c := x.(map[string]any)
		own := c["authorId"] == "ada-id"
		if c["canEdit"] != own || c["canDelete"] != own {
			t.Errorf("ada on %v: %v %v", c["authorId"], c["canEdit"], c["canDelete"])
		}
	}
	for _, x := range e.get(admin, "career", "company", company1, "").message()["reviews"].([]any) {
		if c := x.(map[string]any); c["canEdit"] != false || c["canDelete"] != true {
			t.Errorf("admin: %v %v", c["canEdit"], c["canDelete"])
		}
	}
	for name, tc := range map[string]struct {
		caller identity
		want   bool
	}{"reader": {reader, false}, "writer": {ada, true}, "admin": {admin, true}} {
		if got := e.get(tc.caller, "career", "company", company1, "").message()["canCreate"]; got != tc.want {
			t.Errorf("%s: canCreate = %v, want %v", name, got, tc.want)
		}
	}
	// Mine is found whichever page it would be on.
	for i := 0; i < 3; i++ {
		e.rate(t, who("u"+string(rune('a'+i)), "x", "tool:career:read", "tool:career:write"), company1, 3, "")
	}
	page := e.get(ada, "career", "company", company1, "limit=1").message()
	if len(page["reviews"].([]any)) != 1 || page["nextBefore"] == "" || page["mine"] == nil {
		t.Errorf("%v", page)
	}
	if empty := e.get(reader, "career", "company", company3, "").message(); empty["reviews"] == nil || len(empty["reviews"].([]any)) != 0 ||
		empty["summary"].(map[string]any)["count"] != float64(0) {
		t.Errorf("an entity with no reviews must give [] not null: %v", empty)
	}
	if r := e.get(reader, "career", "company", company1, "limit=0"); r.code != 400 {
		t.Errorf("limit 0: %d", r.code)
	}
	if r := e.get(reader, "career", "company", company1, "limit=abc"); r.code != 400 {
		t.Errorf("limit abc: %d", r.code)
	}
	if r := e.get(reader, "career", "company", company1, "before="+company2); r.code != 400 {
		t.Errorf("a cursor that is no review of this entity: %d", r.code)
	}
}

func TestPermissionsAndNotFound(t *testing.T) {
	e := newEnv(t)
	e.rate(t, ada, company1, 4, "")

	// No read scope: 404 on every operation, indistinguishable from a type that does not exist.
	for name, r := range map[string]result{
		"get":                         e.get(nobody, "career", "company", company1, ""),
		"put":                         e.put(nobody, "career", "company", company1, `{"rating":3}`),
		"delete":                      e.remove(nobody, "career", "company", company1, ""),
		"delete id":                   e.remove(nobody, "career", "company", company1, "reviewId="+company2),
		"summaries":                   e.summaries(nobody, "career", "company", `{"ids":[]}`),
		"unknown type":                e.get(admin, "career", "person", company1, ""),
		"unknown owner":               e.get(admin, "ghost", "company", company1, ""),
		"bad owner":                   e.get(admin, "../x", "company", company1, ""),
		"type of another owner":       e.get(admin, "cinqo", "company", company1, ""),
		"tool scope on a system type": e.get(ada, "cinqo", "note", company1, ""),
		"unknown type, summaries":     e.summaries(admin, "career", "person", `{"ids":[]}`),
	} {
		if r.code != http.StatusNotFound || r.errText() != "not found" {
			t.Errorf("%s: %d %v", name, r.code, r.out)
		}
	}
	// Read but not write: 403 for put and for delete-mine, never for get.
	if r := e.put(reader, "career", "company", company1, `{"rating":3}`); r.code != http.StatusForbidden {
		t.Errorf("reader put: %d", r.code)
	}
	if r := e.remove(reader, "career", "company", company1, ""); r.code != http.StatusForbidden {
		t.Errorf("reader delete: %d", r.code)
	}
	if r := e.get(reader, "career", "company", company1, ""); r.code != http.StatusOK {
		t.Errorf("reader get: %d", r.code)
	}
	// A forbidden put never reached storage, and a refused one changed nothing.
	if m := e.get(admin, "career", "company", company1, "").message(); m["total"] != float64(1) {
		t.Errorf("%v", m)
	}
	// Disabled tool: nobody, not even an admin.
	if _, err := e.db.Exec(`UPDATE tools SET enabled = 0 WHERE id = 't1'`); err != nil {
		t.Fatal(err)
	}
	if r := e.get(admin, "career", "company", company1, ""); r.code != http.StatusNotFound {
		t.Errorf("disabled: %d", r.code)
	}
	if r := e.put(admin, "career", "company", company1, `{"rating":3}`); r.code != http.StatusNotFound {
		t.Errorf("disabled put: %d", r.code)
	}
}

func TestDeletingMineIsIdempotentAndOnlyMine(t *testing.T) {
	e := newEnv(t)
	e.rate(t, ada, company1, 4, "")
	e.rate(t, grace, company1, 2, "")
	if r := e.remove(ada, "career", "company", company1, ""); r.code != http.StatusNoContent {
		t.Fatalf("%d %v", r.code, r.out)
	}
	if r := e.remove(ada, "career", "company", company1, ""); r.code != http.StatusNoContent {
		t.Errorf("deleting again: %d", r.code)
	}
	m := e.get(grace, "career", "company", company1, "").message()
	if m["total"] != float64(1) || m["mine"] == nil {
		t.Errorf("only ada's review should be gone: %v", m)
	}
	if r := e.remove(ada, "career", "company", "bad", ""); r.code != http.StatusBadRequest {
		t.Errorf("bad id: %d", r.code)
	}
}

func TestDeletingByIdNeedsTheAuthorOrAnAdmin(t *testing.T) {
	e := newEnv(t)
	adas := e.rate(t, ada, company1, 4, "")
	graces := e.rate(t, grace, company1, 2, "")

	if r := e.remove(ada, "career", "company", company1, "reviewId="+graces); r.code != http.StatusForbidden {
		t.Errorf("another writer: %d", r.code)
	}
	if r := e.remove(reader, "career", "company", company1, "reviewId="+graces); r.code != http.StatusForbidden {
		t.Errorf("a reader: %d", r.code)
	}
	if r := e.remove(ada, "career", "company", company2, "reviewId="+adas); r.code != http.StatusNotFound {
		t.Errorf("through another entity: %d", r.code)
	}
	if r := e.remove(ada, "career", "company", company1, "reviewId=nope"); r.code != http.StatusBadRequest {
		t.Errorf("a malformed id: %d", r.code)
	}
	if r := e.remove(admin, "career", "company", company1, "reviewId="+graces); r.code != http.StatusNoContent {
		t.Errorf("admin: %d", r.code)
	}
	if r := e.remove(admin, "career", "company", company1, "reviewId="+graces); r.code != http.StatusNotFound {
		t.Errorf("twice: %d", r.code)
	}
	if r := e.remove(ada, "career", "company", company1, "reviewId="+adas); r.code != http.StatusNoContent {
		t.Errorf("the author by id: %d", r.code)
	}
	if m := e.get(admin, "career", "company", company1, "").message(); m["total"] != float64(0) {
		t.Errorf("%v", m)
	}
}

func TestAnAuthorWhoLostTheWriteScopeCannotDeleteByIdEither(t *testing.T) {
	e := newEnv(t)
	id := e.rate(t, ada, company1, 4, "")
	lost := who("ada-id", "Ada", "tool:career:read")
	if r := e.remove(lost, "career", "company", company1, "reviewId="+id); r.code != http.StatusForbidden {
		t.Errorf("%d", r.code)
	}
}

func TestSummariesGiveEveryIdAndUseTheTypesScale(t *testing.T) {
	e := newEnv(t)
	e.rate(t, ada, company1, 5, "")
	e.rate(t, grace, company1, 3, "")
	e.rate(t, ada, company2, 2.5, "")
	if r := e.put(ada, "career", "job", company1, `{"rating":9}`); r.code != 200 {
		t.Fatal(r.out)
	}

	r := e.summaries(reader, "career", "company", `{"ids":["`+company1+`","`+company2+`","`+company3+`"]}`)
	if r.code != 200 {
		t.Fatalf("%d %v", r.code, r.out)
	}
	m := r.message()
	s := m["summaries"].(map[string]any)
	one, two, three := s[company1].(map[string]any), s[company2].(map[string]any), s[company3].(map[string]any)
	if m["scale"] != float64(5) || m["halfStars"] != true || len(s) != 3 ||
		one["count"] != float64(2) || one["average"] != 4.0 || two["count"] != float64(1) || two["average"] != 2.5 || three["count"] != float64(0) {
		t.Errorf("%v", m)
	}
	// Another type of the same entity ids is another set of reviews, on its own scale.
	j := e.summaries(reader, "career", "job", `{"ids":["`+company1+`"]}`).message()
	if j["scale"] != float64(10) || j["halfStars"] != false || j["summaries"].(map[string]any)[company1].(map[string]any)["average"] != 9.0 {
		t.Errorf("%v", j)
	}
	for name, body := range map[string]string{"not json": `x`, "bad id": `{"ids":["nope"]}`, "ids is not a list": `{"ids":"x"}`} {
		if r := e.summaries(reader, "career", "company", body); r.code != 400 {
			t.Errorf("%s: %d", name, r.code)
		}
	}
	var many []string
	for i := 0; i < 101; i++ {
		many = append(many, `"`+company1+`"`)
	}
	// 101 copies of one id is one id: the cap counts what was sent.
	if r := e.summaries(reader, "career", "company", `{"ids":[`+strings.Join(many, ",")+`]}`); r.code != 400 {
		t.Errorf("101 ids: %d", r.code)
	}
	if r := e.summaries(reader, "career", "company", `{}`); r.code != 200 || len(r.message()["summaries"].(map[string]any)) != 0 {
		t.Errorf("no ids: %d %v", r.code, r.out)
	}
}

func TestTheTwoOwnersNeverSeeEachOthersReviews(t *testing.T) {
	e := newEnv(t)
	e.rate(t, ada, company1, 5, "")
	if r := e.put(admin, "cinqo", "note", company1, `{"rating":1}`); r.code != 200 {
		t.Fatal(r.out)
	}
	if m := e.get(admin, "cinqo", "note", company1, "").message(); m["total"] != float64(1) || m["scale"] != float64(3) {
		t.Errorf("%v", m)
	}
	if m := e.get(admin, "career", "company", company1, "").message(); m["total"] != float64(1) || m["summary"].(map[string]any)["average"] != 5.0 {
		t.Errorf("%v", m)
	}
}

func TestAReviewOnAnOlderScaleIsShownButNotAveraged(t *testing.T) {
	e := newEnv(t)
	e.rate(t, ada, company1, 5, "")
	// The tool's update changes the type from 5 to 10 stars.
	if err := tool_persistent.NewToolReviewTypePersistentRepo(e.db).ReplaceAll("t1", []tool_entity.ToolReviewType{
		{EntityType: "company", Scale: 10, HalfStars: false, ReadScope: "tool:career:read", WriteScope: "tool:career:write"},
	}); err != nil {
		t.Fatal(err)
	}
	e.put(grace, "career", "company", company1, `{"rating":8}`)
	m := e.get(reader, "career", "company", company1, "").message()
	sum := m["summary"].(map[string]any)
	if m["scale"] != float64(10) || m["total"] != float64(2) || sum["count"] != float64(1) || sum["average"] != 8.0 || sum["otherScale"] != float64(1) {
		t.Errorf("%v", m)
	}
	if mine := e.get(ada, "career", "company", company1, "").message()["mine"].(map[string]any); mine["scale"] != float64(5) || mine["rating"] != float64(5) {
		t.Errorf("her review still says what it was measured on: %v", mine)
	}
}

func TestFailuresOfTheEngineAre500NotPermissionAnswers(t *testing.T) {
	e := newEnv(t)
	logged := 0
	e.d.logf = func(string, ...any) { logged++ }
	e.d.resolver = access.NewResolver(brokenSource{})
	for name, r := range map[string]result{
		"get": e.get(admin, "career", "company", company1, ""), "put": e.put(admin, "career", "company", company1, `{"rating":3}`),
		"delete": e.remove(admin, "career", "company", company1, ""), "summaries": e.summaries(admin, "career", "company", `{}`),
	} {
		if r.code != http.StatusInternalServerError || r.errText() != "request failed" {
			t.Errorf("%s: %d %v", name, r.code, r.out)
		}
	}
	if logged != 4 {
		t.Errorf("each failure is logged: %d", logged)
	}
}

type brokenSource struct{}

func (brokenSource) Lookup(string, string) (access.EntityType, bool, error) {
	return access.EntityType{}, false, os.ErrInvalid
}

// fakeCtx is the least a route needs: a writer, a request, and a DI that holds no JWKS service (so no token can be validated).
type fakeCtx struct {
	request.RequestContext
	w    *httptest.ResponseRecorder
	r    *http.Request
	vars map[string]string
	db   *sql.DB // when set, the DI also gives out this database (what the purge route reads the service token from)
}

type fakeDI struct{ db *sql.DB }

func (f fakeDI) GetDatabaseManager() *orm.DatabaseManager {
	return &orm.DatabaseManager{Connector: &orm.Connector{DB: f.db}}
}
func (f fakeDI) GetLogger() logger.Logger { return nil }

func coreDBFor(t *testing.T) *sql.DB { return coreDB(t) }

func (f fakeCtx) GetWriter() http.ResponseWriter { return f.w }
func (f fakeCtx) GetRequest() *http.Request      { return f.r }
func (f fakeCtx) GetURI() *request.URI           { return &request.URI{PathVariables: f.vars} }
func (f fakeCtx) GetDI() serverdi.Context {
	if f.db == nil {
		return nil
	}
	return fakeDI{db: f.db}
}

func serve(h func(request.RequestContext), method string) (int, string) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(method, "/x", strings.NewReader(`{"rating":3}`))
	r.Header.Set("Authorization", "Bearer not-a-real-token")
	h(fakeCtx{w: rec, r: r, vars: map[string]string{"owner": "career", "entityType": "company", "entityId": company1}})
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	msg, _ := out["message"].(string)
	return rec.Code, msg
}

func TestEachRouteChecksTheMethodTheEngineAndTheToken(t *testing.T) {
	routes := map[string]struct {
		h      func(request.RequestContext)
		method string
	}{
		"get": {GetHandler, http.MethodGet}, "put": {PutHandler, http.MethodPut},
		"delete": {DeleteHandler, http.MethodDelete}, "summaries": {SummariesHandler, http.MethodPost},
	}
	mu.Lock()
	saved := current
	current = nil
	mu.Unlock()
	t.Cleanup(func() { mu.Lock(); current = saved; mu.Unlock() })

	for name, rt := range routes {
		wrong := http.MethodPatch
		if code, _ := serve(rt.h, wrong); code != http.StatusMethodNotAllowed {
			t.Errorf("%s with %s: %d", name, wrong, code)
		}
		if code, msg := serve(rt.h, rt.method); code != http.StatusServiceUnavailable || msg != "reviews are not available" {
			t.Errorf("%s not configured: %d %q", name, code, msg)
		}
	}
	e := newEnv(t)
	Configure(e.d.stores, e.d.resolver, e.d.logf)
	for name, rt := range routes {
		if code, msg := serve(rt.h, rt.method); code != http.StatusUnauthorized || msg != "unauthorized" {
			t.Errorf("%s without a valid token: %d %q", name, code, msg)
		}
	}
}

func TestTheRouteFileIsAuthenticatedWithoutStaticScopesAndEveryExecutorIsRegistered(t *testing.T) {
	yaml, err := os.ReadFile("../../../config/routes/route-reviews.yaml")
	if err != nil {
		t.Fatal(err)
	}
	code, err := os.ReadFile("../../../config/routes/routes.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(yaml)
	var executors []string
	for _, m := range regexp.MustCompile(`(?m)^\s*executor:\s*(\S+)\s*$`).FindAllStringSubmatch(text, -1) {
		executors = append(executors, m[1])
		if !strings.Contains(string(code), `"`+m[1]+`":`) {
			t.Errorf("executor %s has no handler in routes.go", m[1])
		}
	}
	if len(executors) != 5 {
		t.Errorf("%d executors, want 5", len(executors))
	}
	if n := len(regexp.MustCompile(`(?m)^\s*security:\s*authenticated\s*$`).FindAllString(text, -1)); n != 4 {
		t.Errorf("%d authenticated routes, want 4", n)
	}
	// Exactly one public route, the tool-only purge, authenticated inside the handler by the tool's service token.
	if n := len(regexp.MustCompile(`(?m)^\s*security:\s*public\s*$`).FindAllString(text, -1)); n != 1 {
		t.Errorf("%d public routes, want 1 (the purge)", n)
	}
	if !strings.Contains(text, "path: /api/v1/reviews/purge\n    method: POST\n    executor: ReviewsPurge\n    security: public") {
		t.Error("the purge route is missing or not public")
	}
	if regexp.MustCompile(`(?m)^\s*scopes:`).MatchString(text) {
		t.Error("a static scope: the scope depends on the entity type and is resolved in the handler")
	}
	for _, want := range []string{
		"path: /api/v1/reviews/{owner}/{entityType}/{entityId}\n    method: GET\n    executor: ReviewsGet",
		"path: /api/v1/reviews/{owner}/{entityType}/{entityId}\n    method: PUT\n    executor: ReviewsPut",
		"path: /api/v1/reviews/{owner}/{entityType}/{entityId}\n    method: DELETE\n    executor: ReviewsDelete",
		"path: /api/v1/review-summaries/{owner}/{entityType}\n    method: POST\n    executor: ReviewsSummaries",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing route: %s", want)
		}
	}
}

func TestTheTokenComesFromTheCookieOrTheBearerHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	if tokenFromRequest(r) != "" {
		t.Error("no token")
	}
	r.Header.Set("Authorization", "Bearer abc")
	if tokenFromRequest(r) != "abc" {
		t.Error("bearer")
	}
	r.Header.Set("Authorization", "Basic abc")
	if tokenFromRequest(r) != "" {
		t.Error("basic is not a token")
	}
	r.Header.Set("Authorization", "Bearer abc")
	r.AddCookie(&http.Cookie{Name: "access_token", Value: "cookie"})
	if tokenFromRequest(r) != "cookie" {
		t.Error("the cookie wins")
	}
}

func TestTheRoutesReadTheirPathVariablesByName(t *testing.T) {
	o, ty, id := vars(fakeCtx{vars: map[string]string{"owner": "career", "entityType": "company", "entityId": company1}})
	if o != "career" || ty != "company" || id != company1 {
		t.Errorf("%q %q %q", o, ty, id)
	}
}

func TestEachExecutorIsBoundToItsOwnHandler(t *testing.T) {
	code, err := os.ReadFile("../../../config/routes/routes.go")
	if err != nil {
		t.Fatal(err)
	}
	for executor, handlerName := range map[string]string{
		"ReviewsGet": "GetHandler", "ReviewsPut": "PutHandler", "ReviewsPurge": "PurgeHandler", "ReviewsDelete": "DeleteHandler", "ReviewsSummaries": "SummariesHandler",
	} {
		if !regexp.MustCompile(`"` + executor + `":\s+local_routing\.HandlerFunc\(review_handler\.` + handlerName + `\)`).Match(code) {
			t.Errorf("%s is not bound to review_handler.%s", executor, handlerName)
		}
	}
}

// ownerOnlyTheSourceKnows lets a name through that the review files would refuse, to see what the handler says about it.
type ownerOnlyTheSourceKnows struct{}

func (ownerOnlyTheSourceKnows) Lookup(owner, typ string) (access.EntityType, bool, error) {
	return access.EntityType{Owner: owner, Name: typ, Scale: 5, ReadScope: "r", WriteScope: "w"}, true, nil
}

func TestAnOwnerNameTheFilesRefuseIsNotFoundNotAServerError(t *testing.T) {
	e := newEnv(t)
	e.d.resolver = access.NewResolver(ownerOnlyTheSourceKnows{})
	if r := e.get(admin, "../escape", "company", company1, ""); r.code != http.StatusNotFound || r.errText() != "not found" {
		t.Errorf("%d %v", r.code, r.out)
	}
}
