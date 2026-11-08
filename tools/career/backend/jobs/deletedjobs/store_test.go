package deletedjobs

import (
	"errors"
	"testing"
	"time"

	"career-tool-backend/db"
)

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatalf("InitDatabases: %v", err)
	}
	t.Cleanup(func() {
		db.JobsDB.Close()
		db.CareerDB.Close()
	})
}

type row struct {
	hash, url, title, company, location, portal, posted, reason, deletedAt string
	version                                                                int
	score                                                                  *int
}

func rows(t *testing.T) []row {
	t.Helper()
	r, err := db.DeletedJobsDB.Query(`SELECT url_hash, key_version, source_url, title, company, location, portal_name, posted_at, match_score, reason, deleted_at FROM deleted_jobs ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var out []row
	for r.Next() {
		var x row
		if err := r.Scan(&x.hash, &x.version, &x.url, &x.title, &x.company, &x.location, &x.portal, &x.posted, &x.score, &x.reason, &x.deletedAt); err != nil {
			t.Fatal(err)
		}
		out = append(out, x)
	}
	return out
}

func entry(url string) Entry {
	return Entry{SourceURL: url, Title: "Backend Engineer", Company: "Acme", Location: "Hamburg", PortalName: "StepStone", PostedAt: "today", Reason: ReasonDeleted}
}

func at(t *testing.T, ts string) {
	t.Helper()
	old := now
	parsed, _ := time.Parse("2006-01-02 15:04:05", ts)
	now = func() time.Time { return parsed }
	t.Cleanup(func() { now = old })
}

func TestRememberStoresTheSnapshotAndTheHash(t *testing.T) {
	setup(t)
	at(t, "2026-06-30 12:00:00")
	score := 42
	e := entry(realURLs[0])
	e.MatchScore = &score
	created, err := Remember(e)
	if err != nil || !created {
		t.Fatalf("%v, %v", created, err)
	}
	got := rows(t)
	if len(got) != 1 {
		t.Fatalf("%d rows", len(got))
	}
	r := got[0]
	if r.hash != URLHash(realURLs[0]) || r.version != KeyVersion || r.url != realURLs[0] || r.title != "Backend Engineer" || r.company != "Acme" ||
		r.location != "Hamburg" || r.portal != "StepStone" || r.posted != "today" || r.reason != "deleted" || r.deletedAt != "2026-06-30 12:00:00" ||
		r.score == nil || *r.score != 42 {
		t.Errorf("%+v", r)
	}
	// a job that never had a score keeps NULL
	if _, err := Remember(entry(realURLs[1])); err != nil {
		t.Fatal(err)
	}
	if got := rows(t); got[1].score != nil {
		t.Errorf("score %v, want NULL", *got[1].score)
	}
}

// One row per URL, however it is spelled: remembering again refreshes the snapshot, the reason and the time.
func TestRememberingTheSameURLAgainUpdatesTheRow(t *testing.T) {
	setup(t)
	at(t, "2026-06-30 12:00:00")
	if created, _ := Remember(entry(realURLs[0])); !created {
		t.Fatal("first not created")
	}
	at(t, "2026-07-01 08:30:00")
	again := entry("http://stepstone.de" + realURLs[0][len("https://www.stepstone.de"):] + "?utm_source=x#y")
	again.Title = "Renamed"
	again.Reason = ReasonLowMatch
	score := 7
	again.MatchScore = &score
	created, err := Remember(again)
	if err != nil || created {
		t.Fatalf("created=%v err=%v, want an update", created, err)
	}
	got := rows(t)
	if len(got) != 1 || got[0].title != "Renamed" || got[0].reason != "low_match" || got[0].deletedAt != "2026-07-01 08:30:00" || got[0].score == nil || *got[0].score != 7 || got[0].url != again.SourceURL {
		t.Errorf("%+v", got)
	}
}

func TestDifferentURLsAreDifferentRows(t *testing.T) {
	setup(t)
	for _, u := range realURLs {
		if created, err := Remember(entry(u)); err != nil || !created {
			t.Fatalf("%s: %v %v", u, created, err)
		}
	}
	if got := rows(t); len(got) != len(realURLs) {
		t.Errorf("%d rows, want %d", len(got), len(realURLs))
	}
}

func TestRememberRefusesWhatItCannotUse(t *testing.T) {
	setup(t)
	for _, blank := range []string{"", "  ", "\n"} {
		if _, err := Remember(entry(blank)); !errors.Is(err, ErrNoKey) {
			t.Errorf("blank %q: %v", blank, err)
		}
	}
	for _, reason := range []string{"", "older", "manual", "DELETED"} {
		e := entry(realURLs[0])
		e.Reason = reason
		if _, err := Remember(e); !errors.Is(err, ErrUnknownReason) {
			t.Errorf("reason %q: %v", reason, err)
		}
	}
	if len(rows(t)) != 0 {
		t.Error("something was stored")
	}
}

func TestForget(t *testing.T) {
	setup(t)
	Remember(entry(realURLs[0]))
	Remember(entry(realURLs[1]))
	// any equivalent spelling forgets the row
	if err := Forget("http://www.stepstone.de" + realURLs[0][len("https://www.stepstone.de"):] + "/"); err != nil {
		t.Fatal(err)
	}
	got := rows(t)
	if len(got) != 1 || got[0].url != realURLs[1] {
		t.Errorf("%+v", got)
	}
	if err := Forget("https://nothing.example/here"); err != nil {
		t.Errorf("forgetting an unknown URL: %v", err)
	}
	if err := Forget(""); !errors.Is(err, ErrNoKey) {
		t.Errorf("blank: %v", err)
	}
}

// With no database (it could not be opened) nothing panics and every call says so.
func TestWithoutTheDatabase(t *testing.T) {
	setup(t)
	saved := db.DeletedJobsDB
	db.DeletedJobsDB = nil
	defer func() { db.DeletedJobsDB = saved }()
	if _, err := Remember(entry(realURLs[0])); !errors.Is(err, ErrNoMemory) {
		t.Errorf("Remember: %v", err)
	}
	if err := Forget(realURLs[0]); !errors.Is(err, ErrNoMemory) {
		t.Errorf("Forget: %v", err)
	}
	if _, err := Rehash(); !errors.Is(err, ErrNoMemory) {
		t.Errorf("Rehash: %v", err)
	}
}

func insertOld(t *testing.T, version int, hash, url, deletedAt string) {
	t.Helper()
	if _, err := db.DeletedJobsDB.Exec(`INSERT INTO deleted_jobs (url_hash, key_version, source_url, title, reason, deleted_at) VALUES (?, ?, ?, 't', 'deleted', ?)`, hash, version, url, deletedAt); err != nil {
		t.Fatal(err)
	}
}

// A row made with other rules is recomputed from its readable URL; a current row is left alone; running it again does
// nothing.
func TestRehashRecomputesOutdatedRows(t *testing.T) {
	setup(t)
	insertOld(t, KeyVersion-1, "stale-hash-1", realURLs[0], "2026-01-01 00:00:00")
	insertOld(t, KeyVersion-1, "stale-hash-2", realURLs[1], "2026-01-02 00:00:00")
	Remember(entry(realURLs[2])) // current
	before := rows(t)[2]

	n, err := Rehash()
	if err != nil || n != 2 {
		t.Fatalf("rewrote %d, %v", n, err)
	}
	got := rows(t)
	if len(got) != 3 {
		t.Fatalf("%d rows", len(got))
	}
	for i, u := range realURLs[:2] {
		if got[i].hash != URLHash(u) || got[i].version != KeyVersion || got[i].url != u {
			t.Errorf("row %d: %+v", i, got[i])
		}
	}
	if got[2] != before {
		t.Errorf("a current row changed: %+v -> %+v", before, got[2])
	}
	if n, err := Rehash(); err != nil || n != 0 {
		t.Errorf("second run: %d, %v", n, err)
	}
}

// Two outdated rows that now normalize to the same URL become one, the most recently deleted; a stale row whose URL is
// already held by a current row is dropped.
func TestRehashMergesRowsThatBecomeTheSame(t *testing.T) {
	setup(t)
	insertOld(t, KeyVersion-1, "h-old", "https://www.example.com/jobs/1", "2026-01-01 00:00:00")
	insertOld(t, KeyVersion-1, "h-new", "http://example.com/jobs/1/?utm_source=a", "2026-03-01 00:00:00")
	insertOld(t, KeyVersion-1, "h-other", "https://example.com/jobs/2", "2026-02-01 00:00:00")
	Remember(entry("https://example.com/jobs/3")) // current
	insertOld(t, KeyVersion-1, "h-dup-of-current", "http://www.example.com/jobs/3", "2026-01-15 00:00:00")

	if _, err := Rehash(); err != nil {
		t.Fatal(err)
	}
	got := rows(t)
	byURL := map[string]row{}
	for _, r := range got {
		byURL[r.url] = r
	}
	if len(got) != 3 {
		t.Fatalf("%d rows: %+v", len(got), got)
	}
	if r, ok := byURL["http://example.com/jobs/1/?utm_source=a"]; !ok || r.hash != URLHash("https://example.com/jobs/1") {
		t.Errorf("the newer duplicate should survive: %+v", byURL)
	}
	if _, ok := byURL["https://www.example.com/jobs/1"]; ok {
		t.Error("the older duplicate survived")
	}
	if _, ok := byURL["https://example.com/jobs/2"]; !ok {
		t.Error("an unrelated row was lost")
	}
	if _, ok := byURL["http://www.example.com/jobs/3"]; ok {
		t.Error("a stale row duplicating a current one survived")
	}
	if _, ok := byURL["https://example.com/jobs/3"]; !ok {
		t.Error("the current row was lost")
	}
}

// The schema guards itself: one row per hash, only the two reasons.
func TestTheSchemaGuardsTheRows(t *testing.T) {
	setup(t)
	insertOld(t, KeyVersion, "h1", "https://a.example/1", "2026-01-01 00:00:00")
	if _, err := db.DeletedJobsDB.Exec(`INSERT INTO deleted_jobs (url_hash, key_version, source_url, title, reason) VALUES ('h1', 1, 'u', 't', 'deleted')`); err == nil {
		t.Error("a second row with the same hash was accepted")
	}
	if _, err := db.DeletedJobsDB.Exec(`INSERT INTO deleted_jobs (url_hash, key_version, source_url, title, reason) VALUES ('h2', 1, 'u', 't', 'older')`); err == nil {
		t.Error("a reason outside the two was accepted")
	}
	if _, err := db.DeletedJobsDB.Exec(`INSERT INTO deleted_jobs (url_hash, key_version, source_url, reason) VALUES ('h3', 1, 'u', 'deleted')`); err == nil {
		t.Error("a row without a title was accepted")
	}
}

func TestHas(t *testing.T) {
	setup(t)
	Remember(entry(realURLs[0]))
	yes := []string{
		realURLs[0],
		"http://stepstone.de" + realURLs[0][len("https://www.stepstone.de"):] + "/?utm_source=x#top",
	}
	for _, u := range yes {
		if has, err := Has(u); err != nil || !has {
			t.Errorf("Has(%q) = %v, %v", u, has, err)
		}
	}
	for _, u := range []string{realURLs[1], "https://nothing.example/x", realURLs[0] + "?id=2"} {
		if has, err := Has(u); err != nil || has {
			t.Errorf("Has(%q) = %v, %v", u, has, err)
		}
	}
	for _, blank := range []string{"", "   "} {
		if has, err := Has(blank); err != nil || has {
			t.Errorf("a blank URL has no key and is never remembered: Has(%q) = %v, %v", blank, has, err)
		}
	}
	// once forgotten it is not remembered
	Forget(realURLs[0])
	if has, _ := Has(realURLs[0]); has {
		t.Error("still remembered after Forget")
	}
}

func TestHasWithoutTheDatabase(t *testing.T) {
	setup(t)
	saved := db.DeletedJobsDB
	db.DeletedJobsDB = nil
	defer func() { db.DeletedJobsDB = saved }()
	if has, err := Has(realURLs[0]); has || !errors.Is(err, ErrNoMemory) {
		t.Errorf("%v, %v", has, err)
	}
}
