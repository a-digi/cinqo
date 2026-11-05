package portal

import (
	"errors"

	"career-tool-backend/db"
)

// manual_only.go (plan 182): a link that needs a login is crawled BY HAND only. A person has to log in in the window a crawl opens, so
// nothing may start such a crawl, or the work that follows it, on its own: no auto discovery (global or custom interval), no request
// flag the page's run follower would act on (listing crawl, job detail crawl, job detail instructions by AI), and no flag repair by the
// sweeper. One rule here, used by every path that could switch one of those on.

// ErrManualOnly is a caller mistake (400): the requested automatic behaviour is not allowed for a link that needs a login.
var ErrManualOnly = errors.New("this link needs a login, so it is only crawled by hand with Crawl now: nothing can start it automatically")

// ErrNoAIForLoginLink is a caller mistake (400): the AI cannot write crawl instructions for a link that needs a login, because every page
// it can read is the logged-out one (the AI's browser tools run on the headless tab, not in the window a person logged in to). Starting such
// a run would only fail, cost tokens and count as a failed attempt ("Probably can't be crawled"), so it is refused before anything is recorded.
var ErrNoAIForLoginLink = errors.New("this link needs a login, and the AI cannot read pages behind your login yet: write its crawl instructions by hand (Job list and Job details under Crawl instructions)")

// requireNotLoginLink returns ErrManualOnly when the link needs a login (an unknown link is ErrUnknownPortalLink).
func requireNotLoginLink(id string) error {
	needs, _, err := GetPortalLinkLogin(id)
	if err != nil {
		return err
	}
	if needs {
		return ErrManualOnly
	}
	return nil
}

// ErrLoginWithAutoCrawl is a caller mistake (400): a link cannot need a login and have auto-crawl on at the same time. It is never fixed
// silently: the person switches Auto-discovery off first, then ticks Requires login.
var ErrLoginWithAutoCrawl = errors.New("a link that needs a login cannot be crawled automatically: switch Auto-discovery off for this link first")

// autoDiscoveryEnabledNow reads whether auto-crawl (auto discovery) is switched on for a link.
func autoDiscoveryEnabledNow(id string) (bool, error) {
	var enabled int
	if err := db.JobsDB.QueryRow(`SELECT auto_discovery_enabled FROM portal_links WHERE id = ?`, id).Scan(&enabled); err != nil {
		return false, err
	}
	return enabled != 0, nil
}

// CheckManualOnlyRequest is the check of a whole update request BEFORE anything is written, on the state the link would have after it:
// requiresLogin and autoDiscoveryEnabled are what the request sets (nil = leave as it is), otherAutomatic says the request also switches
// some other automatic behaviour on (an interval, a request flag). A link that needs a login and has any automatic behaviour on is refused,
// whichever of the two the request is changing, so the backend never relies on the page to keep them apart.
func CheckManualOnlyRequest(id string, requiresLogin, autoDiscoveryEnabled *bool, otherAutomatic bool) error {
	if requiresLogin == nil && autoDiscoveryEnabled == nil && !otherAutomatic {
		return nil
	}
	needsLogin, _, err := GetPortalLinkLogin(id)
	if err != nil {
		return err
	}
	autoOn, err := autoDiscoveryEnabledNow(id)
	if err != nil {
		return err
	}
	if requiresLogin != nil {
		needsLogin = *requiresLogin
	}
	if autoDiscoveryEnabled != nil {
		autoOn = *autoDiscoveryEnabled
	}
	if !needsLogin {
		return nil
	}
	if otherAutomatic {
		return ErrManualOnly
	}
	if autoOn {
		return ErrLoginWithAutoCrawl
	}
	return nil
}

// switchOffAutomatic turns every automatic behaviour of a link off: auto discovery (and its own interval) and the three request flags.
func switchOffAutomatic(id string) error {
	_, err := db.JobsDB.Exec(
		`UPDATE portal_links SET auto_discovery_enabled = 0, auto_discovery_interval_minutes = NULL,
		        listing_crawl_requested = 0, job_detail_crawl_requested = 0, job_detail_instructions_ai_requested = 0
		 WHERE id = ?`, id)
	return err
}
