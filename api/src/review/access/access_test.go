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

	"github.com/a-digi/cinqo/src/review"
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
		 VALUES (?, ?, ?, '1.0.0', 'frontend_only', ?, 'running', '/x', '2026-11-09T00:00:00Z')`,
		id, slug, slug, enabled,
	); err != nil {
		t.Fatal(err)
	}
}

func declare(t *testing.T, db *sql.DB, toolID string, types ...tool_entity.ToolReviewType) {
	t.Helper()
	if err := tool_persistent.NewToolReviewTypePersistentRepo(db).ReplaceAll(toolID, types); err != nil {
		t.Fatal(err)
	}
}

func companyType(read, write string) tool_entity.ToolReviewType {
	return tool_entity.ToolReviewType{EntityType: "company", Scale: 5, HalfStars: true, ReadScope: read, WriteScope: write}
}

func rows(t *testing.T, db *sql.DB, toolID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tool_review_types WHERE tool_id = ?`, toolID).Scan(&n); err != nil {
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
	if err := s.Register("media", 10, false, AdminScope, AdminScope); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		name        string
		scale       int
		read, write string
	}{
		"duplicate":  {"media", 5, "a", "b"},
		"bad name":   {"Media", 5, "a", "b"},
		"empty name": {"", 5, "a", "b"},
		"no read":    {"x", 5, "", "b"},
		"no write":   {"x", 5, "a", ""},
		"path-like":  {"../x", 5, "a", "b"},
		"scale 4":    {"x", 4, "a", "b"},
		"scale 0":    {"x", 0, "a", "b"},
		"scale 11":   {"x", 11, "a", "b"},
	} {
		if err := s.Register(c.name, c.scale, true, c.read, c.write); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	for _, scale := range []int{3, 5, 10} {
		if err := NewSystemTypes().Register("x", scale, true, "a", "b"); err != nil {
			t.Errorf("scale %d: %v", scale, err)
		}
	}
	if got, ok, _ := s.Lookup("cinqo", "media"); !ok || got.ReadScope != AdminScope || got.Owner != "cinqo" || got.Scale != 10 || got.HalfStars {
		t.Errorf("lookup = %+v, %v", got, ok)
	}
	if _, ok, _ := s.Lookup("career", "media"); ok {
		t.Error("a system type resolved for a tool owner")
	}
	if _, ok, _ := s.Lookup("cinqo", "company"); ok {
		t.Error("an unregistered type resolved")
	}
}

func TestToolTypesFollowTheToolsState(t *testing.T) {
	db := coreDB(t)
	addTool(t, db, "t1", "career", true)
	addTool(t, db, "t2", "other", true)
	declare(t, db, "t1", companyType("tool:career:read", "tool:career:write"))
	tt := NewToolTypes(db)

	got, ok, err := tt.Lookup("career", "company")
	if err != nil || !ok || got.ReadScope != "tool:career:read" || got.WriteScope != "tool:career:write" || got.Owner != "career" || got.Scale != 5 || !got.HalfStars || got.Name != "company" {
		t.Fatalf("lookup = %+v, %v, %v", got, ok, err)
	}
	if _, ok, _ := tt.Lookup("other", "company"); ok {
		t.Error("a type resolved under a tool that did not declare it")
	}
	if _, ok, _ := tt.Lookup("career", "job"); ok {
		t.Error("an undeclared type resolved")
	}
	if _, ok, _ := tt.Lookup("ghost", "company"); ok {
		t.Error("an unknown tool resolved")
	}
	// The system owner name never reaches a tool, even one installed under that slug.
	addTool(t, db, "t3", "cinqo", true)
	declare(t, db, "t3", companyType("a", "b"))
	if _, ok, _ := tt.Lookup("cinqo", "company"); ok {
		t.Error(`a tool with the slug "cinqo" was treated as the system`)
	}

	// A disabled tool's reviews are not reachable; enabling brings them back.
	if _, err := db.Exec(`UPDATE tools SET enabled = 0 WHERE id = 't1'`); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := tt.Lookup("career", "company"); ok {
		t.Error("a disabled tool's type resolved")
	}
	if _, err := db.Exec(`UPDATE tools SET enabled = 1 WHERE id = 't1'`); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := tt.Lookup("career", "company"); !ok {
		t.Error("re-enabling did not bring the type back")
	}
}

