package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/a-digi/coco-server/server/request"

	"github.com/a-digi/cinqo/src/organisation"
	"github.com/a-digi/cinqo/src/organisation/repository/persistent"
)

func seedTools(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, created_at) VALUES ('tn', 'namako', 'Namako', '1.0.0', 'backend_only', 1, 'running', '/x', '2026-11-15T00:00:00Z')`,
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, created_at) VALUES ('tb', 'browser', 'Browser', '1.0.0', 'backend_only', 1, 'running', '/x', '2026-11-15T00:00:00Z')`,
		`INSERT INTO tool_background_requests (tool_id, target_slug, method, path_suffix) VALUES ('tn', 'browser', 'POST', 'a')`,
		`INSERT INTO tool_background_requests (tool_id, target_slug, method, path_suffix) VALUES ('tn', 'browser', 'POST', 'b')`,
		`INSERT INTO tool_background_requests (tool_id, target_slug, method, path_suffix) VALUES ('tn', 'browser', 'POST', 'c')`,
		`INSERT INTO tool_background_grants (tool_id, target_slug, method, path_suffix, approved_by, approved_at) VALUES ('tn', 'browser', 'POST', 'a', 'u', 'x')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
}

func usageRow(t *testing.T, db *sql.DB, tool, org, day string, calls, denied int) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO background_usage (tool_id, org_id, day, calls, denied) VALUES (?, ?, ?, ?, ?)`, tool, org, day, calls, denied); err != nil {
		t.Fatal(err)
	}
}

func TestTheListShowsEachOrganisationsUnattendedSwitch(t *testing.T) {
	db := openDB(t)
	p := persistent.NewOrganisationPersistentRepo(db)
	a, _ := p.Create("Acme")
	_ = p.SetBackground(a.ID, true, "op")
	_, out := call(t, func(c request.RequestContext) { list(db, c) }, "GET", "", nil)
	items := msg(out)["items"].([]any)
	got := map[string]any{}
	for _, it := range items {
		m := it.(map[string]any)
		got[m["name"].(string)] = m["backgroundEnabled"]
	}
	if got["Acme"] != true || got["Default"] != false {
		t.Errorf("%v", got)
	}
}

