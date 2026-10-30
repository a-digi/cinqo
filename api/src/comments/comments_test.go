package comments

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	job1 = "1fbd3006-0831-448c-8962-2851188e9de9"
	job2 = "2aaaaaaa-0831-448c-8962-2851188e9de9"
)

// testManager gives a Manager over a temp dir with a controllable clock and
// predictable ids, so ordering and cursors can be asserted exactly.
func testManager(t *testing.T) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	m := NewManager(dir)
	t.Cleanup(func() { m.Close() })
	clock := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	n := 0
	m.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		clock = clock.Add(time.Second)
		return clock
	}
	m.newID = func() string {
		mu.Lock()
		defer mu.Unlock()
		n++
		return fmt.Sprintf("00000000-0000-4000-8000-%012d", n)
	}
	return m, dir
}

func store(t *testing.T, m *Manager, owner string) *Store {
	t.Helper()
	s, err := m.Store(owner)
	if err != nil {
		t.Fatalf("Store(%q): %v", owner, err)
	}
	return s
}

func add(t *testing.T, s *Store, entityID, body string) Comment {
	t.Helper()
	c, err := s.Create(NewComment{EntityType: "job", EntityID: entityID, Body: body, AuthorID: "u1", AuthorName: "Ada"})
	if err != nil {
		t.Fatalf("Create(%q): %v", body, err)
	}
	return c
}

func TestDBPath(t *testing.T) {
	dir := t.TempDir()
	want := map[string]string{
		"cinqo":  filepath.Join(dir, "db", "comments.db"),
		"career": filepath.Join(dir, "db", "tools", "career", "comments.db"),
		"my_x9":  filepath.Join(dir, "db", "tools", "my_x9", "comments.db"),
	}
	for owner, path := range want {
		got, err := DBPath(dir, owner)
		if err != nil || got != path {
			t.Errorf("DBPath(%q) = %q, %v; want %q", owner, got, err, path)
		}
	}
	for _, owner := range []string{"", ".", "..", "../etc", "a/b", `a\b`, "Career", "a b", "1abc", "a.b", "x\x00y", strings.Repeat("a", 65), "-x"} {
		if _, err := DBPath(dir, owner); err != ErrInvalidOwner {
			t.Errorf("DBPath(%q): err = %v, want ErrInvalidOwner", owner, err)
		}
	}
	m := NewManager(dir)
	if _, err := m.Store("../../escape"); err != ErrInvalidOwner {
		t.Errorf("Store with a hostile owner: %v", err)
	}
}

func TestOwnersAreSeparateFiles(t *testing.T) {
	m, dir := testManager(t)
	career, system := store(t, m, "career"), store(t, m, "cinqo")

	add(t, career, job1, "career comment")
	add(t, system, job1, "system comment") // the very same entity id, on purpose

	for owner, want := range map[string]string{"career": "career comment", "cinqo": "system comment"} {
		page, err := store(t, m, owner).List("job", job1, ListOptions{Limit: 0, Before: ""})
		if err != nil || page.Total != 1 || page.Comments[0].Body != want {
			t.Errorf("%s: %+v, %v", owner, page, err)
		}
	}
	for _, f := range []string{filepath.Join("db", "tools", "career", "comments.db"), filepath.Join("db", "comments.db")} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("expected file %s: %v", f, err)
		}
	}
	// Purging in one owner does not touch the other.
	if n, err := career.PurgeEntity("job", job1); err != nil || n != 1 {
		t.Fatalf("purge: %d, %v", n, err)
	}
	if page, _ := system.List("job", job1, ListOptions{Limit: 0, Before: ""}); page.Total != 1 {
		t.Error("another owner's comment was removed")
	}
}

func TestCreateAndOrder(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	first := add(t, s, job1, "  first  ")
	add(t, s, job1, "second")
	add(t, s, job2, "other entity")

	if first.Body != "first" || first.AuthorID != "u1" || first.AuthorName != "Ada" || first.UpdatedAt != "" {
		t.Errorf("created = %+v", first)
	}
	if first.CreatedAt != "2026-10-30T09:00:01.000Z" {
		t.Errorf("createdAt = %q", first.CreatedAt)
	}

	page, err := s.List("job", job1, ListOptions{Limit: 0, Before: ""})
	if err != nil || page.Total != 2 || len(page.Comments) != 2 || page.NextBefore != "" {
		t.Fatalf("page = %+v, %v", page, err)
	}
	if page.Comments[0].Body != "second" || page.Comments[1].Body != "first" {
		t.Errorf("not newest first: %s, %s", page.Comments[0].Body, page.Comments[1].Body)
	}
	if empty, _ := s.List("job", "3bbbbbbb-0831-448c-8962-2851188e9de9", ListOptions{Limit: 0, Before: ""}); empty.Total != 0 || empty.Comments == nil || len(empty.Comments) != 0 {
		t.Errorf("an entity with no comments must give an empty, non-nil list: %+v", empty)
	}
}

