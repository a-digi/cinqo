package crawlcriteria

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
	"career-tool-backend/portal"
)

func TestTheSettingsStartOffWithTheRuleOnAtThirty(t *testing.T) {
	setup(t)
	s, err := GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.IgnoreEnabled || !s.MinMatchEnabled || s.MinMatchPercent != 30 || s.UpdatedAt != "" {
		t.Errorf("%+v: an install must change nothing until the switch is turned on", s)
	}
}

func TestSettingsAreSavedAndReadBack(t *testing.T) {
	setup(t)
	now = func() time.Time { return time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC) }
	t.Cleanup(func() { now = time.Now })
	saved, err := UpdateSettings(Settings{IgnoreEnabled: true, MinMatchEnabled: false, MinMatchPercent: 55})
	if err != nil {
		t.Fatal(err)
	}
	want := Settings{IgnoreEnabled: true, MinMatchEnabled: false, MinMatchPercent: 55, UpdatedAt: "2026-11-02 09:30:00"}
	if saved != want {
		t.Errorf("saved %+v, want %+v", saved, want)
	}
	if got, _ := GetSettings(); got != want {
		t.Errorf("read back %+v", got)
	}
}

func TestTheLimitMustBeInRangeEvenWhenTheRuleIsOff(t *testing.T) {
	setup(t)
	for _, p := range []int{-1, 0, 101, 5000} {
		if _, err := UpdateSettings(Settings{MinMatchEnabled: false, MinMatchPercent: p}); !errors.Is(err, ErrInvalidSettings) {
			t.Errorf("%d: %v", p, err)
		}
	}
	for _, p := range []int{1, 30, 100} {
		if _, err := UpdateSettings(Settings{MinMatchPercent: p}); err != nil {
			t.Errorf("%d: %v", p, err)
		}
	}
	if got, _ := GetSettings(); got.MinMatchPercent != 100 {
		t.Errorf("a refused value must not change the row: %+v", got)
	}
}

// A bad value cannot be stored by any path: the schema checks it too.
func TestTheSchemaRefusesABadValueToo(t *testing.T) {
	setup(t)
	for _, q := range []string{
		`UPDATE crawler_settings SET min_match_percent = 0`,
		`UPDATE crawler_settings SET min_match_percent = 101`,
		`UPDATE crawler_settings SET ignore_enabled = 2`,
		`UPDATE crawler_settings SET min_match_enabled = -1`,
		`INSERT INTO crawler_settings (id) VALUES (2)`,
	} {
		if _, err := db.JobsDB.Exec(q); err == nil {
			t.Errorf("%s was accepted", q)
		}
	}
}

func do(h http.HandlerFunc, method, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(method, "/", bytes.NewBufferString(body)))
	return rec
}

