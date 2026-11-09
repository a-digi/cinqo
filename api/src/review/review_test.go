package review

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	company1 = "11111111-1111-4111-8111-111111111111"
	company2 = "22222222-2222-4222-8222-222222222222"
	company3 = "33333333-3333-4333-8333-333333333333"
)

// clock is a fake time source that moves only when told to.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newClock() *clock { return &clock{t: time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)} }

func newManager(t *testing.T) (*Manager, *clock, string) {
	t.Helper()
	dir := t.TempDir()
	m := NewManager(dir)
	c := newClock()
	m.now = c.now
	n := 0
	m.newID = func() string {
		n++
		return fmt.Sprintf("00000000-0000-4000-8000-%012d", n)
	}
	t.Cleanup(func() { m.Close() })
	return m, c, dir
}

func store(t *testing.T, m *Manager, owner string) *Store {
	t.Helper()
	s, err := m.Store(owner)
	if err != nil {
		t.Fatalf("store %s: %v", owner, err)
	}
	return s
}

func put(rating float64, author string) PutReview {
	return PutReview{EntityType: "company", EntityID: company1, Scale: 5, HalfStars: true, Rating: rating, AuthorID: author, AuthorName: author}
}

func mustPut(t *testing.T, s *Store, in PutReview) Review {
	t.Helper()
	r, err := s.Put(in)
	if err != nil {
		t.Fatalf("put %+v: %v", in, err)
	}
	return r
}

func fieldOf(t *testing.T, err error) string {
	t.Helper()
	v, ok := IsValidation(err)
	if !ok {
		t.Fatalf("want a validation error, got %v", err)
	}
	return v.Field
}

func TestAReviewIsStoredWithItsRatingScaleAndAuthor(t *testing.T) {
	m, _, _ := newManager(t)
	s := store(t, m, "career")
	r := mustPut(t, s, PutReview{EntityType: "company", EntityID: strings.ToUpper(company1), Scale: 5, HalfStars: true, Rating: 3.5, Body: "  good place  ", AuthorID: " u1 ", AuthorName: " Ann "})
	if r.Rating != 3.5 || r.Scale != 5 || r.Body != "good place" || r.AuthorID != "u1" || r.AuthorName != "Ann" {
		t.Errorf("%+v", r)
	}
	if r.EntityID != company1 || r.EntityType != "company" {
		t.Errorf("the entity id is stored in its lower-case form: %+v", r)
	}
	if r.ID == "" || r.CreatedAt != "2026-11-09T10:00:00.000Z" || r.UpdatedAt != "" {
		t.Errorf("%+v", r)
	}
	got, err := s.GetByAuthor("company", company1, "u1")
	if err != nil || got != r {
		t.Errorf("%+v %v", got, err)
	}
	byID, err := s.Get("company", company1, r.ID)
	if err != nil || byID != r {
		t.Errorf("%+v %v", byID, err)
	}
}

func TestTheRatingRulesForEachScale(t *testing.T) {
	m, _, _ := newManager(t)
	s := store(t, m, "career")
	cases := []struct {
		scale int
		half  bool
		in    float64
		want  float64 // 0 = refused
	}{
		{5, true, 0.5, 0.5}, {5, true, 3.5, 3.5}, {5, true, 5, 5}, {5, true, 1, 1},
		{5, true, 0, 0}, {5, true, 5.5, 0}, {5, true, 3.25, 0}, {5, true, -1, 0}, {5, true, 3.4999, 0},
		{5, false, 4, 4}, {5, false, 1, 1}, {5, false, 3.5, 0}, {5, false, 0.5, 0}, {5, false, 0, 0}, {5, false, 6, 0},
		{3, true, 2.5, 2.5}, {3, true, 3, 3}, {3, true, 3.5, 0}, {3, false, 3, 3}, {3, false, 2.5, 0},
		{10, true, 9.5, 9.5}, {10, true, 10, 10}, {10, true, 10.5, 0}, {10, false, 10, 10}, {10, false, 9.5, 0},
		{4, true, 2, 0}, {0, true, 2, 0}, {11, false, 2, 0},
		{5, true, math.NaN(), 0}, {5, true, math.Inf(1), 0},
	}
	for i, c := range cases {
		author := fmt.Sprintf("author-%d", i)
		r, err := s.Put(PutReview{EntityType: "company", EntityID: company1, Scale: c.scale, HalfStars: c.half, Rating: c.in, AuthorID: author})
		if c.want == 0 {
			if err == nil {
				t.Errorf("scale %d halves %v rating %v: want refused, got %+v", c.scale, c.half, c.in, r)
				continue
			}
			f := fieldOf(t, err)
			if f != "rating" && f != "scale" {
				t.Errorf("scale %d halves %v rating %v: field %q", c.scale, c.half, c.in, f)
			}
			continue
		}
		if err != nil || r.Rating != c.want || r.Scale != c.scale {
			t.Errorf("scale %d halves %v rating %v: %+v %v", c.scale, c.half, c.in, r, err)
		}
	}
}

