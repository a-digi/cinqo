package shared

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sessionCookies(domain string, expires float64) []SessionCookie {
	return []SessionCookie{
		{Name: "sid", Value: "SECRET-SID-VALUE", Domain: domain, Path: "/", Secure: true, HTTPOnly: true, Expires: expires},
		{Name: "pref", Value: "dark", Domain: domain, Path: "/", Expires: 0},
		{Name: "other", Value: "SECRET-OTHER-SITE", Domain: ".evil.example", Path: "/"},
	}
}

func TestLoginSessionRoundTripKeepsOnlyTheSitesOwnCookies(t *testing.T) {
	dbPath := initSettingsDB(t)
	keyPath := filepath.Join(filepath.Dir(dbPath), loginSessionKeyFile)
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatalf("a fresh install must have no key file, stat error = %v", err)
	}
	info, err := SaveLoginSession("www.example.com", sessionCookies(".example.com", float64(time.Now().Add(48*time.Hour).Unix())))
	if err != nil {
		t.Fatal(err)
	}
	if info.State != LoginSessionValid || info.Host != "www.example.com" || len(info.ID) != 32 {
		t.Errorf("info: %+v", info)
	}
	st, err := os.Stat(keyPath)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("the key file must exist with mode 0600: %v %v", err, st)
	}
	got, err := OpenLoginSession(info.ID, "www.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Value != "SECRET-SID-VALUE" || !got[0].HTTPOnly {
		t.Errorf("the site's two cookies must come back (not the other site's): %+v", got)
	}
}

func TestLoginSessionRowHoldsNoPlainCookieAndInfoNeverCarriesOne(t *testing.T) {
	dbPath := initSettingsDB(t)
	info, err := SaveLoginSession("www.example.com", sessionCookies(".example.com", 0))
	if err != nil {
		t.Fatal(err)
	}
	var sealed []byte
	if err := DB.QueryRow(`SELECT sealed_cookies FROM login_sessions WHERE id = ?`, info.ID).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sealed), "SECRET-SID-VALUE") || strings.Contains(string(sealed), "sid") {
		t.Error("the stored bytes contain a cookie in the clear")
	}
	raw, _ := os.ReadFile(dbPath)
	if strings.Contains(string(raw), "SECRET-SID-VALUE") {
		t.Error("the database file contains a cookie value in the clear")
	}
	list, err := ListLoginSessions()
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %v", list, err)
	}
}

func TestLoginSessionIsNotGivenToAnotherHost(t *testing.T) {
	initSettingsDB(t)
	info, _ := SaveLoginSession("www.example.com", sessionCookies(".example.com", 0))
	if _, err := OpenLoginSession(info.ID, "www.other.org"); !errors.Is(err, ErrLoginSessionWrongHost) {
		t.Errorf("another site: %v", err)
	}
	if _, err := OpenLoginSession(info.ID, "notwww.example.com"); !errors.Is(err, ErrLoginSessionWrongHost) {
		t.Errorf("a look-alike host: %v", err)
	}
	if _, err := OpenLoginSession(info.ID, "jobs.www.example.com"); err != nil {
		t.Errorf("a subdomain of the session's host is allowed: %v", err)
	}
	// The sealed row cannot be read as another host's: the host is authenticated data of the seal.
	var sealed []byte
	_ = DB.QueryRow(`SELECT sealed_cookies FROM login_sessions WHERE id = ?`, info.ID).Scan(&sealed)
	key, _ := loginSessionKey(false)
	if _, err := openSession(key, info.ID, "www.other.org", sealed); !errors.Is(err, ErrLoginSessionUnreadable) {
		t.Errorf("a row opened as another host: %v", err)
	}
}

