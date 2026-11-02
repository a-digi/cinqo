package jobs

import (
	"database/sql"
	"strings"

	"career-tool-backend/domainevent"
	"career-tool-backend/textclean"
)

// description_change.go (step 164) is the one place that decides whether a write changed a job's description and
// announces it. Every match (persona scores, the job's best match, the profiles' boards) is made from the job's text, so
// a change of the text must reach their consumers whoever wrote it: a detail crawl (which fills in the full text of a job
// that only had the listing's snippet), a recrawl (it goes through the same save) and a listing refresh. The event is
// "career.job.description_changed" { job_id, source }, the one the recrawl worker used to publish by itself (plan 148); the
// persona consumer already listens to it. See plan/ai/tools/career/step-164-job-description-changed-rematch.md.

// DescriptionChangedTopic is the event sent when a write changed an existing job's description. It has the same value as
// jobrecrawl.DescriptionChangedTopic (a test in this tool pins that).
const DescriptionChangedTopic = "career.job.description_changed"

// The values of the event's "source" field.
const (
	DescriptionSourceDetailCrawl  = "detail_crawl"
	DescriptionSourceListingCrawl = "listing_crawl"
)

// publishDescriptionChanged sends the event; a var so a test can watch it. Best effort like every domain event: it runs
// synchronously and domainevent.Publish's own short timeout bounds it.
var publishDescriptionChanged = func(jobID, source string) {
	domainevent.Publish(DescriptionChangedTopic, map[string]string{"job_id": jobID, "source": source})
}

// descriptionWouldChange says whether writing incoming (already cleaned text) over the stored old description changes it.
// A job a person added by hand keeps the text they typed (the same rule as the SQL of every write), so nothing changes for
// it; otherwise the cleaned old text is compared with the new one, so a legacy row that only differs by markup is not a
// change.
func descriptionWouldChange(source, old, incoming string) bool {
	if source == SourceManual && strings.TrimSpace(old) != "" {
		return false
	}
	return textclean.Description(old) != incoming
}

// listingRefreshChangesDescription is descriptionWouldChange for a listing crawl that refreshes a known job. A job whose
// detail page was crawled successfully already has its full text; the listing only knows a snippet, so the listing's write is
// not announced as a change (the matches stay made from the full text). Every other job takes the listing's text.
func listingRefreshChangesDescription(source, old string, detailStatus sql.NullString, incoming string) bool {
	if detailStatus.Valid && detailStatus.String == "success" {
		return false
	}
	return descriptionWouldChange(source, old, incoming)
}

// SetDescriptionChangedPublisher replaces the sender of the event until the returned function is called, so a test of
// another package (the recrawl worker) can watch what a real save announces without a core.
func SetDescriptionChangedPublisher(f func(jobID, source string)) (restore func()) {
	old := publishDescriptionChanged
	publishDescriptionChanged = f
	return func() { publishDescriptionChanged = old }
}

// flag turns a bool into the 0 or 1 a SQL CASE reads.
func flag(b bool) int {
	if b {
		return 1
	}
	return 0
}

// oldDescription is what a write needs to know about the row it is about to update.
type oldDescription struct {
	source       string
	description  string
	detailStatus sql.NullString
}

// readOldDescription reads the stored description, source and detail status of the job with sourceURL.
func readOldDescription(q interface {
	QueryRow(query string, args ...any) *sql.Row
}, whereColumn, value string) (oldDescription, error) {
	var o oldDescription
	err := q.QueryRow(
		`SELECT source, COALESCE(description, ''), detail_crawl_status FROM jobs WHERE `+whereColumn+` = ?`, value,
	).Scan(&o.source, &o.description, &o.detailStatus)
	return o, err
}