func TestAnInvalidScaleNamesTheScaleField(t *testing.T) {
	m, _, _ := newManager(t)
	s := store(t, m, "career")
	_, err := s.Put(PutReview{EntityType: "company", EntityID: company1, Scale: 4, Rating: 2, AuthorID: "u"})
	if fieldOf(t, err) != "scale" {
		t.Error(err)
	}
	if _, err := s.Summary("company", company1, 7); fieldOf(t, err) != "scale" {
		t.Error(err)
	}
	if _, err := s.Briefs("company", nil, 7); fieldOf(t, err) != "scale" {
		t.Error(err)
	}
}

func TestASecondPutByTheSameAuthorReplacesAndNeverDuplicates(t *testing.T) {
	m, c, _ := newManager(t)
	s := store(t, m, "career")
	first := mustPut(t, s, put(2, "u1"))
	c.advance(time.Hour)
	in := put(4.5, "u1")
	in.Body = "changed my mind"
	in.AuthorName = "Ann B"
	second := mustPut(t, s, in)
	if second.ID != first.ID || second.CreatedAt != first.CreatedAt {
		t.Errorf("the id and created_at stay: %+v vs %+v", first, second)
	}
	if second.Rating != 4.5 || second.Body != "changed my mind" || second.AuthorName != "Ann B" {
		t.Errorf("%+v", second)
	}
	if second.UpdatedAt != "2026-11-09T11:00:00.000Z" {
		t.Errorf("updated_at: %q", second.UpdatedAt)
	}
	page, _ := s.List("company", company1, ListOptions{})
	if page.Total != 1 || len(page.Reviews) != 1 {
		t.Errorf("one review per author: %+v", page)
	}
	// Another author makes a second review; so does the same author on another entity.
	mustPut(t, s, put(3, "u2"))
	other := put(1, "u1")
	other.EntityID = company2
	mustPut(t, s, other)
	page, _ = s.List("company", company1, ListOptions{})
	if page.Total != 2 {
		t.Errorf("%+v", page)
	}
}

func TestAReviewCanBeAScoreAloneOrCarryText(t *testing.T) {
	m, _, _ := newManager(t)
	s := store(t, m, "career")
	r := mustPut(t, s, put(3, "u1"))
	if r.Body != "" {
		t.Errorf("%q", r.Body)
	}
	in := put(3, "u2")
	in.Body = strings.Repeat("é", MaxBodyRunes)
	mustPut(t, s, in)
	in.AuthorID = "u3"
	in.Body = strings.Repeat("é", MaxBodyRunes+1)
	if _, err := s.Put(in); fieldOf(t, err) != "body" {
		t.Errorf("%v", err)
	}
	in.Body = "a\x00b"
	if _, err := s.Put(in); fieldOf(t, err) != "body" {
		t.Errorf("%v", err)
	}
}

