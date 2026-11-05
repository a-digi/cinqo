package portal

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/db"
)

type autoState struct {
	enabled             int
	interval            *int
	listing, detail, ai int
}

func autoOf(t *testing.T, id string) autoState {
	t.Helper()
	var s autoState
	if err := db.JobsDB.QueryRow(`SELECT auto_discovery_enabled, auto_discovery_interval_minutes, listing_crawl_requested, job_detail_crawl_requested, job_detail_instructions_ai_requested FROM portal_links WHERE id = ?`, id).
		Scan(&s.enabled, &s.interval, &s.listing, &s.detail, &s.ai); err != nil {
		t.Fatal(err)
	}
	return s
}

func setupManualOnly(t *testing.T) (plain, gated string) {
	t.Helper()
	loginSetup(t)
	pid, err := CreatePortal("P")
	if err != nil {
		t.Fatal(err)
	}
	plain, err = AddPortalLink(pid, "https://a.example/plain", "Plain")
	if err != nil {
		t.Fatal(err)
	}
	gated, err = AddPortalLinkWithLogin(pid, "https://a.example/gated", "Gated", true, "")
	if err != nil {
		t.Fatal(err)
	}
	return plain, gated
}

func TestALinkThatNeedsALoginRefusesEveryAutomaticSwitch(t *testing.T) {
	_, gated := setupManualOnly(t)
	before := autoOf(t, gated)
	sixty := 60
	for name, call := range map[string]func() error{
		"auto discovery on":          func() error { return UpdatePortalLinkAutoDiscoveryEnabled(gated, true) },
		"own interval":               func() error { return UpdatePortalLinkAutoDiscoveryInterval(gated, &sixty) },
		"listing crawl requested":    func() error { return UpdatePortalLinkListingCrawlRequested(gated, true) },
		"job detail crawl requested": func() error { return UpdatePortalLinkJobDetailCrawlRequested(gated, true) },
		"detail instructions by AI":  func() error { return UpdatePortalLinkJobDetailInstructionsAIRequested(gated, true) },
	} {
		if err := call(); !errors.Is(err, ErrManualOnly) {
			t.Errorf("%s: err = %v, want ErrManualOnly", name, err)
		}
	}
	if after := autoOf(t, gated); after != before {
		t.Errorf("a refused call changed the link: %+v -> %+v", before, after)
	}
	// switching things OFF stays allowed
	if err := UpdatePortalLinkAutoDiscoveryEnabled(gated, false); err != nil {
		t.Errorf("off: %v", err)
	}
	if err := UpdatePortalLinkAutoDiscoveryInterval(gated, nil); err != nil {
		t.Errorf("clear interval: %v", err)
	}
	for _, f := range []func(string, bool) error{UpdatePortalLinkListingCrawlRequested, UpdatePortalLinkJobDetailCrawlRequested, UpdatePortalLinkJobDetailInstructionsAIRequested} {
		if err := f(gated, false); err != nil {
			t.Errorf("clearing a flag: %v", err)
		}
	}
}

