// Package linkrule is the pure core of Career's "link crawler" (plan 185): jobs
// found by the URL pattern of their links, without AI.
//
// It does no I/O. It turns the URLs a person picked into a partial-URL
// pattern, matches the links of a page against such patterns, and chooses the
// job title of each matching link from the link texts the Browser tool
// reported. Nothing here fetches, stores or logs anything.
//
// Patterns are globs, never regular expressions (a user-supplied regex is a
// ReDoS risk): the matcher is a table walk bounded by MaxPatternLen and
// MaxURLLen, so its cost cannot explode.
//
//	host/path?query
//
//	duapune.com/jobs/*            one path segment after /jobs/
//	example.com/jobs/#            /jobs/ followed by a number (8195), not by a word (types)
//	duapune.com/jobs/#-*          a number, a dash and a slug (121888-backend-developer)
//	example.com/job-*/apply       a segment that starts with "job-"
//	example.com/careers/**        anything below /careers/
//	example.com/view?id=#         the path /view with a numeric id parameter
//
// `*` matches any characters inside ONE path segment (never `/`) or inside one
// query value; `**` matches anything, `/` included; `#` matches a number: one
// or more digits. The scheme is ignored
// (http and https both match). A trailing `/` is ignored. The host is literal
// (no wildcard: a rule belongs to one host). A pattern without `?` ignores the
// URL's query (job links often carry tracking parameters); a pattern with `?`
// requires each of its parameters, in any order, and ignores the others.
// Paths are case-sensitive, hosts are not.
package linkrule

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The bounds that keep a rule small and the matcher cheap.
const (
	MaxPatternLen = 300
	MaxURLLen     = 2048
	MaxIncludes   = 20
	MaxExcludes   = 20
)

var (
	ErrEmptyPattern        = errors.New("the pattern is empty")
	ErrPatternTooLong      = fmt.Errorf("the pattern is longer than %d characters", MaxPatternLen)
	ErrPatternNoHost       = errors.New("the pattern must start with the host, for example example.com/jobs/*")
	ErrPatternHostWildcard = errors.New("the host of a pattern cannot contain *: a rule belongs to one host")
	ErrBadPatternChars     = errors.New("the pattern contains a space or a control character, which no link address can match")
	ErrBadQueryPattern     = errors.New("a query part of the pattern must look like key or key=value (the key cannot contain *)")
)

// ---- URLs ----

// NormalizeURL makes the links of one page comparable: http(s) only, no user
// info, lower-case scheme and host, no fragment, a bare host gets "/". ok is
// false for anything else (javascript:, mailto:, an empty or unparsable
// address, a URL with credentials). It is the same normalisation the Browser
// tool applies to the list it returns, repeated here because the two tools
// are separate programs.
func NormalizeURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil {
		return "", false
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false
	}
	u.Host = strings.ToLower(u.Host)
	u.Fragment, u.RawFragment = "", ""
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), true
}

// parsedURL is a normalised URL in the form matching works on.
type parsedURL struct {
	norm  string     // the normalised URL
	host  string     // lower-case host[:port]
	path  string     // escaped path without a trailing "/" ("" for the root)
	query url.Values // decoded query
}

func parseURL(raw string) (parsedURL, bool) {
	norm, ok := NormalizeURL(raw)
	if !ok || len(norm) > MaxURLLen {
		return parsedURL{}, false
	}
	u, err := url.Parse(norm)
	if err != nil {
		return parsedURL{}, false
	}
	path := u.EscapedPath()
	if path != "/" {
		path = strings.TrimRight(path, "/")
	} else {
		path = ""
	}
	return parsedURL{norm: norm, host: u.Host, path: path, query: u.Query()}, true
}

// segments splits the path into its segments ("" path = none).
func (p parsedURL) segments() []string {
	if p.path == "" {
		return nil
	}
	return strings.Split(strings.TrimPrefix(p.path, "/"), "/")
}

// ---- patterns ----

type tokKind byte

const (
	tokLit    tokKind = 'l' // one literal character
	tokStar   tokKind = 's' // *  : any characters within one segment / value
	tokDbl    tokKind = 'd' // ** : any characters at all
	tokDigits tokKind = 'n' // #  : one or more digits
)

type token struct {
	kind tokKind
	r    rune
}

type queryReq struct {
	key    string
	value  []token // nil: the key only has to be present
	hasVal bool
}

// Pattern is a compiled pattern. The zero value matches nothing.
type Pattern struct {
	src        string
	host       string
	pathTokens []token
	queryReqs  []queryReq
}

// String is the pattern as it was compiled (trimmed, scheme removed).
func (p *Pattern) String() string { return p.src }

// Host is the literal host the pattern belongs to.
func (p *Pattern) Host() string { return p.host }

