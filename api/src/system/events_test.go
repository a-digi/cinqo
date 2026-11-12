package system

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/a-digi/coco-logger/logger"
	"github.com/a-digi/coco-orm/orm"
	serverdi "github.com/a-digi/coco-server/server/di"
	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/routing"
	_ "github.com/mattn/go-sqlite3"

	"github.com/a-digi/cinqo/src/system/runs/entity"
	"github.com/a-digi/cinqo/src/system/runs/repository/persistent"
	"github.com/a-digi/cinqo/src/system/runs/repository/query"
)

type fakeDI struct{ db *sql.DB }

func (f fakeDI) GetDatabaseManager() *orm.DatabaseManager {
	return &orm.DatabaseManager{Connector: &orm.Connector{DB: f.db}}
}
func (f fakeDI) GetLogger() logger.Logger { return nil }

var _ serverdi.Context = fakeDI{}

type fakeRequest struct {
	request.RequestContext
	w     *httptest.ResponseRecorder
	di    serverdi.Context
	vars  map[string]string
	query url.Values
}

func (f fakeRequest) GetURI() *request.URI {
	return &request.URI{PathVariables: f.vars, Query: f.query}
}
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
	return callWith(h, db, vars, nil)
}

func callWith(h func(request.RequestContext), db *sql.DB, vars map[string]string, query url.Values) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h(fakeRequest{w: rec, di: fakeDI{db: db}, vars: vars, query: query})
	return rec
}

func addEvent(t *testing.T, db *sql.DB, id, at string, evidence ...entity.EvidenceItem) {
	t.Helper()
	if err := persistent.NewRunsPersistentRepo(db).InsertEvent(entity.Event{ID: id, At: at, Kind: entity.KindAppUncleanExit, Verdict: "Cinqo stopped " + id, Evidence: evidence}); err != nil {
		t.Fatal(err)
	}
}

type pageAnswer struct {
	Message EventsPageBody `json:"message"`
	Success bool           `json:"success"`
}

type oneAnswer struct {
	Message EventBody `json:"message"`
	Success bool      `json:"success"`
}

type summaryAnswer struct {
	Message SummaryBody `json:"message"`
	Success bool        `json:"success"`
}

func page(t *testing.T, db *sql.DB, q url.Values) (EventsPageBody, *httptest.ResponseRecorder) {
	t.Helper()
	rec := callWith(EventsHandler, db, nil, q)
	var got pageAnswer
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	return got.Message, rec
}

func TestThePageListsTheStopsNewestFirstWithTheirSeenStateAndWithoutEvidence(t *testing.T) {
	db := openMainDB(t)
	addEvent(t, db, "old", "2026-11-09T10:00:00Z", entity.EvidenceItem{Label: "Memory free", Value: "7 GB"})
	addEvent(t, db, "new", "2026-11-09T12:00:00Z")
	if _, err := persistent.NewRunsPersistentRepo(db).Acknowledge("old", "2026-11-09T13:00:00Z"); err != nil {
		t.Fatal(err)
	}
	got, rec := page(t, db, nil)
	if rec.Code != 200 || got.Page != 1 || got.PageSize != 20 || got.Total != 2 {
		t.Fatalf("%d %+v", rec.Code, got)
	}
	ev := got.Events
	if len(ev) != 2 || ev[0].ID != "new" || ev[1].ID != "old" {
		t.Fatalf("%+v", ev)
	}
	if ev[0].Acknowledged || ev[0].AcknowledgedAt != "" {
		t.Errorf("an unseen event: %+v", ev[0])
	}
	if !ev[1].Acknowledged || ev[1].AcknowledgedAt != "2026-11-09T13:00:00Z" || ev[1].Kind != entity.KindAppUncleanExit || ev[1].Verdict != "Cinqo stopped old" {
		t.Errorf("a seen event: %+v", ev[1])
	}
	if strings.Contains(rec.Body.String(), "evidence") || strings.Contains(rec.Body.String(), "Memory free") {
		t.Errorf("the table has no evidence: %s", rec.Body.String())
	}
}

func seedMany(t *testing.T, db *sql.DB, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		addEvent(t, db, fmt.Sprintf("e%03d", i), fmt.Sprintf("2026-11-09T10:%02d:%02dZ", i/60, i%60))
	}
}

