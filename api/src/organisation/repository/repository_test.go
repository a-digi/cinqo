package repository_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/a-digi/cinqo/src/organisation/entity"
	"github.com/a-digi/cinqo/src/organisation/repository/persistent"
	"github.com/a-digi/cinqo/src/organisation/repository/query"
)

// openDB applies every real migration, in file-name order, to an in-memory database: the tests run against the schema that ships.
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

func repos(t *testing.T) (*persistent.OrganisationPersistentRepo, *query.OrganisationQueryRepo, *sql.DB) {
	db := openDB(t)
	return persistent.NewOrganisationPersistentRepo(db), query.NewOrganisationQueryRepo(db), db
}

func TestTheMigrationCreatesTheDefaultOrganisationAndIsSafeToRunAgain(t *testing.T) {
	_, q, db := repos(t)
	o, err := q.FindByID(entity.DefaultID)
	if err != nil || o.Name != "Default" || o.Status != entity.StatusActive || o.UpdatedAt != "" {
		t.Fatalf("%+v %v", o, err)
	}
	raw, _ := os.ReadFile(filepath.Join("..", "..", "..", "config", "db", "migrations", "31_10_2026_23_59_30.sql"))
	for _, stmt := range strings.Split(string(raw), "/***Statement***/") {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("a second run must do nothing: %v", err)
		}
	}
	list, _ := q.List()
	if len(list) != 1 {
		t.Errorf("one default organisation, not two: %+v", list)
	}
}

