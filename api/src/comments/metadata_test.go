package comments

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	p1 = "11111111-1111-4111-8111-111111111111"
	p2 = "22222222-2222-4222-8222-222222222222"
)

func withMeta(t *testing.T, s *Store, entityID, body string, meta map[string]string) Comment {
	t.Helper()
	c, err := s.Create(NewComment{EntityType: "job", EntityID: entityID, Body: body, AuthorID: "u1", AuthorName: "Ada", Metadata: meta})
	if err != nil {
		t.Fatalf("Create(%q): %v", body, err)
	}
	return c
}

func profile(id string) map[string]string { return map[string]string{"profileId": id} }

func listScoped(t *testing.T, s *Store, entityID string, meta map[string]string) Page {
	t.Helper()
	p, err := s.List("job", entityID, ListOptions{Metadata: meta})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	return p
}

// ---- migration ----

// A database as the build before this one left it (migrations 1 and 2, with comments) opens, keeps its comments and gives them no metadata.
func TestMigrationThreeKeepsOldCommentsWithNoMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db", "tools", "career", "comments.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := raw.Exec(migrations[i]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := raw.Exec(`PRAGMA user_version = 2`); err != nil {
		t.Fatal(err)
	}
	old := "00000000-0000-4000-8000-0000000000bb"
	if _, err := raw.Exec(`INSERT INTO comments (id, entity_type, entity_id, body, author_id, created_at, root_id) VALUES (?, 'job', ?, 'old', 'u1', '2026-10-01T00:00:00.000Z', ?)`, old, job1, old); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	m := NewManager(dir)
	defer m.Close()
	s := store(t, m, "career")
	got, err := s.Get("job", job1, old)
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "old" || got.Metadata == nil || len(got.Metadata) != 0 {
		t.Errorf("old comment = %+v", got)
	}
	// it is visible without a scope and hidden under one
	if n := len(listScoped(t, s, job1, nil).Comments); n != 1 {
		t.Errorf("unscoped: %d", n)
	}
	if page := listScoped(t, s, job1, profile(p1)); len(page.Comments) != 0 || page.Total != 0 {
		t.Errorf("scoped: %+v", page)
	}
	// and the file is at the new schema version
	r2, _ := sql.Open("sqlite3", path)
	defer r2.Close()
	var v int
	if err := r2.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil || v != 3 {
		t.Errorf("user_version %d %v", v, err)
	}
}

// The database itself refuses a metadata value that is not JSON.
func TestDatabaseEnforcesValidJSONMetadataItself(t *testing.T) {
	m, dir := testManager(t)
	store(t, m, "career")
	m.Close()
	raw, _ := sql.Open("sqlite3", filepath.Join(dir, "db", "tools", "career", "comments.db"))
	defer raw.Close()
	_, err := raw.Exec(`INSERT INTO comments (id, entity_type, entity_id, body, author_id, created_at, root_id, metadata) VALUES ('x', 'job', ?, 'b', 'u', 't', 'x', 'not json')`, job1)
	if err == nil {
		t.Error("a metadata value that is not JSON was stored")
	}
}

// ---- validation ----

func TestMetadataValidation(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	long := func(n int) string { return strings.Repeat("x", n) }
	nine := map[string]string{}
	for i := 0; i < 9; i++ {
		nine[fmt.Sprintf("k%d", i)] = "v"
	}
	eight := map[string]string{}
	for i := 0; i < 8; i++ {
		eight[fmt.Sprintf("k%d", i)] = "v"
	}
	cases := []struct {
		name string
		meta map[string]string
		ok   bool
		want string // the field in the error
	}{
		{"nil", nil, true, ""},
		{"empty", map[string]string{}, true, ""},
		{"profile id", profile(p1), true, ""},
		{"eight keys", eight, true, ""},
		{"nine keys", nine, false, "metadata"},
		{"key starts with a capital", map[string]string{"ProfileId": "x"}, false, "metadata"},
		{"key starts with a digit", map[string]string{"1a": "x"}, false, "metadata"},
		{"empty key", map[string]string{"": "x"}, false, "metadata"},
		{"key with a dash", map[string]string{"a-b": "x"}, false, "metadata"},
		{"key with an underscore", map[string]string{"a_b": "x"}, false, "metadata"},
		{"key with a space", map[string]string{"a b": "x"}, false, "metadata"},
		{"key with a quote", map[string]string{`a"b`: "x"}, false, "metadata"},
		{"key with a dot", map[string]string{"a.b": "x"}, false, "metadata"},
		{"key of 40 characters", map[string]string{"a" + long(39): "x"}, true, ""},
		{"key of 41 characters", map[string]string{"a" + long(40): "x"}, false, "metadata"},
		{"empty value", map[string]string{"a": ""}, false, "metadata.a"},
		{"blank value", map[string]string{"a": "   "}, false, "metadata.a"},
		{"value with a NUL", map[string]string{"a": "x\x00y"}, false, "metadata.a"},
		{"value of 200 characters", map[string]string{"a": long(200)}, true, ""},
		{"value of 201 characters", map[string]string{"a": long(201)}, false, "metadata.a"},
		{"200 multi-byte characters", map[string]string{"a": strings.Repeat("é", 200)}, true, ""},
		{"201 multi-byte characters", map[string]string{"a": strings.Repeat("é", 201)}, false, "metadata.a"},
	}
	for _, c := range cases {
		_, err := s.Create(NewComment{EntityType: "job", EntityID: job1, Body: "b", AuthorID: "u1", Metadata: c.meta})
		if c.ok {
			if err != nil {
				t.Errorf("%s: %v", c.name, err)
			}
			continue
		}
		v, bad := IsValidation(err)
		if !bad || v.Field != c.want {
			t.Errorf("%s: err %v, want a validation error for %q", c.name, err, c.want)
		}
	}
}