func TestPagingWalksEveryCommentOnce(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	for i := 1; i <= 5; i++ {
		add(t, s, job1, fmt.Sprintf("c%d", i))
	}
	var seen []string
	before := ""
	for pages := 0; pages < 10; pages++ {
		page, err := s.List("job", job1, ListOptions{Limit: 2, Before: before})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 5 {
			t.Errorf("total = %d", page.Total)
		}
		for _, c := range page.Comments {
			seen = append(seen, c.Body)
		}
		if page.NextBefore == "" {
			break
		}
		before = page.NextBefore
	}
	if got := strings.Join(seen, ","); got != "c5,c4,c3,c2,c1" {
		t.Errorf("walked %s", got)
	}

	// Same-millisecond comments keep a stable order through the id tie-break.
	s.now = func() time.Time { return time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC) }
	for i := 1; i <= 3; i++ {
		add(t, s, job2, fmt.Sprintf("t%d", i))
	}
	first, _ := s.List("job", job2, ListOptions{Limit: 2, Before: ""})
	rest, err := s.List("job", job2, ListOptions{Limit: 2, Before: first.NextBefore})
	if err != nil || len(first.Comments)+len(rest.Comments) != 3 || rest.NextBefore != "" {
		t.Errorf("tie-break paging: %+v / %+v, %v", first, rest, err)
	}

	if page, _ := s.List("job", job1, ListOptions{Limit: 1000, Before: ""}); len(page.Comments) != 5 {
		t.Error("an over-large limit should still work")
	}
	if page, _ := s.List("job", job1, ListOptions{Limit: 1, Before: ""}); len(page.Comments) != 1 || page.NextBefore == "" {
		t.Errorf("limit 1: %+v", page)
	}
}

func TestListRejectsAForeignCursor(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	other := add(t, s, job2, "x")
	add(t, s, job1, "y")
	for _, before := range []string{other.ID, "00000000-0000-4000-8000-999999999999", "not-a-uuid"} {
		if _, err := s.List("job", job1, ListOptions{Limit: 10, Before: before}); err == nil {
			t.Errorf("cursor %q accepted", before)
		} else if v, ok := IsValidation(err); !ok || v.Field != "before" {
			t.Errorf("cursor %q: %v", before, err)
		}
	}
}

func TestValidation(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	ok := NewComment{EntityType: "job", EntityID: job1, Body: "hi", AuthorID: "u1"}
	for name, mutate := range map[string]func(*NewComment){
		"empty body":     func(c *NewComment) { c.Body = "" },
		"blank body":     func(c *NewComment) { c.Body = " \n\t " },
		"NUL body":       func(c *NewComment) { c.Body = "a\x00b" },
		"long body":      func(c *NewComment) { c.Body = strings.Repeat("x", MaxBodyRunes+1) },
		"no entity type": func(c *NewComment) { c.EntityType = "" },
		"bad type":       func(c *NewComment) { c.EntityType = "Job" },
		"path type":      func(c *NewComment) { c.EntityType = "../x" },
		"long type":      func(c *NewComment) { c.EntityType = "a" + strings.Repeat("b", MaxEntityTypeRunes) },
		"no entity id":   func(c *NewComment) { c.EntityID = "" },
		"not a uuid":     func(c *NewComment) { c.EntityID = "123" },
		"braced uuid":    func(c *NewComment) { c.EntityID = "{" + job1 + "}" },
		"urn uuid":       func(c *NewComment) { c.EntityID = "urn:uuid:" + job1 },
		"hyphenless":     func(c *NewComment) { c.EntityID = strings.ReplaceAll(job1, "-", "") },
		"no author":      func(c *NewComment) { c.AuthorID = "  " },
	} {
		c := ok
		mutate(&c)
		if _, err := s.Create(c); err == nil {
			t.Errorf("%s: accepted", name)
		} else if _, isVal := IsValidation(err); !isVal {
			t.Errorf("%s: not a ValidationError: %v", name, err)
		}
	}

	// Edges that are fine: exactly the limit, multi-byte text counted in characters,
	// an upper-case UUID (stored lower-case), a very long author name (shortened).
	c := ok
	c.Body = strings.Repeat("é", MaxBodyRunes)
	if _, err := s.Create(c); err != nil {
		t.Errorf("a body of exactly %d characters: %v", MaxBodyRunes, err)
	}
	c = ok
	c.EntityID = strings.ToUpper(job1)
	c.AuthorName = strings.Repeat("n", 500)
	created, err := s.Create(c)
	if err != nil || created.EntityID != job1 || len([]rune(created.AuthorName)) != maxAuthorNameRunes {
		t.Errorf("normalisation: %+v, %v", created, err)
	}
	if page, _ := s.List("job", job1, ListOptions{Limit: 0, Before: ""}); page.Total != 2 {
		t.Errorf("the upper-case spelling must reach the same entity: total %d", page.Total)
	}
}