// escapeLiteral writes characters that are not plain ASCII the way
// net/url writes them in a path, so a pattern typed with "ü" matches the
// normalised URL.
func escapeLiteral(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < utf8.RuneSelf {
			b.WriteRune(r)
			continue
		}
		var buf [utf8.UTFMax]byte
		n := utf8.EncodeRune(buf[:], r)
		for _, c := range buf[:n] {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func tokenize(s string) []token {
	var out []token
	rs := []rune(s)
	for i := 0; i < len(rs); {
		if rs[i] == '#' {
			out = append(out, token{kind: tokDigits})
			i++
			continue
		}
		if rs[i] != '*' {
			out = append(out, token{kind: tokLit, r: rs[i]})
			i++
			continue
		}
		j := i
		for j < len(rs) && rs[j] == '*' {
			j++
		}
		if j-i >= 2 {
			out = append(out, token{kind: tokDbl})
		} else {
			out = append(out, token{kind: tokStar})
		}
		i = j
	}
	return out
}

// Compile validates and compiles one pattern (see the package comment).
func Compile(src string) (*Pattern, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, ErrEmptyPattern
	}
	if utf8.RuneCountInString(src) > MaxPatternLen {
		return nil, ErrPatternTooLong
	}
	for _, r := range src {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return nil, ErrBadPatternChars
		}
	}
	lower := strings.ToLower(src)
	switch {
	case strings.HasPrefix(lower, "https://"):
		src = src[len("https://"):]
	case strings.HasPrefix(lower, "http://"):
		src = src[len("http://"):]
	}
	hostPath, query, hasQuery := strings.Cut(src, "?")
	host, path, _ := strings.Cut(hostPath, "/")
	host = strings.ToLower(host)
	if host == "" {
		return nil, ErrPatternNoHost
	}
	if strings.Contains(host, "*") {
		return nil, ErrPatternHostWildcard
	}
	path = "/" + path
	if path != "/" {
		path = strings.TrimRight(path, "/")
	} else {
		path = "" // the root: a pattern for the bare host
	}
	p := &Pattern{src: src, host: host}
	p.pathTokens = tokenize(escapeLiteral(path))
	if hasQuery {
		for _, pair := range strings.Split(query, "&") {
			if pair == "" {
				continue
			}
			key, val, hasVal := strings.Cut(pair, "=")
			if key == "" || strings.Contains(key, "*") {
				return nil, ErrBadQueryPattern
			}
			q := queryReq{key: key, hasVal: hasVal}
			if hasVal {
				q.value = tokenize(val)
			}
			p.queryReqs = append(p.queryReqs, q)
		}
		if len(p.queryReqs) == 0 {
			return nil, ErrBadQueryPattern
		}
	}
	return p, nil
}

// glob walks the tokens over s once per token: cost O(len(tokens)*len(s)),
// bounded by MaxPatternLen*MaxURLLen. starStopsAtSlash: `*` does not consume "/".
func glob(tokens []token, s string, starStopsAtSlash bool) bool {
	rs := []rune(s)
	n := len(rs)
	prev := make([]bool, n+1)
	cur := make([]bool, n+1)
	prev[0] = true
	for _, t := range tokens {
		for i := range cur {
			cur[i] = false
		}
		switch t.kind {
		case tokLit:
			for i := 0; i < n; i++ {
				if prev[i] && rs[i] == t.r {
					cur[i+1] = true
				}
			}
		case tokDigits:
			for i := 1; i <= n; i++ {
				if rs[i-1] >= '0' && rs[i-1] <= '9' && (prev[i-1] || cur[i-1]) {
					cur[i] = true
				}
			}
		default: // tokStar, tokDbl
			for i := 0; i <= n; i++ {
				if prev[i] {
					cur[i] = true
				}
				if i > 0 && cur[i-1] && (t.kind == tokDbl || !starStopsAtSlash || rs[i-1] != '/') {
					cur[i] = true
				}
			}
		}
		prev, cur = cur, prev
	}
	return prev[n]
}

func (p *Pattern) matchParsed(u parsedURL) bool {
	if p == nil || p.host == "" || u.host != p.host {
		return false
	}
	if !glob(p.pathTokens, u.path, true) {
		return false
	}
	for _, q := range p.queryReqs {
		vals, ok := u.query[q.key]
		if !ok {
			return false
		}
		if !q.hasVal {
			continue
		}
		found := false
		for _, v := range vals {
			if glob(q.value, v, false) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Match reports whether the link address matches the pattern. An address that
// is not a plain http(s) URL, or longer than MaxURLLen, never matches.
func (p *Pattern) Match(rawURL string) bool {
	u, ok := parseURL(rawURL)
	return ok && p.matchParsed(u)
}

// Broad reports whether the pattern takes almost every link of its site: no
// literal character in its path except the slashes, no number wildcard, and no
// query condition. "host/**", "host/*" and "host/*/*" are broad; "host/jobs/*"
// and "host/#" are not.
func (p *Pattern) Broad() bool {
	if p == nil || len(p.queryReqs) > 0 {
		return false
	}
	for _, t := range p.pathTokens {
		if (t.kind == tokLit && t.r != '/') || t.kind == tokDigits {
			return false
		}
	}
	return true
}
