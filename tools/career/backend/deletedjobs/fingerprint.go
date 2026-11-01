// Package deletedjobs remembers the jobs that were deleted, so a crawl does not add them again (step 133). A deleted
// job is recognized by the hash of its normalized external URL: if the hashes match, the job counts as deleted. This
// file is the fingerprint, pure functions with no database. See plan/ai/tools/career/step-133-deleted-jobs-filter.md.
package deletedjobs

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"path"
	"sort"
	"strings"
)

// KeyVersion numbers the normalization rules below. Every remembered job stores the version its hash was made with;
// when the rules change this number goes up and the stored hashes are recomputed from the readable snapshots.
const KeyVersion = 1

// urlPrefix names the kind of key, so a URL hash can never equal a hash made from other text.
const urlPrefix = "url:"

// trackingParams are query parameters that say where a visitor came from, not which page it is. A parameter whose name
// starts with one of trackingPrefixes is dropped too.
var trackingParams = map[string]bool{
	"gclid": true, "fbclid": true, "msclkid": true,
	"ref": true, "referrer": true, "source": true, "src": true, "trk": true, "campaign": true,
}

var trackingPrefixes = []string{"utm_", "mc_"}

func isTracking(name string) bool {
	if trackingParams[name] {
		return true
	}
	for _, p := range trackingPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// NormalizeURL writes an external URL in one canonical form, so the same page written slightly differently gives the
// same text: no scheme (http and https are the same page), the host lower-cased without a leading "www." or a trailing
// dot, the port dropped when it is the scheme's default, no user info or fragment, the path decoded, cleaned of dot
// segments, doubled and trailing slashes, tracking parameters dropped, the remaining parameters sorted, everything
// lower-cased. A URL that is not a web address (no host, another scheme, unparsable) cannot be taken apart, so it is
// only trimmed, stripped of its fragment and lower-cased, and marked "raw:" so it never equals a web address. It
// returns "" for a blank input.
func NormalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return rawForm(raw)
	}
	scheme := strings.ToLower(u.Scheme)
	if u.Hostname() == "" || (scheme != "http" && scheme != "https") {
		return rawForm(raw)
	}

	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	host = strings.TrimPrefix(host, "www.")
	if host == "" {
		return rawForm(raw)
	}
	if strings.Contains(host, ":") { // an IPv6 address keeps its brackets
		host = "[" + host + "]"
	}
	if port := u.Port(); port != "" && !(scheme == "http" && port == "80") && !(scheme == "https" && port == "443") {
		host += ":" + port
	}

	p := path.Clean("/" + strings.ToLower(u.Path))
	if p == "/" {
		p = ""
	}

	out := host + p
	if q := normalizeQuery(u.RawQuery); q != "" {
		out += "?" + q
	}
	return out
}

// normalizeQuery drops the tracking parameters and sorts the rest. A query that cannot be parsed is kept whole
// (lower-cased) rather than guessed at, so two different malformed queries never become equal.
func normalizeQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return strings.ToLower(rawQuery)
	}
	var pairs []string
	for name, vs := range values {
		name = strings.ToLower(name)
		if isTracking(name) {
			continue
		}
		for _, v := range vs {
			pairs = append(pairs, name+"="+strings.ToLower(v))
		}
	}
	sort.Strings(pairs)
	return strings.Join(pairs, "&")
}

func rawForm(raw string) string {
	if i := strings.Index(raw, "#"); i >= 0 {
		raw = raw[:i]
	}
	return "raw:" + strings.ToLower(strings.TrimSpace(raw))
}

// URLHash is the key a deleted job is remembered by: the SHA-256 (hex) of the normalized URL, with a prefix naming
// the kind of key. It returns "" for a blank URL, which has no key.
func URLHash(raw string) string {
	n := NormalizeURL(raw)
	if n == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(urlPrefix + n))
	return hex.EncodeToString(sum[:])
}
