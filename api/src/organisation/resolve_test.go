package organisation_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/a-digi/cinqo/src/organisation"
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
	files, _ := filepath.Glob(filepath.Join("..", "..", "config", "db", "migrations", "*.sql"))
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

func TestOfAttachesANewUserToTheDefaultOrganisationAndKeepsAnExistingOne(t *testing.T) {
	db := openDB(t)
	o, err := organisation.Of(db, "u1")
	if err != nil || o.ID != entity.DefaultID {
		t.Fatalf("%+v %v", o, err)
	}
	other, _ := persistent.NewOrganisationPersistentRepo(db).Create("Other")
	_ = persistent.NewOrganisationPersistentRepo(db).Assign("u2", other.ID)
	if o, _ := organisation.Of(db, "u2"); o.ID != other.ID {
		t.Errorf("a user who has one keeps it: %+v", o)
	}
	if o, _ := organisation.Of(db, "u1"); o.ID != entity.DefaultID {
		t.Errorf("asking twice changes nothing: %+v", o)
	}
	for _, bad := range []string{"", "  "} {
		if _, err := organisation.Of(db, bad); !errors.Is(err, organisation.ErrNoUser) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM user_organisations`).Scan(&n)
	if n != 2 {
		t.Errorf("an empty user id attaches nobody: %d rows", n)
	}
}

func TestTheRouteFileMatchesTheScopeAndTheRegisteredExecutors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "config", "routes", "route-organisation.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	routesGo, _ := os.ReadFile(filepath.Join("..", "..", "config", "routes", "routes.go"))
	var executors, scopes int
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if name, ok := strings.CutPrefix(line, "executor: "); ok {
			executors++
			if !strings.Contains(string(routesGo), `"`+name+`":`) {
				t.Errorf("%s is declared in the route file but not registered in routes.Init", name)
			}
		}
		if line == "- "+organisation.ScopeGroup.Scopes[0].ID {
			scopes++
		}
	}
	if executors != 8 || scopes != 7 {
		t.Errorf("8 routes, 7 with the organisations scope (the eighth is the public one a tool asks with its service token): %d executors, %d scope lines", executors, scopes)
	}
	if strings.Count(string(raw), "- cinqo:super:admin") != 7 || strings.Count(string(raw), "security: public") != 1 || strings.Count(string(raw), "security: authenticated") != 7 {
		t.Error("seven routes are authenticated with the super admin always passing, and exactly one is public")
	}
	i := strings.Index(string(raw), "security: public")
	if block := string(raw)[strings.LastIndex(string(raw)[:i], "- path:"):i]; !strings.Contains(block, "/api/v1/background/organisations") || !strings.Contains(block, "executor: BackgroundOrganisations") {
		t.Errorf("the one public route is the tool's own question: %s", block)
	}
	if !strings.Contains(string(routesGo), "scopes.Register(organisation.ScopeGroup)") {
		t.Error("the scope group is registered")
	}
}

// Of runs on every tool request: for a user who already has an organisation it must only read.
func TestOfDoesNotWriteForAUserWhoAlreadyHasAnOrganisation(t *testing.T) {
	db := openDB(t)
	if _, err := organisation.Of(db, "u1"); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE attempts (n INTEGER)`,
		`CREATE TRIGGER count_attempts BEFORE INSERT ON user_organisations BEGIN INSERT INTO attempts VALUES (1); END`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		if _, err := organisation.Of(db, "u1"); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM attempts`).Scan(&n)
	if n != 0 {
		t.Errorf("%d write attempts for a known user", n)
	}
	if _, err := organisation.Of(db, "new"); err != nil {
		t.Fatal(err)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM attempts`).Scan(&n)
	if n != 1 {
		t.Errorf("a new user is attached once: %d", n)
	}
}