func TestPagesAreCutAtTheSizeAndCountedFromTheNewest(t *testing.T) {
	db := openMainDB(t)
	seedMany(t, db, 25)
	first, _ := page(t, db, url.Values{"pageSize": {"10"}})
	if len(first.Events) != 10 || first.Events[0].ID != "e024" || first.Events[9].ID != "e015" || first.Total != 25 || first.Page != 1 || first.PageSize != 10 {
		t.Fatalf("%+v", first)
	}
	third, _ := page(t, db, url.Values{"pageSize": {"10"}, "page": {"3"}})
	if len(third.Events) != 5 || third.Events[0].ID != "e004" || third.Events[4].ID != "e000" || third.Page != 3 {
		t.Fatalf("%+v", third)
	}
	beyond, rec := page(t, db, url.Values{"pageSize": {"10"}, "page": {"4"}})
	if rec.Code != 200 || len(beyond.Events) != 0 || beyond.Total != 25 || beyond.Page != 4 || !strings.Contains(rec.Body.String(), `"events":[]`) {
		t.Fatalf("a page past the last is an empty list with the real total: %d %s", rec.Code, rec.Body.String())
	}
	if size, _ := page(t, db, url.Values{"pageSize": {"100"}}); len(size.Events) != 25 || size.PageSize != 100 {
		t.Errorf("a size of 100 is allowed: %+v", size)
	}
	seen := map[string]bool{}
	for p := 1; p <= 3; p++ {
		got, _ := page(t, db, url.Values{"pageSize": {"10"}, "page": {fmt.Sprint(p)}})
		for _, e := range got.Events {
			if seen[e.ID] {
				t.Errorf("%s is on two pages", e.ID)
			}
			seen[e.ID] = true
		}
	}
	if len(seen) != 25 {
		t.Errorf("every stop is on exactly one page: %d", len(seen))
	}
}

func TestAPageThatIsNotAskedForRightIs400(t *testing.T) {
	db := openMainDB(t)
	seedMany(t, db, 3)
	for _, q := range []url.Values{
		{"page": {"0"}}, {"page": {"-1"}}, {"page": {"abc"}}, {"page": {"1.5"}}, {"page": {"1000001"}}, {"page": {" 2"}},
		{"pageSize": {"0"}}, {"pageSize": {"15"}}, {"pageSize": {"101"}}, {"pageSize": {"abc"}}, {"pageSize": {"-10"}},
	} {
		rec := callWith(EventsHandler, db, nil, q)
		if rec.Code != http.StatusBadRequest || !(strings.Contains(rec.Body.String(), "page must be") || strings.Contains(rec.Body.String(), "pageSize must be")) {
			t.Errorf("%v: %d %s", q, rec.Code, rec.Body.String())
		}
	}
	if rec := callWith(EventsHandler, db, nil, url.Values{"page": {"1000000"}}); rec.Code != 200 {
		t.Errorf("the highest allowed page: %d", rec.Code)
	}
	for _, size := range []string{"10", "20", "50", "100"} {
		if rec := callWith(EventsHandler, db, nil, url.Values{"pageSize": {size}}); rec.Code != 200 {
			t.Errorf("size %s: %d", size, rec.Code)
		}
	}
}

func TestNoEventsIsAnEmptyListNotNull(t *testing.T) {
	got, rec := page(t, openMainDB(t), nil)
	if !strings.Contains(rec.Body.String(), `"events":[]`) || got.Total != 0 || got.Page != 1 {
		t.Errorf("%s", rec.Body.String())
	}
}

