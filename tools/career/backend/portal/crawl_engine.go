package portal

import (
	"errors"
	"strings"
	"time"

	"career-tool-backend/db"
)

// crawl_engine.go (plan 197, step 1): a crawl engine per link. The Browser tool has ONE selected engine (Katana or Rod); a link may carry an
// engine of its own, stored here, and every decision that asks "which engine reads this link?" asks EffectiveEngine instead of reading the
// Browser's value. Only "rod" is an override (a link Katana cannot read, for example behind Cloudflare, can be read the Rod way while
// Katana stays selected); "" follows the Browser's selected engine, exactly as every link did before this step.

// The values of portal_links.crawl_engine.
const (
	LinkEngineBrowser = "" // follow the Browser tool's selected engine
	LinkEngineRod     = "rod"
)

// ErrInvalidCrawlEngine is a caller mistake (400): the engine of a link is "" or "rod".
var ErrInvalidCrawlEngine = errors.New(`crawlEngine must be "" (follow the Browser tool's engine) or "rod"`)

// ValidateCrawlEngine trims and checks a link's engine value.
func ValidateCrawlEngine(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v != LinkEngineBrowser && v != LinkEngineRod {
		return "", ErrInvalidCrawlEngine
	}
	return v, nil
}

// EngineOf is the pure rule: the link's own engine when it has one, else the Browser's selected engine (which may be "" when it could not be
// read: the caller then judges nothing by an engine, as before).
func EngineOf(linkEngine, browserEngine string) string {
	if linkEngine != LinkEngineBrowser {
		return linkEngine
	}
	return strings.TrimSpace(browserEngine)
}

// GetLinkCrawlEngine reads a link's own engine ("" = follows the Browser). ErrUnknownPortalLink for an unknown id.
func GetLinkCrawlEngine(linkID string) (string, error) {
	if err := RequirePortalLinkExists(linkID); err != nil {
		return "", err
	}
	var engine string
	if err := db.JobsDB.QueryRow(`SELECT crawl_engine FROM portal_links WHERE id = ?`, linkID).Scan(&engine); err != nil {
		return "", err
	}
	return engine, nil
}

// EffectiveEngine is the engine that reads this link now, given the Browser tool's selected engine. An unknown link follows the Browser.
func EffectiveEngine(linkID, browserEngine string) string {
	own, err := GetLinkCrawlEngine(linkID)
	if err != nil {
		return EngineOf(LinkEngineBrowser, browserEngine)
	}
	return EngineOf(own, browserEngine)
}

// SetLinkCrawlEngine sets a link's engine ("" or "rod"). Whatever the link holds for the other engine (a rule, picked data, AI instructions)
// stays: switching is reversible. A new choice forgets the block recorded for the old one and the "Not now" of the suggestion (plan 197,
// step 4), because both belonged to the engine that was in use.
func SetLinkCrawlEngine(linkID, engine string) error {
	v, err := ValidateCrawlEngine(engine)
	if err != nil {
		return err
	}
	current, err := GetLinkCrawlEngine(linkID) // also refuses an unknown link
	if err != nil {
		return err
	}
	if current == v {
		return nil // the same choice again (an Edit that saved the form unchanged) forgets nothing
	}
	_, err = db.JobsDB.Exec(
		`UPDATE portal_links SET crawl_engine = ?, engine_block_code = '', engine_block_at = NULL, engine_hint_dismissed_at = NULL, updated_at = datetime('now') WHERE id = ?`,
		v, linkID)
	return err
}

// --- the block record and the suggestion (plan 197, step 4) ---

// The Browser's codes that mean "this site stopped Katana's read": a Cloudflare check or block, a rate limit (429) and another vendor's bot wall.
const (
	BlockCloudflareBlocked   = "cloudflare_blocked"
	BlockCloudflareChallenge = "cloudflare_challenge_unresolved"
	BlockRateLimited         = "rate_limited"
	BlockBotProtectionFound  = "bot_protection_detected"
)

// IsEngineBlockCode says whether a Browser error code is one of the four that make Rod worth suggesting.
func IsEngineBlockCode(code string) bool {
	switch code {
	case BlockCloudflareBlocked, BlockCloudflareChallenge, BlockRateLimited, BlockBotProtectionFound:
		return true
	}
	return false
}

// NoteEngineBlock records that a Katana-only read of the link was stopped by the site (code: one of the four). A link set to Rod is never
// marked (Katana did not read it), an unknown code or link changes nothing. The latest block wins.
func NoteEngineBlock(linkID, code string) error {
	if !IsEngineBlockCode(code) {
		return nil
	}
	_, err := db.JobsDB.Exec(
		`UPDATE portal_links SET engine_block_code = ?, engine_block_at = datetime('now') WHERE id = ? AND crawl_engine != 'rod'`, code, linkID)
	return err
}

// ClearEngineBlock forgets a recorded block after a Katana read of the link went through.
func ClearEngineBlock(linkID string) error {
	_, err := db.JobsDB.Exec(`UPDATE portal_links SET engine_block_code = '', engine_block_at = NULL WHERE id = ? AND engine_block_code != ''`, linkID)
	return err
}

// DismissEngineHint is the person's "Not now": the suggestion for the block that exists now stays hidden; a NEWER block shows it again.
// Without a recorded block it changes nothing.
func DismissEngineHint(linkID string) error {
	if err := RequirePortalLinkExists(linkID); err != nil {
		return err
	}
	_, err := db.JobsDB.Exec(`UPDATE portal_links SET engine_hint_dismissed_at = datetime('now') WHERE id = ? AND engine_block_code != ''`, linkID)
	return err
}

// EngineSuggestion is what the page offers for a link: the code that stopped Katana and when. Never anything else.
type EngineSuggestion struct {
	Code  string `json:"code"`
	Since string `json:"since"`
}

// suggestionOf derives the suggestion from the stored columns: a block, on a link that is not set to Rod, that the person has not dismissed
// since it happened. nil otherwise.
func suggestionOf(linkEngine, code string, at, dismissed *string) *EngineSuggestion {
	if linkEngine == LinkEngineRod || code == "" || at == nil {
		return nil
	}
	if dismissed != nil && *dismissed >= *at {
		return nil
	}
	return &EngineSuggestion{Code: code, Since: *at}
}

// EngineBlockRecent says whether a block was recorded for the link within the last window (for the Katana AI: stop for now, and no longer
// once the block is old, so that a later try can clear it).
func EngineBlockRecent(linkID string, window time.Duration) (code string, recent bool) {
	var c string
	var at *string
	if err := db.JobsDB.QueryRow(`SELECT engine_block_code, engine_block_at FROM portal_links WHERE id = ? AND crawl_engine != 'rod'`, linkID).Scan(&c, &at); err != nil || c == "" || at == nil {
		return "", false
	}
	t, err := time.Parse("2006-01-02 15:04:05", *at)
	if err != nil {
		return c, false
	}
	return c, time.Since(t) < window
}
