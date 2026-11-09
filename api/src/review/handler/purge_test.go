package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/review"
	"github.com/a-digi/cinqo/src/review/access"
	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
	tool_persistent "github.com/a-digi/cinqo/src/tool/repository/persistent"
)

// addTool installs a tool row with a service token and (optionally) a declared "company" type.
func (e *env) addTool(t *testing.T, id, slug, token string, enabled, declareCompany bool) {
	t.Helper()
	if _, err := e.db.Exec(
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, created_at, service_token)
		 VALUES (?, ?, ?, '1.0.0', 'frontend_only', ?, 'running', '/x', '2026-11-09T00:00:00Z', ?)`,
		id, slug, slug, enabled, token,
	); err != nil {
		t.Fatal(err)
	}
	if declareCompany {
		if err := tool_persistent.NewToolReviewTypePersistentRepo(e.db).ReplaceAll(id, []tool_entity.ToolReviewType{
			{EntityType: "company", Scale: 5, HalfStars: true, ReadScope: "tool:" + slug + ":read", WriteScope: "tool:" + slug + ":write"},
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func (e *env) purge(tool, body string) result {
	return run(func(w http.ResponseWriter) {
		e.d.purge(w, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body)), tool)
	})
}

// seed stores a review directly in an owner's own database.
func (e *env) seed(t *testing.T, owner, entityType, entityID, author string) {
	t.Helper()
	s, err := e.d.stores.Store(owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(review.PutReview{EntityType: entityType, EntityID: entityID, Scale: 5, HalfStars: true, Rating: 3, AuthorID: author}); err != nil {
		t.Fatal(err)
	}
}

func (e *env) count(t *testing.T, owner, entityType, entityID string) int {
	t.Helper()
	s, err := e.d.stores.Store(owner)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.List(entityType, entityID, review.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return p.Total
}

func TestToolAuthentication(t *testing.T) {
	e := newEnv(t)
	e.addTool(t, "t9", "other", "secret-token-9", true, true)
	find := toolFinder(e.db)

	req := func(auth string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/x", nil)
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		return r
	}

	if slug, err := toolFromRequest(req("ToolService secret-token-9"), find); err != nil || slug != "other" {
		t.Errorf("a valid token: %q, %v", slug, err)
	}
	if slug, err := toolFromRequest(req("ToolService   secret-token-9  "), find); err != nil || slug != "other" {
		t.Errorf("a padded token: %q, %v", slug, err)
	}
	for name, auth := range map[string]string{
		"no header":         "",
		"a user's bearer":   "Bearer secret-token-9",
		"wrong scheme case": "toolservice secret-token-9",
		"scheme only":       "ToolService ",
		"blank token":       "ToolService    ",
	} {
		if _, err := toolFromRequest(req(auth), find); !errors.Is(err, errNoServiceToken) {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, auth := range map[string]string{"unknown token": "ToolService nope", "a prefix": "ToolService secret-token"} {
		if _, err := toolFromRequest(req(auth), find); !errors.Is(err, errBadServiceToken) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A tool row that has no service token (empty string) must never match "no token".
	e.addTool(t, "t10", "legacy", "", true, true)
	if _, err := toolFromRequest(req("ToolService x"), find); !errors.Is(err, errBadServiceToken) {
		t.Errorf("a tokenless tool matched: %v", err)
	}
}

func TestPurgeRemovesOnlyTheCallingToolsReviewsOfThatEntity(t *testing.T) {
	e := newEnv(t) // tool "career" declares "company" and "job"
	e.addTool(t, "t9", "other", "tok", true, true)

	e.seed(t, "career", "company", company1, "u1")
	e.seed(t, "career", "company", company1, "u2")
	e.seed(t, "career", "company", company2, "u1")
	e.seed(t, "career", "job", company1, "u1")
	e.seed(t, "other", "company", company1, "u1")
	e.seed(t, "cinqo", "note", company1, "u1")

	r := e.purge("career", `{"entityType":"company","entityId":"`+company1+`"}`)
	if r.code != http.StatusOK || r.message()["deleted"] != float64(2) {
		t.Fatalf("%d %v", r.code, r.out)
	}
	for name, c := range map[string]struct {
		got, want int
	}{
		"career, purged entity":  {e.count(t, "career", "company", company1), 0},
		"career, another entity": {e.count(t, "career", "company", company2), 1},
		"career, another type":   {e.count(t, "career", "job", company1), 1},
		"another tool's":         {e.count(t, "other", "company", company1), 1},
		"the system's":           {e.count(t, "cinqo", "note", company1), 1},
	} {
		if c.got != c.want {
			t.Errorf("%s: %d reviews, want %d", name, c.got, c.want)
		}
	}

	// Idempotent: purging again, or an entity that never had reviews, is a 200 with 0.
	for _, id := range []string{company1, company3} {
		if r := e.purge("career", `{"entityType":"company","entityId":"`+id+`"}`); r.code != http.StatusOK || r.message()["deleted"] != float64(0) {
			t.Errorf("repeat purge: %d %v", r.code, r.out)
		}
	}
}

func TestPurgeIgnoresAnyOwnerInTheRequest(t *testing.T) {
	e := newEnv(t)
	e.addTool(t, "t9", "other", "tok", true, true)
	e.seed(t, "other", "company", company1, "u1")

	r := e.purge("career", `{"entityType":"company","entityId":"`+company1+`","owner":"other","tool":"other"}`)
	if r.code != http.StatusOK {
		t.Fatalf("%d %v", r.code, r.out)
	}
	if e.count(t, "other", "company", company1) != 1 {
		t.Error("a tool purged another tool's reviews by naming it in the body")
	}
}

func TestPurgeRefusals(t *testing.T) {
	e := newEnv(t)
	e.addTool(t, "t8", "silent", "tok8", true, false) // declares nothing
	e.addTool(t, "t7", "asleep", "tok7", false, true) // declared, but disabled
	e.addTool(t, "t6", "cinqo", "tok6", true, true)   // a tool squatting the system's owner name
	e.seed(t, "career", "company", company1, "u1")
	e.seed(t, "cinqo", "note", company1, "u1")

	for name, tc := range map[string]struct{ tool, body string }{
		"a type the tool did not declare":  {"career", `{"entityType":"note","entityId":"` + company1 + `"}`},
		"a tool that declared nothing":     {"silent", `{"entityType":"company","entityId":"` + company1 + `"}`},
		"a disabled tool":                  {"asleep", `{"entityType":"company","entityId":"` + company1 + `"}`},
		"the reserved system owner name":   {"cinqo", `{"entityType":"note","entityId":"` + company1 + `"}`},
		"the system owner and a tool type": {"cinqo", `{"entityType":"company","entityId":"` + company1 + `"}`},
	} {
		if r := e.purge(tc.tool, tc.body); r.code != http.StatusForbidden {
			t.Errorf("%s: %d %v", name, r.code, r.out)
		}
	}
	if e.count(t, "career", "company", company1) != 1 || e.count(t, "cinqo", "note", company1) != 1 {
		t.Error("a refused purge removed something")
	}
	for name, body := range map[string]string{
		"not json": `nope`, "empty": ``, "bad entity id": `{"entityType":"company","entityId":"123"}`,
		"no entity id": `{"entityType":"company"}`, "no entity type": `{"entityId":"` + company1 + `"}`,
		"bad entity type": `{"entityType":"Company","entityId":"` + company1 + `"}`,
	} {
		// An empty or malformed entity type is "not declared" (403); a malformed id or body is a 400.
		if r := e.purge("career", body); r.code != http.StatusBadRequest && r.code != http.StatusForbidden {
			t.Errorf("%s: %d %v", name, r.code, r.out)
		}
	}
	if r := e.purge("career", `{"entityType":"company","entityId":"123"}`); r.code != http.StatusBadRequest || !strings.HasPrefix(r.errText(), "entityId:") {
		t.Errorf("a malformed entity id must be a 400: %d %v", r.code, r.out)
	}
	if r := e.purge("career", `nope`); r.code != http.StatusBadRequest {
		t.Errorf("a malformed body must be a 400: %d", r.code)
	}
	huge := `{"entityType":"company","entityId":"` + company1 + `","x":"` + strings.Repeat("y", 70*1024) + `"}`
	if r := e.purge("career", huge); r.code != http.StatusBadRequest || !strings.HasPrefix(r.errText(), "body must be") {
		t.Errorf("a body over the size limit: %d %v", r.code, r.out)
	}
}

func TestPurgeLookupFailureIs500(t *testing.T) {
	var logged int
	d := &deps{
		stores:   review.NewManager(t.TempDir()),
		resolver: access.NewResolver(brokenSource{}),
		logf:     func(string, ...any) { logged++ },
	}
	r := run(func(w http.ResponseWriter) {
		d.purge(w, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"entityType":"company","entityId":"`+company1+`"}`)), "career")
	})
	if r.code != http.StatusInternalServerError || logged == 0 {
		t.Errorf("%d %v (logged %d)", r.code, r.out, logged)
	}
}

