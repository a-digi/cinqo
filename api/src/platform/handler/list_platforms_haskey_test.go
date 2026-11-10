package handler

import (
	"context"
	"database/sql"
	"encoding/json"
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
	platform_query "github.com/a-digi/cinqo/src/platform/repository/query"
)

type keyDI struct{ db *sql.DB }

func (f keyDI) GetDatabaseManager() *orm.DatabaseManager {
	return &orm.DatabaseManager{Connector: &orm.Connector{DB: f.db}}
}
func (f keyDI) GetLogger() logger.Logger { return nil }

var _ serverdi.Context = keyDI{}

type listRequest struct {
	request.RequestContext
	w  *httptest.ResponseRecorder
	di serverdi.Context
}

func (f listRequest) GetWriter() http.ResponseWriter { return f.w }
func (f listRequest) GetDI() serverdi.Context        { return f.di }

func keysDB(t *testing.T, platforms ...string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE platform_keys (id TEXT PRIMARY KEY, label TEXT NOT NULL, platform TEXT NOT NULL, encrypted_key TEXT NOT NULL, created_by TEXT NOT NULL, created_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for i, p := range platforms {
		if _, err := db.Exec(`INSERT INTO platform_keys VALUES (?, 'l', ?, 'secret-ciphertext', 'u', '2026-11-10T10:00:00Z')`, string(rune('a'+i)), p); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestPlatformsWithKeyIsTheSetOfPlatformsThatHaveAKeyAndNothingElse(t *testing.T) {
	got, err := platform_query.NewKeyQueryRepo(keysDB(t, "openai", "openai", "anthropic")).PlatformsWithKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got["openai"] || !got["anthropic"] || got["openrouter"] {
		t.Errorf("%v", got)
	}
	none, err := platform_query.NewKeyQueryRepo(keysDB(t)).PlatformsWithKey()
	if err != nil || len(none) != 0 {
		t.Errorf("no keys: %v %v", none, err)
	}
}

func TestThePlatformListSaysHasKeyOnlyForPlatformsWithAKeyOrThatNeedNone(t *testing.T) {
	if err := platform.Register(platform.Entry{
		Info:        entity.Platform{ID: "haskey-keyless", Name: "Keyless"},
		Completer:   noopCompleter{},
		Credentials: func(context.Context) (string, string, error) { return "http://127.0.0.1:1", "", nil },
		Models:      func() []string { return []string{"m"} },
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	ListPlatformsHandler(listRequest{w: rec, di: keyDI{db: keysDB(t, "openai")}})
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-ciphertext") {
		t.Error("a key must never be in the list")
	}
	var got struct {
		Message []platformResponse `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"openai": true, "anthropic": false, "openrouter": false, "haskey-keyless": true}
	for id, has := range want {
		p, ok := find(got.Message, id)
		if !ok || p.HasKey != has {
			t.Errorf("%s: listed %v hasKey %v, want %v", id, ok, p.HasKey, has)
		}
	}
	// the keyless platform has no key row at all and is still ready
	if p, _ := find(got.Message, "haskey-keyless"); p.RequiresKey || !p.HasKey {
		t.Errorf("%+v", p)
	}
}

func TestThePlatformListIs500WhenTheKeysCannotBeRead(t *testing.T) {
	db := keysDB(t)
	db.Exec(`DROP TABLE platform_keys`)
	rec := httptest.NewRecorder()
	ListPlatformsHandler(listRequest{w: rec, di: keyDI{db: db}})
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "no such table") {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}