// The whole object may be at most 1024 bytes as JSON: exactly 1024 is accepted, 1025 refused.
func TestMetadataTotalSizeLimit(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	build := func(keys []string, lens []int) map[string]string {
		out := map[string]string{}
		for i, k := range keys {
			out[k] = strings.Repeat("x", lens[i])
		}
		return out
	}
	exact := build([]string{"a", "b", "c", "d", "e"}, []int{197, 197, 197, 197, 200})
	if raw, _ := json.Marshal(exact); len(raw) != 1024 {
		t.Fatalf("setup: %d bytes", len(raw))
	}
	over := build([]string{"aa", "b", "c", "d", "e"}, []int{197, 197, 197, 197, 200})
	if raw, _ := json.Marshal(over); len(raw) != 1025 {
		t.Fatalf("setup: %d bytes", len(raw))
	}
	if _, err := s.Create(NewComment{EntityType: "job", EntityID: job1, Body: "b", AuthorID: "u", Metadata: exact}); err != nil {
		t.Errorf("1024 bytes: %v", err)
	}
	if _, err := s.Create(NewComment{EntityType: "job", EntityID: job1, Body: "b", AuthorID: "u", Metadata: over}); err == nil {
		t.Error("1025 bytes were accepted")
	}
}

// Values are trimmed; what is stored and returned is the trimmed value.
func TestMetadataValuesAreTrimmed(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	c := withMeta(t, s, job1, "b", map[string]string{"profileId": "  " + p1 + "\n"})
	got, _ := s.Get("job", job1, c.ID)
	if got.Metadata["profileId"] != p1 {
		t.Errorf("%q", got.Metadata["profileId"])
	}
}

// ---- storing and reading ----

func TestMetadataIsStoredAndReturned(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	c := withMeta(t, s, job1, "with", map[string]string{"profileId": p1, "other": "x"})
	if c.Metadata["profileId"] != p1 || c.Metadata["other"] != "x" || len(c.Metadata) != 2 {
		t.Errorf("created: %+v", c.Metadata)
	}
	got, err := s.Get("job", job1, c.ID)
	if err != nil || got.Metadata["profileId"] != p1 || len(got.Metadata) != 2 {
		t.Errorf("get: %+v %v", got.Metadata, err)
	}
	page := listScoped(t, s, job1, nil)
	if len(page.Comments) != 1 || page.Comments[0].Metadata["profileId"] != p1 {
		t.Errorf("list: %+v", page.Comments)
	}
}

// A comment with no metadata has {} (never null) in its JSON.
func TestNoMetadataIsAnEmptyObjectInJSON(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	c := add(t, s, job1, "plain")
	if c.Metadata == nil {
		t.Error("created comment has nil metadata")
	}
	raw, _ := json.Marshal(c)
	if !strings.Contains(string(raw), `"metadata":{}`) {
		t.Errorf("json: %s", raw)
	}
	got, _ := s.Get("job", job1, c.ID)
	raw, _ = json.Marshal(got)
	if !strings.Contains(string(raw), `"metadata":{}`) {
		t.Errorf("json after a read: %s", raw)
	}
}

// Editing the text never touches the metadata.
func TestUpdateKeepsTheMetadata(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	c := withMeta(t, s, job1, "before", profile(p1))
	u, err := s.Update("job", job1, c.ID, "after")
	if err != nil || u.Body != "after" || u.Metadata["profileId"] != p1 {
		t.Errorf("%+v %v", u, err)
	}
}