func TestThePurgeRouteChecksMethodEngineAndToken(t *testing.T) {
	serveWith := func(method, auth string) (int, string) {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(method, "/x", strings.NewReader(`{}`))
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		PurgeHandler(fakeCtx{w: rec, r: r, db: coreDBFor(t)})
		return rec.Code, rec.Body.String()
	}
	mu.Lock()
	saved := current
	current = nil
	mu.Unlock()
	t.Cleanup(func() { mu.Lock(); current = saved; mu.Unlock() })

	if code, _ := serveWith(http.MethodGet, ""); code != http.StatusMethodNotAllowed {
		t.Errorf("wrong method: %d", code)
	}
	if code, body := serveWith(http.MethodPost, ""); code != http.StatusServiceUnavailable || !strings.Contains(body, "reviews are not available") {
		t.Errorf("not configured: %d %s", code, body)
	}
	e := newEnv(t)
	Configure(e.d.stores, e.d.resolver, e.d.logf)
	if code, _ := serveWith(http.MethodPost, ""); code != http.StatusUnauthorized {
		t.Errorf("no token: %d", code)
	}
	if code, _ := serveWith(http.MethodPost, "ToolService nope"); code != http.StatusUnauthorized {
		t.Errorf("unknown token: %d", code)
	}
	if code, _ := serveWith(http.MethodPost, "Bearer abc"); code != http.StatusUnauthorized {
		t.Errorf("a user's token is not a service token: %d", code)
	}
}

func TestAFailureToLookUpTheToolIs500NotAnAuthenticationAnswer(t *testing.T) {
	e := newEnv(t)
	logged := 0
	e.d.logf = func(string, ...any) { logged++ }
	mu.Lock()
	saved := current
	mu.Unlock()
	Configure(e.d.stores, e.d.resolver, e.d.logf)
	t.Cleanup(func() { mu.Lock(); current = saved; mu.Unlock() })

	broken := coreDB(t)
	broken.Close() // every query on it now fails
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`))
	r.Header.Set("Authorization", "ToolService some-token")
	PurgeHandler(fakeCtx{w: rec, r: r, db: broken})
	if rec.Code != http.StatusInternalServerError || logged != 1 || !strings.Contains(rec.Body.String(), "failed to authenticate") {
		t.Errorf("%d %s (logged %d)", rec.Code, rec.Body.String(), logged)
	}
}