func TestAPlainLinkStillAcceptsEveryAutomaticSwitch(t *testing.T) {
	plain, _ := setupManualOnly(t)
	sixty := 60
	for name, err := range map[string]error{
		"auto discovery": UpdatePortalLinkAutoDiscoveryEnabled(plain, true),
		"interval":       UpdatePortalLinkAutoDiscoveryInterval(plain, &sixty),
		"listing":        UpdatePortalLinkListingCrawlRequested(plain, true),
		"detail crawl":   UpdatePortalLinkJobDetailCrawlRequested(plain, true),
		"detail AI":      UpdatePortalLinkJobDetailInstructionsAIRequested(plain, true),
	} {
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	s := autoOf(t, plain)
	if s.enabled != 1 || s.interval == nil || *s.interval != 60 || s.listing != 1 || s.detail != 1 || s.ai != 1 {
		t.Errorf("%+v", s)
	}
}

// The other direction: a link with auto-crawl on cannot be made to need a login (nothing is written), until auto-crawl is off.
func TestALinkWithAutoCrawlOnCannotBeSetToNeedALogin(t *testing.T) {
	plain, _ := setupManualOnly(t)
	if s := autoOf(t, plain); s.enabled != 1 {
		t.Fatalf("a new link must start with auto-crawl on: %+v", s)
	}
	yes := true
	url := "https://a.example/login"
	if err := UpdatePortalLinkLogin(plain, &yes, &url); !errors.Is(err, ErrLoginWithAutoCrawl) {
		t.Fatalf("err = %v, want ErrLoginWithAutoCrawl", err)
	}
	if need, u, _ := GetPortalLinkLogin(plain); need || u != "" {
		t.Errorf("a refused call wrote: need=%v url=%q", need, u)
	}
	// the login page address alone is not "needs a login": allowed while auto-crawl is on
	if err := UpdatePortalLinkLogin(plain, nil, &url); err != nil {
		t.Errorf("address only: %v", err)
	}
	// untick/false is always allowed
	no := false
	if err := UpdatePortalLinkLogin(plain, &no, nil); err != nil {
		t.Errorf("false: %v", err)
	}
	// after auto-crawl is off, it works
	if err := UpdatePortalLinkAutoDiscoveryEnabled(plain, false); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePortalLinkLogin(plain, &yes, nil); err != nil {
		t.Fatalf("after switching auto-crawl off: %v", err)
	}
}

func TestSettingTheLoginOnClearsAnIntervalAndPendingFlags(t *testing.T) {
	plain, _ := setupManualOnly(t)
	sixty := 60
	for _, err := range []error{
		UpdatePortalLinkAutoDiscoveryInterval(plain, &sixty),
		UpdatePortalLinkListingCrawlRequested(plain, true),
		UpdatePortalLinkJobDetailCrawlRequested(plain, true),
		UpdatePortalLinkJobDetailInstructionsAIRequested(plain, true),
		UpdatePortalLinkAutoDiscoveryEnabled(plain, false),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	yes := true
	if err := UpdatePortalLinkLogin(plain, &yes, nil); err != nil {
		t.Fatal(err)
	}
	if s := autoOf(t, plain); s != (autoState{}) {
		t.Errorf("pending automatic work must be cleared: %+v", s)
	}
}

func TestANewLinkThatNeedsALoginStartsWithAutoCrawlOff(t *testing.T) {
	_, gated := setupManualOnly(t)
	if s := autoOf(t, gated); s != (autoState{}) {
		t.Errorf("%+v", s)
	}
}

func TestCheckManualOnlyRequest(t *testing.T) {
	plain, gated := setupManualOnly(t)
	yes, no := true, false
	for _, c := range []struct {
		name        string
		id          string
		login, auto *bool
		other       bool
		want        error
	}{
		{"nothing asked", plain, nil, nil, false, nil},
		{"login and auto-crawl on in one request", plain, &yes, &yes, false, ErrLoginWithAutoCrawl},
		{"login on, auto-crawl still on", plain, &yes, nil, false, ErrLoginWithAutoCrawl},
		{"login on and auto-crawl off in one request", plain, &yes, &no, false, nil},
		{"auto-crawl on for a link that needs a login", gated, nil, &yes, false, ErrLoginWithAutoCrawl},
		{"auto-crawl on and the login taken away in one request", gated, &no, &yes, false, nil},
		{"another automatic switch on a login link", gated, nil, nil, true, ErrManualOnly},
		{"another automatic switch together with taking the login away", gated, &no, nil, true, nil},
		{"another automatic switch on a plain link", plain, nil, nil, true, nil},
		{"login off on a plain link", plain, &no, nil, false, nil},
		{"unknown link", "nope", &yes, nil, false, ErrUnknownPortalLink},
	} {
		if err := CheckManualOnlyRequest(c.id, c.login, c.auto, c.other); !errors.Is(err, c.want) && !(c.want == nil && err == nil) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
}

func eventTo(h http.HandlerFunc, linkID string) int {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/event", strings.NewReader(`{"topic":"x","payload":{"portal_link_id":"`+linkID+`"}}`)))
	return rec.Code
}

func TestEventsRaiseNoRequestFlagForALinkThatNeedsALogin(t *testing.T) {
	plain, gated := setupManualOnly(t)
	for _, id := range []string{plain, gated} {
		if code := eventTo(ListingInstructionsReadyEventHandler, id); code != http.StatusAccepted {
			t.Fatalf("listing event: %d", code)
		}
		if code := eventTo(JobsCrawledEventHandler, id); code != http.StatusAccepted {
			t.Fatalf("jobs crawled event: %d", code)
		}
	}
	if s := autoOf(t, plain); s.listing != 1 || s.detail != 1 || s.ai != 1 {
		t.Errorf("a plain link must still get its flags: %+v", s)
	}
	if s := autoOf(t, gated); s != (autoState{}) {
		t.Errorf("a link that needs a login must get no flag: %+v", s)
	}
}

func TestTheSweeperRaisesNoFlagForALinkThatNeedsALogin(t *testing.T) {
	setupAttemptsDB(t, "a", "b")
	for _, q := range []string{
		`UPDATE portal_links SET crawl_instructions = 'L', updated_at = datetime('now', '-1 hour')`,
		`UPDATE portal_links SET requires_login = 1 WHERE id = 'a'`,
	} {
		if _, err := db.JobsDB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	report, err := SweepFlags()
	if err != nil {
		t.Fatal(err)
	}
	if report != (FlagSweepReport{ListingCrawl: 1}) {
		t.Errorf("report %+v: only the plain link may be repaired", report)
	}
	if l, ai, dc := flagsOf(t, "a"); l+ai+dc != 0 {
		t.Errorf("the link that needs a login got flags: %d %d %d", l, ai, dc)
	}
	if l, _, _ := flagsOf(t, "b"); l != 1 {
		t.Errorf("the plain link must still be repaired: %d", l)
	}
}
