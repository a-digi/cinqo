package access

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/a-digi/cinqo/src/comments"
	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
	tool_persistent "github.com/a-digi/cinqo/src/tool/repository/persistent"
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

func addTool(t *testing.T, db *sql.DB, id, slug string, enabled bool) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, created_at)
		 VALUES (?, ?, ?, '1.0.0', 'frontend_only', ?, 'running', '/x', '2026-10-30T00:00:00Z')`,
		id, slug, slug, enabled,
	); err != nil {
		t.Fatal(err)
	}
}

func declare(t *testing.T, db *sql.DB, toolID string, types ...tool_entity.ToolCommentType) {
	t.Helper()
	if err := tool_persistent.NewToolCommentTypePersistentRepo(db).ReplaceAll(toolID, types); err != nil {
		t.Fatal(err)
	}
}

func jobType(read, write string) tool_entity.ToolCommentType {
	return tool_entity.ToolCommentType{EntityType: "job", ReadScope: read, WriteScope: write}
}

func rows(t *testing.T, db *sql.DB, toolID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tool_comment_types WHERE tool_id = ?`, toolID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

var (
	reader = Caller{UserID: "r", Scopes: []string{"tool:career:read"}}
	writer = Caller{UserID: "w", Scopes: []string{"tool:career:read", "tool:career:write"}}
	admin  = Caller{UserID: "root", Scopes: []string{AdminScope}}
	nobody = Caller{UserID: "n"}
)

func TestSystemTypesRegister(t *testing.T) {
	s := NewSystemTypes()
	if err := s.Register("media", AdminScope, AdminScope); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][3]string{
		"duplicate":  {"media", "a", "b"},
		"bad name":   {"Media", "a", "b"},
		"empty name": {"", "a", "b"},
		"no read":    {"x", "", "b"},
		"no write":   {"x", "a", ""},
		"path-like":  {"../x", "a", "b"},
	} {
		if err := s.Register(args[0], args[1], args[2]); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if got, ok, _ := s.Lookup("cinqo", "media"); !ok || got.ReadScope != AdminScope || got.Owner != "cinqo" {
		t.Errorf("lookup = %+v, %v", got, ok)
	}
	if _, ok, _ := s.Lookup("career", "media"); ok {
		t.Error("a system type resolved for a tool owner")
	}
	if _, ok, _ := s.Lookup("cinqo", "job"); ok {
		t.Error("an unregistered type resolved")
	}
}

func TestToolTypesFollowTheToolsState(t *testing.T) {
	db := coreDB(t)
	addTool(t, db, "t1", "career", true)
	addTool(t, db, "t2", "other", true)
	declare(t, db, "t1", jobType("tool:career:read", "tool:career:write"))
	tt := NewToolTypes(db)

	got, ok, err := tt.Lookup("career", "job")
	if err != nil || !ok || got.ReadScope != "tool:career:read" || got.WriteScope != "tool:career:write" || got.Owner != "career" {
		t.Fatalf("lookup = %+v, %v, %v", got, ok, err)
	}
	// Another tool does not have it, and neither does an undeclared type.
	if _, ok, _ := tt.Lookup("other", "job"); ok {
		t.Error("a type resolved under a tool that did not declare it")
	}
	if _, ok, _ := tt.Lookup("career", "note"); ok {
		t.Error("an undeclared type resolved")
	}
	if _, ok, _ := tt.Lookup("ghost", "job"); ok {
		t.Error("an unknown tool resolved")
	}
	// The system owner name never reaches a tool, even one installed under that slug.
	addTool(t, db, "t3", "cinqo", true)
	declare(t, db, "t3", jobType("a", "b"))
	if _, ok, _ := tt.Lookup("cinqo", "job"); ok {
		t.Error(`a tool with the slug "cinqo" was treated as the system`)
	}

	// A disabled tool's comments are not reachable; enabling brings them back.
	if _, err := db.Exec(`UPDATE tools SET enabled = 0 WHERE id = 't1'`); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := tt.Lookup("career", "job"); ok {
		t.Error("a disabled tool's type resolved")
	}
	if _, err := db.Exec(`UPDATE tools SET enabled = 1 WHERE id = 't1'`); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := tt.Lookup("career", "job"); !ok {
		t.Error("re-enabling did not bring the type back")
	}
}