// ---- replies ----

func TestAReplyCarriesItsThreadsMetadata(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	root := withMeta(t, s, job1, "root", profile(p1))
	// nothing supplied: inherited
	r1, err := s.Create(NewComment{EntityType: "job", EntityID: job1, Body: "r1", AuthorID: "u", ParentID: root.ID})
	if err != nil || r1.Metadata["profileId"] != p1 {
		t.Fatalf("%+v %v", r1.Metadata, err)
	}
	// another profile supplied: ignored, the thread's wins
	r2, err := s.Create(NewComment{EntityType: "job", EntityID: job1, Body: "r2", AuthorID: "u", ParentID: r1.ID, Metadata: profile(p2)})
	if err != nil || r2.Metadata["profileId"] != p1 || len(r2.Metadata) != 1 {
		t.Fatalf("%+v %v", r2.Metadata, err)
	}
	got, _ := s.Get("job", job1, r2.ID)
	if got.Metadata["profileId"] != p1 {
		t.Errorf("stored: %+v", got.Metadata)
	}
	// a thread without metadata stays without, even if a reply supplies some
	plain := add(t, s, job1, "plain")
	r3, err := s.Create(NewComment{EntityType: "job", EntityID: job1, Body: "r3", AuthorID: "u", ParentID: plain.ID, Metadata: profile(p2)})
	if err != nil || len(r3.Metadata) != 0 {
		t.Fatalf("%+v %v", r3.Metadata, err)
	}
	// an invalid value on a reply is still refused (the request is bad even if it would be ignored)
	if _, err := s.Create(NewComment{EntityType: "job", EntityID: job1, Body: "bad", AuthorID: "u", ParentID: root.ID, Metadata: map[string]string{"Bad": "x"}}); err == nil {
		t.Error("invalid metadata on a reply was accepted")
	}
}

// ---- the scope ----

func TestTheScopeShowsOnlyThatProfilesComments(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	withMeta(t, s, job1, "mine", profile(p1))
	withMeta(t, s, job1, "theirs", profile(p2))
	add(t, s, job1, "nobodys")

	if page := listScoped(t, s, job1, profile(p1)); bodies(page) != "mine" || page.Total != 1 {
		t.Errorf("p1: %q total %d", bodies(page), page.Total)
	}
	if page := listScoped(t, s, job1, profile(p2)); bodies(page) != "theirs" || page.Total != 1 {
		t.Errorf("p2: %q total %d", bodies(page), page.Total)
	}
	if page := listScoped(t, s, job1, profile("33333333-3333-4333-8333-333333333333")); len(page.Comments) != 0 || page.Total != 0 {
		t.Errorf("unknown profile: %+v", page)
	}
	all := listScoped(t, s, job1, nil)
	if len(all.Comments) != 3 || all.Total != 3 {
		t.Errorf("unscoped: %q total %d", bodies(all), all.Total)
	}
	if page := listScoped(t, s, job1, map[string]string{}); len(page.Comments) != 3 {
		t.Errorf("an empty scope is no scope: %q", bodies(page))
	}
}

// A thread belongs to the scope as a whole: its replies show with it and hide with it, and nothing is shown as "context".
func TestTheScopeKeepsAThreadTogether(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	root := withMeta(t, s, job1, "root", profile(p1))
	rep := reply(t, s, job1, root.ID, "reply")
	reply(t, s, job1, rep.ID, "reply2")
	other := withMeta(t, s, job1, "other root", profile(p2))
	reply(t, s, job1, other.ID, "other reply")

	page := listScoped(t, s, job1, profile(p1))
	if len(page.Comments) != 3 || page.Total != 3 {
		t.Fatalf("p1: %q total %d", bodies(page), page.Total)
	}
	for _, c := range page.Comments {
		if c.Context {
			t.Errorf("%q is shown as context", c.Body)
		}
	}
	if page := listScoped(t, s, job1, profile(p2)); len(page.Comments) != 2 || page.Total != 2 {
		t.Errorf("p2: %q total %d", bodies(page), page.Total)
	}
}

