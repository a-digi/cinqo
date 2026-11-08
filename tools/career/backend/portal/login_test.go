package portal

import (
	"errors"
	"strings"
	"testing"

	"career-tool-backend/db"
)

func loginSetup(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.JobsDB.Close(); db.CareerDB.Close() })
}

func loginLink(t *testing.T, id string) portalLink {
	t.Helper()
	portals, err := ListPortals()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range portals {
		for _, l := range p.Links {
			if l.ID == id {
				return l
			}
		}
	}
	t.Fatalf("link %s not listed", id)
	return portalLink{}
}

func ptr[T any](v T) *T { return &v }

func TestValidateLoginURL(t *testing.T) {
	long := "https://example.com/" + strings.Repeat("a", MaxLoginURLLength)
	exact := "https://example.com/" + strings.Repeat("a", MaxLoginURLLength-len("https://example.com/"))
	for _, c := range []struct {
		in, want string
		ok       bool
	}{
		{"", "", true},
		{"   ", "", true},
		{"https://accounts.example.com/login", "https://accounts.example.com/login", true},
		{"  http://example.com/signin?next=%2Fjobs  ", "http://example.com/signin?next=%2Fjobs", true},
		{exact, exact, true},
		{exact + "a", "", false}, // one character over the limit
		{long, "", false},
		{"ftp://example.com/login", "", false},
		{"javascript:alert(1)", "", false},
		{"file:///etc/passwd", "", false},
		{"//example.com/login", "", false},
		{"example.com/login", "", false},
		{"https:///login", "", false},
		{"https://user:pw@example.com/login", "", false},
		{"https://user@example.com/login", "", false},
		{"https://exa mple.com/", "", false},
	} {
		got, err := ValidateLoginURL(c.in)
		if c.ok && (err != nil || got != c.want) {
			t.Errorf("%q: got %q %v, want %q", c.in, got, err, c.want)
		}
		if !c.ok && !errors.Is(err, ErrInvalidLoginURL) {
			t.Errorf("%q: error %v, want ErrInvalidLoginURL", c.in, err)
		}
	}
}

func TestANewLinkNeedsNoLoginAndTheListShowsIt(t *testing.T) {
	loginSetup(t)
	pid, _ := CreatePortal("Board")
	id, err := AddPortalLink(pid, "https://board.example/jobs", "Jobs")
	if err != nil {
		t.Fatal(err)
	}
	l := loginLink(t, id)
	if l.RequiresLogin || l.LoginURL != "" {
		t.Errorf("a new link: %+v", l)
	}
}

func TestUpdatingTheLoginSettingsOfALink(t *testing.T) {
	loginSetup(t)
	pid, _ := CreatePortal("Board")
	a, _ := AddPortalLink(pid, "https://board.example/a", "A")
	b, _ := AddPortalLink(pid, "https://board.example/b", "B")
	// A link needs auto-crawl off before it can need a login (manual_only.go); a new link has it on.
	for _, id := range []string{a, b} {
		if err := UpdatePortalLinkAutoDiscoveryEnabled(id, false); err != nil {
			t.Fatal(err)
		}
	}

	if err := UpdatePortalLinkLogin(a, ptr(true), nil); err != nil {
		t.Fatal(err)
	}
	if l := loginLink(t, a); !l.RequiresLogin || l.LoginURL != "" {
		t.Errorf("only the flag: %+v", l)
	}
	if err := UpdatePortalLinkLogin(a, nil, ptr("  https://accounts.board.example/login ")); err != nil {
		t.Fatal(err)
	}
	if l := loginLink(t, a); !l.RequiresLogin || l.LoginURL != "https://accounts.board.example/login" {
		t.Errorf("only the address (the flag is kept, the address is trimmed): %+v", l)
	}
	if err := UpdatePortalLinkLogin(a, ptr(false), ptr("")); err != nil {
		t.Fatal(err)
	}
	if l := loginLink(t, a); l.RequiresLogin || l.LoginURL != "" {
		t.Errorf("both cleared: %+v", l)
	}
	// nothing given: nothing changes (and no error)
	UpdatePortalLinkLogin(a, ptr(true), ptr("https://x.example/login"))
	if err := UpdatePortalLinkLogin(a, nil, nil); err != nil {
		t.Fatal(err)
	}
	if l := loginLink(t, a); !l.RequiresLogin || l.LoginURL != "https://x.example/login" {
		t.Errorf("an empty update changed the link: %+v", l)
	}
	// the other link of the portal never changes
	if l := loginLink(t, b); l.RequiresLogin || l.LoginURL != "" {
		t.Errorf("another link changed: %+v", l)
	}
	// the link's own url and title are not touched by the login settings
	if l := loginLink(t, a); l.URL != "https://board.example/a" || l.Title != "A" {
		t.Errorf("url or title changed: %+v", l)
	}
}

