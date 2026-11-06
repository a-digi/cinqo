package detailpick

import (
	"fmt"
	"strings"
)

// Notice says something about the loaded page itself (not about one field). It is advice for the screen: it never blocks anything.
type Notice struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

const (
	// NoticeJobHeadingMissing: the job's own title is in the page's <title> but in no element the Browser read, so every suggestion
	// comes from other parts of the page (a list of recommended jobs). Typical of a site that serves a reduced page to automated visits.
	NoticeJobHeadingMissing = "job_heading_missing"
	// NoticePageNotTheJob: the job's title is nowhere on the page, not even in its <title>.
	NoticePageNotTheJob = "page_not_the_job"
)

// sameJobTitle reports whether text says the job's title: the same words, one containing the other, or nearly all of the words of a
// longer title (a saved title can lack a suffix such as "(m/w/d)" that the page's heading has).
func sameJobTitle(text, jobTitle string) bool {
	if agrees(text, jobTitle) {
		return true
	}
	return len(words(text)) >= 3 && len(words(jobTitle)) >= 3 && overlap(text, jobTitle) >= 0.85
}

// titleOnPage reports whether the page shows the job's title anywhere the picker can offer it: a title candidate, a text element, the
// JobPosting JSON-LD, og:title or twitter:title. The page's <title> is NOT counted (that is the case NoticeJobHeadingMissing names).
func titleOnPage(p Page, jobTitle string) bool {
	for _, c := range p.Candidates.Title {
		if sameJobTitle(c.Value, jobTitle) {
			return true
		}
	}
	for _, e := range p.Elements {
		if sameJobTitle(e.Text, jobTitle) {
			return true
		}
	}
	for _, j := range p.Structured.JSONLD {
		if sameJobTitle(j.Title, jobTitle) {
			return true
		}
	}
	return sameJobTitle(p.Structured.Meta.OgTitle, jobTitle) || sameJobTitle(p.Structured.Meta.TwitterTitle, jobTitle)
}

func shortTitle(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 80 {
		return string(r[:80]) + "…"
	}
	return s
}

// PageNotices says what looks wrong with the loaded page, judged against the saved title of the job it was loaded for. jobTitle "" (a
// page loaded by address, or a title too short to judge) judges nothing. The list is never nil.
func PageNotices(p Page, jobTitle string) []Notice {
	out := []Notice{}
	jt := strings.TrimSpace(jobTitle)
	if runeLen(norm(jt)) < 6 || titleOnPage(p, jt) {
		return out
	}
	cut := ""
	if p.Truncated {
		cut = " The page is long: only its first elements were listed."
	}
	if sameJobTitle(p.Title, jt) {
		return append(out, Notice{Code: NoticeJobHeadingMissing, Message: fmt.Sprintf(
			"The title of this job (“%s”) appears only in the page's <title>, not in any heading or element the Browser read. What is suggested below comes from other parts of the page, often a list of other jobs. The site may show a reduced page to automated visits, so picking here may not work well. The page's <title> is offered for the title.%s",
			shortTitle(jt), cut)})
	}
	return append(out, Notice{Code: NoticePageNotTheJob, Message: fmt.Sprintf(
		"This page does not mention the job's title (“%s”). It may not be the job's own page (a redirect, a sign-in or a block page), so the suggestions below may have nothing to do with the job.%s",
		shortTitle(jt), cut)})
}

// documentTitleCandidate is the page's <title> as a place the job's title could be read from: sites that show no heading to an automated
// visit still state the title there. ok is false when the page has no <title> or a candidate already carries that text.
func documentTitleCandidate(p Page, existing []Candidate) (c Candidate, ok bool) {
	t := strings.Join(strings.Fields(p.Title), " ")
	if t == "" {
		return Candidate{}, false
	}
	if r := []rune(t); len(r) > 200 {
		t = string(r[:200])
	}
	for _, e := range existing {
		if norm(e.Value) == norm(t) {
			return Candidate{}, false
		}
	}
	return Candidate{Selector: "title", Unique: true, Matches: 1, Value: t, Chars: runeLen(t), Tag: "title", Reason: "the page's <title>", Source: "dom"}, true
}