func TestCreateTrimsValidatesAndListsByName(t *testing.T) {
	p, q, _ := repos(t)
	for _, bad := range []string{"", "   ", strings.Repeat("x", entity.MaxNameLength+1)} {
		if _, err := p.Create(bad); !errors.Is(err, persistent.ErrInvalidName) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	long := strings.Repeat("ä", entity.MaxNameLength) // counted in characters, not bytes
	if _, err := p.Create(long); err != nil {
		t.Errorf("exactly the limit is fine: %v", err)
	}
	b, err := p.Create("  beta  ")
	if err != nil || b.Name != "beta" || b.Status != entity.StatusActive || b.ID == "" || b.CreatedAt == "" {
		t.Fatalf("%+v %v", b, err)
	}
	a, _ := p.Create("Alpha")
	if again, _ := p.Create("Alpha"); again.ID == a.ID {
		t.Error("every organisation has its own id")
	}
	list, err := q.List()
	if err != nil || len(list) != 5 || list[0].Name != "Alpha" || list[1].Name != "Alpha" || list[2].Name != "beta" { // Default sorts after beta; case-insensitive
		t.Errorf("%+v %v", list, err)
	}
}

func TestUpdateChangesNameAndStatusAndRefusesWhatIsWrong(t *testing.T) {
	p, q, _ := repos(t)
	o, _ := p.Create("Acme")
	if err := p.Update(o.ID, " Acme Ltd ", entity.StatusSuspended); err != nil {
		t.Fatal(err)
	}
	got, _ := q.FindByID(o.ID)
	if got.Name != "Acme Ltd" || got.Status != entity.StatusSuspended || got.UpdatedAt == "" {
		t.Errorf("%+v", got)
	}
	if err := p.Update(o.ID, "x", "paused"); !errors.Is(err, persistent.ErrInvalidStatus) {
		t.Errorf("%v", err)
	}
	if err := p.Update(o.ID, " ", entity.StatusActive); !errors.Is(err, persistent.ErrInvalidName) {
		t.Errorf("%v", err)
	}
	if err := p.Update("nope", "x", entity.StatusActive); !errors.Is(err, persistent.ErrNoSuchOrg) {
		t.Errorf("%v", err)
	}
	if after, _ := q.FindByID(o.ID); after.Name != "Acme Ltd" || after.Status != entity.StatusSuspended {
		t.Errorf("a refused update changes nothing: %+v", after)
	}
	if _, err := q.FindByID("nope"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("%v", err)
	}
}

func TestAUserBelongsToExactlyOneOrganisation(t *testing.T) {
	p, q, db := repos(t)
	a, _ := p.Create("A")
	b, _ := p.Create("B")
	if _, err := q.FindByUser("u1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("a user nobody assigned has none: %v", err)
	}
	if err := p.Assign("u1", a.ID); err != nil {
		t.Fatal(err)
	}
	if err := p.Assign("u1", b.ID); err != nil { // a move, not a second membership
		t.Fatal(err)
	}
	o, err := q.FindByUser("u1")
	if err != nil || o.ID != b.ID {
		t.Fatalf("%+v %v", o, err)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM user_organisations WHERE user_id = 'u1'`).Scan(&n)
	if n != 1 {
		t.Errorf("%d memberships", n)
	}
	if m, _ := q.Members(a.ID); len(m) != 0 {
		t.Errorf("the old organisation lost the user: %+v", m)
	}
	_ = p.Assign("u2", b.ID)
	m, _ := q.Members(b.ID)
	if len(m) != 2 || m[0].UserID != "u1" || m[1].UserID != "u2" {
		t.Errorf("%+v", m)
	}
	// a user can never be put into nothing or into an organisation that does not exist
	if err := p.Assign("u1", "nope"); !errors.Is(err, persistent.ErrNoSuchOrg) {
		t.Errorf("%v", err)
	}
	if o, _ := q.FindByUser("u1"); o.ID != b.ID {
		t.Errorf("a refused move leaves the user where they were: %+v", o)
	}
	for _, bad := range []string{"", "  "} {
		if err := p.Assign(bad, a.ID); !errors.Is(err, persistent.ErrInvalidUser) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO user_organisations (user_id, org_id, created_at) VALUES ('u9', 'ghost', 'x')`); err == nil {
		t.Error("the database itself refuses an unknown organisation")
	}
}

func TestAttachDefaultOnlyAttachesAUserWhoHasNone(t *testing.T) {
	p, q, _ := repos(t)
	if ok, err := p.AttachDefault("u1"); err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if o, _ := q.FindByUser("u1"); o.ID != entity.DefaultID {
		t.Errorf("%+v", o)
	}
	if ok, _ := p.AttachDefault("u1"); ok {
		t.Error("a second attach does nothing")
	}
	other, _ := p.Create("Other")
	_ = p.Assign("u2", other.ID)
	if ok, _ := p.AttachDefault("u2"); ok {
		t.Error("a user who has an organisation is left alone")
	}
	if o, _ := q.FindByUser("u2"); o.ID != other.ID {
		t.Errorf("%+v", o)
	}
	if _, err := p.AttachDefault(" "); !errors.Is(err, persistent.ErrInvalidUser) {
		t.Errorf("%v", err)
	}
}

func TestAMoveUpdatesTheJoinTimeAndMembersAreOldestFirst(t *testing.T) {
	p, q, _ := repos(t)
	clock := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	p.SetClock(func() time.Time { return clock })
	a, _ := p.Create("A")
	b, _ := p.Create("B")
	_ = p.Assign("zed", b.ID) // 10:00
	clock = clock.Add(time.Hour)
	_ = p.Assign("amy", b.ID) // 11:00
	m, _ := q.Members(b.ID)
	if len(m) != 2 || m[0].UserID != "zed" || m[1].UserID != "amy" {
		t.Fatalf("oldest first, not by name: %+v", m)
	}
	clock = clock.Add(time.Hour)
	_ = p.Assign("zed", a.ID)
	_ = p.Assign("zed", b.ID) // moved away and back at 12:00
	m, _ = q.Members(b.ID)
	if len(m) != 2 || m[0].UserID != "amy" || m[1].UserID != "zed" || m[1].CreatedAt != "2026-11-15T12:00:00Z" {
		t.Errorf("a move restarts the join time: %+v", m)
	}
}

func TestTheDatabaseItselfRefusesABadStatusOrName(t *testing.T) {
	_, _, db := repos(t)
	for _, q := range []string{
		`INSERT INTO organisations (id, name, status, created_at) VALUES ('x1', 'ok', 'paused', 'x')`,
		`INSERT INTO organisations (id, name, status, created_at) VALUES ('x2', '', 'active', 'x')`,
		`INSERT INTO organisations (id, name, status, created_at) VALUES ('x3', '` + strings.Repeat("n", 121) + `', 'active', 'x')`,
	} {
		if _, err := db.Exec(q); err == nil {
			t.Errorf("accepted: %.80s", q)
		}
	}
	if _, err := db.Exec(`INSERT INTO organisations (id, name, status, created_at) VALUES ('x4', 'ok', 'suspended', 'x')`); err != nil {
		t.Errorf("a suspended organisation is valid: %v", err)
	}
}

func TestTheBackgroundSwitchIsOffUntilSomeoneSwitchesItOnAndASuspendedOrganisationIsNeverListed(t *testing.T) {
	p, q, _ := repos(t)
	p.SetClock(func() time.Time { return time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC) })
	a, _ := p.Create("A")
	b, _ := p.Create("B")
	if on, err := q.BackgroundEnabled(a.ID); err != nil || on {
		t.Fatalf("off by default: %v %v", on, err)
	}
	if on, _ := q.BackgroundEnabled("no-such-organisation"); on {
		t.Error("an unknown organisation is off")
	}
	if list, _ := q.BackgroundOrganisations(); len(list) != 0 {
		t.Errorf("%v", list)
	}
	if err := p.SetBackground(a.ID, true, "andet"); err != nil {
		t.Fatal(err)
	}
	if err := p.SetBackground(b.ID, true, "andet"); err != nil {
		t.Fatal(err)
	}
	if on, _ := q.BackgroundEnabled(a.ID); !on {
		t.Error("switched on")
	}
	want := []string{a.ID, b.ID}
	sort.Strings(want)
	if list, _ := q.BackgroundOrganisations(); strings.Join(list, ",") != strings.Join(want, ",") {
		t.Errorf("%v, want %v", list, want)
	}
	_ = p.Update(a.ID, "A", entity.StatusSuspended)
	if list, _ := q.BackgroundOrganisations(); len(list) != 1 || list[0] != b.ID {
		t.Errorf("a suspended organisation is never listed, whatever its switch says: %v", list)
	}
	if on, _ := q.BackgroundEnabled(a.ID); !on {
		t.Error("the switch itself keeps its value (it is the list that leaves the suspended one out)")
	}
	_ = p.Update(a.ID, "A", entity.StatusActive)
	if err := p.SetBackground(a.ID, false, "andet"); err != nil {
		t.Fatal(err)
	}
	if err := p.SetBackground(a.ID, false, "andet"); err != nil {
		t.Errorf("switching to the state it has is no error: %v", err)
	}
	if on, _ := q.BackgroundEnabled(a.ID); on {
		t.Error("switched off")
	}
	if list, _ := q.BackgroundOrganisations(); len(list) != 1 || list[0] != b.ID {
		t.Errorf("%v", list)
	}
	if err := p.SetBackground("nope", true, "andet"); !errors.Is(err, persistent.ErrNoSuchOrg) {
		t.Errorf("%v", err)
	}
	if err := p.SetBackground(a.ID, true, "  "); !errors.Is(err, persistent.ErrInvalidUser) {
		t.Errorf("a switch needs the person who made it: %v", err)
	}
	if on, _ := q.BackgroundEnabled(a.ID); on {
		t.Error("a refused switch changes nothing")
	}
}