func TestTheScaleAndTheHalfStarFlagAreStoredAsDeclared(t *testing.T) {
	db := coreDB(t)
	addTool(t, db, "t1", "career", true)
	declare(t, db, "t1",
		tool_entity.ToolReviewType{EntityType: "company", Scale: 5, HalfStars: true, ReadScope: "r", WriteScope: "w"},
		tool_entity.ToolReviewType{EntityType: "job", Scale: 10, HalfStars: false, ReadScope: "r", WriteScope: "w"},
		tool_entity.ToolReviewType{EntityType: "persona", Scale: 3, HalfStars: true, ReadScope: "r", WriteScope: "w"},
	)
	tt := NewToolTypes(db)
	for typ, want := range map[string][2]any{"company": {5, true}, "job": {10, false}, "persona": {3, true}} {
		got, ok, _ := tt.Lookup("career", typ)
		if !ok || got.Scale != want[0] || got.HalfStars != want[1] {
			t.Errorf("%s: %+v", typ, got)
		}
	}
	// The table itself refuses a scale that is not 3, 5 or 10.
	if _, err := db.Exec(`INSERT INTO tool_review_types (tool_id, entity_type, scale, half_stars, read_scope, write_scope) VALUES ('t1','x',4,0,'r','w')`); err == nil {
		t.Error("a scale of 4 was stored")
	}
}

func TestReplaceAllIsReplaceNotMerge(t *testing.T) {
	db := coreDB(t)
	addTool(t, db, "t1", "career", true)
	declare(t, db, "t1", companyType("r", "w"), tool_entity.ToolReviewType{EntityType: "job", Scale: 5, ReadScope: "r", WriteScope: "w"})
	if rows(t, db, "t1") != 2 {
		t.Fatal("expected two types")
	}
	next := companyType("r2", "w2")
	next.Scale, next.HalfStars = 10, false
	declare(t, db, "t1", next) // an update declares one, with new scopes and scale
	tt := NewToolTypes(db)
	if got, _, _ := tt.Lookup("career", "company"); got.ReadScope != "r2" || got.Scale != 10 || got.HalfStars {
		t.Errorf("not replaced: %+v", got)
	}
	if _, ok, _ := tt.Lookup("career", "job"); ok {
		t.Error("a type dropped by an update still resolves")
	}
	declare(t, db, "t1") // an update declaring none clears everything
	if rows(t, db, "t1") != 0 {
		t.Error("an update with no reviews must clear the types")
	}
	// Another tool's declaration is never touched.
	addTool(t, db, "t2", "other", true)
	declare(t, db, "t2", companyType("r", "w"))
	declare(t, db, "t1")
	if rows(t, db, "t2") != 1 {
		t.Error("replacing one tool's types removed another's")
	}
}

func TestToolDeleteRemovesTheDeclarationOnly(t *testing.T) {
	db := coreDB(t)
	addTool(t, db, "t1", "career", true)
	addTool(t, db, "t2", "other", true)
	declare(t, db, "t1", companyType("r", "w"))
	declare(t, db, "t2", companyType("r", "w"))
	if err := tool_persistent.NewToolPersistentRepo(db).Delete("t1"); err != nil {
		t.Fatal(err)
	}
	if rows(t, db, "t1") != 0 {
		t.Error("types left after uninstall")
	}
	if rows(t, db, "t2") != 1 {
		t.Error("another tool's types were removed")
	}
}

