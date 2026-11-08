package portal

import (
	"errors"
	"net"
	"regexp"
	"strconv"
	"strings"

	"career-tool-backend/db"
)

// job_host.go (plan 203, step 2): the ONE other host a person may confirm for a link, because the page of the link is on one host and its
// job links are on another (an aggregator, an applicant system). Without it a link's rule and its job pages are for the link's own host only.
// Only a person's request sets it (the link update route); nothing in the AI tools can, and the Browser still refuses private addresses.

// ErrInvalidJobHost: the host is not one a link may use as its job host.
var ErrInvalidJobHost = errors.New("the job host must be a public host name (for example jobs.example.com): not this link's own host, not an address, not a local name")

// ErrLinkRulesJobHost: a rule names a job host that is not the one confirmed on the link.
var ErrLinkRulesJobHost = errors.New("the rule's job host is not the job host confirmed on this link")

var jobHostLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// NormalizeJobHost checks a job host and returns it lower-case. "" is allowed (it clears the job host). The host must be a host name with
// at least one dot (letters, digits, dashes, the xn-- form of an international name), optionally with a port; never the link's own host,
// never an IP address (literal addresses are refused altogether) and never a local name (localhost, .local, .localhost, .internal).
func NormalizeJobHost(raw, linkURL string) (string, error) {
	h := strings.ToLower(strings.TrimSpace(raw))
	if h == "" {
		return "", nil
	}
	if strings.ContainsAny(h, "/\\?#@*% \t\r\n") || strings.Contains(h, "://") {
		return "", ErrInvalidJobHost
	}
	name, port := h, ""
	if i := strings.LastIndex(h, ":"); i >= 0 {
		name, port = h[:i], h[i+1:]
		n, err := strconv.Atoi(port)
		if port == "" || err != nil || n < 1 || n > 65535 {
			return "", ErrInvalidJobHost
		}
	}
	if len(name) > 253 || !strings.Contains(name, ".") || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") {
		return "", ErrInvalidJobHost
	}
	if net.ParseIP(name) != nil || net.ParseIP(strings.Trim(name, "[]")) != nil {
		return "", ErrInvalidJobHost
	}
	for _, label := range strings.Split(name, ".") {
		if !jobHostLabel.MatchString(label) {
			return "", ErrInvalidJobHost
		}
	}
	if name == "localhost" || strings.HasSuffix(name, ".localhost") || strings.HasSuffix(name, ".local") || strings.HasSuffix(name, ".internal") {
		return "", ErrInvalidJobHost
	}
	if h == linkHost(linkURL) {
		return "", ErrInvalidJobHost
	}
	return h, nil
}

// LinkJobHost is the job host confirmed on a link ("" when none). ErrUnknownPortalLink for an unknown link.
func LinkJobHost(id string) (string, error) {
	if err := RequirePortalLinkExists(id); err != nil {
		return "", err
	}
	var h string
	if err := db.JobsDB.QueryRow(`SELECT job_host FROM portal_links WHERE id = ?`, id).Scan(&h); err != nil {
		return "", err
	}
	return h, nil
}

// LinkSiteHosts is the hosts a link's job links and job pages may be on: its own host and, when a person confirmed one, its job host.
// The pages of the list itself (the link's address and its next pages) are always the own host only.
func LinkSiteHosts(id string) (own, job string, err error) {
	if own, err = LinkSiteHost(id); err != nil {
		return "", "", err
	}
	job, err = LinkJobHost(id)
	return own, job, err
}

// SetLinkJobHost sets or clears (raw "") a link's job host. A rule that depended on the old job host is taken away with it (the link goes
// back to the method "instructions"), exactly as a rule is when the link's address moves to another host. The same value again changes
// nothing.
func SetLinkJobHost(id, raw string) error {
	linkURL, err := GetPortalLinkURL(id) // also refuses an unknown link
	if err != nil {
		return err
	}
	v, err := NormalizeJobHost(raw, linkURL)
	if err != nil {
		return err
	}
	current, err := LinkJobHost(id)
	if err != nil {
		return err
	}
	if current == v {
		return nil
	}
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET job_host = ?, updated_at = datetime('now') WHERE id = ?`, v, id); err != nil {
		return err
	}
	return dropLinkRulesOfOtherHost(id)
}

// jobHostByLink reads every link's job host in one query (for the portals list); links without one are absent.
func jobHostByLink() (map[string]string, error) {
	rows, err := db.JobsDB.Query(`SELECT id, job_host FROM portal_links WHERE job_host != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, h string
		if err := rows.Scan(&id, &h); err != nil {
			return nil, err
		}
		out[id] = h
	}
	return out, rows.Err()
}

// RunningAttemptIsManual (plan 198): whether the AI run now going for this link was started by a person (the Generate button), not by
// auto-discovery. Only a person's run may ask the Browser to open its Katana window for a check.
func RunningAttemptIsManual(linkID string) bool {
	var by string
	err := db.JobsDB.QueryRow(`SELECT triggered_by FROM instruction_attempts WHERE portal_link_id = ? AND status = 'running' ORDER BY started_at DESC LIMIT 1`, linkID).Scan(&by)
	return err == nil && by == "manual"
}
