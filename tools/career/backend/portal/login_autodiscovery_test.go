package portal

import (
	"testing"

	"career-tool-backend/db"
)

func autoLink(t *testing.T, portalID, url string, needsLogin bool, minutes *int) string {
	t.Helper()
	id, err := AddPortalLink(portalID, url, "L")
	if err != nil {
		t.Fatal(err)
	}
	if err := UpdatePortalLinkCrawlInstructions(id, "container: \".x\"\nfields:\n  - label: title\n    selector: h2\n  - label: url\n    selector: a\n    attribute: href\npagination:\n  nextSelector: a.next\n  maxPages: 3\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET auto_discovery_enabled = 1, auto_discovery_interval_minutes = ?, requires_login = ? WHERE id = ?`, minutes, boolInt(needsLogin), id); err != nil {
		t.Fatal(err)
	}
	return id
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func contains(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func TestAutoDiscoverySkipsLinksThatNeedALogin(t *testing.T) {
	loginSetup(t)
	pid, err := CreatePortal("P")
	if err != nil {
		t.Fatal(err)
	}
	ten := 10
	openGlobal := autoLink(t, pid, "https://a.example/1", false, nil)
	loginGlobal := autoLink(t, pid, "https://a.example/2", true, nil)
	openCustom := autoLink(t, pid, "https://a.example/3", false, &ten)
	loginCustom := autoLink(t, pid, "https://a.example/4", true, &ten)

	global, err := ListPortalLinksEligibleForAutoDiscovery(CrawlMethodInstructions)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(global, openGlobal) || contains(global, loginGlobal) {
		t.Errorf("global = %v", global)
	}
	custom, err := ListDuePortalLinksWithCustomInterval(CrawlMethodInstructions)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(custom, openCustom) || contains(custom, loginCustom) {
		t.Errorf("custom = %v", custom)
	}
}

func TestGetPortalLinkLogin(t *testing.T) {
	loginSetup(t)
	pid, _ := CreatePortal("P")
	plain, _ := AddPortalLink(pid, "https://a.example/plain", "A")
	gated, err := AddPortalLinkWithLogin(pid, "https://a.example/gated", "B", true, "https://a.example/login")
	if err != nil {
		t.Fatal(err)
	}
	if need, u, err := GetPortalLinkLogin(plain); err != nil || need || u != "" {
		t.Errorf("plain = %v %q %v", need, u, err)
	}
	if need, u, err := GetPortalLinkLogin(gated); err != nil || !need || u != "https://a.example/login" {
		t.Errorf("gated = %v %q %v", need, u, err)
	}
	if _, _, err := GetPortalLinkLogin("nope"); err == nil {
		t.Error("an unknown link must be an error")
	}
}
