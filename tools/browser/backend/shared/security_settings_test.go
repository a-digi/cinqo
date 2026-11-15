package shared

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecuritySettingsAreOffByDefault(t *testing.T) {
	initSettingsDB(t)
	s, err := LoadSecuritySettings()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.AllowedTestHosts) != 0 || s.AllowMediaDownloads {
		t.Errorf("fresh defaults: %+v", s)
	}
	if TestHostAllowed("mock.localhost") || MediaDownloadsAllowed() {
		t.Error("nothing is allowed by default")
	}
}

func TestWithNoDatabaseNothingIsAllowed(t *testing.T) {
	old := DB
	DB = nil
	defer func() { DB = old }()
	if TestHostAllowed("mock.localhost") || MediaDownloadsAllowed() {
		t.Error("no database must mean no permission")
	}
	if _, err := LoadSecuritySettings(); err == nil {
		t.Error("expected an error with no database")
	}
}

func TestValidTestHostIsOneExactHostName(t *testing.T) {
	for _, ok := range []string{"mock.localhost", "namako-mock.test", "a.b.c.example", "x1", "localhost"} {
		if !ValidTestHost(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{
		"", "*.localhost", "*", "127.0.0.1", "::1", "[::1]", "0x7f.1", "Mock.localhost", "mock.localhost:8080", "mock.localhost/", "http://mock.localhost",
		".localhost", "mock.", "a..b", "-a.test", "a-.test", "a b.test", "mock_.test", "ü.test", strings.Repeat("a", 64) + ".test", strings.Repeat("a.", 130) + "x",
		"10.0.0.1", "192.168.1.5", "169.254.169.254", "localhost.", "mock.localhost?x=1", "mock.localhost#x",
	} {
		if ValidTestHost(bad) {
			t.Errorf("%q must be refused", bad)
		}
	}
}

func TestNormalizeTestHostsTrimsLowersDedupesSortsAndRefusesAllOrNothing(t *testing.T) {
	got, err := NormalizeTestHosts([]string{" B.localhost ", "a.localhost", "b.localhost"})
	if err != nil || strings.Join(got, ",") != "a.localhost,b.localhost" {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := NormalizeTestHosts([]string{"a.localhost", "*.x"}); err == nil {
		t.Error("one bad entry must refuse the whole list")
	}
	many := make([]string, 0, MaxTestHosts+1)
	for i := 0; i <= MaxTestHosts; i++ {
		many = append(many, "h"+strings.Repeat("x", i)+".test")
	}
	if _, err := NormalizeTestHosts(many); err == nil {
		t.Error("more than the cap must be refused")
	}
	if got, err := NormalizeTestHosts(nil); err != nil || got == nil || len(got) != 0 {
		t.Errorf("an empty list is an empty (non-nil) list: %v %v", got, err)
	}
}

func putSecurity(body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	SecuritySettingsHandler(rec, httptest.NewRequest(http.MethodPut, "/security-settings", strings.NewReader(body)))
	return rec
}

func TestSecuritySettingsRoundTripAndMatchIsExact(t *testing.T) {
	initSettingsDB(t)
	rec := putSecurity(`{"allowedTestHosts":["Mock.localhost"],"allowMediaDownloads":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var got SecuritySettings
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.AllowedTestHosts) != 1 || got.AllowedTestHosts[0] != "mock.localhost" || !got.AllowMediaDownloads {
		t.Errorf("%+v", got)
	}
	if !TestHostAllowed("MOCK.localhost") || !MediaDownloadsAllowed() {
		t.Error("the listed host (any case) and the switch must read back")
	}
	for _, other := range []string{"evil.mock.localhost", "mock.localhost.evil.com", "localhost", "127.0.0.1", "xmock.localhost", ""} {
		if TestHostAllowed(other) {
			t.Errorf("%q must not match (exact names only)", other)
		}
	}
	rec = httptest.NewRecorder()
	SecuritySettingsHandler(rec, httptest.NewRequest(http.MethodGet, "/security-settings", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "mock.localhost") {
		t.Errorf("GET: %d %s", rec.Code, rec.Body)
	}
}

func TestSecuritySettingsPutIsStrict(t *testing.T) {
	initSettingsDB(t)
	for name, body := range map[string]string{
		"a missing field":   `{"allowedTestHosts":["a.test"]}`,
		"the other missing": `{"allowMediaDownloads":true}`,
		"an unknown field":  `{"allowedTestHosts":[],"allowMediaDownloads":false,"x":1}`,
		"a wildcard":        `{"allowedTestHosts":["*.test"],"allowMediaDownloads":false}`,
		"an IP literal":     `{"allowedTestHosts":["127.0.0.1"],"allowMediaDownloads":false}`,
		"not json":          `nope`,
		"null hosts":        `{"allowedTestHosts":null,"allowMediaDownloads":false}`,
		"a huge body":       `{"allowedTestHosts":["` + strings.Repeat("a", 1<<15) + `"],"allowMediaDownloads":false}`,
	} {
		if rec := putSecurity(body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, rec.Code)
		}
	}
	if s, _ := LoadSecuritySettings(); len(s.AllowedTestHosts) != 0 || s.AllowMediaDownloads {
		t.Errorf("a refused PUT must change nothing: %+v", s)
	}
	rec := httptest.NewRecorder()
	SecuritySettingsHandler(rec, httptest.NewRequest(http.MethodPost, "/security-settings", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", rec.Code)
	}
}

func TestAHandEditedRowCanNeverWidenAnything(t *testing.T) {
	initSettingsDB(t)
	if _, err := DB.Exec(`UPDATE browser_security_settings SET allowed_test_hosts = '["*.evil.com","127.0.0.1"]' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if TestHostAllowed("a.evil.com") || TestHostAllowed("127.0.0.1") {
		t.Error("an invalid stored list must grant nothing")
	}
	if _, err := LoadSecuritySettings(); err == nil {
		t.Error("an invalid stored list must be an error")
	}
	if _, err := DB.Exec(`UPDATE browser_security_settings SET allowed_test_hosts = 'not json' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if TestHostAllowed("a.test") {
		t.Error("unreadable JSON must grant nothing")
	}
}
