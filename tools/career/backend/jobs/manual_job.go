// manual_job.go (step 97) lets a person add a job by hand: one found on
// LinkedIn, in a recruiter's email, from a friend. It gets everything a crawled
// job gets (Job Match, Generate CV, the company link), because all of that keys
// on the job id. See plan/ai/tools/career/step-97-add-jobs-manually.md.
package jobs

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"career-tool-backend/db"
)

// Source values stored in jobs.source.
const (
	SourceCrawled = "crawled"
	SourceManual  = "manual"
)

// manualURLPrefix is the placeholder source_url of a manual job without a
// posting URL (the column is NOT NULL UNIQUE). A real URL must start with http,
// so it can never collide with this.
const manualURLPrefix = "manual:"

// Field limits. No job field had any limit before; a manual form must.
const (
	maxManualTitle       = 300
	maxManualCompany     = 200
	maxManualLocation    = 200
	maxManualPostedAt    = 100
	maxManualDescription = 100_000
	maxManualURL         = 2000
)

// publicSourceURL hides the placeholder of a manual job without a URL.
func publicSourceURL(stored string) string {
	if strings.HasPrefix(stored, manualURLPrefix) {
		return ""
	}
	return stored
}

// ManualJobInput is what a person enters. Only Title is required.
type ManualJobInput struct {
	Title       string
	Company     string
	Location    string
	Description string
	PostedAt    string
	SourceURL   string
}

// ValidationError is a caller mistake (a 400): which field and why.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// DuplicateJobError says the job already exists (a 409), with the existing id so
// the caller can open it.
type DuplicateJobError struct {
	ExistingID string
	Reason     string // "source_url" or "title_company"
}

func (e *DuplicateJobError) Error() string {
	if e.Reason == "source_url" {
		return "a job with this posting URL already exists"
	}
	return "a job with this title and company already exists"
}

func tooLong(field string, value string, max int) *ValidationError {
	return &ValidationError{Field: field, Message: fmt.Sprintf("must be at most %d characters", max)}
}

// ValidateManualJob trims every field in place and checks the limits and the URL.
func ValidateManualJob(in *ManualJobInput) error {
	in.Title = strings.TrimSpace(in.Title)
	in.Company = strings.TrimSpace(in.Company)
	in.Location = strings.TrimSpace(in.Location)
	in.Description = strings.TrimSpace(in.Description)
	in.PostedAt = strings.TrimSpace(in.PostedAt)
	in.SourceURL = strings.TrimSpace(in.SourceURL)

	if in.Title == "" {
		return &ValidationError{Field: "title", Message: "is required"}
	}
	for _, f := range []struct {
		name  string
		value string
		max   int
	}{
		{"title", in.Title, maxManualTitle},
		{"company", in.Company, maxManualCompany},
		{"location", in.Location, maxManualLocation},
		{"postedAt", in.PostedAt, maxManualPostedAt},
		{"description", in.Description, maxManualDescription},
		{"sourceUrl", in.SourceURL, maxManualURL},
	} {
		if strings.ContainsRune(f.value, 0) {
			return &ValidationError{Field: f.name, Message: "contains an invalid character"}
		}
		if utf8.RuneCountInString(f.value) > f.max {
			return tooLong(f.name, f.value, f.max)
		}
	}
	if in.SourceURL != "" {
		u, err := url.Parse(in.SourceURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || strings.ContainsAny(in.SourceURL, " \t\r\n") {
			return &ValidationError{Field: "sourceUrl", Message: "must be an absolute http or https URL"}
		}
	}
	return nil
}

// AddManualJob validates and stores a job a person entered, refusing a
// duplicate (the same posting URL, or the same title and company: the rule
// portal ingestion already uses). A company name publishes the usual
// job-created event, which links or creates the company.
func AddManualJob(in ManualJobInput) (job, error) {
	if err := ValidateManualJob(&in); err != nil {
		return job{}, err
	}

	if in.SourceURL != "" {
		var existing string
		err := db.JobsDB.QueryRow(`SELECT id FROM jobs WHERE source_url = ?`, in.SourceURL).Scan(&existing)
		switch {
		case err == nil:
			return job{}, &DuplicateJobError{ExistingID: existing, Reason: "source_url"}
		case err != sql.ErrNoRows:
			return job{}, err
		}
	}
	if in.Company != "" {
		var existing string
		err := db.JobsDB.QueryRow(
			`SELECT id FROM jobs WHERE LOWER(TRIM(title)) = ? AND LOWER(TRIM(company)) = ?`,
			toLower(in.Title), toLower(in.Company),
		).Scan(&existing)
		switch {
		case err == nil:
			return job{}, &DuplicateJobError{ExistingID: existing, Reason: "title_company"}
		case err != sql.ErrNoRows:
			return job{}, err
		}
	}

	id := uuid.NewString()
	sourceURL := in.SourceURL
	if sourceURL == "" {
		sourceURL = manualURLPrefix + id
	}
	_, err := db.JobsDB.Exec(
		`INSERT INTO jobs (id, source_url, title, company, location, description, posted_at, source, crawled_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
		id, sourceURL, in.Title, nullIfEmpty(in.Company), nullIfEmpty(in.Location), nullIfEmpty(in.Description), nullIfEmpty(in.PostedAt), SourceManual,
	)
	if err != nil {
		// Lost a race with another insert of the same URL: the UNIQUE constraint
		// is the real guard, the check above only gives the friendly answer.
		if in.SourceURL != "" && strings.Contains(strings.ToLower(err.Error()), "unique") {
			var existing string
			if qerr := db.JobsDB.QueryRow(`SELECT id FROM jobs WHERE source_url = ?`, in.SourceURL).Scan(&existing); qerr == nil {
				return job{}, &DuplicateJobError{ExistingID: existing, Reason: "source_url"}
			}
		}
		return job{}, err
	}
	publishJobCreatedEvent(id, in.Company)
	return GetJobByID(id)
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// IsValidationError / IsDuplicate are for callers that map errors to statuses.
func IsValidationError(err error) (*ValidationError, bool) {
	var v *ValidationError
	return v, errors.As(err, &v)
}

func IsDuplicate(err error) (*DuplicateJobError, bool) {
	var d *DuplicateJobError
	return d, errors.As(err, &d)
}
