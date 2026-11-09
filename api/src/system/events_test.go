package system

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/a-digi/coco-logger/logger"
	"github.com/a-digi/coco-orm/orm"
	serverdi "github.com/a-digi/coco-server/server/di"
	"github.com/a-digi/coco-server/server/request"
	_ "github.com/mattn/go-sqlite3"

	"github.com/a-digi/cinqo/src/system/runs/entity"
	"github.com/a-digi/cinqo/src/system/runs/repository/persistent"
)

type fakeDI struct{ db *sql.DB }

func (f fakeDI) GetDatabaseManager() *orm.DatabaseManager {
	return &orm.DatabaseManager{Connector: &orm.Connector{DB: f.db}}
}
func (f fakeDI) GetLogger() logger.Logger { return nil }

var _ serverdi.Context = fakeDI{}

type fakeRequest struct {
	request.RequestContext
	w    *httptest.ResponseRecorder
	di   serverdi.Context
	vars map[string]string
}

func (f fakeRequest) GetURI() *request.URI           { return &request.URI{PathVariables: f.vars} }
func (f fakeRequest) GetWriter() http.ResponseWriter { return f.w }
func (f fakeRequest) GetDI() serverdi.Context        { return f.di }

func openMainDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	files, _ := filepath.Glob("../../config/db/migrations/*.sql")
	sort.Strings(files)
	for _, f := range files {
		raw, _ := os.ReadFile(f)
		for _, stmt := range strings.Split(string(raw), ";") {
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

func call(h func(request.RequestContext), db *sql.DB, vars map[string]string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h(fakeRequest{w: rec, di: fakeDI{db: db}, vars: vars})
	return rec
}

func addEvent(t *testing.T, db *sql.DB, id, at string, evidence ...entity.EvidenceItem) {
	t.Helper()
	if err := persistent.NewRunsPersistentRepo(db).InsertEvent(entity.Event{ID: id, At: at, Kind: entity.KindAppUncleanExit, Verdict: "Cinqo stopped " + id, Evidence: evidence}); err != nil {
		t.Fatal(err)
	}
}

type eventsAnswer struct {
	Message EventsBody `json:"message"`
	Success bool       `json:"success"`
}

func TestTheEventsAreListedNewestFirstWithTheirEvidenceAndSeenState(t *testing.T) {
	db := openMainDB(t)
	addEvent(t, db, "old", "2026-11-09T10:00:00Z", entity.EvidenceItem{Label: "Memory free", Value: "7 GB"})
	addEvent(t, db, "new", "2026-11-09T12:00:00Z")
	if _, err := persistent.NewRunsPersistentRepo(db).Acknowledge("old", "2026-11-09T13:00:00Z"); err != nil {
		t.Fatal(err)
	}
	rec := call(EventsHandler, db, nil)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var got eventsAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || !got.Success {
		t.Fatal(err)
	}
	ev := got.Message.Events
	if len(ev) != 2 || ev[0].ID != "new" || ev[1].ID != "old" {
		t.Fatalf("%+v", ev)
	}
	if ev[0].Acknowledged || ev[0].AcknowledgedAt != "" || len(ev[0].Evidence) != 0 {
		t.Errorf("an unseen event: %+v", ev[0])
	}
	if !ev[1].Acknowledged || ev[1].AcknowledgedAt != "2026-11-09T13:00:00Z" || len(ev[1].Evidence) != 1 || ev[1].Evidence[0].Label != "Memory free" || ev[1].Kind != entity.KindAppUncleanExit || ev[1].Verdict != "Cinqo stopped old" {
		t.Errorf("a seen event: %+v", ev[1])
	}
}

func TestNoEventsIsAnEmptyListNotNull(t *testing.T) {
	rec := call(EventsHandler, openMainDB(t), nil)
	if !strings.Contains(rec.Body.String(), `"events":[]`) {
		t.Errorf("%s", rec.Body.String())
	}
	raw, _ := json.Marshal(EventBody{ID: "x", Evidence: nil})
	_ = raw
}

func TestAnEventWithNoEvidenceHasAnEmptyListNotNull(t *testing.T) {
	db := openMainDB(t)
	addEvent(t, db, "e", "2026-11-09T10:00:00Z")
	if rec := call(EventsHandler, db, nil); !strings.Contains(rec.Body.String(), `"evidence":[]`) {
		t.Errorf("%s", rec.Body.String())
	}
}

func TestADatabaseThatCannotBeReadIsA500WithAPlainMessage(t *testing.T) {
	db := openMainDB(t)
	db.Exec(`DROP TABLE system_events`)
	rec := call(EventsHandler, db, nil)
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "failed to read the record of unexpected stops") || strings.Contains(rec.Body.String(), "no such table") {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestMarkingAStopAsSeenSetsItsTimeOnceAndAnswers204(t *testing.T) {
	db := openMainDB(t)
	addEvent(t, db, "e1", "2026-11-09T10:00:00Z")
	addEvent(t, db, "e2", "2026-11-09T11:00:00Z")
	rec := call(AcknowledgeEventHandler, db, map[string]string{"id": "e1"})
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("%d %q", rec.Code, rec.Body.String())
	}
	first := readEvents(t, db)
	if !first["e1"].Acknowledged || first["e2"].Acknowledged {
		t.Fatalf("only the named event is marked: %+v", first)
	}
	seenAt := first["e1"].AcknowledgedAt
	// again: still 204 and the first time is kept
	rec = call(AcknowledgeEventHandler, db, map[string]string{"id": "e1"})
	if rec.Code != http.StatusNoContent {
		t.Errorf("%d", rec.Code)
	}
	if readEvents(t, db)["e1"].AcknowledgedAt != seenAt {
		t.Error("marking twice must keep the first time")
	}
}

func readEvents(t *testing.T, db *sql.DB) map[string]EventBody {
	t.Helper()
	bodies, err := eventBodies(db)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]EventBody{}
	for _, b := range bodies {
		m[b.ID] = b
	}
	return m
}

func TestMarkingAnUnknownStopIs404AndAnOddIdIs400(t *testing.T) {
	db := openMainDB(t)
	addEvent(t, db, "e1", "2026-11-09T10:00:00Z")
	if rec := call(AcknowledgeEventHandler, db, map[string]string{"id": "nope"}); rec.Code != http.StatusNotFound {
		t.Errorf("unknown: %d", rec.Code)
	}
	for _, id := range []string{"", strings.Repeat("x", 65)} {
		if rec := call(AcknowledgeEventHandler, db, map[string]string{"id": id}); rec.Code != http.StatusBadRequest {
			t.Errorf("%q: %d", id, rec.Code)
		}
	}
	if rec := call(AcknowledgeEventHandler, db, map[string]string{"id": strings.Repeat("x", 64)}); rec.Code != http.StatusNotFound {
		t.Errorf("an id of exactly 64 characters is allowed (and unknown): %d", rec.Code)
	}
	if readEvents(t, db)["e1"].Acknowledged {
		t.Error("a refused request must change nothing")
	}
}

func TestAnIdWithSQLInItIsOnlyAValueNeverQuery(t *testing.T) {
	db := openMainDB(t)
	addEvent(t, db, "e1", "2026-11-09T10:00:00Z")
	if rec := call(AcknowledgeEventHandler, db, map[string]string{"id": "x' OR '1'='1"}); rec.Code != http.StatusNotFound {
		t.Errorf("%d", rec.Code)
	}
	if readEvents(t, db)["e1"].Acknowledged {
		t.Error("an injected id must not mark anything")
	}
}

func TestADatabaseThatCannotBeWrittenIsA500(t *testing.T) {
	db := openMainDB(t)
	db.Exec(`DROP TABLE system_events`)
	if rec := call(AcknowledgeEventHandler, db, map[string]string{"id": "e1"}); rec.Code != http.StatusInternalServerError {
		t.Errorf("%d", rec.Code)
	}
}

func TestTheFirstTimeAStopWasSeenIsKept(t *testing.T) {
	db := openMainDB(t)
	addEvent(t, db, "e1", "2026-11-09T10:00:00Z")
	pers := persistent.NewRunsPersistentRepo(db)
	if found, err := pers.Acknowledge("e1", "2026-11-09T11:00:00Z"); err != nil || !found {
		t.Fatalf("%v %v", found, err)
	}
	if found, err := pers.Acknowledge("e1", "2026-11-09T12:00:00Z"); err != nil || !found {
		t.Fatalf("seeing it again is still found: %v %v", found, err)
	}
	if got := readEvents(t, db)["e1"].AcknowledgedAt; got != "2026-11-09T11:00:00Z" {
		t.Errorf("the first time must be kept, got %s", got)
	}
}
