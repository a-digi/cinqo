// link_rules.go (plan 185, step 4) — the stored settings of the link crawler
// (jobs found by the URL pattern of their links, without AI): which method a
// portal link is crawled with, and its rule.
//
// This file holds the rules of the DATA: a link has one active method; a
// rule belongs to the link's own host; a link that needs a login is never
// crawled with the link crawler; the rule is validated by the linkrule
// package and bounded in size. Whether the Browser tool's engine is Katana is
// not known here (it needs the person's session and core): the crawl package's
// handler asks for it before calling anything that writes a rule.
package portal

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"career-tool-backend/db"
	"career-tool-backend/portal/linkrule"
)

// The two ways a link's listing is crawled.
const (
	CrawlMethodInstructions = "instructions" // the AI-written selectors (the default, as before)
	CrawlMethodLinks        = "links"        // the link crawler: URL patterns, no AI
)

// MaxLinkRulesJSON bounds the stored rule. The biggest valid rule is 20+20 patterns of 300 characters; as JSON a character can take up to
// 6 bytes (an escape such as \u003c), so the bound holds even that.
const MaxLinkRulesJSON = 80 << 10

var (
	ErrInvalidCrawlMethod = errors.New(`crawlMethod must be "instructions" or "links"`)
	ErrLinkRulesNeeded    = errors.New("this link has no rules for the Katana engine yet: pick its job links in the Job list rules and save the proposed pattern, or select Rod on the Browser & Engine page to crawl it with its AI instructions")
	ErrLinkRulesLogin     = errors.New("a link that needs a login can use the link crawler only with a kept login: log in and tick Keep this login first")
	ErrLinkRulesHost      = errors.New("the rule's host must be the host of this link's address")
	ErrLinkRulesInvalid   = errors.New("the rule is not valid")
	ErrLinkRulesTooLarge  = fmt.Errorf("the rule is larger than %d bytes", MaxLinkRulesJSON)
)

// LinkCrawlSettings is what is stored for one link.
type LinkCrawlSettings struct {
	CrawlMethod string         `json:"crawlMethod"`
	Rules       *linkrule.Rule `json:"rules"`
}

func validCrawlMethod(m string) bool { return m == CrawlMethodInstructions || m == CrawlMethodLinks }

// linkHost is the lower-case host[:port] of a link's address.
func linkHost(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}

func decodeStoredRules(raw sql.NullString) *linkrule.Rule {
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return nil
	}
	var r linkrule.Rule
	if err := json.Unmarshal([]byte(raw.String), &r); err != nil {
		return nil
	}
	return &r
}

// GetLinkCrawlSettings reads a link's method and rule (ErrUnknownPortalLink for an unknown link).
func GetLinkCrawlSettings(id string) (LinkCrawlSettings, error) {
	if err := RequirePortalLinkExists(id); err != nil {
		return LinkCrawlSettings{}, err
	}
	var method string
	var raw sql.NullString
	if err := db.JobsDB.QueryRow(`SELECT crawl_method, link_rules FROM portal_links WHERE id = ?`, id).Scan(&method, &raw); err != nil {
		return LinkCrawlSettings{}, err
	}
	return LinkCrawlSettings{CrawlMethod: method, Rules: decodeStoredRules(raw)}, nil
}

// SaveLinkCrawlSettings stores a link's crawl method and, optionally, its rule.
//
//   - rules != nil stores that rule (replacing the stored one); clearRules removes the stored rule; both unset keep it.
//   - Storing a rule, or choosing the method "links", is refused for a link that needs a login (ErrLinkRulesLogin).
//   - A rule must have the host of the link's own address, and pass linkrule.ValidateRule, and fit MaxLinkRulesJSON.
//   - The method "links" needs a rule (the new one or the stored one that stays).
//   - Switching to "instructions" and clearing a rule are always allowed (no engine or rule conditions).
//
// Nothing is written unless everything is valid.
func SaveLinkCrawlSettings(id, method string, rules *linkrule.Rule, clearRules bool) error {
	if !validCrawlMethod(method) {
		return ErrInvalidCrawlMethod
	}
	if rules != nil && clearRules {
		return fmt.Errorf("%w: give a rule or ask to clear it, not both", ErrLinkRulesInvalid)
	}
	if err := RequirePortalLinkExists(id); err != nil {
		return err
	}
	var linkURL, confirmedJobHost string
	var loginFlag int
	var storedRaw sql.NullString
	if err := db.JobsDB.QueryRow(`SELECT url, requires_login, link_rules, job_host FROM portal_links WHERE id = ?`, id).Scan(&linkURL, &loginFlag, &storedRaw, &confirmedJobHost); err != nil {
		return err
	}
	needsLogin := loginFlag != 0
	// A link that needs a login may use the link crawler only with a kept login (plan 195, step 4): Katana then reads it with the session.
	if needsLogin && (rules != nil || method == CrawlMethodLinks) && KeptLoginFor(id) == "" {
		return ErrLinkRulesLogin
	}

	var newJSON string
	if rules != nil {
		norm, err := checkRuleForHost(*rules, linkURL, confirmedJobHost)
		if err != nil {
			return err
		}
		b, err := json.Marshal(norm)
		if err != nil {
			return err
		}
		if len(b) > MaxLinkRulesJSON {
			return ErrLinkRulesTooLarge
		}
		newJSON = string(b)
	}

	hasRule := (rules != nil) || (!clearRules && decodeStoredRules(storedRaw) != nil)
	if method == CrawlMethodLinks && !hasRule {
		return ErrLinkRulesNeeded
	}

	switch {
	case rules != nil:
		_, err := db.JobsDB.Exec(`UPDATE portal_links SET crawl_method = ?, link_rules = ?, updated_at = datetime('now') WHERE id = ?`, method, newJSON, id)
		return err
	case clearRules:
		_, err := db.JobsDB.Exec(`UPDATE portal_links SET crawl_method = ?, link_rules = NULL, updated_at = datetime('now') WHERE id = ?`, method, id)
		return err
	default:
		_, err := db.JobsDB.Exec(`UPDATE portal_links SET crawl_method = ?, updated_at = datetime('now') WHERE id = ?`, method, id)
		return err
	}
}