func TestEntityTypeEntityIdAndAuthorAreChecked(t *testing.T) {
	m, _, _ := newManager(t)
	s := store(t, m, "career")
	base := put(3, "u1")
	for name, mod := range map[string]func(*PutReview){
		"entityType": func(p *PutReview) { p.EntityType = "Company" },
		"entityId":   func(p *PutReview) { p.EntityID = "not-a-uuid" },
		"authorId":   func(p *PutReview) { p.AuthorID = "  " },
	} {
		in := base
		mod(&in)
		if _, err := s.Put(in); fieldOf(t, err) != name {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, bad := range []string{"{" + company1 + "}", "urn:uuid:" + company1, strings.ReplaceAll(company1, "-", ""), ""} {
		in := base
		in.EntityID = bad
		if _, err := s.Put(in); fieldOf(t, err) != "entityId" {
			t.Errorf("%q: %v", bad, err)
		}
	}
	long := base
	long.AuthorID = strings.Repeat("a", maxAuthorIDRunes+1)
	if _, err := s.Put(long); fieldOf(t, err) != "authorId" {
		t.Errorf("%v", err)
	}
	name := base
	name.AuthorName = strings.Repeat("n", maxAuthorNameRunes+50)
	if r := mustPut(t, s, name); len([]rune(r.AuthorName)) != maxAuthorNameRunes {
		t.Errorf("a long display name is shortened: %d", len(r.AuthorName))
	}
	if !ValidEntityType("company") || ValidEntityType("1x") || ValidEntityType(strings.Repeat("a", 41)) {
		t.Error("ValidEntityType")
	}
}

func TestTwoOwnersAreTwoFilesAndNeverSeeEachOther(t *testing.T) {
	m, _, dir := newManager(t)
	career := store(t, m, "career")
	system := store(t, m, SystemOwner)
	other := store(t, m, "browser")
	mustPut(t, career, put(5, "u1"))
	if p, _ := system.List("company", company1, ListOptions{}); p.Total != 0 {
		t.Errorf("the system sees %+v", p)
	}
	if p, _ := other.List("company", company1, ListOptions{}); p.Total != 0 {
		t.Errorf("another tool sees %+v", p)
	}
	for _, f := range []string{"db/tools/career/career_review.db", "db/cinqo_review.db", "db/tools/browser/browser_review.db"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}

func TestOwnerNamesAndPaths(t *testing.T) {
	dir := t.TempDir()
	root, _ := filepath.Abs(dir)
	if p, err := DBPath(dir, "career"); err != nil || p != filepath.Join(root, "db", "tools", "career", "career_review.db") {
		t.Errorf("%s %v", p, err)
	}
	if p, err := DBPath(dir, SystemOwner); err != nil || p != filepath.Join(root, "db", "cinqo_review.db") {
		t.Errorf("%s %v", p, err)
	}
	for _, bad := range []string{"", "..", "../x", "a/b", `a\b`, "A", "a b", "a.b", ".hidden", "1x", "a-b", strings.Repeat("a", 65), "a\x00b", "/etc"} {
		if _, err := DBPath(dir, bad); !errors.Is(err, ErrInvalidOwner) {
			t.Errorf("%q: %v", bad, err)
		}
		m := NewManager(dir)
		if _, err := m.Store(bad); !errors.Is(err, ErrInvalidOwner) {
			t.Errorf("store %q: %v", bad, err)
		}
	}
	if _, err := DBPath(dir, strings.Repeat("a", 64)); err != nil {
		t.Errorf("64 characters are fine: %v", err)
	}
}

func TestAReopenedFileKeepsItsData(t *testing.T) {
	m, _, dir := newManager(t)
	mustPut(t, store(t, m, "career"), put(4, "u1"))
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	m2 := NewManager(dir)
	defer m2.Close()
	r, err := store(t, m2, "career").GetByAuthor("company", company1, "u1")
	if err != nil || r.Rating != 4 {
		t.Errorf("%+v %v", r, err)
	}
	if s1, s2 := store(t, m2, "career"), store(t, m2, "career"); s1 != s2 {
		t.Error("one store per owner")
	}
}

func TestAFileFromANewerBuildIsRefusedNotModified(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db", "tools", "career", "career_review.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf(`CREATE TABLE future (x); PRAGMA user_version = %d`, len(migrations)+1)); err != nil {
		t.Fatal(err)
	}
	db.Close()
	m := NewManager(dir)
	if _, err := m.Store("career"); err == nil || !strings.Contains(err.Error(), "newer version") {
		t.Errorf("%v", err)
	}
	db, _ = sql.Open("sqlite3", path)
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'reviews'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("the newer file was modified: %d %v", n, err)
	}
	var v int
	db.QueryRow(`PRAGMA user_version`).Scan(&v)
	if v != len(migrations)+1 {
		t.Errorf("version %d", v)
	}
}

func TestTheSchemaRefusesWhatItShouldNotStore(t *testing.T) {
	m, _, _ := newManager(t)
	s := store(t, m, "career")
	cases := []string{
		`INSERT INTO reviews (id, entity_type, entity_id, scale, half_steps, author_id, created_at) VALUES ('a','t','e',4,2,'u','x')`,
		`INSERT INTO reviews (id, entity_type, entity_id, scale, half_steps, author_id, created_at) VALUES ('a','t','e',5,0,'u','x')`,
		`INSERT INTO reviews (id, entity_type, entity_id, scale, half_steps, author_id, created_at) VALUES ('a','t','e',5,11,'u','x')`,
		`INSERT INTO reviews (id, entity_type, entity_id, scale, half_steps, author_id, created_at) VALUES ('a','t','e',3,7,'u','x')`,
		fmt.Sprintf(`INSERT INTO reviews (id, entity_type, entity_id, scale, half_steps, body, author_id, created_at) VALUES ('a','t','e',5,2,'%s','u','x')`, strings.Repeat("x", 2001)),
	}
	for _, q := range cases {
		if _, err := s.db.Exec(q); err == nil {
			t.Errorf("stored: %s", q)
		}
	}
	ok := `INSERT INTO reviews (id, entity_type, entity_id, scale, half_steps, author_id, created_at) VALUES ('%s','t','e',%d,%d,'u%s','x')`
	for i, c := range [][2]int{{3, 6}, {5, 10}, {10, 20}, {5, 1}} {
		if _, err := s.db.Exec(fmt.Sprintf(ok, fmt.Sprint(i), c[0], c[1], fmt.Sprint(i))); err != nil {
			t.Errorf("scale %d steps %d: %v", c[0], c[1], err)
		}
	}
	// One review per author per entity, enforced by the database too.
	dup := `INSERT INTO reviews (id, entity_type, entity_id, scale, half_steps, author_id, created_at) VALUES ('dup','t','e',5,2,'u0','x')`
	if _, err := s.db.Exec(dup); err == nil {
		t.Error("a second review by the same author was stored")
	}
}

func TestListIsNewestFirstWithCursorPaging(t *testing.T) {
	m, c, _ := newManager(t)
	s := store(t, m, "career")
	var ids []string
	for i := 0; i < 5; i++ {
		ids = append(ids, mustPut(t, s, put(3, fmt.Sprintf("u%d", i))).ID)
		c.advance(time.Minute)
	}
	p1, err := s.List("company", company1, ListOptions{Limit: 2})
	if err != nil || len(p1.Reviews) != 2 || p1.Total != 5 || p1.Reviews[0].ID != ids[4] || p1.Reviews[1].ID != ids[3] || p1.NextBefore != ids[3] {
		t.Fatalf("%+v %v", p1, err)
	}
	p2, _ := s.List("company", company1, ListOptions{Limit: 2, Before: p1.NextBefore})
	if len(p2.Reviews) != 2 || p2.Reviews[0].ID != ids[2] || p2.Reviews[1].ID != ids[1] || p2.NextBefore != ids[1] {
		t.Fatalf("%+v", p2)
	}
	p3, _ := s.List("company", company1, ListOptions{Limit: 2, Before: p2.NextBefore})
	if len(p3.Reviews) != 1 || p3.Reviews[0].ID != ids[0] || p3.NextBefore != "" {
		t.Fatalf("%+v", p3)
	}
	exact, _ := s.List("company", company1, ListOptions{Limit: 5})
	if len(exact.Reviews) != 5 || exact.NextBefore != "" {
		t.Errorf("a full last page has no next: %+v", exact)
	}
	empty, err := s.List("company", company2, ListOptions{})
	if err != nil || empty.Reviews == nil || len(empty.Reviews) != 0 || empty.Total != 0 {
		t.Errorf("%+v %v", empty, err)
	}
	if _, err := s.List("company", company1, ListOptions{Before: "nope"}); fieldOf(t, err) != "before" {
		t.Error(err)
	}
	if _, err := s.List("company", company1, ListOptions{Before: ids[0][:35] + "9"}); fieldOf(t, err) != "before" {
		t.Error(err)
	}
	other := put(2, "x")
	other.EntityID = company2
	foreign := mustPut(t, s, other)
	if _, err := s.List("company", company1, ListOptions{Before: foreign.ID}); fieldOf(t, err) != "before" {
		t.Errorf("a cursor from another entity: %v", err)
	}
}

func TestPageSizeDefaultsAndIsCapped(t *testing.T) {
	m, _, _ := newManager(t)
	s := store(t, m, "career")
	for i := 0; i < MaxPageSize+5; i++ {
		mustPut(t, s, put(3, fmt.Sprintf("u%03d", i)))
	}
	def, _ := s.List("company", company1, ListOptions{})
	if len(def.Reviews) != DefaultPageSize {
		t.Errorf("default %d", len(def.Reviews))
	}
	huge, _ := s.List("company", company1, ListOptions{Limit: 100000})
	if len(huge.Reviews) != MaxPageSize || huge.Total != MaxPageSize+5 {
		t.Errorf("capped %d of %d", len(huge.Reviews), huge.Total)
	}
	one, _ := s.List("company", company1, ListOptions{Limit: MaxPageSize + 1})
	if len(one.Reviews) != MaxPageSize {
		t.Errorf("one over the cap: %d", len(one.Reviews))
	}
	neg, _ := s.List("company", company1, ListOptions{Limit: -3})
	if len(neg.Reviews) != DefaultPageSize {
		t.Errorf("negative %d", len(neg.Reviews))
	}
}

func TestReviewsMadeInTheSameMillisecondKeepTheirOrder(t *testing.T) {
	m, c, _ := newManager(t)
	s := store(t, m, "career")
	a := mustPut(t, s, put(3, "a"))
	b := mustPut(t, s, put(3, "b"))
	c.advance(-time.Hour) // the clock steps back
	d := mustPut(t, s, put(3, "d"))
	if !(a.CreatedAt < b.CreatedAt && b.CreatedAt < d.CreatedAt) {
		t.Errorf("%s %s %s", a.CreatedAt, b.CreatedAt, d.CreatedAt)
	}
	p, _ := s.List("company", company1, ListOptions{})
	if p.Reviews[0].ID != d.ID || p.Reviews[2].ID != a.ID {
		t.Errorf("%+v", p)
	}
}

func TestTheSummaryCountsAveragesAndDistributes(t *testing.T) {
	m, _, _ := newManager(t)
	s := store(t, m, "career")
	for i, r := range []float64{5, 4, 4, 2.5, 0.5} {
		mustPut(t, s, put(r, fmt.Sprintf("u%d", i)))
	}
	sum, err := s.Summary("company", company1, 5)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Count != 5 || sum.Average != 3.2 || sum.OtherScale != 0 {
		t.Errorf("%+v", sum)
	}
	want := []int{1, 0, 0, 0, 1, 0, 0, 2, 0, 1}
	if fmt.Sprint(sum.Distribution) != fmt.Sprint(want) {
		t.Errorf("%v want %v", sum.Distribution, want)
	}
	none, err := s.Summary("company", company2, 5)
	if err != nil || none.Count != 0 || none.Average != 0 || len(none.Distribution) != 10 || none.OtherScale != 0 {
		t.Errorf("%+v %v", none, err)
	}
	for _, scale := range []int{3, 10} {
		z, _ := s.Summary("company", company2, scale)
		if len(z.Distribution) != scale*2 {
			t.Errorf("scale %d: %d entries", scale, len(z.Distribution))
		}
	}
	// 4 of 3 reviews... rounding to two decimals: 1, 1, 2 on three stars = 1.333.
	for i, r := range []float64{1, 1, 2} {
		in := PutReview{EntityType: "company", EntityID: company3, Scale: 3, Rating: r, AuthorID: fmt.Sprintf("u%d", i)}
		mustPut(t, s, in)
	}
	third, _ := s.Summary("company", company3, 3)
	if third.Average != 1.33 {
		t.Errorf("%v", third.Average)
	}
	if _, err := s.Summary("Company", company1, 5); fieldOf(t, err) != "entityType" {
		t.Error(err)
	}
	if _, err := s.Summary("company", "x", 5); fieldOf(t, err) != "entityId" {
		t.Error(err)
	}
}

func TestReviewsOnAnotherScaleAreLeftOutAndCounted(t *testing.T) {
	m, _, _ := newManager(t)
	s := store(t, m, "career")
	mustPut(t, s, put(4, "u1")) // five stars
	mustPut(t, s, put(5, "u2"))
	old := PutReview{EntityType: "company", EntityID: company1, Scale: 3, Rating: 1, AuthorID: "u3"}
	mustPut(t, s, old)
	sum, _ := s.Summary("company", company1, 5)
	if sum.Count != 2 || sum.Average != 4.5 || sum.OtherScale != 1 {
		t.Errorf("%+v", sum)
	}
	three, _ := s.Summary("company", company1, 3)
	if three.Count != 1 || three.Average != 1 || three.OtherScale != 2 {
		t.Errorf("%+v", three)
	}
	// The same author rating again on the new scale replaces the old one, scale included.
	r := mustPut(t, s, put(3, "u3"))
	if r.Scale != 5 || r.Rating != 3 {
		t.Errorf("%+v", r)
	}
	sum, _ = s.Summary("company", company1, 5)
	if sum.Count != 3 || sum.OtherScale != 0 {
		t.Errorf("%+v", sum)
	}
	// The list still shows every review, whatever its scale.
	p, _ := s.List("company", company1, ListOptions{})
	if p.Total != 3 {
		t.Errorf("%+v", p)
	}
}

func TestBriefsGiveEveryRequestedIdAndOnlyThatType(t *testing.T) {
	m, _, _ := newManager(t)
	s := store(t, m, "career")
	mustPut(t, s, put(5, "u1"))
	mustPut(t, s, put(3, "u2"))
	two := put(2, "u1")
	two.EntityID = company2
	mustPut(t, s, two)
	job := put(1, "u1")
	job.EntityType = "job"
	mustPut(t, s, job)
	old := PutReview{EntityType: "company", EntityID: company2, Scale: 10, Rating: 9, AuthorID: "u9"}
	mustPut(t, s, old)

	got, err := s.Briefs("company", []string{strings.ToUpper(company1), company2, company3, company1}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("%v", got)
	}
	if got[company1] != (Brief{Count: 2, Average: 4}) || got[company2] != (Brief{Count: 1, Average: 2}) || got[company3] != (Brief{}) {
		t.Errorf("%v", got)
	}
	empty, err := s.Briefs("company", nil, 5)
	if err != nil || len(empty) != 0 {
		t.Errorf("%v %v", empty, err)
	}
	var many []string
	for i := 0; i < MaxSummaryIDs; i++ {
		many = append(many, fmt.Sprintf("00000000-0000-4000-8000-%012d", i))
	}
	if _, err := s.Briefs("company", many, 5); err != nil {
		t.Errorf("%d ids are allowed: %v", MaxSummaryIDs, err)
	}
	if _, err := s.Briefs("company", append(many, company1), 5); fieldOf(t, err) != "ids" {
		t.Errorf("%v", err)
	}
	if _, err := s.Briefs("company", []string{company1, "nope"}, 5); fieldOf(t, err) != "ids" {
		t.Errorf("%v", err)
	}
	if _, err := s.Briefs("Company", []string{company1}, 5); fieldOf(t, err) != "entityType" {
		t.Errorf("%v", err)
	}
}

func TestDeletingMineIsIdempotentAndOnlyMine(t *testing.T) {
	m, _, _ := newManager(t)
	s := store(t, m, "career")
	mustPut(t, s, put(5, "u1"))
	mustPut(t, s, put(2, "u2"))
	gone, err := s.DeleteMine("company", company1, "u1")
	if err != nil || !gone {
		t.Fatalf("%v %v", gone, err)
	}
	if _, err := s.GetByAuthor("company", company1, "u1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("%v", err)
	}
	again, err := s.DeleteMine("company", company1, "u1")
	if err != nil || again {
		t.Errorf("deleting again is not an error: %v %v", again, err)
	}
	if _, err := s.GetByAuthor("company", company1, "u2"); err != nil {
		t.Errorf("someone else's review was touched: %v", err)
	}
	// Having deleted it, the author can review again, and it is a new review.
	back := mustPut(t, s, put(3, "u1"))
	if back.UpdatedAt != "" {
		t.Errorf("%+v", back)
	}
	if _, err := s.DeleteMine("company", company1, ""); fieldOf(t, err) != "authorId" {
		t.Error(err)
	}
	if _, err := s.DeleteMine("company", "x", "u1"); fieldOf(t, err) != "entityId" {
		t.Error(err)
	}
	if _, err := s.GetByAuthor("company", company1, " "); fieldOf(t, err) != "authorId" {
		t.Error(err)
	}
}

func TestDeletingByIdIsOnlyThroughTheEntity(t *testing.T) {
	m, _, _ := newManager(t)
	s := store(t, m, "career")
	r := mustPut(t, s, put(5, "u1"))
	if err := s.Delete("company", company2, r.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("another entity: %v", err)
	}
	if err := s.Delete("job", company1, r.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("another type: %v", err)
	}
	if _, err := s.Get("company", company2, r.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("get through another entity: %v", err)
	}
	if err := s.Delete("company", company1, "bad"); fieldOf(t, err) != "id" {
		t.Error(err)
	}
	if err := s.Delete("company", company1, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("company", company1, r.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("twice: %v", err)
	}
}

func TestPurgeRemovesOneEntityOnly(t *testing.T) {
	m, _, _ := newManager(t)
	s := store(t, m, "career")
	mustPut(t, s, put(5, "u1"))
	mustPut(t, s, put(4, "u2"))
	other := put(3, "u1")
	other.EntityID = company2
	mustPut(t, s, other)
	job := put(3, "u1")
	job.EntityType = "job"
	mustPut(t, s, job)
	n, err := s.PurgeEntity("company", company1)
	if err != nil || n != 2 {
		t.Fatalf("%d %v", n, err)
	}
	if p, _ := s.List("company", company2, ListOptions{}); p.Total != 1 {
		t.Errorf("another entity lost reviews: %+v", p)
	}
	if p, _ := s.List("job", company1, ListOptions{}); p.Total != 1 {
		t.Errorf("another type lost reviews: %+v", p)
	}
	n, err = s.PurgeEntity("company", company1)
	if err != nil || n != 0 {
		t.Errorf("purging nothing is fine: %d %v", n, err)
	}
	if _, err := s.PurgeEntity("Company", company1); fieldOf(t, err) != "entityType" {
		t.Error(err)
	}
	if _, err := s.PurgeEntity("company", "x"); fieldOf(t, err) != "entityId" {
		t.Error(err)
	}
}

func TestAHostileTextIsStoredAsTextAndNeverInterpreted(t *testing.T) {
	m, _, _ := newManager(t)
	s := store(t, m, "career")
	in := put(3, "u1'); DROP TABLE reviews;--")
	in.Body = `'); DROP TABLE reviews;-- <script>alert(1)</script>`
	r := mustPut(t, s, in)
	if r.Body != in.Body {
		t.Errorf("%q", r.Body)
	}
	if p, _ := s.List("company", company1, ListOptions{}); p.Total != 1 {
		t.Errorf("%+v", p)
	}
}

func TestConcurrentPutsByTheSameAuthorLeaveOneReview(t *testing.T) {
	m, _, _ := newManager(t)
	s := store(t, m, "career")
	done := make(chan error, 20)
	for i := 0; i < 20; i++ {
		go func(i int) {
			_, err := s.Put(put(float64(i%10+1)/2, "u1"))
			done <- err
		}(i)
	}
	for i := 0; i < 20; i++ {
		if err := <-done; err != nil {
			t.Error(err)
		}
	}
	if p, _ := s.List("company", company1, ListOptions{}); p.Total != 1 {
		t.Errorf("%+v", p)
	}
}

func TestTheLimitsAreTheDocumentedNumbers(t *testing.T) {
	if MaxSummaryIDs != 100 || MaxPageSize != 100 || DefaultPageSize != 50 || MaxBodyRunes != 2000 || MaxEntityTypeRunes != 40 {
		t.Errorf("the plan says 100 ids, pages of 50 up to 100, 2000 characters, 40 for a type: %d %d %d %d %d",
			MaxSummaryIDs, MaxPageSize, DefaultPageSize, MaxBodyRunes, MaxEntityTypeRunes)
	}
}

func TestADisplayNameIsShortenedOnlyWhenOverTheLimit(t *testing.T) {
	m, _, _ := newManager(t)
	s := store(t, m, "career")
	in := put(3, "u1")
	in.AuthorName = strings.Repeat("n", maxAuthorNameRunes)
	if r := mustPut(t, s, in); len([]rune(r.AuthorName)) != maxAuthorNameRunes {
		t.Errorf("%d", len(r.AuthorName))
	}
	in.AuthorName = strings.Repeat("n", maxAuthorNameRunes+1)
	if r := mustPut(t, s, in); len([]rune(r.AuthorName)) != maxAuthorNameRunes {
		t.Errorf("one over is cut: %d", len(r.AuthorName))
	}
}
