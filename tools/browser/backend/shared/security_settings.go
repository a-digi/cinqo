// security_settings.go (plan 271-style gate for the Namako tool, plan/ai/tools/namako/step-01) — the two switches that widen what the Browser
// tool may reach or write, kept apart from BrowserSettings on purpose:
//
//   - AllowedTestHosts: exact host names that may resolve to a LOOPBACK address, so a test site on this computer can be crawled (the crawl
//     normally refuses every loopback/private target). Default empty. No wildcards, no IP literals, no ranges; only loopback is let through,
//     never a private or link-local address.
//   - AllowMediaDownloads: whether POST /fetch-media may write media files to the temporary folder at all. Default off.
//
// Its own route (GET/PUT /security-settings) carries its own, stricter scope than /browser-settings, because a route has one scope: a client of
// the ordinary settings page can never change these.
package shared

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
)

// MaxTestHosts caps the list so it stays a short, reviewable list.
const MaxTestHosts = 20

// SecuritySettings is the stored object (and the JSON of the route).
type SecuritySettings struct {
	AllowedTestHosts    []string `json:"allowedTestHosts"`
	AllowMediaDownloads bool     `json:"allowMediaDownloads"`
}

// ErrInvalidTestHost is a caller mistake (a 400).
var ErrInvalidTestHost = errors.New("a test host must be one exact host name (letters, digits, hyphens and dots; no wildcard, no IP address, no port, no path)")

// ValidTestHost reports whether h is exactly one DNS host name (RFC 1123 labels, lower case) that is NOT an IP literal and has no wildcard.
func ValidTestHost(h string) bool {
	if h == "" || len(h) > 253 || h != strings.ToLower(h) || net.ParseIP(h) != nil || strings.HasPrefix(h, ".") || strings.HasSuffix(h, ".") {
		return false
	}
	labels := strings.Split(h, ".")
	// A name whose last label is all digits or looks hexadecimal ("0x7f.1") can be read by a system resolver as a numeric IP address.
	if last := labels[len(labels)-1]; strings.HasPrefix(last, "0x") || strings.Trim(last, "0123456789") == "" {
		return false
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

// NormalizeTestHosts trims and lower-cases each entry, drops duplicates and sorts. Any invalid entry (or more than MaxTestHosts) refuses the whole
// list: nothing is half-accepted.
func NormalizeTestHosts(in []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, raw := range in {
		h := strings.ToLower(strings.TrimSpace(raw))
		if !ValidTestHost(h) {
			return nil, fmt.Errorf("%w: %q", ErrInvalidTestHost, raw)
		}
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	if len(out) > MaxTestHosts {
		return nil, fmt.Errorf("at most %d test hosts are allowed", MaxTestHosts)
	}
	sort.Strings(out)
	return out, nil
}

func ensureSecuritySettingsTable(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS browser_security_settings (
		id                    INTEGER PRIMARY KEY CHECK (id = 1),
		allowed_test_hosts    TEXT NOT NULL DEFAULT '[]',
		allow_media_downloads INTEGER NOT NULL DEFAULT 0,
		updated_at            TEXT
	)`); err != nil {
		return err
	}
	_, err := db.Exec(`INSERT OR IGNORE INTO browser_security_settings (id, allowed_test_hosts, allow_media_downloads) VALUES (1, '[]', 0)`)
	return err
}

// LoadSecuritySettings reads fresh every time (a live switch). With no database (a test, a failed start) it returns the safe defaults and the error.
func LoadSecuritySettings() (SecuritySettings, error) {
	def := SecuritySettings{AllowedTestHosts: []string{}}
	if DB == nil {
		return def, errors.New("no database")
	}
	var hosts string
	var media int
	if err := DB.QueryRow(`SELECT allowed_test_hosts, allow_media_downloads FROM browser_security_settings WHERE id = 1`).Scan(&hosts, &media); err != nil {
		return def, err
	}
	var list []string
	if err := json.Unmarshal([]byte(hosts), &list); err != nil {
		return def, err
	}
	// Read through the same allow-list the writer uses: a hand-edited row can never widen anything.
	clean, err := NormalizeTestHosts(list)
	if err != nil {
		return def, err
	}
	return SecuritySettings{AllowedTestHosts: clean, AllowMediaDownloads: media != 0}, nil
}

// SaveSecuritySettings validates and stores both fields.
func SaveSecuritySettings(s SecuritySettings) error {
	clean, err := NormalizeTestHosts(s.AllowedTestHosts)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(clean)
	media := 0
	if s.AllowMediaDownloads {
		media = 1
	}
	_, err = DB.Exec(`UPDATE browser_security_settings SET allowed_test_hosts = ?, allow_media_downloads = ?, updated_at = datetime('now') WHERE id = 1`, string(raw), media)
	return err
}

// TestHostAllowed reports whether host (any case) is exactly one of the allowed test hosts. Never true on any error.
func TestHostAllowed(host string) bool {
	s, err := LoadSecuritySettings()
	if err != nil {
		return false
	}
	h := strings.ToLower(host)
	for _, a := range s.AllowedTestHosts {
		if a == h {
			return true
		}
	}
	return false
}

// MediaDownloadsAllowed reports the stored switch; false on any error.
func MediaDownloadsAllowed() bool {
	s, err := LoadSecuritySettings()
	return err == nil && s.AllowMediaDownloads
}

// SecuritySettingsHandler handles GET/PUT /security-settings. The PUT must carry both fields and nothing else (a missing field would otherwise
// silently switch something off or on).
func SecuritySettingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s, err := LoadSecuritySettings()
		if err != nil {
			http.Error(w, "failed to load settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeSecuritySettings(w, s)
	case http.MethodPut:
		var in struct {
			AllowedTestHosts    *[]string `json:"allowedTestHosts"`
			AllowMediaDownloads *bool     `json:"allowMediaDownloads"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil || in.AllowedTestHosts == nil || in.AllowMediaDownloads == nil {
			http.Error(w, "invalid request body: allowedTestHosts and allowMediaDownloads are both required", http.StatusBadRequest)
			return
		}
		next := SecuritySettings{AllowedTestHosts: *in.AllowedTestHosts, AllowMediaDownloads: *in.AllowMediaDownloads}
		if err := SaveSecuritySettings(next); err != nil {
			if errors.Is(err, ErrInvalidTestHost) || strings.Contains(err.Error(), "at most") {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, "failed to save settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
		saved, err := LoadSecuritySettings()
		if err != nil {
			http.Error(w, "failed to reload settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeSecuritySettings(w, saved)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeSecuritySettings(w http.ResponseWriter, s SecuritySettings) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s)
}