func TestUpdateAndDeleteAreScopedToTheirEntity(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	c := add(t, s, job1, "original")

	updated, err := s.Update("job", job1, c.ID, "  edited  ")
	if err != nil || updated.Body != "edited" || updated.CreatedAt != c.CreatedAt || updated.AuthorID != c.AuthorID || updated.UpdatedAt == "" || updated.UpdatedAt == c.CreatedAt {
		t.Fatalf("update: %+v, %v", updated, err)
	}
	if _, err := s.Update("job", job1, c.ID, ""); err == nil {
		t.Error("an empty edit was accepted")
	}

	// The same comment id through a different entity (or type) is simply not found.
	if _, err := s.Update("job", job2, c.ID, "x"); err != ErrNotFound {
		t.Errorf("update through another entity: %v", err)
	}
	if _, err := s.Get("note", job1, c.ID); err != ErrNotFound {
		t.Errorf("get through another type: %v", err)
	}
	if err := s.Delete("job", job2, c.ID); err != ErrNotFound {
		t.Errorf("delete through another entity: %v", err)
	}
	if _, err := s.Update("job", job1, "00000000-0000-4000-8000-999999999999", "x"); err != ErrNotFound {
		t.Errorf("update of an unknown comment: %v", err)
	}

	if err := s.Delete("job", job1, c.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.Delete("job", job1, c.ID); err != ErrNotFound {
		t.Errorf("second delete: %v", err)
	}
}

func TestPurgeEntity(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	add(t, s, job1, "a")
	add(t, s, job1, "b")
	add(t, s, job2, "keep")
	if _, err := s.Create(NewComment{EntityType: "note", EntityID: job1, Body: "other type", AuthorID: "u"}); err != nil {
		t.Fatal(err)
	}

	if n, err := s.PurgeEntity("job", job1); err != nil || n != 2 {
		t.Fatalf("purge = %d, %v", n, err)
	}
	if page, _ := s.List("job", job1, ListOptions{Limit: 0, Before: ""}); page.Total != 0 {
		t.Error("comments of the purged entity remain")
	}
	if page, _ := s.List("job", job2, ListOptions{Limit: 0, Before: ""}); page.Total != 1 {
		t.Error("another entity's comment was purged")
	}
	if page, _ := s.List("note", job1, ListOptions{Limit: 0, Before: ""}); page.Total != 1 {
		t.Error("the same id under another type was purged")
	}
	if n, err := s.PurgeEntity("job", job1); err != nil || n != 0 {
		t.Errorf("purging nothing: %d, %v", n, err)
	}
	if _, err := s.PurgeEntity("job", "bad"); err == nil {
		t.Error("an invalid entity id was accepted")
	}
}

func TestDataSurvivesReopen(t *testing.T) {
	m, dir := testManager(t)
	add(t, store(t, m, "career"), job1, "persisted")
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	again := NewManager(dir)
	defer again.Close()
	page, err := store(t, again, "career").List("job", job1, ListOptions{Limit: 0, Before: ""})
	if err != nil || page.Total != 1 || page.Comments[0].Body != "persisted" {
		t.Errorf("after reopen: %+v, %v", page, err)
	}
}

func TestSchemaVersioning(t *testing.T) {
	m, dir := testManager(t)
	store(t, m, "career")
	m.Close()

	path := filepath.Join(dir, "db", "tools", "career", "comments.db")
	raw, _ := sql.Open("sqlite3", path)
	defer raw.Close()
	var version int
	if err := raw.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != len(migrations) {
		t.Fatalf("user_version = %d, %v", version, err)
	}

	// Reopening does not re-run (or fail on) already-applied steps.
	if _, err := NewManager(dir).Store("career"); err != nil {
		t.Errorf("reopen: %v", err)
	}

	// A file written by a newer build is refused, not modified.
	if _, err := raw.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, len(migrations)+1)); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(dir).Store("career"); err == nil || !strings.Contains(err.Error(), "newer version") {
		t.Errorf("a newer schema must be refused: %v", err)
	}
}