func TestTheOperatorSwitchesUnattendedUseOnAndOffAndItIsRecordedWho(t *testing.T) {
	db := openDB(t)
	p := persistent.NewOrganisationPersistentRepo(db)
	a, _ := p.Create("Acme")
	_ = p.Assign("u1", a.ID)
	_ = p.Assign("u2", a.ID)
	set := func(caller, id, body string) (*httptest.ResponseRecorder, map[string]any) {
		return call(t, func(c request.RequestContext) { setBackground(db, caller, c) }, "PUT", body, map[string]string{"id": id})
	}
	rec, out := set("op-1", a.ID, `{"enabled":true}`)
	m := msg(out)
	if rec.Code != 200 || m["backgroundEnabled"] != true || m["name"] != "Acme" || m["memberCount"] != float64(2) {
		t.Fatalf("%d %v", rec.Code, out)
	}
	var by string
	_ = db.QueryRow(`SELECT enabled_by FROM organisation_background WHERE org_id = ?`, a.ID).Scan(&by)
	if by != "op-1" {
		t.Errorf("recorded by %q", by)
	}
	if rec, out := set("op-2", a.ID, `{"enabled":false}`); rec.Code != 200 || msg(out)["backgroundEnabled"] != false {
		t.Errorf("%d %v", rec.Code, out)
	}
	_ = db.QueryRow(`SELECT enabled_by FROM organisation_background WHERE org_id = ?`, a.ID).Scan(&by)
	if by != "op-2" {
		t.Errorf("the last person who switched is recorded: %q", by)
	}
	for name, c := range map[string]struct {
		caller, id, body string
		want             int
	}{
		"no known operator":    {"", a.ID, `{"enabled":true}`, http.StatusUnauthorized},
		"unknown organisation": {"op", "nope", `{"enabled":true}`, http.StatusNotFound},
		"no value":             {"op", a.ID, `{}`, http.StatusBadRequest},
		"not json":             {"op", a.ID, `nope`, http.StatusBadRequest},
		"a text, not a bool":   {"op", a.ID, `{"enabled":"yes"}`, http.StatusBadRequest},
	} {
		if rec, _ := set(c.caller, c.id, c.body); rec.Code != c.want {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	if on, _ := queryBackground(db, a.ID); on {
		t.Error("a refused switch changes nothing")
	}
}

func queryBackground(db *sql.DB, id string) (bool, error) {
	var on int
	err := db.QueryRow(`SELECT enabled FROM organisation_background WHERE org_id = ?`, id).Scan(&on)
	return on == 1, err
}

func overviewOf(t *testing.T, db *sql.DB, now time.Time, target string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	ctx := request.NewContext(rec, httptest.NewRequest("GET", target, nil), nil)
	backgroundOverview(db, func() time.Time { return now }, ctx)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, msg(out)
}

func TestTheOverviewShowsToolsThatAskAndTheUsageOfTheLastDays(t *testing.T) {
	db := openDB(t)
	seedTools(t, db)
	acme, _ := persistent.NewOrganisationPersistentRepo(db).Create("Acme")
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	usageRow(t, db, "tn", acme.ID, "2026-11-15", 10, 1)
	usageRow(t, db, "tn", acme.ID, "2026-11-09", 5, 0)   // exactly 7 days with today: the first day of the window
	usageRow(t, db, "tn", acme.ID, "2026-11-08", 99, 99) // one day older: out
	usageRow(t, db, "tn", "", "2026-11-14", 0, 2)        // refused with no usable organisation
	usageRow(t, db, "gone-tool", "gone-org", "2026-11-13", 1, 0)
	code, out := overviewOf(t, db, now, "/x")
	if code != 200 || out["days"] != float64(7) || out["since"] != "2026-11-09" || out["killSwitch"] != false {
		t.Fatalf("%d %v", code, out)
	}
	tools := out["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["slug"] != "namako" || tools[0].(map[string]any)["requested"] != float64(3) || tools[0].(map[string]any)["approved"] != float64(1) {
		t.Errorf("only a tool that asks is listed: %v", tools)
	}
	usage := out["usage"].([]any)
	var lines []string
	for _, u := range usage {
		m := u.(map[string]any)
		lines = append(lines, m["day"].(string)+" "+m["toolSlug"].(string)+" "+m["orgName"].(string)+" "+itoa(m["calls"])+"/"+itoa(m["denied"]))
	}
	want := []string{"2026-11-15 namako Acme 10/1", "2026-11-14 namako  0/2", "2026-11-13 gone-tool gone-org 1/0", "2026-11-09 namako Acme 5/0"}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Errorf("%v\nwant %v", lines, want)
	}
	if usage[2].(map[string]any)["toolName"] != "gone-tool" {
		t.Errorf("a tool removed since is shown by its id: %v", usage[2])
	}
}

func itoa(v any) string { f, _ := v.(float64); return strconv.Itoa(int(f)) }

func TestTheOverviewPeriodAndTheKillSwitch(t *testing.T) {
	db := openDB(t)
	seedTools(t, db)
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	usageRow(t, db, "tn", "default", "2026-11-15", 1, 0)
	usageRow(t, db, "tn", "default", "2026-11-14", 1, 0)
	if _, out := overviewOf(t, db, now, "/x?days=1"); len(out["usage"].([]any)) != 1 || out["since"] != "2026-11-15" || out["days"] != float64(1) {
		t.Errorf("one day means today only: %v", out)
	}
	if _, out := overviewOf(t, db, now, "/x?days=90"); out["since"] != "2026-08-18" || len(out["usage"].([]any)) != 2 {
		t.Errorf("%v", out)
	}
	for _, d := range []string{"0", "-3", "91", "x", "1.5"} {
		if code, _ := overviewOf(t, db, now, "/x?days="+d); code != http.StatusBadRequest {
			t.Errorf("days=%s: %d", d, code)
		}
	}
	t.Setenv(organisation.BackgroundKillSwitchEnv, "off")
	if _, out := overviewOf(t, db, now, "/x"); out["killSwitch"] != true {
		t.Errorf("%v", out)
	}
	t.Setenv(organisation.BackgroundKillSwitchEnv, "on")
	if _, out := overviewOf(t, db, now, "/x"); out["killSwitch"] != false {
		t.Errorf("only the value off is the switch: %v", out)
	}
}

func TestAnEmptyOverviewHasEmptyListsNotNull(t *testing.T) {
	db := openDB(t)
	_, out := overviewOf(t, db, time.Now(), "/x")
	if tools, ok := out["tools"].([]any); !ok || len(tools) != 0 {
		t.Errorf("%v", out["tools"])
	}
	if usage, ok := out["usage"].([]any); !ok || len(usage) != 0 {
		t.Errorf("%v", out["usage"])
	}
}

func askOrganisations(t *testing.T, db *sql.DB, auth string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/x", nil)
	if auth != "" {
		r.Header.Set("Authorization", auth)
	}
	backgroundOrganisations(db, request.NewContext(rec, r, nil))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func itemsOf(out map[string]any) []string {
	m, _ := out["message"].(map[string]any)
	raw, _ := m["items"].([]any)
	var ids []string
	for _, v := range raw {
		ids = append(ids, v.(string))
	}
	return ids
}

func TestAToolIsToldWhichOrganisationsToWorkForOnlyWhenItHoldsAnApproval(t *testing.T) {
	db := openDB(t)
	seedTools(t, db) // namako asks for three routes and holds one approval; browser asks for nothing
	for _, q := range []string{
		`UPDATE tools SET service_token = 'tok-namako' WHERE id = 'tn'`,
		`UPDATE tools SET service_token = 'tok-browser' WHERE id = 'tb'`,
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, service_token, created_at) VALUES ('td', 'dormant', 'D', '1', 'backend_only', 0, 'stopped', '/x', 'tok-dormant', 'x')`,
		`INSERT INTO tool_background_grants (tool_id, target_slug, method, path_suffix, approved_by, approved_at) VALUES ('td', 'browser', 'POST', 'a', 'u', 'x')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	p := persistent.NewOrganisationPersistentRepo(db)
	on, _ := p.Create("On")
	off, _ := p.Create("Off")
	susp, _ := p.Create("Susp")
	_ = p.SetBackground(on.ID, true, "op")
	_ = p.SetBackground(susp.ID, true, "op")
	_ = p.Update(susp.ID, "Susp", "suspended")
	_ = off
	code, out := askOrganisations(t, db, "ToolService tok-namako")
	if got := itemsOf(out); code != 200 || len(got) != 1 || got[0] != on.ID {
		t.Fatalf("only an active organisation that switched it on: %d %v", code, got)
	}
	if code, out := askOrganisations(t, db, "ToolService tok-browser"); code != 200 || len(itemsOf(out)) != 0 || out["message"].(map[string]any)["items"] == nil {
		t.Errorf("a tool that holds no approval is told nobody (an empty list, not null): %d %v", code, out)
	}
	if code, out := askOrganisations(t, db, "ToolService tok-dormant"); code != 200 || len(itemsOf(out)) != 0 {
		t.Errorf("a disabled tool is told nobody: %d %v", code, out)
	}
	t.Setenv(organisation.BackgroundKillSwitchEnv, "off")
	if code, out := askOrganisations(t, db, "ToolService tok-namako"); code != 200 || len(itemsOf(out)) != 0 {
		t.Errorf("the kill switch tells nobody: %d %v", code, out)
	}
	t.Setenv(organisation.BackgroundKillSwitchEnv, "")
	for name, auth := range map[string]string{
		"no header": "", "a person's bearer": "Bearer tok-namako", "an unknown token": "ToolService nope", "a blank token": "ToolService ", "only blanks": "ToolService    ",
		"another scheme": "toolservice tok-namako", "token without the scheme": "tok-namako",
	} {
		if code, _ := askOrganisations(t, db, auth); code != http.StatusUnauthorized {
			t.Errorf("%s: %d", name, code)
		}
	}
	if _, err := db.Exec(`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, service_token, created_at) VALUES ('tx', 'secretless', 'S', '1', 'backend_only', 1, 'running', '/x', '', 'x')`); err != nil {
		t.Fatal(err)
	}
	if code, _ := askOrganisations(t, db, "ToolService "); code != http.StatusUnauthorized {
		t.Errorf("a tool with no secret is not reachable with a blank one: %d", code)
	}
}