func TestADatabaseThatCannotBeReadIsA500WithAPlainMessage(t *testing.T) {
	for name, h := range map[string]func(request.RequestContext){"list": EventsHandler, "summary": EventsSummaryHandler, "one": EventHandler, "mark all": AcknowledgeAllHandler} {
		db := openMainDB(t)
		db.Exec(`DROP TABLE system_events`)
		rec := callWith(h, db, map[string]string{"id": "x"}, nil)
		if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "no such table") || strings.Contains(rec.Body.String(), "system_events") {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	db := openMainDB(t)
	db.Exec(`DROP TABLE system_events`)
	if rec := call(EventsHandler, db, nil); !strings.Contains(rec.Body.String(), "failed to read the record of unexpected stops") {
		t.Errorf("%s", rec.Body.String())
	}
}

func TestTheSummaryCountsAndNamesTheNewestUnseenStop(t *testing.T) {
	db := openMainDB(t)
	empty := call(EventsSummaryHandler, db, nil)
	if !strings.Contains(empty.Body.String(), `"total":0`) || !strings.Contains(empty.Body.String(), `"unseen":0`) || strings.Contains(empty.Body.String(), "latest") {
		t.Errorf("nothing recorded: %s", empty.Body.String())
	}
	addEvent(t, db, "a", "2026-11-09T10:00:00Z")
	addEvent(t, db, "b", "2026-11-09T11:00:00Z")
	addEvent(t, db, "c", "2026-11-09T12:00:00Z")
	if _, err := persistent.NewRunsPersistentRepo(db).Acknowledge("c", "2026-11-09T13:00:00Z"); err != nil {
		t.Fatal(err)
	}
	var got summaryAnswer
	rec := call(EventsSummaryHandler, db, nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 200 {
		t.Fatal(err, rec.Body.String())
	}
	s := got.Message
	if s.Total != 3 || s.Unseen != 2 || s.Latest == nil || s.Latest.ID != "b" || s.Latest.At != "2026-11-09T11:00:00Z" || s.Latest.Kind != entity.KindAppUncleanExit {
		t.Errorf("the newest UNSEEN stop is b, not the newest stop c: %+v %+v", s, s.Latest)
	}
	if _, err := persistent.NewRunsPersistentRepo(db).AcknowledgeAll("2026-11-09T14:00:00Z"); err != nil {
		t.Fatal(err)
	}
	rec = call(EventsSummaryHandler, db, nil)
	if !strings.Contains(rec.Body.String(), `"total":3`) || !strings.Contains(rec.Body.String(), `"unseen":0`) || strings.Contains(rec.Body.String(), "latest") {
		t.Errorf("all seen: %s", rec.Body.String())
	}
}

func TestOneStopIsReadWithItsEvidence(t *testing.T) {
	db := openMainDB(t)
	addEvent(t, db, "e1", "2026-11-09T10:00:00Z", entity.EvidenceItem{Label: "Memory free", Value: "7 GB"}, entity.EvidenceItem{Label: "Swap in use", Value: "2 GB"})
	addEvent(t, db, "e2", "2026-11-09T11:00:00Z")
	if _, err := persistent.NewRunsPersistentRepo(db).Acknowledge("e1", "2026-11-09T13:00:00Z"); err != nil {
		t.Fatal(err)
	}
	var got oneAnswer
	rec := call(EventHandler, db, map[string]string{"id": "e1"})
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 200 || !got.Success {
		t.Fatal(err, rec.Body.String())
	}
	e := got.Message
	if e.ID != "e1" || e.At != "2026-11-09T10:00:00Z" || e.Kind != entity.KindAppUncleanExit || e.Verdict != "Cinqo stopped e1" || !e.Acknowledged || e.AcknowledgedAt != "2026-11-09T13:00:00Z" {
		t.Errorf("%+v", e)
	}
	if len(e.Evidence) != 2 || e.Evidence[0].Label != "Memory free" || e.Evidence[1].Value != "2 GB" {
		t.Errorf("the facts, in order: %+v", e.Evidence)
	}
	if rec := call(EventHandler, db, map[string]string{"id": "e2"}); !strings.Contains(rec.Body.String(), `"evidence":[]`) || !strings.Contains(rec.Body.String(), `"acknowledged":false`) {
		t.Errorf("a stop without facts and not seen: %s", rec.Body.String())
	}
}

func TestOneStopThatIsNotThereIs404AndAnOddIdIs400AndSQLIsOnlyAValue(t *testing.T) {
	db := openMainDB(t)
	addEvent(t, db, "e1", "2026-11-09T10:00:00Z")
	if rec := call(EventHandler, db, map[string]string{"id": "nope"}); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "no such stop") {
		t.Errorf("unknown: %d %s", rec.Code, rec.Body.String())
	}
	for _, id := range []string{"", strings.Repeat("x", 65)} {
		if rec := call(EventHandler, db, map[string]string{"id": id}); rec.Code != http.StatusBadRequest {
			t.Errorf("%q: %d", id, rec.Code)
		}
	}
	if rec := call(EventHandler, db, map[string]string{"id": strings.Repeat("x", 64)}); rec.Code != http.StatusNotFound {
		t.Errorf("an id of exactly 64 characters is allowed (and unknown): %d", rec.Code)
	}
	if rec := call(EventHandler, db, map[string]string{"id": "x' OR '1'='1"}); rec.Code != http.StatusNotFound {
		t.Errorf("%d", rec.Code)
	}
}

func TestMarkingAllAsSeenMarksOnlyTheUnseenOnesOnceAndSaysHowMany(t *testing.T) {
	db := openMainDB(t)
	addEvent(t, db, "a", "2026-11-09T10:00:00Z")
	addEvent(t, db, "b", "2026-11-09T11:00:00Z")
	addEvent(t, db, "c", "2026-11-09T12:00:00Z")
	if _, err := persistent.NewRunsPersistentRepo(db).Acknowledge("a", "2026-11-09T10:30:00Z"); err != nil {
		t.Fatal(err)
	}
	rec := call(AcknowledgeAllHandler, db, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"marked":2`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	ev := readEvents(t, db)
	if ev["a"].AcknowledgedAt != "2026-11-09T10:30:00Z" {
		t.Errorf("the first time of a stop that was seen is kept: %+v", ev["a"])
	}
	if !ev["b"].Acknowledged || !ev["c"].Acknowledged || ev["b"].AcknowledgedAt == "" {
		t.Errorf("%+v", ev)
	}
	rec = call(AcknowledgeAllHandler, db, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"marked":0`) {
		t.Errorf("again, nothing is left: %d %s", rec.Code, rec.Body.String())
	}
	if readEvents(t, db)["b"].AcknowledgedAt != ev["b"].AcknowledgedAt {
		t.Error("marking again must keep the first time")
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
	all, err := query.NewRunsQueryRepo(db).Events()
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]EventBody{}
	for _, e := range all {
		m[e.ID] = eventBody(e)
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

// "summary" is a literal segment and {id} a placeholder: the router picks the longest literal prefix, so the summary is never read as the stop "summary", and
// "acknowledge-all" (two segments after events) never meets {id}/acknowledge.
func TestTheLiteralRoutesWinOverTheOnesWithAnId(t *testing.T) {
	raw, err := os.ReadFile("../../config/routes/route-system.yaml")
	if err != nil {
		t.Fatal(err)
	}
	type route struct{ path, method string }
	var routes []route
	re := regexp.MustCompile(`(?s)- path: (\S+)\s+method: (\S+)`)
	for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
		routes = append(routes, route{m[1], m[2]})
	}
	best := func(method, url string) string {
		bestPath, bestScore := "", -1
		for _, r := range routes {
			if r.method != method || !routing.MatchPath(r.path, url) {
				continue
			}
			if score := routing.LiteralPrefixLen(r.path); score > bestScore {
				bestPath, bestScore = r.path, score
			}
		}
		return bestPath
	}
	for url, want := range map[string]string{
		"/api/v1/system/events":                       "/api/v1/system/events",
		"/api/v1/system/events/summary":               "/api/v1/system/events/summary",
		"/api/v1/system/events/7b1c-uuid":             "/api/v1/system/events/{id}",
		"/api/v1/system/events/7b1c-uuid/acknowledge": "/api/v1/system/events/{id}/acknowledge",
	} {
		if got := best("GET", url); url != "/api/v1/system/events/7b1c-uuid/acknowledge" && got != want {
			t.Errorf("GET %s -> %s, want %s", url, got, want)
		}
	}
	if got := best("POST", "/api/v1/system/events/acknowledge-all"); got != "/api/v1/system/events/acknowledge-all" {
		t.Errorf("POST acknowledge-all -> %s", got)
	}
	if got := best("POST", "/api/v1/system/events/7b1c-uuid/acknowledge"); got != "/api/v1/system/events/{id}/acknowledge" {
		t.Errorf("POST {id}/acknowledge -> %s", got)
	}
	if got := best("GET", "/api/v1/system/events/acknowledge-all"); got != "/api/v1/system/events/{id}" {
		t.Errorf("a GET of acknowledge-all can only be the stop of that name (a 404): %s", got)
	}
}

// Each executor of the route file is wired to its own handler (a route that answers with another route's handler would still start).
func TestEveryExecutorIsWiredToItsOwnHandler(t *testing.T) {
	code, err := os.ReadFile("../../config/routes/routes.go")
	if err != nil {
		t.Fatal(err)
	}
	for executor, handler := range map[string]string{
		"SystemResourcesGet":         "system.GetHandler",
		"SystemMemoryTopGet":         "system.TopHandler",
		"SystemEventsGet":            "system.EventsHandler",
		"SystemEventsSummaryGet":     "system.EventsSummaryHandler",
		"SystemEventGet":             "system.EventHandler",
		"SystemEventsAcknowledgeAll": "system.AcknowledgeAllHandler",
		"SystemEventAcknowledge":     "system.AcknowledgeEventHandler",
	} {
		re := regexp.MustCompile(`"` + executor + `":\s+local_routing\.HandlerFunc\(` + regexp.QuoteMeta(handler) + `\)`)
		if !re.Match(code) {
			t.Errorf("%s is not wired to %s", executor, handler)
		}
	}
}