func TestDatabaseEnforcesBodyLimitItself(t *testing.T) {
	m, dir := testManager(t)
	store(t, m, "career")
	raw, _ := sql.Open("sqlite3", filepath.Join(dir, "db", "tools", "career", "comments.db"))
	defer raw.Close()
	_, err := raw.Exec(`INSERT INTO comments (id, entity_type, entity_id, body, author_id, created_at) VALUES ('x','job','y','','u','t')`)
	if err == nil {
		t.Error("the CHECK constraint did not refuse an empty body")
	}
}

func TestConcurrentUse(t *testing.T) {
	m, _ := testManager(t)
	var wg sync.WaitGroup
	stores := make([]*Store, 16)
	for i := range stores {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s, err := m.Store("career")
			if err != nil {
				t.Error(err)
				return
			}
			stores[i] = s
			if _, err := s.Create(NewComment{EntityType: "job", EntityID: job1, Body: fmt.Sprintf("c%d", i), AuthorID: "u"}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	for _, s := range stores {
		if s != stores[0] {
			t.Fatal("the manager opened the same owner more than once")
		}
	}
	if page, _ := stores[0].List("job", job1, ListOptions{Limit: 100, Before: ""}); page.Total != 16 {
		t.Errorf("total = %d, want 16", page.Total)
	}
}

// addAs stores a comment by a given author at a given time.
func addAs(t *testing.T, m *Manager, s *Store, entityID, author, body, at string) Comment {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, at)
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return ts }
	c, err := s.Create(NewComment{EntityType: "job", EntityID: entityID, Body: body, AuthorID: author, AuthorName: author})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func bodies(p Page) string {
	var out []string
	for _, c := range p.Comments {
		out = append(out, c.Body)
	}
	return strings.Join(out, ",")
}

func TestFilterByAuthor(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	alice, bob := "9f1c2b4e-0000-4000-8000-00000000000a", "9f1c2b4e-0000-4000-8000-00000000000b"
	addAs(t, m, s, job1, alice, "a1", "2026-10-01T10:00:00Z")
	addAs(t, m, s, job1, bob, "b1", "2026-10-02T10:00:00Z")
	addAs(t, m, s, job1, alice, "a2", "2026-10-03T10:00:00Z")
	addAs(t, m, s, job2, alice, "other entity", "2026-10-04T10:00:00Z")

	page, err := s.List("job", job1, ListOptions{AuthorID: alice})
	if err != nil || bodies(page) != "a2,a1" || page.Total != 2 {
		t.Errorf("alice: %q total %d, %v", bodies(page), page.Total, err)
	}
	if page, _ := s.List("job", job1, ListOptions{AuthorID: "  " + bob + " "}); bodies(page) != "b1" {
		t.Errorf("bob (padded): %q", bodies(page))
	}
	if page, err := s.List("job", job1, ListOptions{AuthorID: "nobody"}); err != nil || page.Total != 0 || page.Comments == nil || len(page.Comments) != 0 {
		t.Errorf("unknown author: %+v, %v", page, err)
	}
	// The author filter is exact, not a prefix match.
	if page, _ := s.List("job", job1, ListOptions{AuthorID: alice[:10]}); page.Total != 0 {
		t.Errorf("a prefix matched: %+v", page)
	}
	if _, err := s.List("job", job1, ListOptions{AuthorID: strings.Repeat("x", 300)}); err == nil {
		t.Error("an over-long author id was accepted")
	}
}

func TestFilterByCreatedTime(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	addAs(t, m, s, job1, "u", "oct1", "2026-10-01T00:00:00Z")
	addAs(t, m, s, job1, "u", "oct2-early", "2026-10-02T00:00:00Z")
	addAs(t, m, s, job1, "u", "oct2-late", "2026-10-02T23:59:59Z")
	addAs(t, m, s, job1, "u", "oct3", "2026-10-03T00:00:00Z")
	addAs(t, m, s, job1, "u", "oct5", "2026-10-05T12:00:00Z")

	for name, tc := range map[string]struct {
		opts ListOptions
		want string
	}{
		"from only (inclusive)":     {ListOptions{CreatedFrom: "2026-10-03T00:00:00Z"}, "oct5,oct3"},
		"to only (exclusive)":       {ListOptions{CreatedTo: "2026-10-02T00:00:00Z"}, "oct1"},
		"a whole day by dates":      {ListOptions{CreatedFrom: "2026-10-02", CreatedTo: "2026-10-03"}, "oct2-late,oct2-early"},
		"timestamps with an offset": {ListOptions{CreatedFrom: "2026-10-02T02:00:00+02:00", CreatedTo: "2026-10-03T02:00:00+02:00"}, "oct2-late,oct2-early"},
		"nothing in range":          {ListOptions{CreatedFrom: "2026-11-01"}, ""},
		"everything":                {ListOptions{CreatedFrom: "2000-01-01", CreatedTo: "2100-01-01"}, "oct5,oct3,oct2-late,oct2-early,oct1"},
	} {
		page, err := s.List("job", job1, tc.opts)
		if err != nil || bodies(page) != tc.want {
			t.Errorf("%s: got %q (total %d), %v; want %q", name, bodies(page), page.Total, err, tc.want)
		}
	}

	for name, opts := range map[string]ListOptions{
		"bad from":       {CreatedFrom: "yesterday"},
		"bad to":         {CreatedTo: "2026-13-45"},
		"to before from": {CreatedFrom: "2026-10-05", CreatedTo: "2026-10-01"},
		"equal bounds":   {CreatedFrom: "2026-10-05", CreatedTo: "2026-10-05"},
	} {
		if _, err := s.List("job", job1, opts); err == nil {
			t.Errorf("%s: accepted", name)
		} else if _, ok := IsValidation(err); !ok {
			t.Errorf("%s: not a ValidationError: %v", name, err)
		}
	}
}

func TestFiltersCombineAndPage(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	for day := 1; day <= 6; day++ {
		at := fmt.Sprintf("2026-10-%02dT10:00:00Z", day)
		addAs(t, m, s, job1, "alice", fmt.Sprintf("a%d", day), at)
		addAs(t, m, s, job1, "bob", fmt.Sprintf("b%d", day), at)
	}
	opts := ListOptions{AuthorID: "alice", CreatedFrom: "2026-10-02", CreatedTo: "2026-10-06", Limit: 2}
	var seen []string
	for i := 0; i < 5; i++ {
		page, err := s.List("job", job1, opts)
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 4 {
			t.Errorf("total = %d, want the 4 matching (not the 12 stored)", page.Total)
		}
		for _, c := range page.Comments {
			seen = append(seen, c.Body)
		}
		if page.NextBefore == "" {
			break
		}
		opts.Before = page.NextBefore
	}
	if got := strings.Join(seen, ","); got != "a5,a4,a3,a2" {
		t.Errorf("filtered paging walked %s", got)
	}
}

func TestCommentsMadeInTheSameMillisecondKeepTheirOrder(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	s.now = func() time.Time { return time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC) } // a clock that never moves
	for i := 1; i <= 5; i++ {
		add(t, s, job1, fmt.Sprintf("c%d", i))
	}
	page, err := s.List("job", job1, ListOptions{})
	if err != nil || bodies(page) != "c5,c4,c3,c2,c1" {
		t.Errorf("got %q, %v", bodies(page), err)
	}
	// A clock that goes backwards does not reorder them either.
	s.now = func() time.Time { return time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC) }
	add(t, s, job1, "c6")
	if page, _ := s.List("job", job1, ListOptions{}); !strings.HasPrefix(bodies(page), "c6,") {
		t.Errorf("a clock step back put a new comment in the past: %q", bodies(page))
	}
}

