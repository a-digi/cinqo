package portal

import (
	"errors"
	"strings"

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
	if err := RequirePortalLinkExists(linkID); err != nil {
		return err
	}
	_, err = db.JobsDB.Exec(
		`UPDATE portal_links SET crawl_engine = ?, engine_block_code = '', engine_block_at = NULL, engine_hint_dismissed_at = NULL, updated_at = datetime('now') WHERE id = ?`,
		v, linkID)
	return err
}
