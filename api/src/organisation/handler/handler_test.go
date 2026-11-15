package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/a-digi/coco-server/server/request"

	"github.com/a-digi/cinqo/src/organisation/entity"
	"github.com/a-digi/cinqo/src/organisation/repository/persistent"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	files, _ := filepath.Glob(filepath.Join("..", "..", "..", "config", "db", "migrations", "*.sql"))
	sort.Strings(files)
	for _, f := range files {
		raw, _ := os.ReadFile(f)
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

// call runs a handler body against a real request context; vars are the path variables the router would have filled in.
func call(t *testing.T, fn func(request.RequestContext), method, body string, vars map[string]string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	ctx := request.NewContext(rec, httptest.NewRequest(method, "/x", strings.NewReader(body)), nil)
	for k, v := range vars {
		ctx.GetURI().PathVariables[k] = v
	}
	fn(ctx)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func msg(out map[string]any) map[string]any { m, _ := out["message"].(map[string]any); return m }

func TestListShowsEveryOrganisationWithItsMemberCount(t *testing.T) {
	db := openDB(t)
	p := persistent.NewOrganisationPersistentRepo(db)
	a, _ := p.Create("Acme")
	_ = p.Assign("u1", a.ID)
	_ = p.Assign("u2", a.ID)
	rec, out := call(t, func(c request.RequestContext) { list(db, c) }, "GET", "", nil)
	items, _ := msg(out)["items"].([]any)
	if rec.Code != 200 || len(items) != 2 {
		t.Fatalf("%d %v", rec.Code, out)
	}
	first := items[0].(map[string]any)
	second := items[1].(map[string]any)
	if first["name"] != "Acme" || first["memberCount"] != float64(2) || second["name"] != "Default" || second["memberCount"] != float64(0) || first["status"] != "active" {
		t.Errorf("%v %v", first, second)
	}
}

func TestCreateMakesAnActiveOrganisationAndRefusesABadBody(t *testing.T) {
	db := openDB(t)
	rec, out := call(t, func(c request.RequestContext) { create(db, c) }, "POST", `{"name":"  Acme  "}`, nil)
	m := msg(out)
	if rec.Code != http.StatusCreated || m["name"] != "Acme" || m["status"] != "active" || m["id"] == "" {
		t.Fatalf("%d %v", rec.Code, out)
	}
	for body, want := range map[string]int{`{"name":""}`: 400, `{"name":"   "}`: 400, `nope`: 400, `{}`: 400} {
		if rec, _ := call(t, func(c request.RequestContext) { create(db, c) }, "POST", body, nil); rec.Code != want {
			t.Errorf("%s: %d", body, rec.Code)
		}
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM organisations`).Scan(&n)
	if n != 2 {
		t.Errorf("only the good one was made (plus default): %d", n)
	}
}

func TestUpdateRenamesAndSuspendsAndMapsErrors(t *testing.T) {
	db := openDB(t)
	o, _ := persistent.NewOrganisationPersistentRepo(db).Create("Acme")
	up := func(id, body, caller string) (*httptest.ResponseRecorder, map[string]any) {
		return call(t, func(c request.RequestContext) { update(db, caller, c) }, "PUT", body, map[string]string{"id": id})
	}
	rec, out := up(o.ID, `{"name":"Acme Ltd","status":"suspended"}`, "someone-else")
	if rec.Code != 200 || msg(out)["name"] != "Acme Ltd" || msg(out)["status"] != "suspended" {
		t.Fatalf("%d %v", rec.Code, out)
	}
	for _, c := range []struct {
		id, body string
		want     int
	}{{o.ID, `{"name":"x","status":"paused"}`, 400}, {o.ID, `{"name":" ","status":"active"}`, 400}, {"nope", `{"name":"x","status":"active"}`, 404}, {o.ID, `bad`, 400}} {
		if rec, _ := up(c.id, c.body, ""); rec.Code != c.want {
			t.Errorf("%s %s: %d", c.id, c.body, rec.Code)
		}
	}
}

func TestTheOperatorCannotSuspendTheirOwnOrganisation(t *testing.T) {
	db := openDB(t)
	p := persistent.NewOrganisationPersistentRepo(db)
	mine, _ := p.Create("Mine")
	other, _ := p.Create("Other")
	_ = p.Assign("op", mine.ID)
	up := func(id, status string) int {
		rec, _ := call(t, func(c request.RequestContext) { update(db, "op", c) }, "PUT", `{"name":"n","status":"`+status+`"}`, map[string]string{"id": id})
		return rec.Code
	}
	if code := up(mine.ID, "suspended"); code != http.StatusConflict {
		t.Errorf("own organisation: %d", code)
	}
	if code := up(mine.ID, "active"); code != 200 {
		t.Errorf("renaming and keeping it active is fine: %d", code)
	}
	if code := up(other.ID, "suspended"); code != 200 {
		t.Errorf("someone else's organisation can be suspended: %d", code)
	}
	var st string
	_ = db.QueryRow(`SELECT status FROM organisations WHERE id = ?`, mine.ID).Scan(&st)
	if st != "active" {
		t.Errorf("the refused suspension changed nothing: %s", st)
	}
}

func TestMembersListsTheUsersOfOneOrganisation(t *testing.T) {
	db := openDB(t)
	p := persistent.NewOrganisationPersistentRepo(db)
	o, _ := p.Create("Acme")
	_ = p.Assign("u1", o.ID)
	mem := func(id string) (*httptest.ResponseRecorder, map[string]any) {
		return call(t, func(c request.RequestContext) { members(db, c) }, "GET", "", map[string]string{"id": id})
	}
	rec, out := mem(o.ID)
	items, _ := msg(out)["items"].([]any)
	if rec.Code != 200 || len(items) != 1 || items[0].(map[string]any)["userId"] != "u1" {
		t.Errorf("%d %v", rec.Code, out)
	}
	if rec, out := mem(entity.DefaultID); rec.Code != 200 || len(msg(out)["items"].([]any)) != 0 {
		t.Errorf("an empty organisation answers an empty list: %d %v", rec.Code, out)
	}
	if rec, _ := mem("nope"); rec.Code != 404 {
		t.Errorf("%d", rec.Code)
	}
}

func TestAssignMovesAUserAndMapsErrors(t *testing.T) {
	db := openDB(t)
	p := persistent.NewOrganisationPersistentRepo(db)
	a, _ := p.Create("A")
	b, _ := p.Create("B")
	as := func(user, body, caller string) (*httptest.ResponseRecorder, map[string]any) {
		return call(t, func(c request.RequestContext) { assign(db, caller, c) }, "PUT", body, map[string]string{"sub": user})
	}
	if rec, out := as("u1", `{"orgId":"`+a.ID+`"}`, "op"); rec.Code != 200 || msg(out)["orgId"] != a.ID || msg(out)["userId"] != "u1" {
		t.Fatalf("%d %v", rec.Code, out)
	}
	if rec, _ := as("u1", `{"orgId":"`+b.ID+`"}`, "op"); rec.Code != 200 {
		t.Errorf("a move: %d", rec.Code)
	}
	var n int
	var org string
	_ = db.QueryRow(`SELECT COUNT(*), MAX(org_id) FROM user_organisations WHERE user_id = 'u1'`).Scan(&n, &org)
	if n != 1 || org != b.ID {
		t.Errorf("one membership, in B: %d %s", n, org)
	}
	for _, c := range []struct {
		user, body string
		want       int
	}{{"u1", `{"orgId":"nope"}`, 404}, {"u1", `{}`, 404}, {"u1", `bad`, 400}, {" ", `{"orgId":"` + a.ID + `"}`, 400}, {strings.Repeat("x", 201), `{"orgId":"` + a.ID + `"}`, 400}} {
		if rec, _ := as(c.user, c.body, "op"); rec.Code != c.want {
			t.Errorf("%.20q %s: %d", c.user, c.body, rec.Code)
		}
	}
	if rec, _ := as(strings.Repeat("x", 200), `{"orgId":"`+a.ID+`"}`, "op"); rec.Code != 200 {
		t.Errorf("exactly 200 characters is fine: %d", rec.Code)
	}
}

func TestTheOperatorCannotMoveThemselvesIntoASuspendedOrganisation(t *testing.T) {
	db := openDB(t)
	p := persistent.NewOrganisationPersistentRepo(db)
	s, _ := p.Create("Sus")
	_ = p.Update(s.ID, "Sus", "suspended")
	as := func(user string) int {
		rec, _ := call(t, func(c request.RequestContext) { assign(db, "op", c) }, "PUT", `{"orgId":"`+s.ID+`"}`, map[string]string{"sub": user})
		return rec.Code
	}
	if code := as("op"); code != http.StatusConflict {
		t.Errorf("self: %d", code)
	}
	if code := as("someone"); code != 200 {
		t.Errorf("another user can be put there: %d", code)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM user_organisations WHERE user_id = 'op'`).Scan(&n)
	if n != 0 {
		t.Errorf("the refused move changed nothing: %d", n)
	}
}

func TestACallerWithoutAValidTokenHasNoUserID(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx := request.NewContext(rec, httptest.NewRequest("GET", "/x", nil), nil)
	if callerUserID(ctx) != "" {
		t.Error("no token, no caller")
	}
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer abc")
	if callerUserID(request.NewContext(rec, req, nil)) != "" {
		t.Error("without a validator in the DI nobody is trusted")
	}
}