// Rows can share a timestamp when they come from elsewhere (another process, an import):
// the id tie-break keeps paging complete and stable.
func TestPagingIsStableForIdenticalTimestamps(t *testing.T) {
	m, dir := testManager(t)
	store(t, m, "career")
	raw, err := sql.Open("sqlite3", filepath.Join(dir, "db", "tools", "career", "comments.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	for i := 1; i <= 5; i++ {
		if _, err := raw.Exec(
			`INSERT INTO comments (id, root_id, entity_type, entity_id, body, author_id, created_at) VALUES (?, ?, 'job', ?, ?, 'u', '2026-11-01T12:00:00.000Z')`,
			fmt.Sprintf("00000000-0000-4000-8000-%012d", i), fmt.Sprintf("00000000-0000-4000-8000-%012d", i), job1, fmt.Sprintf("t%d", i),
		); err != nil {
			t.Fatal(err)
		}
	}
	s := store(t, m, "career")
	var seen []string
	before := ""
	for i := 0; i < 6; i++ {
		page, err := s.List("job", job1, ListOptions{Limit: 2, Before: before})
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range page.Comments {
			seen = append(seen, c.Body)
		}
		if page.NextBefore == "" {
			break
		}
		before = page.NextBefore
	}
	if got := strings.Join(seen, ","); got != "t5,t4,t3,t2,t1" {
		t.Errorf("walked %s", got)
	}
}

func reply(t *testing.T, s *Store, entityID, parentID, body string) Comment {
	t.Helper()
	c, err := s.Create(NewComment{EntityType: "job", EntityID: entityID, Body: body, AuthorID: "u1", AuthorName: "Ada", ParentID: parentID})
	if err != nil {
		t.Fatalf("reply(%q): %v", body, err)
	}
	return c
}

func TestMigrationTwoKeepsOldCommentsAsTopLevelThreads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db", "tools", "career", "comments.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	// A database as the first build left it: only migration 1, one comment.
	if _, err := raw.Exec(migrations[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	old := "00000000-0000-4000-8000-0000000000aa"
	if _, err := raw.Exec(`INSERT INTO comments (id, entity_type, entity_id, body, author_id, created_at) VALUES (?, 'job', ?, 'old', 'u1', '2026-10-01T00:00:00.000Z')`, old, job1); err != nil {
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
	if got.ParentID != "" || got.Depth != 0 || got.RootID != old {
		t.Errorf("old comment = parent %q depth %d root %q; want a top-level comment of its own thread", got.ParentID, got.Depth, got.RootID)
	}
	// And it can now be replied to.
	r := reply(t, s, job1, old, "hello")
	if r.RootID != old || r.Depth != 1 {
		t.Errorf("reply = root %q depth %d", r.RootID, r.Depth)
	}
}

func TestReplyFieldsFollowTheParent(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	top := add(t, s, job1, "top")
	if top.ParentID != "" || top.Depth != 0 || top.RootID != top.ID {
		t.Errorf("top-level = %+v", top)
	}
	a := reply(t, s, job1, top.ID, "a")
	b := reply(t, s, job1, a.ID, "b")
	if a.ParentID != top.ID || a.Depth != 1 || a.RootID != top.ID {
		t.Errorf("a = %+v", a)
	}
	if b.ParentID != a.ID || b.Depth != 2 || b.RootID != top.ID {
		t.Errorf("b = %+v", b)
	}
	got, err := s.Get("job", job1, b.ID)
	if err != nil || got.ParentID != a.ID || got.Depth != 2 || got.RootID != top.ID {
		t.Errorf("stored b = %+v, %v", got, err)
	}
}

func TestReplyParentMustBeACommentOfTheSameEntity(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	other := add(t, s, job2, "on another entity")
	for name, parent := range map[string]string{
		"missing":      "00000000-0000-4000-8000-00000000ffff",
		"other entity": other.ID,
		"not a uuid":   "nope",
		"hyphen-less":  strings.ReplaceAll(other.ID, "-", ""),
	} {
		_, err := s.Create(NewComment{EntityType: "job", EntityID: job1, Body: "x", AuthorID: "u1", ParentID: parent})
		if v, ok := IsValidation(err); !ok || v.Field != "parentId" {
			t.Errorf("%s: err = %v, want a parentId validation error", name, err)
		}
	}
	// A parent in another owner's file is simply not there.
	careerTop := add(t, s, job1, "career")
	_, err := store(t, m, "cinqo").Create(NewComment{EntityType: "job", EntityID: job1, Body: "x", AuthorID: "u1", ParentID: careerTop.ID})
	if v, ok := IsValidation(err); !ok || v.Field != "parentId" {
		t.Errorf("cross-owner: err = %v", err)
	}
}

func TestReplyDepthIsCapped(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	c := add(t, s, job1, "level 0")
	for d := 1; d <= MaxDepth; d++ {
		c = reply(t, s, job1, c.ID, fmt.Sprintf("level %d", d))
		if c.Depth != d {
			t.Fatalf("depth = %d, want %d", c.Depth, d)
		}
	}
	_, err := s.Create(NewComment{EntityType: "job", EntityID: job1, Body: "too deep", AuthorID: "u1", ParentID: c.ID})
	if v, ok := IsValidation(err); !ok || v.Field != "parentId" || !strings.Contains(v.Message, "levels deep") {
		t.Errorf("err = %v, want the depth error", err)
	}
}

func TestThreadSizeIsCapped(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	top := add(t, s, job1, "top")
	for i := 1; i < MaxThreadSize; i++ {
		reply(t, s, job1, top.ID, fmt.Sprintf("r%d", i))
	}
	_, err := s.Create(NewComment{EntityType: "job", EntityID: job1, Body: "one too many", AuthorID: "u1", ParentID: top.ID})
	if v, ok := IsValidation(err); !ok || v.Field != "parentId" || !strings.Contains(v.Message, "full") {
		t.Errorf("err = %v, want the thread-full error", err)
	}
	// Another thread of the same entity is unaffected.
	other := add(t, s, job1, "another thread")
	reply(t, s, job1, other.ID, "fine")
}

func TestDeleteRemovesTheWholeSubtreeOnly(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	top := add(t, s, job1, "top")
	a := reply(t, s, job1, top.ID, "a")
	a1 := reply(t, s, job1, a.ID, "a1")
	a2 := reply(t, s, job1, a1.ID, "a2")
	b := reply(t, s, job1, top.ID, "b")
	otherThread := add(t, s, job1, "other thread")
	onOtherEntity := add(t, s, job2, "other entity")

	n, err := s.DeleteThread("job", job1, a.ID)
	if err != nil || n != 3 {
		t.Fatalf("DeleteThread = %d, %v; want 3", n, err)
	}
	for _, gone := range []Comment{a, a1, a2} {
		if _, err := s.Get("job", job1, gone.ID); err != ErrNotFound {
			t.Errorf("%q should be deleted: %v", gone.Body, err)
		}
	}
	for _, kept := range []Comment{top, b, otherThread} {
		if _, err := s.Get("job", job1, kept.ID); err != nil {
			t.Errorf("%q should remain: %v", kept.Body, err)
		}
	}
	if _, err := s.Get("job", job2, onOtherEntity.ID); err != nil {
		t.Errorf("another entity's comment must remain: %v", err)
	}

	// A leaf leaves its siblings; deleting what is already gone is not found.
	if err := s.Delete("job", job1, b.ID); err != nil {
		t.Errorf("delete leaf: %v", err)
	}
	if _, err := s.Get("job", job1, top.ID); err != nil {
		t.Errorf("parent of a deleted leaf must remain: %v", err)
	}
	if err := s.Delete("job", job1, a.ID); err != ErrNotFound {
		t.Errorf("second delete = %v, want ErrNotFound", err)
	}
	// Only through its own entity.
	if err := s.Delete("job", job2, top.ID); err != ErrNotFound {
		t.Errorf("delete under the wrong entity = %v, want ErrNotFound", err)
	}
}

func TestDatabaseEnforcesDepthItself(t *testing.T) {
	m, dir := testManager(t)
	store(t, m, "career")
	raw, _ := sql.Open("sqlite3", filepath.Join(dir, "db", "tools", "career", "comments.db"))
	defer raw.Close()
	_, err := raw.Exec(`INSERT INTO comments (id, entity_type, entity_id, body, author_id, created_at, depth) VALUES ('x','job','y','b','u','t', 6)`)
	if err == nil {
		t.Error("the CHECK constraint did not refuse depth 6")
	}
}

func TestRepliesShowDirectlyBelowTheirParent(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	t1 := add(t, s, job1, "t1")
	a := reply(t, s, job1, t1.ID, "a")
	t2 := add(t, s, job1, "t2")
	b := reply(t, s, job1, t1.ID, "b")
	reply(t, s, job1, a.ID, "a1")
	reply(t, s, job1, t2.ID, "c")
	reply(t, s, job1, b.ID, "b1")
	reply(t, s, job1, a.ID, "a2")

	page, err := s.List("job", job1, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Threads newest first; inside, depth-first with replies oldest first.
	if got, want := bodies(page), "t2,c,t1,a,a1,a2,b,b1"; got != want {
		t.Errorf("order = %s, want %s", got, want)
	}
	depths := map[string]int{"t1": 0, "a": 1, "a1": 2, "b": 1, "b1": 2}
	for _, c := range page.Comments {
		if want, ok := depths[c.Body]; ok && c.Depth != want {
			t.Errorf("%s depth = %d, want %d", c.Body, c.Depth, want)
		}
		if c.Context {
			t.Errorf("%s is context without a filter", c.Body)
		}
	}
	if page.Total != 8 || page.NextBefore != "" {
		t.Errorf("total = %d, next = %q", page.Total, page.NextBefore)
	}
}

func TestPagingCountsThreadsAndNeverSplitsOne(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	var tops []Comment
	for i := 1; i <= 5; i++ {
		top := add(t, s, job1, fmt.Sprintf("t%d", i))
		tops = append(tops, top)
		reply(t, s, job1, top.ID, fmt.Sprintf("t%d-r1", i))
		reply(t, s, job1, top.ID, fmt.Sprintf("t%d-r2", i))
	}
	var walked []string
	before := ""
	for i := 0; i < 6; i++ {
		page, err := s.List("job", job1, ListOptions{Limit: 2, Before: before})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Comments) != 0 && page.NextBefore != "" && len(page.Comments) != 6 {
			t.Errorf("page %d has %d comments, want 2 whole threads of 3", i, len(page.Comments))
		}
		walked = append(walked, bodies(page))
		if page.NextBefore == "" {
			break
		}
		before = page.NextBefore
	}
	want := "t5,t5-r1,t5-r2,t4,t4-r1,t4-r2|t3,t3-r1,t3-r2,t2,t2-r1,t2-r2|t1,t1-r1,t1-r2"
	if got := strings.Join(walked, "|"); got != want {
		t.Errorf("walked %s, want %s", got, want)
	}

	// A reply's id works as a cursor and means its thread.
	r := reply(t, s, job1, tops[2].ID, "late reply to t3")
	page, err := s.List("job", job1, ListOptions{Limit: 10, Before: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got := bodies(page); got != "t2,t2-r1,t2-r2,t1,t1-r1,t1-r2" {
		t.Errorf("after a reply cursor: %s", got)
	}
}

func TestFiltersKeepAncestorsAsContext(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	mk := func(parent, author, body string) Comment {
		c, err := s.Create(NewComment{EntityType: "job", EntityID: job1, Body: body, AuthorID: author, ParentID: parent})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	t1 := mk("", "ann", "t1")
	a := mk(t1.ID, "bob", "a")
	mk(a.ID, "ann", "a1") // the only match in thread 1
	mk("", "bob", "t2")   // a thread with no match at all
	t3 := mk("", "ann", "t3")
	mk(t3.ID, "bob", "t3-r")

	page, err := s.List("job", job1, ListOptions{AuthorID: "ann"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := bodies(page), "t3,t1,a,a1"; got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}
	ctx := map[string]bool{}
	for _, c := range page.Comments {
		ctx[c.Body] = c.Context
	}
	if ctx["a"] != true || ctx["t1"] || ctx["a1"] || ctx["t3"] {
		t.Errorf("context flags = %v; only a (bob's, shown for ann's reply) is context", ctx)
	}
	if page.Total != 3 {
		t.Errorf("total = %d, want the 3 matches (t1, a1, t3)", page.Total)
	}

	// A date range works the same way and combines with paging by thread.
	page, err = s.List("job", job1, ListOptions{AuthorID: "ann", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if bodies(page) != "t3" || page.NextBefore == "" {
		t.Errorf("first page = %s next %q", bodies(page), page.NextBefore)
	}
	page, err = s.List("job", job1, ListOptions{AuthorID: "ann", Limit: 1, Before: page.NextBefore})
	if err != nil || bodies(page) != "t1,a,a1" || page.NextBefore != "" {
		t.Errorf("second page = %s next %q, %v", bodies(page), page.NextBefore, err)
	}
}

func TestListRejectsACursorFromAnotherEntityOrAThatNeverExisted(t *testing.T) {
	m, _ := testManager(t)
	s := store(t, m, "career")
	other := add(t, s, job2, "elsewhere")
	for _, before := range []string{other.ID, "00000000-0000-4000-8000-00000000ffff"} {
		_, err := s.List("job", job1, ListOptions{Before: before})
		if v, ok := IsValidation(err); !ok || v.Field != "before" {
			t.Errorf("before %s: err = %v", before, err)
		}
	}
}