func TestAuthorize(t *testing.T) {
	db := coreDB(t)
	addTool(t, db, "t1", "career", true)
	declare(t, db, "t1", companyType("tool:career:read", "tool:career:write"))
	sys := NewSystemTypes()
	if err := sys.Register("media", 5, false, "cinqo:media:read", "cinqo:media:write"); err != nil {
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
		"reader reads":                   {reader, "career", "company", Read, nil},
		"reader cannot write":            {reader, "career", "company", Write, ErrForbidden},
		"writer reads":                   {writer, "career", "company", Read, nil},
		"writer writes":                  {writer, "career", "company", Write, nil},
		"admin reads":                    {admin, "career", "company", Read, nil},
		"admin writes":                   {admin, "career", "company", Write, nil},
		"no scope is not found (read)":   {nobody, "career", "company", Read, ErrNotFound},
		"no scope is not found (write)":  {nobody, "career", "company", Write, ErrNotFound},
		"unknown type":                   {admin, "career", "job", Read, ErrNotFound},
		"unknown owner":                  {admin, "ghost", "company", Read, ErrNotFound},
		"a type of another owner":        {admin, "cinqo", "company", Read, ErrNotFound},
		"system type, admin":             {admin, "cinqo", "media", Write, nil},
		"system type, scoped reader":     {Caller{UserID: "m", Scopes: []string{"cinqo:media:read"}}, "cinqo", "media", Read, nil},
		"system type, no scope":          {nobody, "cinqo", "media", Read, ErrNotFound},
		"tool scope on a system type":    {writer, "cinqo", "media", Read, ErrNotFound},
		"write scope alone does not see": {Caller{UserID: "x", Scopes: []string{"tool:career:write"}}, "career", "company", Write, ErrNotFound},
	} {
		got, err := r.Authorize(tc.caller, tc.owner, tc.typ, tc.action)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
			continue
		}
		if tc.want == nil && (got.Owner != tc.owner || got.Name != tc.typ) {
			t.Errorf("%s: returned %+v", name, got)
		}
		if tc.want == nil && tc.owner == "career" && (got.Scale != 5 || !got.HalfStars) {
			t.Errorf("%s: the scale is returned with the type: %+v", name, got)
		}
	}

	// Once the tool is disabled, nobody, not even an admin, reaches its reviews.
	if _, err := db.Exec(`UPDATE tools SET enabled = 0 WHERE id = 't1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Authorize(admin, "career", "company", Read); !errors.Is(err, ErrNotFound) {
		t.Errorf("disabled tool: %v", err)
	}
}

type failing struct{}

func (failing) Lookup(string, string) (EntityType, bool, error) {
	return EntityType{}, false, errors.New("database is down")
}

func TestAuthorizeSurfacesLookupFailuresAndStopsAtTheFirstMatch(t *testing.T) {
	if _, err := NewResolver(failing{}).Authorize(admin, "career", "company", Read); err == nil || errors.Is(err, ErrNotFound) || errors.Is(err, ErrForbidden) {
		t.Errorf("a lookup failure must not look like a permission answer: %v", err)
	}
	first, second := NewSystemTypes(), NewSystemTypes()
	_ = first.Register("media", 3, false, "first:read", "first:write")
	_ = second.Register("media", 5, false, "second:read", "second:write")
	got, err := NewResolver(first, second).Authorize(Caller{UserID: "u", Scopes: []string{"first:read"}}, "cinqo", "media", Read)
	if err != nil || got.ReadScope != "first:read" || got.Scale != 3 {
		t.Errorf("the first source must win: %+v, %v", got, err)
	}
}

func TestEditAndDeleteRules(t *testing.T) {
	ty := EntityType{Owner: "career", Name: "company", ReadScope: "tool:career:read", WriteScope: "tool:career:write"}
	byWriter := review.Review{AuthorID: "w"}
	byOther := review.Review{AuthorID: "someone-else"}
	lostScope := Caller{UserID: "w", Scopes: []string{"tool:career:read"}} // the author, but no longer allowed to write

	for name, tc := range map[string]struct {
		caller       Caller
		r            review.Review
		edit, delete bool
	}{
		"author with write scope":       {writer, byWriter, true, true},
		"author lost the write scope":   {lostScope, byWriter, false, false},
		"another writer":                {writer, byOther, false, false},
		"admin on someone else's":       {admin, byOther, false, true},
		"admin on their own":            {Caller{UserID: "someone-else", Scopes: []string{AdminScope}}, byOther, true, true},
		"reader on their own":           {Caller{UserID: "w", Scopes: []string{"tool:career:read"}}, byWriter, false, false},
		"anonymous never matches empty": {Caller{Scopes: []string{"tool:career:write"}}, review.Review{}, false, false},
	} {
		if got := CanEdit(ty, tc.caller, tc.r); got != tc.edit {
			t.Errorf("%s: CanEdit = %v, want %v", name, got, tc.edit)
		}
		if got := CanDelete(ty, tc.caller, tc.r); got != tc.delete {
			t.Errorf("%s: CanDelete = %v, want %v", name, got, tc.delete)
		}
	}
}

func TestResolverFindNeedsNoPermission(t *testing.T) {
	db := coreDB(t)
	addTool(t, db, "t1", "career", true)
	declare(t, db, "t1", companyType("tool:career:read", "tool:career:write"))
	sys := NewSystemTypes()
	_ = sys.Register("media", 5, false, "m:read", "m:write")
	r := NewResolver(sys, NewToolTypes(db))

	if got, ok, err := r.Find("career", "company"); err != nil || !ok || got.Owner != "career" || got.WriteScope != "tool:career:write" {
		t.Errorf("tool type: %+v, %v, %v", got, ok, err)
	}
	if got, ok, _ := r.Find("cinqo", "media"); !ok || got.Owner != "cinqo" {
		t.Errorf("system type: %+v, %v", got, ok)
	}
	for _, c := range [][2]string{{"career", "job"}, {"ghost", "company"}, {"cinqo", "company"}, {"career", "media"}} {
		if _, ok, _ := r.Find(c[0], c[1]); ok {
			t.Errorf("Find(%q, %q) found something", c[0], c[1])
		}
	}
	if _, _, err := NewResolver(failing{}).Find("career", "company"); err == nil {
		t.Error("a lookup failure was swallowed")
	}
	if _, err := db.Exec(`UPDATE tools SET enabled = 0 WHERE id = 't1'`); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := r.Find("career", "company"); ok {
		t.Error("a disabled tool's type was found")
	}
}