func TestReplaceAllIsReplaceNotMerge(t *testing.T) {
	db := coreDB(t)
	addTool(t, db, "t1", "career", true)
	declare(t, db, "t1", jobType("r", "w"), tool_entity.ToolCommentType{EntityType: "company", ReadScope: "r", WriteScope: "w"})
	if rows(t, db, "t1") != 2 {
		t.Fatal("expected two types")
	}
	declare(t, db, "t1", jobType("r2", "w2")) // an update declares one, with new scopes
	tt := NewToolTypes(db)
	if got, _, _ := tt.Lookup("career", "job"); got.ReadScope != "r2" {
		t.Errorf("scopes not replaced: %+v", got)
	}
	if _, ok, _ := tt.Lookup("career", "company"); ok {
		t.Error("a type dropped by an update still resolves")
	}
	declare(t, db, "t1") // an update declaring none clears everything
	if rows(t, db, "t1") != 0 {
		t.Error("an update with no comments must clear the types")
	}
}

func TestToolDeleteRemovesTheDeclaration(t *testing.T) {
	db := coreDB(t)
	addTool(t, db, "t1", "career", true)
	declare(t, db, "t1", jobType("r", "w"))
	if err := tool_persistent.NewToolPersistentRepo(db).Delete("t1"); err != nil {
		t.Fatal(err)
	}
	if rows(t, db, "t1") != 0 {
		t.Error("types left after uninstall")
	}
}