// checkRuleForHost normalises a rule (lower-case hosts) and checks it belongs to the site of linkURL and is valid. The rule's host is the
// link's own host; its job host (plan 203) is empty or exactly the job host a person confirmed on the link (jobHost), never anything else:
// this is the one place that keeps a rule, whoever wrote it, from reaching a host the person did not confirm.
func checkRuleForHost(rule linkrule.Rule, linkURL, jobHost string) (linkrule.Rule, error) {
	rule.Host = strings.ToLower(strings.TrimSpace(rule.Host))
	rule.JobHost = strings.ToLower(strings.TrimSpace(rule.JobHost))
	if rule.Host == "" || rule.Host != linkHost(linkURL) {
		return linkrule.Rule{}, fmt.Errorf("%w (rule host %q, link host %q)", ErrLinkRulesHost, rule.Host, linkHost(linkURL))
	}
	if rule.JobHost != "" && rule.JobHost != jobHost {
		return linkrule.Rule{}, fmt.Errorf("%w (rule job host %q, confirmed job host %q)", ErrLinkRulesJobHost, rule.JobHost, jobHost)
	}
	if err := linkrule.ValidateRule(rule); err != nil {
		return linkrule.Rule{}, fmt.Errorf("%w: %v", ErrLinkRulesInvalid, err)
	}
	return rule, nil
}

// ValidateRuleForLink is the check a rule passes before it is stored, previewed or tested: the link exists, the rule is for the link's
// own host and valid. It returns the normalised rule. Nothing is written.
func ValidateRuleForLink(id string, rule linkrule.Rule) (linkrule.Rule, error) {
	linkURL, err := GetPortalLinkURL(id)
	if err != nil {
		return linkrule.Rule{}, err
	}
	jobHost, err := LinkJobHost(id)
	if err != nil {
		return linkrule.Rule{}, err
	}
	return checkRuleForHost(rule, linkURL, jobHost)
}

// LinkSiteHost is the lower-case host[:port] of a link's address (the site its rule must belong to).
func LinkSiteHost(id string) (string, error) {
	linkURL, err := GetPortalLinkURL(id)
	if err != nil {
		return "", err
	}
	return linkHost(linkURL), nil
}

// resetLinkCrawlForLogin puts a link back to the method "instructions": a link that needs a login is never crawled with the link
// crawler. Its rule stays stored (the person can use it again if the login is switched off).
func resetLinkCrawlForLogin(id string) error {
	_, err := db.JobsDB.Exec(`UPDATE portal_links SET crawl_method = 'instructions' WHERE id = ? AND crawl_method != 'instructions'`, id)
	return err
}

// dropLinkRulesOfOtherHost removes a link's rule (and returns it to "instructions") when the link's address moved to another host, or when
// the rule names a job host the link no longer has (plan 203): a rule belongs to the sites of its link.
func dropLinkRulesOfOtherHost(id string) error {
	var linkURL, jobHost string
	var raw sql.NullString
	if err := db.JobsDB.QueryRow(`SELECT url, link_rules, job_host FROM portal_links WHERE id = ?`, id).Scan(&linkURL, &raw, &jobHost); err != nil {
		return err
	}
	rule := decodeStoredRules(raw)
	if rule == nil || (strings.EqualFold(rule.Host, linkHost(linkURL)) && (rule.JobHost == "" || strings.EqualFold(rule.JobHost, jobHost))) {
		return nil
	}
	_, err := db.JobsDB.Exec(`UPDATE portal_links SET crawl_method = 'instructions', link_rules = NULL WHERE id = ?`, id)
	return err
}

// linkCrawlByLink reads every link's method and rule in one query (for the portals list).
func linkCrawlByLink() (map[string]LinkCrawlSettings, error) {
	rows, err := db.JobsDB.Query(`SELECT id, crawl_method, link_rules FROM portal_links`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]LinkCrawlSettings{}
	for rows.Next() {
		var id, method string
		var raw sql.NullString
		if err := rows.Scan(&id, &method, &raw); err != nil {
			return nil, err
		}
		out[id] = LinkCrawlSettings{CrawlMethod: method, Rules: decodeStoredRules(raw)}
	}
	return out, rows.Err()
}