// Rows that do not fit the scope are never shown to complete a thread: a legacy reply that carries metadata under a root that does not
// cannot appear without its parent, and a legacy root without metadata hides its whole thread.
func TestTheScopeNeverShowsAReplyWithoutItsParent(t *testing.T) {
	m, dir := testManager(t)
	s := store(t, m, "career")
	root := add(t, s, job1, "plain root")
	rep := reply(t, s, job1, root.ID, "reply")
	m.Close()
	raw, _ := sql.Open("sqlite3", filepath.Join(dir, "db", "tools", "career", "comments.db"))
	if _, err := raw.Exec(`UPDATE comments SET metadata = ? WHERE id = ?`, `{"profileId":"`+p1+`"}`, rep.ID); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	m2 := NewManager(dir)
	defer m2.Close()
	s2 := store(t, m2, "career")
	page := listScoped(t, s2, job1, profile(p1))
	if len(page.Comments) != 0 {
		t.Errorf("shown: %q", bodies(page))
	}
	for _, c := range page.Comments {
		if c.Context {
			t.Error("context row under a scope")
		}
	}
}

// Every key of the scope must match.
func TestEveryKeyOfTheScopeMustMatch(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	withMeta(t, s, job1, "both", map[string]string{"profileId": p1, "view": "board"})
	withMeta(t, s, job1, "only profile", profile(p1))
	withMeta(t, s, job1, "other view", map[string]string{"profileId": p1, "view": "list"})
	page := listScoped(t, s, job1, map[string]string{"profileId": p1, "view": "board"})
	if bodies(page) != "both" || page.Total != 1 {
		t.Errorf("%q total %d", bodies(page), page.Total)
	}
	if page := listScoped(t, s, job1, profile(p1)); len(page.Comments) != 3 {
		t.Errorf("one key: %q", bodies(page))
	}
}

// The scope combines with the author and the date filters.
func TestTheScopeCombinesWithTheOtherFilters(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	at := func(d string) {
		ts, _ := time.Parse(time.RFC3339, d)
		s.now = func() time.Time { return ts }
	}
	mk := func(author, body, day string, meta map[string]string) {
		at(day)
		if _, err := s.Create(NewComment{EntityType: "job", EntityID: job1, Body: body, AuthorID: author, AuthorName: author, Metadata: meta}); err != nil {
			t.Fatal(err)
		}
	}
	mk("ann", "ann p1 early", "2026-10-01T10:00:00Z", profile(p1))
	mk("ann", "ann p1 late", "2026-10-20T10:00:00Z", profile(p1))
	mk("bob", "bob p1 late", "2026-10-21T10:00:00Z", profile(p1))
	mk("ann", "ann p2 late", "2026-10-22T10:00:00Z", profile(p2))
	got, err := s.List("job", job1, ListOptions{Metadata: profile(p1), AuthorID: "ann"})
	if err != nil || bodies(got) != "ann p1 late,ann p1 early" || got.Total != 2 {
		t.Errorf("author: %q total %d %v", bodies(got), got.Total, err)
	}
	got, err = s.List("job", job1, ListOptions{Metadata: profile(p1), CreatedFrom: "2026-10-15"})
	if err != nil || bodies(got) != "bob p1 late,ann p1 late" || got.Total != 2 {
		t.Errorf("from: %q total %d %v", bodies(got), got.Total, err)
	}
	got, err = s.List("job", job1, ListOptions{Metadata: profile(p1), AuthorID: "ann", CreatedFrom: "2026-10-15"})
	if err != nil || bodies(got) != "ann p1 late" || got.Total != 1 {
		t.Errorf("both: %q total %d %v", bodies(got), got.Total, err)
	}
}

// Paging under a scope walks exactly the scope's threads, each once, and Total is the scope's.
func TestPagingUnderAScopeWalksOnlyItsThreads(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	for i := 1; i <= 5; i++ {
		withMeta(t, s, job1, fmt.Sprintf("p1-%d", i), profile(p1))
		withMeta(t, s, job1, fmt.Sprintf("p2-%d", i), profile(p2))
	}
	var seen []string
	before := ""
	for pages := 0; pages < 10; pages++ {
		p, err := s.List("job", job1, ListOptions{Limit: 2, Before: before, Metadata: profile(p1)})
		if err != nil {
			t.Fatal(err)
		}
		if p.Total != 5 {
			t.Errorf("total %d", p.Total)
		}
		for _, c := range p.Comments {
			seen = append(seen, c.Body)
		}
		if p.NextBefore == "" {
			break
		}
		before = p.NextBefore
	}
	if strings.Join(seen, ",") != "p1-5,p1-4,p1-3,p1-2,p1-1" {
		t.Errorf("walked %v", seen)
	}
}

// The scope is checked like the metadata of a comment: a bad key or value is a validation error, and nothing is listed.
func TestAScopeIsValidated(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	add(t, s, job1, "x")
	for name, meta := range map[string]map[string]string{
		"bad key":   {"Profile": "x"},
		"quote key": {`a"] OR 1=1 --`: "x"},
		"empty":     {"profileId": ""},
		"too long":  {"profileId": strings.Repeat("x", 201)},
		"nine keys": {"a": "1", "b": "1", "c": "1", "d": "1", "e": "1", "f": "1", "g": "1", "h": "1", "i": "1"},
	} {
		if _, err := s.List("job", job1, ListOptions{Metadata: meta}); err == nil {
			t.Errorf("%s: accepted", name)
		} else if _, bad := IsValidation(err); !bad {
			t.Errorf("%s: %v is not a validation error", name, err)
		}
	}
}