func TestAuthorize(t *testing.T) {
	db := coreDB(t)
	addTool(t, db, "t1", "career", true)
	declare(t, db, "t1", jobType("tool:career:read", "tool:career:write"))
	sys := NewSystemTypes()
	if err := sys.Register("media", "cinqo:media:read", "cinqo:media:write"); err != nil {
		t.Fatal(err)
	}
	r := NewResolver(sys, NewToolTypes(db))

	for name, tc := range map[string]struct {
		caller Caller
		owner  string
		typ    string
		action Action
		want   error
	}{
		"reader reads":                   {reader, "career", "job", Read, nil},
		"reader cannot create":           {reader, "career", "job", Create, ErrForbidden},
		"writer reads":                   {writer, "career", "job", Read, nil},
		"writer creates":                 {writer, "career", "job", Create, nil},
		"admin reads":                    {admin, "career", "job", Read, nil},
		"admin creates":                  {admin, "career", "job", Create, nil},
		"no scope is not found (read)":   {nobody, "career", "job", Read, ErrNotFound},
		"no scope is not found (create)": {nobody, "career", "job", Create, ErrNotFound},
		"unknown type":                   {admin, "career", "note", Read, ErrNotFound},
		"unknown owner":                  {admin, "ghost", "job", Read, ErrNotFound},
		"a type of another owner":        {admin, "cinqo", "job", Read, ErrNotFound},
		"system type, admin":             {admin, "cinqo", "media", Create, nil},
		"system type, scoped reader":     {Caller{UserID: "m", Scopes: []string{"cinqo:media:read"}}, "cinqo", "media", Read, nil},
		"system type, no scope":          {nobody, "cinqo", "media", Read, ErrNotFound},
		"tool scope on a system type":    {writer, "cinqo", "media", Read, ErrNotFound},
	} {
		got, err := r.Authorize(tc.caller, tc.owner, tc.typ, tc.action)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
			continue
		}
		if tc.want == nil && (got.Owner != tc.owner || got.Name != tc.typ) {
			t.Errorf("%s: returned %+v", name, got)
		}
	}

	// Once the tool is disabled, nobody — not even an admin — reaches its comments.
	if _, err := db.Exec(`UPDATE tools SET enabled = 0 WHERE id = 't1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Authorize(admin, "career", "job", Read); !errors.Is(err, ErrNotFound) {
		t.Errorf("disabled tool: %v", err)
	}
}

type failing struct{}

func (failing) Lookup(string, string) (EntityType, bool, error) {
	return EntityType{}, false, errors.New("database is down")
}

func TestAuthorizeSurfacesLookupFailuresAndStopsAtTheFirstMatch(t *testing.T) {
	if _, err := NewResolver(failing{}).Authorize(admin, "career", "job", Read); err == nil || errors.Is(err, ErrNotFound) || errors.Is(err, ErrForbidden) {
		t.Errorf("a lookup failure must not look like a permission answer: %v", err)
	}

	first, second := NewSystemTypes(), NewSystemTypes()
	_ = first.Register("media", "first:read", "first:write")
	_ = second.Register("media", "second:read", "second:write")
	got, err := NewResolver(first, second).Authorize(Caller{UserID: "u", Scopes: []string{"first:read"}}, "cinqo", "media", Read)
	if err != nil || got.ReadScope != "first:read" {
		t.Errorf("the first source must win: %+v, %v", got, err)
	}
}

func TestEditAndDeleteRules(t *testing.T) {
	ty := EntityType{Owner: "career", Name: "job", ReadScope: "tool:career:read", WriteScope: "tool:career:write"}
	byWriter := comments.Comment{AuthorID: "w"}
	byOther := comments.Comment{AuthorID: "someone-else"}
	lostScope := Caller{UserID: "w", Scopes: []string{"tool:career:read"}} // the author, but no longer allowed to write

	for name, tc := range map[string]struct {
		caller       Caller
		c            comments.Comment
		edit, delete bool
	}{
		"author with write scope":       {writer, byWriter, true, true},
		"author lost the write scope":   {lostScope, byWriter, false, false},
		"another writer":                {writer, byOther, false, false},
		"admin on someone else's":       {admin, byOther, false, true},
		"admin on their own":            {Caller{UserID: "someone-else", Scopes: []string{AdminScope}}, byOther, true, true},
		"reader on their own":           {Caller{UserID: "w", Scopes: []string{"tool:career:read"}}, byWriter, false, false},
		"anonymous never matches empty": {Caller{Scopes: []string{"tool:career:write"}}, comments.Comment{}, false, false},
	} {
		if got := CanEdit(ty, tc.caller, tc.c); got != tc.edit {
			t.Errorf("%s: CanEdit = %v, want %v", name, got, tc.edit)
		}
		if got := CanDelete(ty, tc.caller, tc.c); got != tc.delete {
			t.Errorf("%s: CanDelete = %v, want %v", name, got, tc.delete)
		}
	}
}

func TestResolverFindNeedsNoPermission(t *testing.T) {
	db := coreDB(t)
	addTool(t, db, "t1", "career", true)
	declare(t, db, "t1", jobType("tool:career:read", "tool:career:write"))
	sys := NewSystemTypes()
	_ = sys.Register("media", "m:read", "m:write")
	r := NewResolver(sys, NewToolTypes(db))

	if got, ok, err := r.Find("career", "job"); err != nil || !ok || got.Owner != "career" || got.WriteScope != "tool:career:write" {
		t.Errorf("tool type: %+v, %v, %v", got, ok, err)
	}
	if got, ok, _ := r.Find("cinqo", "media"); !ok || got.Owner != "cinqo" {
		t.Errorf("system type: %+v, %v", got, ok)
	}
	for _, c := range [][2]string{{"career", "note"}, {"ghost", "job"}, {"cinqo", "job"}, {"career", "media"}} {
		if _, ok, _ := r.Find(c[0], c[1]); ok {
			t.Errorf("Find(%q, %q) found something", c[0], c[1])
		}
	}
	if _, _, err := NewResolver(failing{}).Find("career", "job"); err == nil {
		t.Error("a lookup failure was swallowed")
	}
	if _, err := db.Exec(`UPDATE tools SET enabled = 0 WHERE id = 't1'`); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := r.Find("career", "job"); ok {
		t.Error("a disabled tool's type was found")
	}
}
