package linkrule

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrNoHost       = errors.New("the rule has no host")
	ErrNoInclude    = errors.New("the rule needs at least one pattern for the job links")
	ErrTooManyRules = fmt.Errorf("a rule can have at most %d include and %d exclude patterns", MaxIncludes, MaxExcludes)
	ErrHostMismatch = errors.New("every pattern of a rule must be for the rule's own host")
	// ErrBadJobHost: the rule's job host is the rule's own host or not a plain host name.
	ErrBadJobHost = errors.New("the job host of a rule must be one other plain host name")
)

// Rule is what a person's picks became: the host the links belong to, the
// patterns that say "this link is a job", and patterns that say "this one is
// not" (exclusions win over inclusions).
type Rule struct {
	Host string `json:"host"`
	// JobHost (plan 203) is the ONE other host the job links may be on, when the person confirmed it for the link: the page is on Host and
	// its postings on JobHost. Include and exclude patterns may be for either host; the pagination (the pages of the list) stays on Host.
	// Empty (the default and what every rule had before) means every pattern is for Host.
	JobHost string   `json:"jobHost,omitempty"`
	Include []string `json:"include"`
	Exclude []string `json:"exclude,omitempty"`
	// Pagination says where the pages after the first are (nil = one page).
	Pagination *Pagination `json:"pagination,omitempty"`
}

// Item is one job found by a link: its title and its address.
type Item struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Result is what a rule found on one page.
type Result struct {
	// Items are the jobs, in page order, one per distinct URL.
	Items []Item `json:"items"`
	// Matched counts the distinct matching URLs; SkippedNoTitle those without a usable title (not invented).
	Matched        int `json:"matched"`
	SkippedNoTitle int `json:"skippedNoTitle"`
	// SkippedSamples are up to 10 of the skipped URLs, for the person to look at.
	SkippedSamples []string `json:"skippedSamples,omitempty"`
	// OtherHost counts links ignored because they belong to another host than the rule's.
	OtherHost int `json:"otherHost"`
}

type compiledRule struct {
	host string
	// hosts is Host and JobHost: the hosts a job link may be on.
	hosts   map[string]bool
	include []*Pattern
	exclude []*Pattern
	paging  compiledPagination
}

func compileRule(r Rule) (compiledRule, error) {
	host := strings.ToLower(strings.TrimSpace(r.Host))
	if host == "" {
		return compiledRule{}, ErrNoHost
	}
	if strings.Contains(host, "*") || strings.ContainsAny(host, "/ ?#") {
		return compiledRule{}, fmt.Errorf("%w: %q", ErrNoHost, r.Host)
	}
	if len(r.Include) == 0 {
		return compiledRule{}, ErrNoInclude
	}
	if len(r.Include) > MaxIncludes || len(r.Exclude) > MaxExcludes {
		return compiledRule{}, ErrTooManyRules
	}
	jobHost := strings.ToLower(strings.TrimSpace(r.JobHost))
	if jobHost != "" && (jobHost == host || strings.Contains(jobHost, "*") || strings.ContainsAny(jobHost, "/ ?#")) {
		return compiledRule{}, fmt.Errorf("%w: %q", ErrBadJobHost, r.JobHost)
	}
	c := compiledRule{host: host, hosts: map[string]bool{host: true}}
	if jobHost != "" {
		c.hosts[jobHost] = true
	}
	for _, src := range r.Include {
		p, err := Compile(src)
		if err != nil {
			return compiledRule{}, fmt.Errorf("pattern %q: %w", truncate(src, 80), err)
		}
		if !c.hosts[p.host] {
			return compiledRule{}, fmt.Errorf("%w: %q is not for %s", ErrHostMismatch, truncate(src, 80), hostsText(host, jobHost))
		}
		c.include = append(c.include, p)
	}
	for _, src := range r.Exclude {
		p, err := Compile(src)
		if err != nil {
			return compiledRule{}, fmt.Errorf("exclude pattern %q: %w", truncate(src, 80), err)
		}
		if !c.hosts[p.host] {
			return compiledRule{}, fmt.Errorf("%w: %q is not for %s", ErrHostMismatch, truncate(src, 80), hostsText(host, jobHost))
		}
		c.exclude = append(c.exclude, p)
	}
	pg, err := validatePagination(r.Pagination, host)
	if err != nil {
		return compiledRule{}, err
	}
	c.paging = pg
	return c, nil
}

// hostsText names the host or the two hosts a rule's patterns may be for.
func hostsText(host, jobHost string) string {
	if jobHost == "" {
		return host
	}
	return host + " or " + jobHost
}

// ValidateRule checks a rule the way saving it will: a host, 1 to MaxIncludes
// include patterns and at most MaxExcludes exclude patterns, each valid and
// each for the rule's own host.
func ValidateRule(r Rule) error {
	_, err := compileRule(r)
	return err
}

