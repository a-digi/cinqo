// Package toolmeta is the "who made it" information of a tool or a repository (authors, company, licence, links,
// copyright, category, keywords) and the one set of rules for it. The tool repository file and a tool's
// manifest.json both carry these fields, so both validate them here and can never accept different things. See
// plan/ai/tools/repository/tool-metadata.md.
//
// Everything here is plain text for a person to read. Core never fetches an address and never sends mail; an
// address is only ever shown as a link, which is why only https (and mailto: for a security contact) is accepted.
package toolmeta

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

const (
	MaxAuthors      = 20
	MaxNameLen      = 100
	MaxEmailLen     = 254
	MaxURLLen       = 2000
	MaxLicenseLen   = 100
	MaxCopyrightLen = 200
	MaxKeywords     = 10
	MaxKeywordLen   = 30

	// Proprietary is the licence value for a tool that is not under an open licence.
	Proprietary = "Proprietary"
)

// Categories is the fixed list a tool's category comes from.
var Categories = []string{"productivity", "data", "ai", "browser", "documents", "developer", "other"}

var (
	emailPattern   = regexp.MustCompile(`^[^@\s<>"]+@[^@\s<>"]+\.[^@\s<>"]+$`)
	licensePattern = regexp.MustCompile(`^[A-Za-z0-9.+-]+( (AND|OR|WITH) [A-Za-z0-9.+-]+)*$`)
	keywordPattern = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N} -]*$`)
)

// Author is one person who made a tool or maintains a repository. Only the name is required.
type Author struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	URL   string `json:"url,omitempty"`
}

// Company is the organisation behind a tool or a repository.
type Company struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

// Links are the addresses around a tool or a repository; every one is optional and an https address. Security
// (where to report a vulnerability) may also be a mailto: address.
type Links struct {
	Homepage      string `json:"homepage,omitempty"`
	Documentation string `json:"documentation,omitempty"`
	Source        string `json:"source,omitempty"`
	Issues        string `json:"issues,omitempty"`
	Support       string `json:"support,omitempty"`
	Security      string `json:"security,omitempty"`
}

// Info is the attribution of one tool version. A struct embeds it (anonymously) so its fields sit at the top level
// of the JSON object: in a repository file's version entry and in a tool's manifest.json alike.
type Info struct {
	Authors   []Author `json:"authors,omitempty"`
	Company   *Company `json:"company,omitempty"`
	License   string   `json:"license,omitempty"`
	Links     *Links   `json:"links,omitempty"`
	Copyright string   `json:"copyright,omitempty"`
	Category  string   `json:"category,omitempty"`
	Keywords  []string `json:"keywords,omitempty"`
}

// Problem is one thing wrong with a field. Path locates it, for example `tools[2].versions[0].authors[1].name`.
type Problem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (p Problem) String() string {
	if p.Path == "" {
		return p.Message
	}
	return p.Path + ": " + p.Message
}

type collector struct {
	prefix string
	probs  []Problem
}

func (c *collector) add(field, format string, args ...any) {
	path := field
	if c.prefix != "" {
		path = c.prefix + "." + field
	}
	c.probs = append(c.probs, Problem{Path: path, Message: fmt.Sprintf(format, args...)})
}

// Validate checks the attribution of one tool version. path locates it ("" for a manifest, "tools[0].versions[1]"
// in a repository file). With required set, authors and a licence must be present (a repository file of format
// version 2, and every manifest at install); otherwise they are only checked when they are there.
func (i Info) Validate(path string, required bool) []Problem {
	c := &collector{prefix: path}
	c.authors("authors", i.Authors, required)
	c.company("company", i.Company)
	c.license("license", i.License, required)
	c.links("links", i.Links)
	if msg := text(i.Copyright, MaxCopyrightLen); msg != "" {
		c.add("copyright", "%s", msg)
	}
	c.category("category", i.Category)
	c.keywords("keywords", i.Keywords)
	return c.probs
}

// ValidateAuthors, ValidateCompany and ValidateLinks check the pieces a repository file carries at its top level.
// path is the path of the object that holds them ("" for the top level), the same as for Info.Validate: the
// problems come back as "authors[0].name", "company.url", "links.security".
func ValidateAuthors(path string, authors []Author, required bool) []Problem {
	c := &collector{prefix: path}
	c.authors("authors", authors, required)
	return c.probs
}

func ValidateCompany(path string, company *Company) []Problem {
	c := &collector{prefix: path}
	c.company("company", company)
	return c.probs
}

func ValidateLinks(path string, links *Links) []Problem {
	c := &collector{prefix: path}
	c.links("links", links)
	return c.probs
}

// text says what is wrong with a short text, or "": it must not be longer than max characters and must not hold
// control characters or the characters that reorder text (used to make a name look like another one).
func text(value string, max int) string {
	if len([]rune(value)) > max {
		return fmt.Sprintf("is longer than %d characters", max)
	}
	for _, r := range value {
		if unicode.IsControl(r) || (r >= '\u202a' && r <= '\u202e') || (r >= '\u2066' && r <= '\u2069') || r == '\u2028' || r == '\u2029' {
			return "must not contain control or text-direction characters"
		}
	}
	return ""
}

func (c *collector) name(field, value string) {
	if strings.TrimSpace(value) == "" {
		c.add(field, "is required")
		return
	}
	if msg := text(value, MaxNameLen); msg != "" {
		c.add(field, "%s", msg)
	}
}

func (c *collector) authors(field string, authors []Author, required bool) {
	switch {
	case len(authors) == 0 && required:
		c.add(field, "is required: name at least one author")
		return
	case len(authors) > MaxAuthors:
		c.add(field, "has %d entries, at most %d are allowed", len(authors), MaxAuthors)
		return
	}
	for i, a := range authors {
		p := fmt.Sprintf("%s[%d]", field, i)
		c.name(p+".name", a.Name)
		if a.Email != "" && !validEmail(a.Email) {
			c.add(p+".email", "must look like name@example.com (at most %d characters)", MaxEmailLen)
		}
		if a.URL != "" {
			if msg := validURL(a.URL, false); msg != "" {
				c.add(p+".url", "%s", msg)
			}
		}
	}
}

func (c *collector) company(field string, company *Company) {
	if company == nil {
		return
	}
	c.name(field+".name", company.Name)
	if company.URL != "" {
		if msg := validURL(company.URL, false); msg != "" {
			c.add(field+".url", "%s", msg)
		}
	}
}

func (c *collector) links(field string, links *Links) {
	if links == nil {
		return
	}
	for _, l := range []struct {
		key, value string
		mailto     bool
	}{
		{"homepage", links.Homepage, false}, {"documentation", links.Documentation, false}, {"source", links.Source, false},
		{"issues", links.Issues, false}, {"support", links.Support, false}, {"security", links.Security, true},
	} {
		if l.value == "" {
			continue
		}
		if msg := validURL(l.value, l.mailto); msg != "" {
			c.add(field+"."+l.key, "%s", msg)
		}
	}
}

func (c *collector) license(field, value string, required bool) {
	switch {
	case value == "" && required:
		c.add(field, "is required (an SPDX identifier such as MIT or Apache-2.0, or %s)", Proprietary)
	case value == "":
	case len(value) > MaxLicenseLen:
		c.add(field, "is longer than %d characters", MaxLicenseLen)
	case value != Proprietary && !licensePattern.MatchString(value):
		c.add(field, "must be an SPDX identifier or expression (MIT, Apache-2.0, MIT OR Apache-2.0) or %s", Proprietary)
	}
}

func (c *collector) category(field, value string) {
	if value == "" {
		return
	}
	for _, known := range Categories {
		if value == known {
			return
		}
	}
	c.add(field, "must be one of %s", strings.Join(Categories, ", "))
}

func (c *collector) keywords(field string, keywords []string) {
	if len(keywords) > MaxKeywords {
		c.add(field, "has %d entries, at most %d are allowed", len(keywords), MaxKeywords)
		return
	}
	seen := map[string]bool{}
	for i, k := range keywords {
		p := fmt.Sprintf("%s[%d]", field, i)
		switch {
		case len([]rune(k)) > MaxKeywordLen || !keywordPattern.MatchString(k):
			c.add(p, "must be 1-%d letters, digits, spaces or dashes, starting with a letter or digit", MaxKeywordLen)
		case seen[strings.ToLower(k)]:
			c.add(p, "duplicate keyword %q", k)
		default:
			seen[strings.ToLower(k)] = true
		}
	}
}

func validEmail(s string) bool { return len(s) <= MaxEmailLen && emailPattern.MatchString(s) }

// validURL says what is wrong with an address shown as a link, or "" when it is fine: https only, a host, no
// credentials, no spaces or control characters, a bounded length. mailto: (a plain address) is also accepted when
// allowMailto is set.
func validURL(raw string, allowMailto bool) string {
	if len(raw) > MaxURLLen {
		return fmt.Sprintf("is longer than %d characters", MaxURLLen)
	}
	if strings.IndexFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return "must not contain spaces or control characters"
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "is not a valid address"
	}
	if allowMailto && u.Scheme == "mailto" {
		if u.Opaque == "" || u.RawQuery != "" || !validEmail(u.Opaque) {
			return "must look like mailto:name@example.com"
		}
		return ""
	}
	switch {
	case u.Scheme != "https":
		if allowMailto {
			return "must be an https address or mailto:name@example.com"
		}
		return "must be an https address"
	case u.Hostname() == "":
		return "must have a host"
	case u.User != nil:
		return "must not contain credentials"
	}
	return ""
}

// IsZero reports whether nothing at all is stated.
func (i Info) IsZero() bool {
	return len(i.Authors) == 0 && i.Company == nil && i.License == "" && i.Links == nil && i.Copyright == "" && i.Category == "" && len(i.Keywords) == 0
}

// Encode is how an installed tool's validated attribution is stored (one JSON document in tools.metadata).
func Encode(i Info) (string, error) {
	raw, err := json.Marshal(i)
	return string(raw), err
}

// Decode reads what Encode wrote. A tool installed before the column existed has nothing stored, and a row that
// cannot be read is treated the same: both come back nil, which the pages show as "developer not stated"; neither
// may ever break a listing.
func Decode(raw string) *Info {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var i Info
	if err := json.Unmarshal([]byte(raw), &i); err != nil || i.IsZero() {
		return nil
	}
	return &i
}

// Publisher is the attribution of a repository as a whole: who maintains it, and when its file was written.
type Publisher struct {
	Authors     []Author `json:"authors"`
	Company     *Company `json:"company,omitempty"`
	Links       *Links   `json:"links,omitempty"`
	GeneratedAt string   `json:"generatedAt,omitempty"`
}

// Mismatch compares what a repository says about a version with what the downloaded package says about itself,
// and describes the first disagreement ("" when they agree). Only what the repository states is compared: a
// repository of the older format states nothing and so can never disagree. It catches a hand-edited file (the
// generator copies the manifest), it is not a security check: the file and the package come from one publisher.
func Mismatch(claimed, actual Info) string {
	if len(claimed.Authors) > 0 && !sameNames(claimed.Authors, actual.Authors) {
		return fmt.Sprintf("the repository lists the authors as %s but the package says %s", names(claimed.Authors), names(actual.Authors))
	}
	if claimed.License != "" && claimed.License != actual.License {
		return fmt.Sprintf("the repository lists the license as %q but the package says %q", claimed.License, actual.License)
	}
	return ""
}

func sameNames(a, b []Author) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name {
			return false
		}
	}
	return true
}

func names(authors []Author) string {
	if len(authors) == 0 {
		return "nobody"
	}
	parts := make([]string, len(authors))
	for i, a := range authors {
		parts[i] = a.Name
	}
	return strings.Join(parts, ", ")
}
