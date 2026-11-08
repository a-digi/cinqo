package portal

import (
	"errors"
	"testing"
	"time"

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

// Saving the same choice again (an Edit form that was saved unchanged) must not forget the block or the "Not now".
func TestTheSameEngineChoiceAgainForgetsNothing(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET engine_block_code = 'rate_limited', engine_block_at = datetime('now') WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if err := SetLinkCrawlEngine(id, ""); err != nil {
		t.Fatal(err)
	}
	var code string
	if err := db.JobsDB.QueryRow(`SELECT engine_block_code FROM portal_links WHERE id = ?`, id).Scan(&code); err != nil || code != "rate_limited" {
		t.Errorf("an unchanged choice forgot the block: %q %v", code, err)
	}
}

// --- the block record and the suggestion (plan 197, step 4) ---

func suggestionFor(t *testing.T, id string) *EngineSuggestion {
	t.Helper()
	return loginLink(t, id).EngineSuggestion
}

func TestOnlyTheFourBlockCodesAreRecordedAndNeverForALinkOnRod(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	for _, bad := range []string{"", "redirect_refused", "bad_request", "login_expired", "engine_not_katana", "cloudflare"} {
		if err := NoteEngineBlock(id, bad); err != nil {
			t.Fatal(err)
		}
		if s := suggestionFor(t, id); s != nil {
			t.Errorf("code %q must not make a suggestion: %+v", bad, s)
		}
	}
	for _, code := range []string{BlockCloudflareBlocked, BlockCloudflareChallenge, BlockRateLimited, BlockBotProtectionFound} {
		if !IsEngineBlockCode(code) {
			t.Errorf("%s must be a block code", code)
		}
		if err := NoteEngineBlock(id, code); err != nil {
			t.Fatal(err)
		}
		if s := suggestionFor(t, id); s == nil || s.Code != code || s.Since == "" {
			t.Errorf("%s: %+v", code, s)
		}
	}
	// a link set to Rod is never marked: Katana did not read it
	if err := SetLinkCrawlEngine(id, "rod"); err != nil {
		t.Fatal(err)
	}
	if err := NoteEngineBlock(id, BlockRateLimited); err != nil {
		t.Fatal(err)
	}
	if s := suggestionFor(t, id); s != nil {
		t.Errorf("a Rod link must have no suggestion: %+v", s)
	}
	// an unknown link changes nothing and is not an error
	if err := NoteEngineBlock("nope", BlockRateLimited); err != nil {
		t.Errorf("unknown link: %v", err)
	}
}

func TestAReadThatGoesThroughForgetsTheBlock(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	_ = NoteEngineBlock(id, BlockCloudflareChallenge)
	if suggestionFor(t, id) == nil {
		t.Fatal("no suggestion after a block")
	}
	if err := ClearEngineBlock(id); err != nil {
		t.Fatal(err)
	}
	if s := suggestionFor(t, id); s != nil {
		t.Errorf("the block must be forgotten: %+v", s)
	}
}

// "Not now" hides the suggestion for the block that exists; a NEWER block shows it again; it needs a block to mean anything.
func TestNotNowHidesTheSuggestionUntilANewerBlock(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	if err := DismissEngineHint(id); err != nil { // no block yet: nothing to dismiss
		t.Fatal(err)
	}
	_ = NoteEngineBlock(id, BlockRateLimited)
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET engine_block_at = '2026-01-01 10:00:00' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if suggestionFor(t, id) == nil {
		t.Fatal("a dismissal before any block must not hide the first one")
	}
	if err := DismissEngineHint(id); err != nil {
		t.Fatal(err)
	}
	if s := suggestionFor(t, id); s != nil {
		t.Errorf("dismissed: %+v", s)
	}
	// a block that happens after the dismissal shows it again
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET engine_block_at = '2999-01-01 10:00:00' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if s := suggestionFor(t, id); s == nil {
		t.Error("a newer block must show the suggestion again")
	}
	if err := DismissEngineHint("nope"); !errors.Is(err, ErrUnknownPortalLink) {
		t.Errorf("unknown link: %v", err)
	}
}

func TestARecentBlockIsReportedForTheAIAndAnOldOneIsNot(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	if _, recent := EngineBlockRecent(id, time.Hour); recent {
		t.Error("no block, nothing recent")
	}
	_ = NoteEngineBlock(id, BlockCloudflareBlocked)
	if code, recent := EngineBlockRecent(id, time.Hour); !recent || code != BlockCloudflareBlocked {
		t.Errorf("fresh block: %q %v", code, recent)
	}
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET engine_block_at = datetime('now', '-2 hours') WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if code, recent := EngineBlockRecent(id, time.Hour); recent || code != BlockCloudflareBlocked {
		t.Errorf("old block: %q %v (the code is still known, it is just not recent)", code, recent)
	}
}