func TestLoginSessionWrongOrLostKeyFails(t *testing.T) {
	dbPath := initSettingsDB(t)
	info, _ := SaveLoginSession("www.example.com", sessionCookies(".example.com", 0))
	keyPath := filepath.Join(filepath.Dir(dbPath), loginSessionKeyFile)
	if err := os.WriteFile(keyPath, make([]byte, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenLoginSession(info.ID, "www.example.com"); !errors.Is(err, ErrLoginSessionUnreadable) {
		t.Errorf("a wrong key: %v", err)
	}
	os.Remove(keyPath)
	if _, err := OpenLoginSession(info.ID, "www.example.com"); !errors.Is(err, ErrLoginSessionUnreadable) {
		t.Errorf("a lost key must not create a new one on read: %v", err)
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Error("opening must never create a key file")
	}
}

func TestLoginSessionLifetimeIsTheEarlierOfTheCookiesAndThirtyDays(t *testing.T) {
	initSettingsDB(t)
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	loginSessionNow = func() time.Time { return now }
	t.Cleanup(func() { loginSessionNow = time.Now })

	short, _ := SaveLoginSession("a.example.com", sessionCookies(".example.com", float64(now.Add(2*time.Hour).Unix())))
	if short.ExpiresAt != now.Add(2*time.Hour).Format(time.RFC3339) {
		t.Errorf("cookie expiry earlier than the cap: %s", short.ExpiresAt)
	}
	long, _ := SaveLoginSession("b.example.com", sessionCookies(".example.com", float64(now.Add(400*24*time.Hour).Unix())))
	if long.ExpiresAt != now.Add(LoginSessionMaxAge).Format(time.RFC3339) {
		t.Errorf("a 400-day cookie must be capped at 30 days: %s", long.ExpiresAt)
	}
	sessionOnly, _ := SaveLoginSession("c.example.com", []SessionCookie{{Name: "sid", Value: "v", Domain: "c.example.com", Path: "/"}})
	if sessionOnly.ExpiresAt != now.Add(LoginSessionMaxAge).Format(time.RFC3339) {
		t.Errorf("a session-only cookie lives the cap: %s", sessionOnly.ExpiresAt)
	}

	now = now.Add(3 * time.Hour) // the short one is over
	if _, err := OpenLoginSession(short.ID, "a.example.com"); !errors.Is(err, ErrLoginSessionExpired) {
		t.Errorf("an expired session must not open: %v", err)
	}
	if info, err := GetLoginSession(short.ID); err != nil || info.State != LoginSessionExpired {
		t.Errorf("its state must say expired: %+v %v", info, err)
	}
	if _, err := OpenLoginSession(long.ID, "b.example.com"); err != nil {
		t.Errorf("the long one still opens: %v", err)
	}
	if err := prepareLoginSessions(DB, loginSessionKeyDir); err != nil { // what a start does
		t.Fatal(err)
	}
	if _, err := GetLoginSession(short.ID); !errors.Is(err, ErrLoginSessionNotFound) {
		t.Errorf("an expired row is deleted at start: %v", err)
	}
}

func TestLoginSessionExpiredCookiesAreNotKeptOrReplayed(t *testing.T) {
	initSettingsDB(t)
	now := time.Now()
	cs := []SessionCookie{
		{Name: "old", Value: "x", Domain: "www.example.com", Path: "/", Expires: float64(now.Add(-time.Hour).Unix())},
		{Name: "sid", Value: "y", Domain: "www.example.com", Path: "/", Expires: float64(now.Add(time.Hour).Unix())},
	}
	info, err := SaveLoginSession("www.example.com", cs)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := OpenLoginSession(info.ID, "www.example.com")
	if len(got) != 1 || got[0].Name != "sid" {
		t.Errorf("only the live cookie is kept: %+v", got)
	}
}

func TestLoginSessionNothingToKeepAndDelete(t *testing.T) {
	initSettingsDB(t)
	if _, err := SaveLoginSession("www.example.com", []SessionCookie{{Name: "x", Value: "y", Domain: ".evil.example"}}); !errors.Is(err, ErrLoginSessionNoCookies) {
		t.Errorf("only other sites' cookies: %v", err)
	}
	if _, err := SaveLoginSession("", sessionCookies(".example.com", 0)); !errors.Is(err, ErrLoginSessionNoCookies) {
		t.Errorf("no host: %v", err)
	}
	info, _ := SaveLoginSession("www.example.com", sessionCookies(".example.com", 0))
	if err := DeleteLoginSession(info.ID); err != nil {
		t.Fatal(err)
	}
	if err := DeleteLoginSession(info.ID); err != nil {
		t.Errorf("delete is idempotent: %v", err)
	}
	if _, err := GetLoginSession(info.ID); !errors.Is(err, ErrLoginSessionNotFound) {
		t.Errorf("after delete: %v", err)
	}
	if _, err := OpenLoginSession(info.ID, "www.example.com"); !errors.Is(err, ErrLoginSessionNotFound) {
		t.Errorf("open after delete: %v", err)
	}
}

func TestLoginSessionErrorsNeverCarryACookie(t *testing.T) {
	initSettingsDB(t)
	info, _ := SaveLoginSession("www.example.com", sessionCookies(".example.com", 0))
	for _, host := range []string{"www.other.org", "www.example.com"} {
		_, err := OpenLoginSession(info.ID, host)
		if err != nil && strings.Contains(err.Error(), "SECRET") {
			t.Errorf("error text carries a cookie: %v", err)
		}
	}
}
