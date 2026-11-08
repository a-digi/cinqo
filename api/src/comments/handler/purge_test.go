package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/comments"
	"github.com/a-digi/cinqo/src/comments/access"
	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
	tool_persistent "github.com/a-digi/cinqo/src/tool/repository/persistent"
)

// addTool installs a tool row with a service token and (optionally) a declared "job" type.
func (e *env) addTool(t *testing.T, id, slug, token string, enabled, declareJob bool) {
	t.Helper()
	if _, err := e.db.Exec(
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, created_at, service_token)
		 VALUES (?, ?, ?, '1.0.0', 'frontend_only', ?, 'running', '/x', '2026-10-30T00:00:00Z', ?)`,
		id, slug, slug, enabled, token,
	); err != nil {
		t.Fatal(err)
	}
	if declareJob {
		if err := tool_persistent.NewToolCommentTypePersistentRepo(e.db).ReplaceAll(id, []tool_entity.ToolCommentType{
			{EntityType: "job", ReadScope: "tool:" + slug + ":read", WriteScope: "tool:" + slug + ":write"},
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

// seed stores a comment directly in an owner's own database.
func (e *env) seed(t *testing.T, owner, entityType, entityID, text string) {
	t.Helper()
	s, err := e.d.stores.Store(owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(comments.NewComment{EntityType: entityType, EntityID: entityID, Body: text, AuthorID: "u"}); err != nil {
		t.Fatal(err)
	}
}

func (e *env) count(t *testing.T, owner, entityType, entityID string) int {
	t.Helper()
	s, err := e.d.stores.Store(owner)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.List(entityType, entityID, comments.ListOptions{})
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
	for name, auth := range map[string]string{
		"unknown token":        "ToolService nope",
		"a prefix":             "ToolService secret-token",
		"the empty-token tool": "ToolService x",
	} {
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

func TestPurgeRemovesOnlyTheCallingToolsComments(t *testing.T) {
	e := newEnv(t) // tool "career" declares "job"
	e.addTool(t, "t9", "other", "tok", true, true)

	e.seed(t, "career", "job", job1, "career 1")
	e.seed(t, "career", "job", job1, "career 2")
	e.seed(t, "career", "job", job2, "career other job")
	e.seed(t, "other", "job", job1, "the other tool's, same entity id")
	e.seed(t, "cinqo", "note", job1, "the system's, same entity id")

	r := e.purge("career", `{"entityType":"job","entityId":"`+job1+`"}`)
	if r.code != http.StatusOK || r.message()["deleted"] != float64(2) {
		t.Fatalf("%d %v", r.code, r.out)
	}
	for name, got := range map[string]int{
		"career, purged entity": e.count(t, "career", "job", job1),
		"career, another job":   e.count(t, "career", "job", job2),
		"another tool's":        e.count(t, "other", "job", job1),
		"the system's":          e.count(t, "cinqo", "note", job1),
	} {
		want := map[string]int{"career, purged entity": 0, "career, another job": 1, "another tool's": 1, "the system's": 1}[name]
		if got != want {
			t.Errorf("%s: %d comments, want %d", name, got, want)
		}
	}

	// Idempotent: purging again, or an entity that never had comments, is a 200 with 0.
	for _, id := range []string{job1, "3bbbbbbb-0831-448c-8962-2851188e9de9"} {
		if r := e.purge("career", `{"entityType":"job","entityId":"`+id+`"}`); r.code != http.StatusOK || r.message()["deleted"] != float64(0) {
			t.Errorf("repeat purge: %d %v", r.code, r.out)
		}
	}
}

func TestPurgeIgnoresAnyOwnerInTheRequest(t *testing.T) {
	e := newEnv(t)
	e.addTool(t, "t9", "other", "tok", true, true)
	e.seed(t, "other", "job", job1, "must survive")

	r := e.purge("career", `{"entityType":"job","entityId":"`+job1+`","owner":"other","tool":"other"}`)
	if r.code != http.StatusOK {
		t.Fatalf("%d %v", r.code, r.out)
	}
	if e.count(t, "other", "job", job1) != 1 {
		t.Error("a tool purged another tool's comments by naming it in the body")
	}
}

func TestPurgeRefusals(t *testing.T) {
	e := newEnv(t)
	e.addTool(t, "t8", "silent", "tok8", true, false) // declares nothing
	e.addTool(t, "t7", "asleep", "tok7", false, true) // declared, but disabled
	e.addTool(t, "t6", "cinqo", "tok6", true, true)   // a tool squatting the system's owner name
	e.seed(t, "career", "job", job1, "x")
	e.seed(t, "cinqo", "note", job1, "system comment")

	for name, tc := range map[string]struct {
		tool, body string
	}{
		"a type the tool did not declare": {"career", `{"entityType":"note","entityId":"` + job1 + `"}`},
		"a tool that declared nothing":    {"silent", `{"entityType":"job","entityId":"` + job1 + `"}`},
		"a disabled tool":                 {"asleep", `{"entityType":"job","entityId":"` + job1 + `"}`},
		"the reserved system owner name":  {"cinqo", `{"entityType":"note","entityId":"` + job1 + `"}`},
		"the system owner and a job type": {"cinqo", `{"entityType":"job","entityId":"` + job1 + `"}`},
	} {
		if r := e.purge(tc.tool, tc.body); r.code != http.StatusForbidden {
			t.Errorf("%s: %d %v", name, r.code, r.out)
		}
	}
	if e.count(t, "career", "job", job1) != 1 || e.count(t, "cinqo", "note", job1) != 1 {
		t.Error("a refused purge removed something")
	}

	for name, body := range map[string]string{
		"not json":        `nope`,
		"empty":           ``,
		"bad entity id":   `{"entityType":"job","entityId":"123"}`,
		"no entity id":    `{"entityType":"job"}`,
		"no entity type":  `{"entityId":"` + job1 + `"}`,
		"bad entity type": `{"entityType":"Job","entityId":"` + job1 + `"}`,
	} {
		r := e.purge("career", body)
		// An empty or malformed entity type is "not declared" (403); a malformed id is a 400.
		if r.code != http.StatusBadRequest && r.code != http.StatusForbidden {
			t.Errorf("%s: %d %v", name, r.code, r.out)
		}
	}
	if r := e.purge("career", `{"entityType":"job","entityId":"123"}`); r.code != http.StatusBadRequest {
		t.Errorf("a malformed entity id must be a 400: %d", r.code)
	}
	if r := e.purge("career", `nope`); r.code != http.StatusBadRequest {
		t.Errorf("a malformed body must be a 400: %d", r.code)
	}
}

func TestPurgeLookupFailureIs500(t *testing.T) {
	var logged int
	d := &deps{
		stores:   comments.NewManager(t.TempDir()),
		resolver: access.NewResolver(brokenSource{}),
		logf:     func(string, ...any) { logged++ },
	}
	r := run(func(w http.ResponseWriter) {
		d.purge(w, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"entityType":"job","entityId":"`+job1+`"}`)), "career")
	})
	if r.code != http.StatusInternalServerError || logged == 0 {
		t.Errorf("%d %v (logged %d)", r.code, r.out, logged)
	}
}
