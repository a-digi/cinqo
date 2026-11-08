package crawler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"browser-tool-backend/shared"

	"github.com/go-rod/rod/lib/proto"
)

// katana_window_sites.go (plan 198, step V3): what the person sees and controls about the sites the Katana window serves.
//
//	GET    /katana-window-hosts          the sites, and whether the window is open now
//	DELETE /katana-window-host?host=     forget a site: reads of it go the headless way again, and its cookies are removed from the window's
//	                                     profile (at once when the window is open, otherwise at its next start)
//	POST   /katana-window-open {url}     open the window at an address for the person to clear a check (the card's "keep Katana in the visible
//	                                     browser"); the person's answer arrives at /katana-window-cleared

type windowSite struct {
	Host          string `json:"host"`
	CreatedAt     string `json:"createdAt"`
	LastClearedAt string `json:"lastClearedAt,omitempty"`
}

func listWindowHosts() []windowSite {
	out := []windowSite{}
	db := windowHostsTable()
	if db == nil {
		return out
	}
	rows, err := db.Query(`SELECT host, created_at, COALESCE(last_cleared_at, '') FROM katana_window_hosts ORDER BY host`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var w windowSite
		if rows.Scan(&w.Host, &w.CreatedAt, &w.LastClearedAt) == nil {
			out = append(out, w)
		}
	}
	return out
}

// forgetWindowHost removes the site and, when the window is open, its cookies; otherwise the cookies are removed at the window's next start.
func forgetWindowHost(host string) {
	host = strings.ToLower(strings.TrimSpace(host))
	if db := windowHostsTable(); db != nil {
		_, _ = db.Exec(`DELETE FROM katana_window_hosts WHERE host = ?`, host)
		_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS katana_window_forget (host TEXT PRIMARY KEY)`)
		_, _ = db.Exec(`INSERT OR IGNORE INTO katana_window_forget (host) VALUES (?)`, host)
	} else {
		windowHostsMemMu.Lock()
		delete(windowHostsMem, host)
		windowHostsMemMu.Unlock()
	}
	windowMu.Lock()
	open := windowBrowser != nil
	windowMu.Unlock()
	if open {
		applyPendingCookieForgetting()
	}
}

// applyPendingCookieForgetting removes the cookies of every forgotten site from the open window's Chrome.
func applyPendingCookieForgetting() {
	db := shared.DB
	windowMu.Lock()
	b := windowBrowser
	windowMu.Unlock()
	if db == nil || b == nil {
		return
	}
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS katana_window_forget (host TEXT PRIMARY KEY)`)
	rows, err := db.Query(`SELECT host FROM katana_window_forget`)
	if err != nil {
		return
	}
	var hosts []string
	for rows.Next() {
		var h string
		if rows.Scan(&h) == nil {
			hosts = append(hosts, h)
		}
	}
	rows.Close()
	if len(hosts) == 0 {
		return
	}
	all, err := proto.StorageGetCookies{}.Call(b)
	if err != nil {
		return
	}
	// Network.deleteCookies needs a page session (the cookie store is the default context's, shared by every tab), so a blank tab does it.
	tab, err := b.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return
	}
	defer func() { _ = tab.Close() }()
	for _, h := range hosts {
		for _, c := range all.Cookies {
			if cookieDomainIs(c.Domain, h) {
				_ = proto.NetworkDeleteCookies{Name: c.Name, Domain: c.Domain, Path: c.Path}.Call(tab)
			}
		}
		_, _ = db.Exec(`DELETE FROM katana_window_forget WHERE host = ?`, h)
	}
}

// cookieDomainIs: the cookie's domain is the host or a parent domain of it (".example.com" for "www.example.com"), so a site's cookies go
// whichever way they were set.
func cookieDomainIs(cookieDomain, host string) bool {
	d := strings.TrimPrefix(strings.ToLower(cookieDomain), ".")
	h := strings.ToLower(host)
	return d == h || strings.HasSuffix(h, "."+d)
}

func writeWindowJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// KatanaWindowHostsHandler handles GET /katana-window-hosts.
func KatanaWindowHostsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeWindowJSON(w, http.StatusOK, map[string]any{"hosts": listWindowHosts(), "windowOpen": katanaWindowOpen()})
}

// KatanaWindowHostHandler handles DELETE /katana-window-host?host=...
func KatanaWindowHostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	host := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("host")))
	if host == "" || strings.ContainsAny(host, "/ ?#@") {
		http.Error(w, "host is required (a bare host name)", http.StatusBadRequest)
		return
	}
	forgetWindowHost(host)
	writeWindowJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// KatanaWindowOpenHandler handles POST /katana-window-open {"url": "..."}.
func KatanaWindowOpenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		URL string `json:"url"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || strings.TrimSpace(body.URL) == "" {
		http.Error(w, "url is required (and only url is accepted)", http.StatusBadRequest)
		return
	}
	if err := validateCrawlURL(body.URL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	host, err := hostnameOf(body.URL)
	if err != nil || host == "" {
		http.Error(w, "the address has no host", http.StatusBadRequest)
		return
	}
	waiting := beginWindowRequest(host, body.URL)
	writeWindowJSON(w, http.StatusOK, map[string]any{"ok": true, "alreadyOpen": waiting, "at": time.Now().UTC().Format(time.RFC3339)})
}
