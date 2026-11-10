package handler

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-digi/coco-logger/logger"
	"github.com/a-digi/coco-orm/orm"
	serverdi "github.com/a-digi/coco-server/server/di"
	"github.com/a-digi/coco-server/server/request"
	_ "github.com/mattn/go-sqlite3"

	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/entity"
)

type keyCheckDI struct{ db *sql.DB }

func (f keyCheckDI) GetDatabaseManager() *orm.DatabaseManager {
	return &orm.DatabaseManager{Connector: &orm.Connector{DB: f.db}}
}
func (f keyCheckDI) GetLogger() logger.Logger { return nil }

var _ serverdi.Context = keyCheckDI{}

type keyCheckRequest struct {
	fakeCreateRequest
	di serverdi.Context
}

func (f keyCheckRequest) GetDI() serverdi.Context        { return f.di }
func (f keyCheckRequest) GetRequest() *http.Request      { return httptest.NewRequest("POST", "/", nil) }
func (f keyCheckRequest) GetWriter() http.ResponseWriter { return f.w }

var _ request.RequestContext = keyCheckRequest{}

func createWithKeys(t *testing.T, body string, keyed ...string) *httptest.ResponseRecorder {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	db.Exec(`CREATE TABLE platform_keys (id TEXT PRIMARY KEY, label TEXT NOT NULL, platform TEXT NOT NULL, encrypted_key TEXT NOT NULL, created_by TEXT NOT NULL, created_at TEXT NOT NULL)`)
	for i, p := range keyed {
		db.Exec(`INSERT INTO platform_keys VALUES (?, 'l', ?, 'x', 'u', '2026-11-10T10:00:00Z')`, string(rune('a'+i)), p)
	}
	rec := httptest.NewRecorder()
	CreateHandler(keyCheckRequest{fakeCreateRequest: fakeCreateRequest{w: rec, body: body}, di: keyCheckDI{db: db}})
	return rec
}

func TestCreateOnAPlatformThatNeedsAKeyNeedsOneStored(t *testing.T) {
	if err := platform.Register(platform.Entry{
		Info:           entity.Platform{ID: "create-keyed", Name: "Keyed"},
		Completer:      refusingCompleter{},
		DefaultBaseURL: "http://127.0.0.1:1",
		DefaultModel:   "m",
	}); err != nil {
		t.Fatal(err)
	}
	body := `{"platformId":"create-keyed"}`
	if rec := createWithKeys(t, body); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "no API key yet") {
		t.Errorf("no key: %d %s", rec.Code, rec.Body.String())
	}
	if rec := createWithKeys(t, body, "another-platform"); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "no API key yet") {
		t.Errorf("a key for another platform: %d %s", rec.Code, rec.Body.String())
	}
	// with its key the request gets past the check (and stops at the missing sign-in, which this test has no token for)
	if rec := createWithKeys(t, body, "create-keyed"); rec.Code != http.StatusUnauthorized {
		t.Errorf("with a key: %d %s, want it past the key check (401 here)", rec.Code, rec.Body.String())
	}
}

func TestCreateOnAPlatformThatNeedsNoKeyDoesNotLookForOne(t *testing.T) {
	if err := platform.Register(platform.Entry{
		Info:         entity.Platform{ID: "create-nokey", Name: "NoKey"},
		Completer:    refusingCompleter{},
		Credentials:  func(context.Context) (string, string, error) { return "http://127.0.0.1:1", "", nil },
		NoTools:      true,
		DefaultModel: "m",
	}); err != nil {
		t.Fatal(err)
	}
	if rec := createWithKeys(t, `{"platformId":"create-nokey"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("%d %s, want past the key check (401 here)", rec.Code, rec.Body.String())
	}
}

func TestCreateIs500WhenTheKeysCannotBeRead(t *testing.T) {
	// the table is missing: the check cannot run
	db, _ := sql.Open("sqlite3", ":memory:")
	defer db.Close()
	rec := httptest.NewRecorder()
	CreateHandler(keyCheckRequest{fakeCreateRequest: fakeCreateRequest{w: rec, body: `{"platformId":"create-keyed"}`}, di: keyCheckDI{db: db}})
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "no such table") {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}