func TestARefusedLoginAddressChangesNothingAtAll(t *testing.T) {
	loginSetup(t)
	pid, _ := CreatePortal("Board")
	a, _ := AddPortalLink(pid, "https://board.example/a", "A")
	UpdatePortalLinkLogin(a, ptr(true), ptr("https://ok.example/login"))
	before := loginLink(t, a)

	err := UpdatePortalLinkLogin(a, ptr(false), ptr("ftp://bad.example/login"))
	if !errors.Is(err, ErrInvalidLoginURL) {
		t.Fatalf("error %v", err)
	}
	if after := loginLink(t, a); after != before {
		t.Errorf("a refused update changed the link: %+v -> %+v", before, after)
	}
}

func TestLoginSettingsOfAnUnknownLinkAreRefused(t *testing.T) {
	loginSetup(t)
	if err := UpdatePortalLinkLogin("nobody", ptr(true), nil); !errors.Is(err, ErrUnknownPortalLink) {
		t.Errorf("error %v", err)
	}
}

func TestAddingALinkWithLoginSettings(t *testing.T) {
	loginSetup(t)
	pid, _ := CreatePortal("Board")
	id, err := AddPortalLinkWithLogin(pid, "https://board.example/a", "A", true, "https://accounts.board.example/login")
	if err != nil {
		t.Fatal(err)
	}
	if l := loginLink(t, id); !l.RequiresLogin || l.LoginURL != "https://accounts.board.example/login" || l.URL != "https://board.example/a" || l.Title != "A" {
		t.Errorf("%+v", l)
	}
	plain, err := AddPortalLinkWithLogin(pid, "https://board.example/b", "B", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if l := loginLink(t, plain); l.RequiresLogin || l.LoginURL != "" {
		t.Errorf("a link added without login settings: %+v", l)
	}
	// a login page can be given without the flag (it is only used once the flag is on)
	onlyURL, err := AddPortalLinkWithLogin(pid, "https://board.example/c", "C", false, "https://board.example/login")
	if err != nil {
		t.Fatal(err)
	}
	if l := loginLink(t, onlyURL); l.RequiresLogin || l.LoginURL != "https://board.example/login" {
		t.Errorf("only the address: %+v", l)
	}
}

func TestABadLoginAddressAddsNoLink(t *testing.T) {
	loginSetup(t)
	pid, _ := CreatePortal("Board")
	if _, err := AddPortalLinkWithLogin(pid, "https://board.example/a", "A", true, "javascript:alert(1)"); !errors.Is(err, ErrInvalidLoginURL) {
		t.Fatalf("error %v", err)
	}
	portals, _ := ListPortals()
	for _, p := range portals {
		if len(p.Links) != 0 {
			t.Errorf("a refused add left a link: %+v", p.Links)
		}
	}
	// the same url can still be added afterwards: nothing was reserved
	if _, err := AddPortalLinkWithLogin(pid, "https://board.example/a", "A", true, ""); err != nil {
		t.Errorf("adding after the refusal: %v", err)
	}
}

func TestAddingADuplicateLinkWithLoginSettingsIsStillADuplicate(t *testing.T) {
	loginSetup(t)
	pid, _ := CreatePortal("Board")
	AddPortalLink(pid, "https://board.example/a", "A")
	if _, err := AddPortalLinkWithLogin(pid, "https://board.example/a", "A again", true, ""); !errors.Is(err, ErrDuplicatePortalLink) {
		t.Errorf("error %v", err)
	}
}

func TestDeletingTheLinkLeavesNothingOfItsLoginSettings(t *testing.T) {
	loginSetup(t)
	pid, _ := CreatePortal("Board")
	id, _ := AddPortalLinkWithLogin(pid, "https://board.example/a", "A", true, "https://board.example/login")
	if err := RemovePortalLink(id); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM portal_links WHERE id = ?`, id).Scan(&n); err != nil || n != 0 {
		t.Errorf("the link is still there: %d %v", n, err)
	}
}