func TestTheSettingsRoute(t *testing.T) {
	setup(t)
	rec := do(SettingsHandler, http.MethodGet, "")
	var got Settings
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.IgnoreEnabled || got.MinMatchPercent != 30 {
		t.Fatalf("GET: %d %s", rec.Code, rec.Body)
	}
	rec = do(SettingsHandler, http.MethodPut, `{"ignoreEnabled":true,"minMatchEnabled":true,"minMatchPercent":42,"updatedAt":"ignored"}`)
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || !got.IgnoreEnabled || got.MinMatchPercent != 42 || got.UpdatedAt == "ignored" || got.UpdatedAt == "" {
		t.Errorf("PUT: %d %s", rec.Code, rec.Body)
	}
	for name, body := range map[string]string{
		"not json": "nope", "percent missing": `{"ignoreEnabled":true,"minMatchEnabled":true}`, "percent too big": `{"minMatchPercent":101}`, "percent negative": `{"minMatchPercent":-3}`,
		"percent not a number": `{"minMatchPercent":"high"}`, "switch not a bool": `{"ignoreEnabled":"yes","minMatchPercent":30}`,
	} {
		if rec := do(SettingsHandler, http.MethodPut, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	if got, _ := GetSettings(); got.MinMatchPercent != 42 {
		t.Errorf("a refused PUT changed the row: %+v", got)
	}
	if rec := do(SettingsHandler, http.MethodDelete, ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE: %d", rec.Code)
	}
}

// The settings are a few numbers: a far bigger body is refused.
func TestAnOversizedSettingsBodyIsRefused(t *testing.T) {
	setup(t)
	big := `{"ignoreEnabled":true,"minMatchEnabled":true,"minMatchPercent":50,"padding":"` + strings.Repeat("x", 20*1024) + `"}`
	if rec := do(SettingsHandler, http.MethodPut, big); rec.Code != http.StatusBadRequest {
		t.Errorf("%d", rec.Code)
	}
	if got, _ := GetSettings(); got.IgnoreEnabled || got.MinMatchPercent != 30 {
		t.Errorf("a refused PUT changed the row: %+v", got)
	}
}

// ---- the source ----

func matchersFor(t *testing.T, s Settings) []jobs.CriteriaMatcher {
	t.Helper()
	if _, err := UpdateSettings(s); err != nil {
		t.Fatal(err)
	}
	ms, err := Source{}.Matchers()
	if err != nil {
		t.Fatal(err)
	}
	return ms
}

func TestTheSourceBuildsExactlyTheEnabledMatchers(t *testing.T) {
	setup(t)
	if ms := matchersFor(t, Settings{IgnoreEnabled: false, MinMatchEnabled: true, MinMatchPercent: 30}); len(ms) != 0 {
		t.Errorf("the switch is off: %d matchers", len(ms))
	}
	if ms := matchersFor(t, Settings{IgnoreEnabled: true, MinMatchEnabled: false, MinMatchPercent: 30}); len(ms) != 0 {
		t.Errorf("the rule is off: %d matchers", len(ms))
	}
	ms := matchersFor(t, Settings{IgnoreEnabled: true, MinMatchEnabled: true, MinMatchPercent: 55})
	if len(ms) != 1 || ms[0].Name() != MinProfileMatchName {
		t.Fatalf("%d matchers", len(ms))
	}
	if m, ok := ms[0].(*MinProfileMatch); !ok || m.Percent != 55 {
		t.Errorf("the matcher must carry the saved limit: %#v", ms[0])
	}
}

func TestTheSourceFailsWhenTheSettingsCannotBeRead(t *testing.T) {
	setup(t)
	if _, err := db.JobsDB.Exec(`DROP TABLE crawler_settings`); err != nil {
		t.Fatal(err)
	}
	if _, err := (Source{}).Matchers(); err == nil {
		t.Error("expected an error")
	}
}

// A builder is the way a later matcher is added: each enabled builder's matcher joins the list, in order, and a failing one fails the
// source (the crawler then keeps every job).
func TestSeveralBuildersGiveSeveralMatchersInOrder(t *testing.T) {
	setup(t)
	old := matcherBuilders
	t.Cleanup(func() { matcherBuilders = old })
	var log []string
	matcherBuilders = append(append([]func(Settings) (jobs.CriteriaMatcher, error){}, old...),
		func(Settings) (jobs.CriteriaMatcher, error) { return nil, nil }, // a rule that is off
		func(Settings) (jobs.CriteriaMatcher, error) { return namedMatcher{"second", &log}, nil },
	)
	ms := matchersFor(t, Settings{IgnoreEnabled: true, MinMatchEnabled: true, MinMatchPercent: 30})
	if len(ms) != 2 || ms[0].Name() != MinProfileMatchName || ms[1].Name() != "second" {
		t.Fatalf("%d matchers", len(ms))
	}
	matcherBuilders = append(matcherBuilders, func(Settings) (jobs.CriteriaMatcher, error) { return nil, errors.New("cannot build") })
	if _, err := (Source{}).Matchers(); err == nil {
		t.Error("a failing builder fails the source")
	}
}

type namedMatcher struct {
	name string
	log  *[]string
}

func (m namedMatcher) Name() string { return m.name }
func (m namedMatcher) Match(jobs.Candidate) (jobs.Verdict, error) {
	*m.log = append(*m.log, m.name)
	return jobs.Verdict{Keep: true}, nil
}

// ---- the log of ignored jobs ----

func refusal(reason string) *jobs.FilteredOutError {
	return &jobs.FilteredOutError{Matcher: MinProfileMatchName, Reason: reason}
}

func ignoredCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM crawler_ignored_jobs`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAnIgnoredJobIsLoggedWithWhatTheCrawlKnew(t *testing.T) {
	setup(t)
	if _, err := db.JobsDB.Exec(`INSERT INTO portals (id, name) VALUES ('p1', 'StepStone')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.JobsDB.Exec(`INSERT INTO portal_links (id, portal_id, url) VALUES ('l1', 'p1', 'https://x.example/search')`); err != nil {
		t.Fatal(err)
	}
	now = func() time.Time { return time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = time.Now })
	RecordIgnored(jobs.Candidate{SourceURL: "https://x.example/jobs/1", Title: "Sales Manager", Company: "Acme", Location: "Munich", PortalLinkID: "l1"}, refusal("best profile match 0% is under the limit of 30%"))
	list, err := ListIgnored(10)
	if err != nil || list.Total != 1 || len(list.Items) != 1 {
		t.Fatalf("%+v %v", list, err)
	}
	got := list.Items[0]
	want := IgnoredJob{ID: got.ID, SourceURL: "https://x.example/jobs/1", Title: "Sales Manager", Company: "Acme", Location: "Munich", PortalName: "StepStone",
		Matcher: "min-profile-match", Reason: "best profile match 0% is under the limit of 30%", IgnoredAt: "2026-11-02 10:00:00"}
	if got != want {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

// The same posting refused again updates its row (one row per URL) instead of adding another.
func TestARefusedAgainJobUpdatesItsRow(t *testing.T) {
	setup(t)
	clock := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	now = func() time.Time { return clock }
	t.Cleanup(func() { now = time.Now })
	c := jobs.Candidate{SourceURL: "https://x.example/jobs/1", Title: "Sales Manager", Company: "Acme"}
	RecordIgnored(c, refusal("best profile match 0% is under the limit of 30%"))
	clock = clock.Add(time.Hour)
	c.Title = "Sales Manager (m/w/d)"
	RecordIgnored(c, refusal("best profile match 4% is under the limit of 30%"))
	list, _ := ListIgnored(10)
	if ignoredCount(t) != 1 || list.Items[0].Title != "Sales Manager (m/w/d)" || list.Items[0].Reason != "best profile match 4% is under the limit of 30%" || list.Items[0].IgnoredAt != "2026-11-02 11:00:00" {
		t.Errorf("%d rows: %+v", ignoredCount(t), list.Items)
	}
}

func TestOnlyTheNewestTwoHundredAreKept(t *testing.T) {
	setup(t)
	clock := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	now = func() time.Time { clock = clock.Add(time.Second); return clock }
	t.Cleanup(func() { now = time.Now })
	for i := 0; i < KeptIgnored+7; i++ {
		RecordIgnored(jobs.Candidate{SourceURL: fmt.Sprintf("https://x.example/jobs/%03d", i), Title: fmt.Sprintf("Job %03d", i)}, refusal("under"))
	}
	if ignoredCount(t) != KeptIgnored {
		t.Fatalf("%d rows kept", ignoredCount(t))
	}
	list, _ := ListIgnored(KeptIgnored)
	if list.Items[0].Title != "Job 206" || list.Items[len(list.Items)-1].Title != "Job 007" {
		t.Errorf("newest %q, oldest kept %q: the seven oldest must be gone", list.Items[0].Title, list.Items[len(list.Items)-1].Title)
	}
	// a job refused again moves to the front and is not the one pruned next
	RecordIgnored(jobs.Candidate{SourceURL: "https://x.example/jobs/007", Title: "Job 007 again"}, refusal("under"))
	RecordIgnored(jobs.Candidate{SourceURL: "https://x.example/jobs/new", Title: "Brand new"}, refusal("under"))
	list, _ = ListIgnored(KeptIgnored)
	titles := map[string]bool{}
	for _, it := range list.Items {
		titles[it.Title] = true
	}
	if !titles["Job 007 again"] || !titles["Brand new"] || titles["Job 008"] || ignoredCount(t) != KeptIgnored {
		t.Errorf("the refreshed row must survive and the oldest go: %d rows, 007=%v new=%v 008=%v", ignoredCount(t), titles["Job 007 again"], titles["Brand new"], titles["Job 008"])
	}
}

func TestTheListIsNewestFirstLimitedAndCountsAll(t *testing.T) {
	setup(t)
	clock := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	now = func() time.Time { clock = clock.Add(time.Minute); return clock }
	t.Cleanup(func() { now = time.Now })
	for i := 0; i < 5; i++ {
		RecordIgnored(jobs.Candidate{SourceURL: fmt.Sprintf("https://x.example/%d", i), Title: fmt.Sprintf("J%d", i)}, refusal("r"))
	}
	list, err := ListIgnored(3)
	if err != nil || list.Total != 5 || len(list.Items) != 3 || list.Items[0].Title != "J4" || list.Items[2].Title != "J2" {
		t.Errorf("%+v %v", list, err)
	}
	// the portal is not known any more: the name is simply empty
	RecordIgnored(jobs.Candidate{SourceURL: "https://x.example/lost", Title: "Lost", PortalLinkID: "deleted-link"}, refusal("r"))
	list, _ = ListIgnored(1)
	if list.Items[0].Title != "Lost" || list.Items[0].PortalName != "" {
		t.Errorf("%+v", list.Items)
	}
}

func TestAnEmptyLogIsAnEmptyListNotNull(t *testing.T) {
	setup(t)
	rec := do(IgnoredHandler, http.MethodGet, "")
	if rec.Code != 200 || !bytes.Contains(rec.Body.Bytes(), []byte(`"items":[]`)) || !bytes.Contains(rec.Body.Bytes(), []byte(`"total":0`)) {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
}

func TestTheIgnoredRoute(t *testing.T) {
	setup(t)
	for i := 0; i < 3; i++ {
		RecordIgnored(jobs.Candidate{SourceURL: fmt.Sprintf("https://x.example/%d", i), Title: fmt.Sprintf("J%d", i)}, refusal("r"))
	}
	get := func(q string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		IgnoredHandler(rec, httptest.NewRequest(http.MethodGet, "/?"+q, nil))
		return rec
	}
	count := func(rec *httptest.ResponseRecorder) int {
		var l IgnoredList
		if err := json.Unmarshal(rec.Body.Bytes(), &l); err != nil {
			t.Fatal(err)
		}
		return len(l.Items)
	}
	if rec := get(""); rec.Code != 200 || count(rec) != 3 {
		t.Errorf("default: %d", rec.Code)
	}
	if rec := get("limit=2"); count(rec) != 2 {
		t.Errorf("limit 2: %d", count(rec))
	}
	if rec := get("limit=0"); rec.Code != 200 || count(rec) != 1 {
		t.Errorf("limit 0 is clamped to 1: %d %d", rec.Code, count(rec))
	}
	if rec := get("limit=9999"); rec.Code != 200 || count(rec) != 3 {
		t.Errorf("a big limit is clamped: %d", rec.Code)
	}
	if rec := get("limit=abc"); rec.Code != http.StatusBadRequest {
		t.Errorf("limit=abc: %d", rec.Code)
	}
	if rec := do(IgnoredHandler, http.MethodPost, ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", rec.Code)
	}
}

// ---- the whole crawl ----

func addPortal(t *testing.T) {
	t.Helper()
	if _, err := db.JobsDB.Exec(`INSERT INTO portals (id, name) VALUES ('p1', 'Portal')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.JobsDB.Exec(`INSERT INTO portal_links (id, portal_id, url) VALUES ('l1', 'p1', 'https://portal.example/search')`); err != nil {
		t.Fatal(err)
	}
}

func inject(t *testing.T) {
	t.Helper()
	jobs.UseCriteria(Source{})
	jobs.UseFilteredOutLog(RecordIgnored)
	t.Cleanup(func() { jobs.UseCriteria(nil); jobs.UseFilteredOutLog(nil) })
}

func crawl(t *testing.T) portal.IngestCrawlResultsResult {
	t.Helper()
	res, err := portal.IngestCrawlResults("l1", []portal.CrawlResultPage{{URL: "https://portal.example/list", Items: []map[string]any{
		{"title": fitting.Title, "url": "https://portal.example/jobs/fit", "company": fitting.Company, "location": fitting.Location, "description": fitting.Description},
		{"title": unfitting.Title, "url": "https://portal.example/jobs/sales", "company": unfitting.Company, "location": unfitting.Location, "description": unfitting.Description},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func jobsSaved(t *testing.T) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The three states of the feature, through the real ingest: the switch off (nothing changes), on with the rule (the unfitting new job is
// kept out, counted and logged), on with the rule off (nothing is refused); and turning it off lets the ignored job in at the next crawl.
func TestTheWholeCrawlFollowsTheSettings(t *testing.T) {
	setup(t)
	addPortal(t)
	seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	inject(t)

	// off (the install default): both jobs are saved, nothing is logged
	if res := crawl(t); res.JobsSaved != 2 || res.JobsFilteredOut != 0 || ignoredCount(t) != 0 {
		t.Fatalf("switch off: %+v, %d logged", res, ignoredCount(t))
	}
	if _, err := db.JobsDB.Exec(`DELETE FROM jobs`); err != nil {
		t.Fatal(err)
	}

	// on with the rule: the sales job stays out
	if _, err := UpdateSettings(Settings{IgnoreEnabled: true, MinMatchEnabled: true, MinMatchPercent: 30}); err != nil {
		t.Fatal(err)
	}
	res := crawl(t)
	if res.JobsSaved != 1 || res.JobsFilteredOut != 1 || res.JobsUpdated != 1 || jobsSaved(t) != 1 {
		t.Fatalf("switch on: %+v, %d jobs", res, jobsSaved(t))
	}
	list, _ := ListIgnored(10)
	if len(list.Items) != 1 || list.Items[0].Title != "Sales Manager" || list.Items[0].PortalName != "Portal" || list.Items[0].Matcher != MinProfileMatchName {
		t.Errorf("the log: %+v", list.Items)
	}
	// the same crawl again: the saved job is refreshed, the sales job is judged again and logged once
	if res := crawl(t); res.JobsSaved != 0 || res.JobsFilteredOut != 1 || res.JobsUpdated != 2 || ignoredCount(t) != 1 {
		t.Errorf("second crawl: %+v, %d logged", res, ignoredCount(t))
	}

	// on, but the rule off: nothing refused, so the ignored job gets in (it was never remembered as deleted)
	if _, err := UpdateSettings(Settings{IgnoreEnabled: true, MinMatchEnabled: false, MinMatchPercent: 30}); err != nil {
		t.Fatal(err)
	}
	if res := crawl(t); res.JobsSaved != 1 || res.JobsFilteredOut != 0 || jobsSaved(t) != 2 {
		t.Errorf("rule off: %+v, %d jobs", res, jobsSaved(t))
	}
}

// A high limit refuses even the fitting job; an existing job is refreshed whatever the limit becomes.
func TestTheLimitIsWhatDecides(t *testing.T) {
	setup(t)
	addPortal(t)
	seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	inject(t)
	if _, err := UpdateSettings(Settings{IgnoreEnabled: true, MinMatchEnabled: true, MinMatchPercent: 100}); err != nil {
		t.Fatal(err)
	}
	if res := crawl(t); res.JobsSaved != 0 || res.JobsFilteredOut != 2 || jobsSaved(t) != 0 {
		t.Fatalf("limit 100: %+v", res)
	}
	if _, err := UpdateSettings(Settings{IgnoreEnabled: true, MinMatchEnabled: true, MinMatchPercent: 30}); err != nil {
		t.Fatal(err)
	}
	if res := crawl(t); res.JobsSaved != 1 || res.JobsFilteredOut != 1 {
		t.Fatalf("limit 30: %+v", res)
	}
	if _, err := UpdateSettings(Settings{IgnoreEnabled: true, MinMatchEnabled: true, MinMatchPercent: 100}); err != nil {
		t.Fatal(err)
	}
	if res := crawl(t); res.JobsSaved != 0 || res.JobsFilteredOut != 1 || res.JobsUpdated != 2 || jobsSaved(t) != 1 {
		t.Errorf("the saved job is refreshed even above its score: %+v, %d jobs", res, jobsSaved(t))
	}
}

// No profile to judge with: nothing is refused even with the feature on.
func TestWithNoProfileTheFeatureRefusesNothing(t *testing.T) {
	setup(t)
	addPortal(t)
	inject(t)
	if _, err := UpdateSettings(Settings{IgnoreEnabled: true, MinMatchEnabled: true, MinMatchPercent: 100}); err != nil {
		t.Fatal(err)
	}
	if res := crawl(t); res.JobsSaved != 2 || res.JobsFilteredOut != 0 || ignoredCount(t) != 0 {
		t.Errorf("%+v", res)
	}
}

// A broken settings table never loses jobs: the crawl keeps everything.
func TestUnreadableSettingsKeepEveryJob(t *testing.T) {
	setup(t)
	addPortal(t)
	seedProfile(t, "Fit", []string{"Go", "Docker"})
	inject(t)
	if _, err := db.JobsDB.Exec(`DROP TABLE crawler_settings`); err != nil {
		t.Fatal(err)
	}
	if res := crawl(t); res.JobsSaved != 2 || res.JobsFilteredOut != 0 {
		t.Errorf("%+v", res)
	}
}