// hasLetterOrDigit: an icon, a bullet or an arrow is not a title.
func hasLetterOrDigit(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func foldText(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// GenericTexts finds link texts that are not job titles ("More", "Apply",
// "Log in"): a text is generic when it is used by three or more different
// URLs of the page AND it lost against a longer text on at least one URL that
// has two (so a job title that really repeats, with one text per URL, is
// kept). The map is keyed by the folded text.
func GenericTexts(links []Link) map[string]bool {
	urlsOf := map[string]map[string]bool{}
	lost := map[string]bool{}
	for _, l := range links {
		norm, ok := NormalizeURL(l.URL)
		if !ok {
			continue
		}
		longest := 0
		for _, t := range l.Texts {
			if n := utf8.RuneCountInString(strings.Join(strings.Fields(t), " ")); n > longest {
				longest = n
			}
		}
		for _, t := range l.Texts {
			f := foldText(t)
			if f == "" {
				continue
			}
			if urlsOf[f] == nil {
				urlsOf[f] = map[string]bool{}
			}
			urlsOf[f][norm] = true
			if len(l.Texts) > 1 && utf8.RuneCountInString(f) < longest {
				lost[f] = true
			}
		}
	}
	generic := map[string]bool{}
	for f, urls := range urlsOf {
		if len(urls) >= 3 && lost[f] {
			generic[f] = true
		}
	}
	return generic
}

// PickTitle chooses the title of one link: the longest of its texts that is
// not generic and holds a letter or digit; else its title attribute under the
// same conditions; else "" (the caller skips the item, nothing is invented).
func PickTitle(l Link, generic map[string]bool) string {
	best, bestLen := "", 0
	for _, t := range l.Texts {
		t = strings.Join(strings.Fields(t), " ")
		if t == "" || generic[foldText(t)] || !hasLetterOrDigit(t) {
			continue
		}
		if n := utf8.RuneCountInString(t); n > bestLen {
			best, bestLen = t, n
		}
	}
	if best != "" {
		return best
	}
	t := strings.Join(strings.Fields(l.TitleAttr), " ")
	if t != "" && !generic[foldText(t)] && hasLetterOrDigit(t) {
		return t
	}
	return ""
}

// applyLinks runs the compiled rule over the links of one page, adding what it finds to res. seen holds the addresses already looked
// at (on this or an earlier page of the run) so one address makes one item; it returns how many matching addresses were NEW.
func (c compiledRule) applyLinks(links []Link, seen map[string]bool, res *Result) int {
	generic := GenericTexts(links)
	newMatched := 0
	for _, l := range links {
		u, ok := parseURL(l.URL)
		if !ok || seen[u.norm] {
			continue
		}
		seen[u.norm] = true
		if !c.hosts[u.host] {
			res.OtherHost++
			continue
		}
		if !anyMatch(c.include, u) || anyMatch(c.exclude, u) {
			continue
		}
		res.Matched++
		newMatched++
		title := PickTitle(l, generic)
		if title == "" {
			res.SkippedNoTitle++
			if len(res.SkippedSamples) < 10 {
				res.SkippedSamples = append(res.SkippedSamples, u.norm)
			}
			continue
		}
		res.Items = append(res.Items, Item{Title: title, URL: u.norm})
	}
	return newMatched
}

// Apply runs a rule over the links of one page: the jobs it finds, in page
// order, one per distinct URL, each with its title. An invalid rule is an
// error (nothing is returned for it).
func Apply(r Rule, links []Link) (Result, error) {
	c, err := compileRule(r)
	if err != nil {
		return Result{}, err
	}
	res := Result{Items: []Item{}}
	c.applyLinks(links, map[string]bool{}, &res)
	return res, nil
}

// Preview is what a rule would do on a page, with the cautions the person
// must see before saving it.
type Preview struct {
	Result
	// NeedsConfirm: the rule is very broad or matches more than BroadMatchLimit links of this page.
	NeedsConfirm bool     `json:"needsConfirm"`
	Notes        []string `json:"notes,omitempty"`
}

// PreviewRule runs a rule over the links of a page (Apply) and adds the
// confirm flag and notes. An invalid rule is an error.
func PreviewRule(r Rule, links []Link) (Preview, error) {
	c, err := compileRule(r)
	if err != nil {
		return Preview{}, err
	}
	res, err := Apply(r, links)
	if err != nil {
		return Preview{}, err
	}
	pv := Preview{Result: res}
	for _, p := range c.include {
		if p.Broad() {
			pv.NeedsConfirm = true
			pv.Notes = append(pv.Notes, "A pattern is very broad: it matches almost every link of the site.")
			break
		}
	}
	if res.Matched > BroadMatchLimit {
		pv.NeedsConfirm = true
		pv.Notes = append(pv.Notes, fmt.Sprintf("The rule matches %d links on this page: confirm that they are all jobs.", res.Matched))
	}
	if res.SkippedNoTitle > 0 {
		pv.Notes = append(pv.Notes, fmt.Sprintf("%d matching link(s) have no usable text, so no job is created for them.", res.SkippedNoTitle))
	}
	return pv, nil
}
