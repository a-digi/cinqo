package portal

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"career-tool-backend/db"
)

// The pure rules of "a new link first tries the portal's existing instructions"
// (step 103): which instructions are tried, what counts as "it worked", and what is
// copied in each outcome. Nothing here touches the database or the browser; the
// crawl package runs the trial with these rules. See
// plan/ai/tools/career/step-103-inherit-portal-instructions-for-new-links.md.

const (
	// maxInheritCandidates is how many different instruction documents are tried.
	maxInheritCandidates = 3
	// MaxDetailTrialJobs is how many jobs from the trial list are opened with the
	// candidate's job-detail instructions.
	MaxDetailTrialJobs = 2
	// minDetailDescriptionRunes is how long a job's description must be for the
	// job-detail instructions to count as working on it.
	minDetailDescriptionRunes = 100
)

// Statuses a trial reports for the job-detail part (fixed words, also in the API).
const (
	DetailOK             = "ok"
	DetailFailed         = "failed"
	DetailNoInstructions = "no-instructions"
	DetailNotTried       = "not-tried"
)

// InheritCandidate is one sibling link's instructions, offered to the new link.
type InheritCandidate struct {
	LinkID string
	Title  string
	// Listing is the sibling's listing instructions (never empty).
	Listing string
	// JobDetail is its job-detail instructions, empty when it has none.
	JobDetail string
}