func TestTheDatabaseItselfRefusesABackgroundSwitchThatIsNeitherOnNorOff(t *testing.T) {
	p, _, db := repos(t)
	a, _ := p.Create("A")
	if _, err := db.Exec(`INSERT INTO organisation_background (org_id, enabled, enabled_by, updated_at) VALUES (?, 2, 'u', 'x')`, a.ID); err == nil {
		t.Error("accepted")
	}
	if _, err := db.Exec(`INSERT INTO organisation_background (org_id, enabled, enabled_by, updated_at) VALUES (?, 1, 'u', 'x')`, a.ID); err != nil {
		t.Errorf("%v", err)
	}
}

func TestBackgroundStatesListsTheSwitchOfEveryOrganisationThatHasOne(t *testing.T) {
	p, q, _ := repos(t)
	a, _ := p.Create("A")
	b, _ := p.Create("B")
	c, _ := p.Create("C")
	_ = p.SetBackground(a.ID, true, "op")
	_ = p.SetBackground(b.ID, true, "op")
	_ = p.SetBackground(b.ID, false, "op")
	got, err := q.BackgroundStates()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[a.ID] || got[b.ID] {
		t.Errorf("%v", got)
	}
	if _, has := got[c.ID]; has {
		t.Error("an organisation nobody touched has no row")
	}
	if empty, _ := repos2(t); len(empty) != 0 {
		t.Errorf("%v", empty)
	}
}

func repos2(t *testing.T) (map[string]bool, error) {
	_, q, _ := repos(t)
	return q.BackgroundStates()
}