// A value is only ever a parameter: one that looks like SQL matches itself and nothing else.
func TestAScopeValueIsNeverSQL(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	evil := `x' OR '1'='1`
	withMeta(t, s, job1, "evil", map[string]string{"profileId": evil})
	withMeta(t, s, job1, "plain", profile(p1))
	if page := listScoped(t, s, job1, map[string]string{"profileId": evil}); bodies(page) != "evil" {
		t.Errorf("%q", bodies(page))
	}
	if page := listScoped(t, s, job1, profile(p1)); bodies(page) != "plain" {
		t.Errorf("%q", bodies(page))
	}
}

// Another entity's comments are never mixed in, scope or not.
func TestTheScopeStaysInsideTheEntity(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	withMeta(t, s, job1, "job1", profile(p1))
	withMeta(t, s, job2, "job2", profile(p1))
	if page := listScoped(t, s, job1, profile(p1)); bodies(page) != "job1" || page.Total != 1 {
		t.Errorf("%q total %d", bodies(page), page.Total)
	}
}

// A newer database is still refused after this migration.
func TestANewerDatabaseIsStillRefused(t *testing.T) {
	m, dir := testManager(t)
	store(t, m, "career")
	m.Close()
	path := filepath.Join(dir, "db", "tools", "career", "comments.db")
	raw, _ := sql.Open("sqlite3", path)
	if _, err := raw.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, len(migrations)+1)); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	if len(migrations) != 3 {
		t.Errorf("%d migrations", len(migrations))
	}
	if _, err := NewManager(dir).Store("career"); err == nil || !strings.Contains(err.Error(), "newer version") {
		t.Errorf("a newer schema must be refused: %v", err)
	}
}

// A stored value that is valid JSON but not a flat string map (nothing writes one) reads as no metadata instead of failing the read.
func TestMetadataThatIsNotAFlatStringMapReadsAsNone(t *testing.T) {
	m, dir := testManager(t)
	s := store(t, m, "career")
	var ids []string
	for _, body := range []string{"array", "number value", "nested", "null"} {
		ids = append(ids, add(t, s, job1, body).ID)
	}
	m.Close()
	raw, _ := sql.Open("sqlite3", filepath.Join(dir, "db", "tools", "career", "comments.db"))
	for i, v := range []string{`[1,2]`, `{"a":1}`, `{"a":{"b":"c"}}`, `null`} {
		if _, err := raw.Exec(`UPDATE comments SET metadata = ? WHERE id = ?`, v, ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	raw.Close()
	m2 := NewManager(dir)
	defer m2.Close()
	s2 := store(t, m2, "career")
	for _, id := range ids {
		got, err := s2.Get("job", job1, id)
		if err != nil || got.Metadata == nil || len(got.Metadata) != 0 {
			t.Errorf("%s: %+v %v", id, got.Metadata, err)
		}
	}
	if page := listScoped(t, s2, job1, nil); len(page.Comments) != 4 {
		t.Errorf("list: %d", len(page.Comments))
	}
}

// Under a scope the rows of a thread are scoped too: a legacy reply that does not carry the scope's metadata is hidden even though its
// root matches (a reply written now always inherits, so this only happens to rows from elsewhere).
func TestTheScopeHidesALegacyReplyUnderAMatchingRoot(t *testing.T) {
	m, dir := testManager(t)
	s := store(t, m, "career")
	root := withMeta(t, s, job1, "root", profile(p1))
	rep := reply(t, s, job1, root.ID, "mixed reply")
	m.Close()
	raw, _ := sql.Open("sqlite3", filepath.Join(dir, "db", "tools", "career", "comments.db"))
	if _, err := raw.Exec(`UPDATE comments SET metadata = '{}' WHERE id = ?`, rep.ID); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	m2 := NewManager(dir)
	defer m2.Close()
	s2 := store(t, m2, "career")
	page := listScoped(t, s2, job1, profile(p1))
	if bodies(page) != "root" || page.Total != 1 {
		t.Errorf("scoped: %q total %d", bodies(page), page.Total)
	}
	if page := listScoped(t, s2, job1, nil); len(page.Comments) != 2 {
		t.Errorf("unscoped: %q", bodies(page))
	}
}