// normalizeInstructions makes two copies of the same document compare equal: line
// endings, trailing spaces and surrounding blank space do not count.
func normalizeInstructions(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// crawledAtTime reads a link's last-crawled text (the database's "2006-01-02 15:04:05"
// or RFC 3339); text that is neither sorts as never crawled.
func crawledAtTime(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// pickInheritCandidates chooses which of the portal's links to try for newLinkID: the
// other links with valid listing instructions, de-duplicated by text, the most
// recently crawled first (ties keep the order given), at most maxInheritCandidates.
// links is the portal's own links only; a link of another portal must never be passed.
// When two links hold the same listing document and only one has job-detail
// instructions, the candidate takes them.
func pickInheritCandidates(links []portalLink, newLinkID string) []InheritCandidate {
	eligible := make([]portalLink, 0, len(links))
	for _, l := range links {
		if l.ID == newLinkID || strings.TrimSpace(l.CrawlInstructions) == "" {
			continue
		}
		if validateCrawlInstructions(l.CrawlInstructions) != nil {
			continue
		}
		eligible = append(eligible, l)
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		return crawledAtTime(eligible[i].LastCrawledAt).After(crawledAtTime(eligible[j].LastCrawledAt))
	})

	var out []InheritCandidate
	seen := map[string]int{} // normalized listing text -> index in out
	for _, l := range eligible {
		key := normalizeInstructions(l.CrawlInstructions)
		detail := ""
		if strings.TrimSpace(l.JobDetailCrawlInstructions) != "" && validateJobDetailCrawlInstructions(l.JobDetailCrawlInstructions) == nil {
			detail = l.JobDetailCrawlInstructions
		}
		if i, dup := seen[key]; dup {
			if out[i].JobDetail == "" {
				out[i].JobDetail = detail
			}
			continue
		}
		if len(out) == maxInheritCandidates {
			continue
		}
		seen[key] = len(out)
		out = append(out, InheritCandidate{LinkID: l.ID, Title: l.Title, Listing: l.CrawlInstructions, JobDetail: detail})
	}
	return out
}

// LimitToPages caps a request to its first n pages: a trial looks at page one only.
func LimitToPages(req CrawlRequest, n int) CrawlRequest {
	if n < 1 {
		n = 1
	}
	if req.RequestedMaxPages > n {
		req.RequestedMaxPages = n
	}
	if req.EffectiveMaxPages > n {
		req.EffectiveMaxPages = n
	}
	return req
}

// ListingTrialPasses says whether a trial crawl of the new link's list worked: every
// item has a title and a url (the same ok as check_crawl_extraction_result), there is
// at least one, and there is no low-count warning (1 or 2 items is not trusted, the
// same rule the agent follows when it writes instructions itself).
func ListingTrialPasses(report CrawlExtractionReport) bool {
	return report.OK && report.ItemsExtractable >= 1 && report.Warning == ""
}

// ClassifyTrialPages runs the extraction check over a trial crawl's pages (the check
// behind check_crawl_extraction_result, without saving anything).
func ClassifyTrialPages(pages []CrawlResultPage) CrawlExtractionReport {
	return classifyCrawlResultPages(pages)
}

// TrialJobURLs returns the urls of the first n extractable items of a trial crawl,
// in page order, without repeats; these are the jobs whose detail pages are tried.
func TrialJobURLs(pages []CrawlResultPage, n int) []string {
	var urls []string
	seen := map[string]bool{}
	add := func(title, sourceURL string) bool {
		if title == "" || sourceURL == "" || seen[sourceURL] {
			return len(urls) >= n
		}
		seen[sourceURL] = true
		urls = append(urls, sourceURL)
		return len(urls) >= n
	}
	for _, page := range pages {
		if len(page.Items) > 0 {
			for _, item := range page.Items {
				title, sourceURL, _, _, _, _ := extractCrawlResultItemFields(item)
				if add(title, sourceURL) {
					return urls
				}
			}
			continue
		}
		for i := range ExtractResultStrings(page.Results["title"]) {
			title, sourceURL, _, _, _, _ := extractCrawlResultIndexedFields(page, i)
			if add(title, sourceURL) {
				return urls
			}
		}
	}
	return urls
}

// DetailTrialsPass says whether the job-detail instructions worked: each opened job gave
// a description of at least minDetailDescriptionRunes, and at least one job was opened.
// values holds one extracted field map per job (label to text).
func DetailTrialsPass(values []map[string]string) bool {
	if len(values) == 0 {
		return false
	}
	for _, v := range values {
		if utf8.RuneCountInString(strings.TrimSpace(v["description"])) < minDetailDescriptionRunes {
			return false
		}
	}
	return true
}

// InheritCopies says what is saved on the new link once a candidate's list worked, given
// how its job-detail trial went: both documents when the details also worked, only the
// listing otherwise (the usual pipeline then writes the detail instructions with AI),
// and nothing when the list did not work.
func InheritCopies(listingOK bool, detailStatus string) (copyListing, copyJobDetail bool) {
	if !listingOK {
		return false, false
	}
	return true, detailStatus == DetailOK
}

// InheritCandidatesFor loads the portal that owns linkID and picks the instructions to
// try for it from that portal's other links (never another portal's). ErrUnknownPortalLink
// when there is no such link.
func InheritCandidatesFor(linkID string) ([]InheritCandidate, error) {
	portals, err := ListPortals()
	if err != nil {
		return nil, err
	}
	for _, p := range portals {
		for _, l := range p.Links {
			if l.ID == linkID {
				return pickInheritCandidates(p.Links, linkID), nil
			}
		}
	}
	return nil, ErrUnknownPortalLink
}

// ListingTrialFailureReason says in a few words why a trial list crawl did not pass
// (empty when it did), for the "tried" list the user sees.
func ListingTrialFailureReason(report CrawlExtractionReport) string {
	switch {
	case ListingTrialPasses(report):
		return ""
	case report.ItemsFound == 0:
		return "no items found"
	case report.ItemsSkipped > 0:
		return fmt.Sprintf("%d of %d items had no title or url", report.ItemsSkipped, report.ItemsFound)
	case report.Warning != "":
		return fmt.Sprintf("only %d item(s) found", report.ItemsFound)
	default:
		return "no usable items"
	}
}

// SaveInheritedInstructions saves the winning candidate's documents on the new link,
// but only into what is still empty: a listing document is written only when the link
// has none (listingSaved false means it got some while the trial ran, and then nothing
// at all is written), and the job-detail document only when the link has none of its
// own. Both texts are validated first. A saved listing settles any pending "does this
// page list jobs?" question, as every other listing save does.
func SaveInheritedInstructions(linkID, listing, jobDetail string) (listingSaved, jobDetailSaved bool, err error) {
	if err := RequirePortalLinkExists(linkID); err != nil {
		return false, false, err
	}
	if err := validateCrawlInstructions(listing); err != nil {
		return false, false, err
	}
	if jobDetail != "" {
		if err := validateJobDetailCrawlInstructions(jobDetail); err != nil {
			return false, false, err
		}
	}
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return false, false, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`UPDATE portal_links SET crawl_instructions = ?, jobs_question_asked_at = NULL, jobs_question_note = NULL, updated_at = datetime('now')
		 WHERE id = ? AND TRIM(COALESCE(crawl_instructions, ''), ' ' || char(9) || char(10) || char(13)) = ''`,
		listing, linkID,
	)
	if err != nil {
		return false, false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, false, nil
	}
	if jobDetail != "" {
		res, err := tx.Exec(
			`UPDATE portal_links SET job_detail_crawl_instructions = ?, updated_at = datetime('now')
			 WHERE id = ? AND TRIM(COALESCE(job_detail_crawl_instructions, ''), ' ' || char(9) || char(10) || char(13)) = ''`,
			jobDetail, linkID,
		)
		if err != nil {
			return false, false, err
		}
		n, _ := res.RowsAffected()
		jobDetailSaved = n > 0
	}
	if err := tx.Commit(); err != nil {
		return false, false, err
	}
	return true, jobDetailSaved, nil
}
