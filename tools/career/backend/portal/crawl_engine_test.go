package portal

import (
	"errors"
	"testing"

	"career-tool-backend/db"
)

func TestEngineOfPrefersTheLinksOwnEngine(t *testing.T) {
	for _, c := range []struct{ own, browser, want string }{
		{"", "katana", "katana"},
		{"", " katana ", "katana"}, // the Browser's value is trimmed like everywhere else
		{"", "rod", "rod"},
		{"", "", ""}, // an engine that could not be read: nothing is judged
		{"rod", "katana", "rod"},
		{"rod", "rod", "rod"},
		{"rod", "", "rod"},
	} {
		if got := EngineOf(c.own, c.browser); got != c.want {
			t.Errorf("EngineOf(%q, %q) = %q, want %q", c.own, c.browser, got, c.want)
		}
	}
}

func TestOnlyAnEmptyOrRodEngineIsValid(t *testing.T) {
	for in, want := range map[string]string{"": "", " ": "", "rod": "rod", " rod ": "rod"} {
		if got, err := ValidateCrawlEngine(in); err != nil || got != want {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"katana", "Rod", "ROD", "selenium", "rod,katana", "playwright"} {
		if _, err := ValidateCrawlEngine(bad); !errors.Is(err, ErrInvalidCrawlEngine) {
			t.Errorf("%q must be refused: %v", bad, err)
		}
	}
}

func TestALinkFollowsTheBrowserUntilItIsSetToRodAndCanGoBack(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	if got, err := GetLinkCrawlEngine(id); err != nil || got != "" {
		t.Fatalf("a new link follows the Browser: %q %v", got, err)
	}
	if got := EffectiveEngine(id, "katana"); got != "katana" {
		t.Errorf("effective: %q", got)
	}
	if err := SetLinkCrawlEngine(id, "rod"); err != nil {
		t.Fatal(err)
	}
	if got := EffectiveEngine(id, "katana"); got != "rod" {
		t.Errorf("a Rod link under Katana: %q", got)
	}
	if l := loginLink(t, id); l.CrawlEngine != "rod" {
		t.Errorf("the portals list: %q", l.CrawlEngine)
	}
	if err := SetLinkCrawlEngine(id, ""); err != nil {
		t.Fatal(err)
	}
	if got := EffectiveEngine(id, "katana"); got != "katana" {
		t.Errorf("back to the Browser: %q", got)
	}
	if l := loginLink(t, id); l.CrawlEngine != "" {
		t.Errorf("the portals list: %q", l.CrawlEngine)
	}
	// an unknown link follows the Browser, and cannot be set
	if got := EffectiveEngine("nope", "katana"); got != "katana" {
		t.Errorf("unknown link: %q", got)
	}
	if err := SetLinkCrawlEngine("nope", "rod"); !errors.Is(err, ErrUnknownPortalLink) {
		t.Errorf("unknown link: %v", err)
	}
	if err := SetLinkCrawlEngine(id, "katana"); !errors.Is(err, ErrInvalidCrawlEngine) {
		t.Errorf("a bad engine: %v", err)
	}
}

// Switching keeps what the link holds for the other engine and forgets the block and the "Not now" that belonged to the engine in use.
func TestSwitchingTheEngineKeepsTheDataAndForgetsTheBlock(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	rule := ruleFor("board.example", "board.example/jobs/*")
	if err := SaveLinkCrawlSettings(id, CrawlMethodLinks, rule, false); err != nil {
		t.Fatal(err)
	}
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET engine_block_code = 'cloudflare_blocked', engine_block_at = datetime('now'), engine_hint_dismissed_at = datetime('now') WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if err := SetLinkCrawlEngine(id, "rod"); err != nil {
		t.Fatal(err)
	}
	var code string
	var at, dismissed *string
	if err := db.JobsDB.QueryRow(`SELECT engine_block_code, engine_block_at, engine_hint_dismissed_at FROM portal_links WHERE id = ?`, id).Scan(&code, &at, &dismissed); err != nil {
		t.Fatal(err)
	}
	if code != "" || at != nil || dismissed != nil {
		t.Errorf("the block must be forgotten: %q %v %v", code, at, dismissed)
	}
	if s, err := GetLinkCrawlSettings(id); err != nil || s.Rules == nil {
		t.Errorf("the rule of the other engine must stay: %+v %v", s, err)
	}
}
