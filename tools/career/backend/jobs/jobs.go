// Package jobs is the AI-facing surface over jobs.db — how a crawled
// job posting actually gets saved, and how it's read back. Deliberately
// does not crawl anything itself: browser's own crawl_paginated
// (plan/ai/tools/browser/step-16-paginated-crawl-instructions.md)
// already does the actual multi-page extraction; the AI's own real
// workflow is to call that, then call this package's own save_job once
// per posting found. See plan/ai/tools/career/step-04-jobs-tools.md.
//
// Step 20 added SavePortalJob — a separate, dedicated tool for the
// portal-driven crawling workflow (plan/ai/tools/career/
// step-18-portals.md), deliberately not a new parameter on save_job
// itself: save_job's own existing source_url-only dedup contract is
// left completely unchanged for its own existing (ad-hoc/manual
// crawling) callers. save_portal_job additionally dedupes by a
// case-insensitive, trimmed (title, company) match — the real gap
// save_job's own source_url-only dedup leaves open when the same
// posting reaches the AI via more than one portal, each with its own
// different URL for it. See
// plan/ai/tools/career/step-20-portal-job-ingestion.md.
//
// Extracted into its own package (step XX) as part of this backend's
// split into subpackages mirroring tools/browser/backend's own auth/
// crawler/shared layout. Imports companies (RequireCompanyExists/
// ErrUnknownCompany, for LinkJobToCompany) — one-directional, companies
// never imports jobs back. Deliberately does NOT import the portal
// package even though FindPortalLinkURL conceptually checks "does this
// portal link exist": the portal package itself needs SavePortalJob
// (below) for its own IngestCrawlResults, so importing portal from here
// would be a real cycle — errUnknownPortalLinkRef below is a small,
// deliberately independent local copy of portal.ErrUnknownPortalLink,
// same "two separate concerns, kept in sync by hand" convention this
// tool already uses for browser's own cross-module JSON shapes. See
// plan/ai/tools/career/step-XX-package-split.md.
package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/companies"
	"career-tool-backend/db"
	"career-tool-backend/media"
)

// defaultJobsLimit/maxJobsLimit bound list_jobs/search_jobs the same
// way browser's own extract_page_data/crawl_paginated already clamp
// their own size-bounding parameters — a plain, non-configurable
// ceiling, not premature configurability.
const (
	defaultJobsLimit = 50
	maxJobsLimit     = 200
)

type job struct {
	ID           string `json:"id"`
	SourceURL    string `json:"sourceUrl"`
	Title        string `json:"title"`
	Company      string `json:"company,omitempty"`
	CompanyID    string `json:"companyId,omitempty"`
	PortalLinkID string `json:"portalLinkId,omitempty"`
	// PortalID/PortalName (step — Jobs page platform column/filter) are
	// resolved via a LEFT JOIN through portal_links to portals in
	// queryJobs, below — a job only ever stores portal_link_id itself;
	// these two are read-only, derived convenience fields for display
	// and for filtering by the owning portal rather than one specific
	// link. Both empty when PortalLinkID is empty or points at a portal
	// link whose own portal has since been deleted (LEFT JOIN, not
	// JOIN, so a job row is never hidden by a dangling reference).
	PortalID    string `json:"portalId,omitempty"`
	PortalName  string `json:"portalName,omitempty"`
	Location    string `json:"location,omitempty"`
	Description string `json:"description,omitempty"`
	PostedAt    string `json:"postedAt,omitempty"`
	CrawledAt   string `json:"crawledAt"`
	// DetailCrawlStatus (step XX) records the outcome of the most
	// recent job-DETAIL-page crawl attempt — "" (empty, never "null")
	// means no attempt has ever been made, as opposed to CrawledAt,
	// which is set unconditionally by the LISTING crawl that produced
	// this row and says nothing about whether the job's own detail
	// page was ever separately visited. "failed" | "success". Read-only
	// — written only via save_job_detail_extraction (this file) and
	// MarkJobDetailCrawlFailed. See
	// plan/ai/tools/career/step-XX-job-detail-crawl-status-eye-icon.md.
	DetailCrawlStatus string `json:"detailCrawlStatus,omitempty"`
	// MatchScore/MatchStatus/MatchConversationID/MatchError (step XX)
	// mirror DetailCrawlStatus's own "read-only, written only via a
	// dedicated function" posture — resolved via a LEFT JOIN to
	// job_matches (job_match.go, package main) in queryJobs/GetJobByID,
	// below. MatchScore is nil until a match completes; MatchStatus is
	// "" (never "null") when this job has never been matched at all.
	// See plan/ai/tools/career/step-XX-job-match.md.
	MatchScore          *int   `json:"matchScore,omitempty"`
	MatchStatus         string `json:"matchStatus,omitempty"`
	MatchConversationID string `json:"matchConversationId,omitempty"`
	MatchError          string `json:"matchError,omitempty"`
	// MatchedSkills (step XX) — the specific skills (verbatim, from
	// that persona's own career.db skills list) that explain
	// MatchScore, from the one-to-many job_match_skills table (this
	// file's own GetJobMatchSkills/GetJobMatchSkillsBatch, below) a
	// single LEFT JOIN can't attach here without multiplying this row.
	// Always [] (never omitted/null) so frontend code never has to
	// special-case "field absent" vs "no matched skills".
	MatchedSkills []string `json:"matchedSkills"`
	// CvStatus/CvConversationID/CvMediaFileID/CvError (step XX) mirror
	// MatchStatus's own "read-only, written only via a dedicated
	// function" posture, for the separate "Generate CV PDF" feature —
	// resolved via a LEFT JOIN to job_cv_pdfs (jobs/cv_pdf.go) in
	// queryJobs/GetJobByID, below. CvMediaFileID is set once CvStatus
	// reaches "completed"; the frontend builds a download link from it.
	// See plan/ai/tools/career/step-XX-cv-pdf.md.
	CvStatus         string `json:"cvStatus,omitempty"`
	CvConversationID string `json:"cvConversationId,omitempty"`
	CvMediaFileID    string `json:"cvMediaFileId,omitempty"`
	CvError          string `json:"cvError,omitempty"`
}

// SaveJob upserts by source_url — re-saving a posting already known
// updates it (refreshing crawled_at) rather than duplicating it.
// Returns the row's own id and whether this was a fresh insert.
func saveJob(sourceURL, title, company, location, description, postedAt string) (id string, created bool, err error) {
	var existingID string
	err = db.JobsDB.QueryRow(`SELECT id FROM jobs WHERE source_url = ?`, sourceURL).Scan(&existingID)
	switch {
	case err == sql.ErrNoRows:
		id = uuid.NewString()
		created = true
	case err != nil:
		return "", false, err
	default:
		id = existingID
		created = false
	}

	_, err = db.JobsDB.Exec(
		`INSERT INTO jobs (id, source_url, title, company, location, description, posted_at, crawled_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))
		 ON CONFLICT(source_url) DO UPDATE SET
			title = excluded.title,
			company = excluded.company,
			location = excluded.location,
			description = excluded.description,
			posted_at = excluded.posted_at,
			crawled_at = excluded.crawled_at`,
		id, sourceURL, title, company, location, description, postedAt,
	)
	if err != nil {
		return "", false, err
	}
	return id, created, nil
}

type jobsListResult struct {
	Jobs  []job `json:"jobs"`
	Total int   `json:"total"`
}

// listJobs is a plain paged read, newest crawled_at first. companyId,
// if non-empty, restricts to jobs linked to that company (step 14) —
// see plan/ai/tools/career/step-14-companies.md. portalLinkId, if
// non-empty, restricts to jobs found via that portal link (step 20) —
// see plan/ai/tools/career/step-20-portal-job-ingestion.md. portalId,
// if non-empty, restricts to jobs found via ANY link belonging to that
// portal — the coarser, "which platform" filter the Jobs page's own
// portal dropdown uses, as opposed to portalLinkId's one-specific-URL
// granularity. See plan/ai/tools/career/step-XX-jobs-page-platform-
// column-filter-pagination.md.
func listJobs(companyId, portalLinkId, portalId string, limit, offset int) (jobsListResult, error) {
	where := `WHERE 1=1`
	args := []any{}
	if companyId != "" {
		where += ` AND j.company_id = ?`
		args = append(args, companyId)
	}
	if portalLinkId != "" {
		where += ` AND j.portal_link_id = ?`
		args = append(args, portalLinkId)
	}
	if portalId != "" {
		where += ` AND pl.portal_id = ?`
		args = append(args, portalId)
	}
	return queryJobs(where, args, limit, offset)
}

// SearchJobs matches query against title/company/description; location
// restricts to an exact location value (step — Jobs page Location
// dropdown: the frontend now offers a fixed list of distinct location
// values via ListDistinctJobLocations, rather than free text, so
// partial LIKE matching no longer applies). companyId/portalLinkId/
// portalId, if non-empty, additionally restrict as documented on
// listJobs, above. Omitting every filter is equivalent to listJobs.
// Plain parameterized LIKE for query, case-insensitive via LOWER(...)
// — no full-text-search extension for a first pass (see this step's
// own open question 1).
func SearchJobs(query, location, companyId, portalLinkId, portalId string, limit, offset int) (jobsListResult, error) {
	where := `WHERE 1=1`
	args := []any{}
	if query != "" {
		where += ` AND (LOWER(j.title) LIKE ? OR LOWER(j.company) LIKE ? OR LOWER(j.description) LIKE ?)`
		pattern := "%" + toLower(query) + "%"
		args = append(args, pattern, pattern, pattern)
	}
	if location != "" {
		where += ` AND j.location = ?`
		args = append(args, location)
	}
	if companyId != "" {
		where += ` AND j.company_id = ?`
		args = append(args, companyId)
	}
	if portalLinkId != "" {
		where += ` AND j.portal_link_id = ?`
		args = append(args, portalLinkId)
	}
	if portalId != "" {
		where += ` AND pl.portal_id = ?`
		args = append(args, portalId)
	}
	return queryJobs(where, args, limit, offset)
}

// jobsFromClause is shared by queryJobs's own COUNT(*) and paged SELECT
// so the two can never drift apart — a LEFT JOIN (not JOIN) on both
// hops so a job is never hidden by a portal link, or that link's own
// portal, having since been deleted out from under it.
const jobsFromClause = `FROM jobs j
	LEFT JOIN portal_links pl ON pl.id = j.portal_link_id
	LEFT JOIN portals p ON p.id = pl.portal_id
	LEFT JOIN job_matches jm ON jm.job_id = j.id
	LEFT JOIN job_cv_pdfs cv ON cv.job_id = j.id`

// hideDanglingCvMedia clears CvStatus/CvMediaFileID (in-memory only,
// on the given slice — never written back to the database from a read
// path) for any job whose own media_file_id no longer refers to a real
// Media row, e.g. one deleted from the core admin Media page, or the
// pre-existing-reference bug step 10 fixed for future saves. Checks
// run CONCURRENTLY (bounded by however many rows in jobs actually
// claim a completed CV, typically a small subset of any page) so a
// jobs-list request's own latency stays close to one round trip
// regardless of how many rows need checking — media.Exists itself
// fails open on anything but a definitive 404, so a slow/unreachable
// core never hides an otherwise-real CV. See
// plan/ai/media/step-11-media-exists-check.md.
func hideDanglingCvMedia(jobs []job) {
	var wg sync.WaitGroup
	for i := range jobs {
		if jobs[i].CvStatus != "completed" || jobs[i].CvMediaFileID == "" {
			continue
		}
		wg.Add(1)
		go func(j *job) {
			defer wg.Done()
			if !media.Exists(j.CvMediaFileID) {
				j.CvStatus = ""
				j.CvMediaFileID = ""
			}
		}(&jobs[i])
	}
	wg.Wait()
}

func queryJobs(where string, whereArgs []any, limit, offset int) (jobsListResult, error) {
	var total int
	countArgs := append([]any{}, whereArgs...)
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) `+jobsFromClause+` `+where, countArgs...).Scan(&total); err != nil {
		return jobsListResult{}, err
	}

	pageArgs := append(append([]any{}, whereArgs...), limit, offset)
	rows, err := db.JobsDB.Query(
		`SELECT j.id, j.source_url, j.title, j.company, j.company_id, j.portal_link_id, p.id, p.name, j.location, j.description, j.posted_at, j.crawled_at, j.detail_crawl_status,
			jm.score, jm.status, jm.conversation_id, jm.error,
			cv.status, cv.conversation_id, cv.media_file_id, cv.error
		 `+jobsFromClause+` `+where+` ORDER BY j.crawled_at DESC LIMIT ? OFFSET ?`,
		pageArgs...,
	)
	if err != nil {
		return jobsListResult{}, err
	}
	defer rows.Close()

	jobs := []job{}
	for rows.Next() {
		var j job
		var company, companyID, portalLinkID, portalID, portalName, location, description, postedAt, detailCrawlStatus sql.NullString
		var matchStatus, matchConversationID, matchError sql.NullString
		var matchScore sql.NullInt64
		var cvStatus, cvConversationID, cvMediaFileID, cvError sql.NullString
		if err := rows.Scan(&j.ID, &j.SourceURL, &j.Title, &company, &companyID, &portalLinkID, &portalID, &portalName, &location, &description, &postedAt, &j.CrawledAt, &detailCrawlStatus,
			&matchScore, &matchStatus, &matchConversationID, &matchError,
			&cvStatus, &cvConversationID, &cvMediaFileID, &cvError); err != nil {
			return jobsListResult{}, err
		}
		j.Company = company.String
		j.CompanyID = companyID.String
		j.PortalLinkID = portalLinkID.String
		j.PortalID = portalID.String
		j.PortalName = portalName.String
		j.Location = location.String
		j.Description = description.String
		j.DetailCrawlStatus = detailCrawlStatus.String
		j.PostedAt = postedAt.String
		if matchScore.Valid {
			score := int(matchScore.Int64)
			j.MatchScore = &score
		}
		j.MatchStatus = matchStatus.String
		j.MatchConversationID = matchConversationID.String
		j.MatchError = matchError.String
		j.MatchedSkills = []string{}
		j.CvStatus = cvStatus.String
		j.CvConversationID = cvConversationID.String
		j.CvMediaFileID = cvMediaFileID.String
		j.CvError = cvError.String
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return jobsListResult{}, err
	}

	// One batched query for the whole page rather than one per job —
	// job_match_skills is one-to-many, so a single LEFT JOIN above
	// can't attach it without multiplying each job row. See
	// GetJobMatchSkillsBatch's own doc comment, below.
	jobIDs := make([]string, len(jobs))
	for i, j := range jobs {
		jobIDs[i] = j.ID
	}
	skillsByJob, err := GetJobMatchSkillsBatch(jobIDs)
	if err != nil {
		return jobsListResult{}, err
	}
	for i := range jobs {
		if skills, ok := skillsByJob[jobs[i].ID]; ok {
			jobs[i].MatchedSkills = skills
		}
	}

	hideDanglingCvMedia(jobs)

	return jobsListResult{Jobs: jobs, Total: total}, nil
}

func DeleteJob(id string) error {
	_, err := db.JobsDB.Exec(`DELETE FROM jobs WHERE id = ?`, id)
	return err
}

// GetJobByID reads one job by id, with the exact same portal_links/
// portals join queryJobs' own paged read already uses — the Jobs page's
// own Eye icon opens a details page keyed by this. ErrUnknownJob on no
// rows.
func GetJobByID(id string) (job, error) {
	row := db.JobsDB.QueryRow(
		`SELECT j.id, j.source_url, j.title, j.company, j.company_id, j.portal_link_id, p.id, p.name, j.location, j.description, j.posted_at, j.crawled_at, j.detail_crawl_status,
			jm.score, jm.status, jm.conversation_id, jm.error,
			cv.status, cv.conversation_id, cv.media_file_id, cv.error
		 `+jobsFromClause+` WHERE j.id = ?`,
		id,
	)
	var j job
	var company, companyID, portalLinkID, portalID, portalName, location, description, postedAt, detailCrawlStatus sql.NullString
	var matchStatus, matchConversationID, matchError sql.NullString
	var matchScore sql.NullInt64
	var cvStatus, cvConversationID, cvMediaFileID, cvError sql.NullString
	err := row.Scan(&j.ID, &j.SourceURL, &j.Title, &company, &companyID, &portalLinkID, &portalID, &portalName, &location, &description, &postedAt, &j.CrawledAt, &detailCrawlStatus,
		&matchScore, &matchStatus, &matchConversationID, &matchError,
		&cvStatus, &cvConversationID, &cvMediaFileID, &cvError)
	switch {
	case err == sql.ErrNoRows:
		return job{}, ErrUnknownJob
	case err != nil:
		return job{}, err
	}
	j.Company = company.String
	j.CompanyID = companyID.String
	j.PortalLinkID = portalLinkID.String
	j.PortalID = portalID.String
	j.PortalName = portalName.String
	j.Location = location.String
	j.Description = description.String
	j.DetailCrawlStatus = detailCrawlStatus.String
	j.PostedAt = postedAt.String
	if matchScore.Valid {
		score := int(matchScore.Int64)
		j.MatchScore = &score
	}
	j.MatchStatus = matchStatus.String
	j.MatchConversationID = matchConversationID.String
	j.MatchError = matchError.String
	j.CvStatus = cvStatus.String
	j.CvConversationID = cvConversationID.String
	j.CvMediaFileID = cvMediaFileID.String
	j.CvError = cvError.String
	skills, err := GetJobMatchSkills(j.ID)
	if err != nil {
		return job{}, err
	}
	j.MatchedSkills = skills
	single := []job{j}
	hideDanglingCvMedia(single)
	return single[0], nil
}

// GetJobMatchSkills reads one job's own matched skills, alphabetically
// — empty (never nil) when the job has no completed match, or its
// match had no matching skills at all. Moved here from job_match.go
// (package main, step XX package split) since queryJobs/GetJobByID
// above are this function's own only real callers — job_match.go's own
// remaining WRITE side (saveJobMatchResult) stays in package main,
// which can reach db.JobsDB directly; only the READ side needed to move
// with this package.
func GetJobMatchSkills(jobId string) ([]string, error) {
	rows, err := db.JobsDB.Query(`SELECT skill FROM job_match_skills WHERE job_id = ? ORDER BY skill ASC`, jobId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	skills := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		skills = append(skills, s)
	}
	return skills, rows.Err()
}

// GetJobMatchSkillsBatch is queryJobs' own batched counterpart to
// GetJobMatchSkills — one query for a whole page of jobs rather than
// one per row (avoiding N+1), since job_match_skills is a one-to-many
// relation a single LEFT JOIN can't attach to a job row without
// multiplying it. A jobId with no entries simply has no key in the
// returned map, exactly like the portal package's own
// lastCrawledByPortalLink established "absent, not empty-value"
// convention.
func GetJobMatchSkillsBatch(jobIds []string) (map[string][]string, error) {
	result := make(map[string][]string, len(jobIds))
	if len(jobIds) == 0 {
		return result, nil
	}
	placeholders := make([]string, len(jobIds))
	args := make([]any, len(jobIds))
	for i, id := range jobIds {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := db.JobsDB.Query(
		`SELECT job_id, skill FROM job_match_skills WHERE job_id IN (`+strings.Join(placeholders, ",")+`) ORDER BY job_id, skill ASC`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var jobId, skill string
		if err := rows.Scan(&jobId, &skill); err != nil {
			return nil, err
		}
		result[jobId] = append(result[jobId], skill)
	}
	return result, rows.Err()
}

// JobDetailCrawlTarget is one job's own minimal identity for the
// deterministic "Crawl job details now" loop (crawl package) — just
// enough to navigate to its detail page and, once extracted, name
// which row to update.
type JobDetailCrawlTarget struct {
	ID        string
	SourceURL string
	Title     string
}

// maxJobDetailCrawlJobs bounds how many jobs a single "Crawl job
// details now" run ever visits — a plain, non-configurable ceiling,
// matching maxJobsLimit's own established precedent. A link with more
// saved jobs than this needs more than one run to fully backfill;
// ordering by crawled_at ASC (oldest touched first) means each run
// still makes forward progress on whichever jobs have gone longest
// without a detail crawl, rather than re-visiting the same jobs every
// time a link is over this cap.
const maxJobDetailCrawlJobs = 200

// ListJobsForDetailCrawl returns up to maxJobDetailCrawlJobs jobs
// belonging to portalLinkID, oldest-crawled first.
func ListJobsForDetailCrawl(portalLinkID string) ([]JobDetailCrawlTarget, error) {
	rows, err := db.JobsDB.Query(
		`SELECT id, source_url, title FROM jobs WHERE portal_link_id = ? ORDER BY crawled_at ASC LIMIT ?`,
		portalLinkID, maxJobDetailCrawlJobs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targets := []JobDetailCrawlTarget{}
	for rows.Next() {
		var t JobDetailCrawlTarget
		if err := rows.Scan(&t.ID, &t.SourceURL, &t.Title); err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

// ListDistinctJobLocations returns every distinct non-empty location
// value across all saved jobs, alphabetically — the Jobs page's own
// Location filter dropdown's data source. Raw, unnormalized strings as
// crawled (e.g. "Berlin" and "Berlin, Germany" are two separate
// values, never grouped) — exact match is what SearchJobs's own
// location filter now expects. See plan/ai/tools/career/step-55-jobs-
// page-location-dropdown-active-filters-live-search.md.
func ListDistinctJobLocations() ([]string, error) {
	rows, err := db.JobsDB.Query(`SELECT DISTINCT location FROM jobs WHERE location IS NOT NULL AND location != '' ORDER BY location`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	locations := []string{}
	for rows.Next() {
		var loc string
		if err := rows.Scan(&loc); err != nil {
			return nil, err
		}
		locations = append(locations, loc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return locations, nil
}

// LinkJobToCompany sets (or, if companyId is "", clears) a job's own
// company_id. A non-empty, unknown companyId is a real error
// (companies.ErrUnknownCompany), never a silent no-op — same
// RequireCompanyExists pattern the profile/persona domain already uses
// for its own parent references. See
// plan/ai/tools/career/step-14-companies.md.
func LinkJobToCompany(jobId, companyId string) error {
	if companyId != "" {
		if err := companies.RequireCompanyExists(companyId); err != nil {
			return err
		}
		_, err := db.JobsDB.Exec(`UPDATE jobs SET company_id = ? WHERE id = ?`, companyId, jobId)
		return err
	}
	_, err := db.JobsDB.Exec(`UPDATE jobs SET company_id = NULL WHERE id = ?`, jobId)
	return err
}

// errUnknownPortalLinkRef is a deliberately independent local copy of
// the portal package's own ErrUnknownPortalLink — see this file's own
// top comment for why this package can't import portal (a real
// cycle). Only checked within this package's own MCP tool
// registrations below (RegisterSavePortalJob/RegisterSavePortalJobs),
// never by any external caller — nothing outside this package needs it
// to be the SAME sentinel as portal's own.
var errUnknownPortalLinkRef = errors.New("unknown portal link id")

// SavePortalJob is the dedup-aware save for the portal-driven crawling
// workflow — see this file's own top comment and
// plan/ai/tools/career/step-20-portal-job-ingestion.md for why this is
// a separate tool from saveJob, and why the dedup key is
// (title, company) rather than also including postedAt.
//
// source_url identity is checked FIRST, before the title/company
// dedup check — a real ordering bug, not the original design, was
// caught live: checking title/company first meant a plain re-crawl of
// the exact same URL (the ordinary, expected case — an AI re-visiting
// a portal link it already crawled before) always matched its own
// already-saved row's own title/company and was reported as
// duplicate:true, so the source_url-based refresh path below could
// never actually run for any job this function had ever saved. Now:
// an exact source_url match is always treated as "this is the same
// posting, refresh it," regardless of title/company — the title/
// company check below only ever runs for a source_url this function
// hasn't seen before, which is exactly the cross-portal,
// different-URL case this step exists for.
func SavePortalJob(portalLinkId, sourceURL, title, company, location, description, postedAt string) (id string, created, duplicate bool, err error) {
	portalLinkURL, err := findPortalLinkURL(portalLinkId)
	if err != nil {
		return "", false, false, err
	}
	// Some portals return job posting URLs without a scheme/host (e.g.
	// "/job/path?query=x") — stored as-is, a browser resolves that
	// relative to whatever page it's currently on (this app's own
	// origin), not the portal's, so the link silently opens the wrong
	// site. Normalized here, once, before it's ever used as the
	// dedup/conflict key below, so the stored value is always the real,
	// absolute posting URL going forward. See
	// plan/ai/tools/career/step-XX-normalize-relative-job-urls.md.
	sourceURL = normalizeJobURL(sourceURL, portalLinkURL)

	var existingBySourceURL string
	err = db.JobsDB.QueryRow(`SELECT id FROM jobs WHERE source_url = ?`, sourceURL).Scan(&existingBySourceURL)
	switch {
	case err == sql.ErrNoRows:
		// a source_url this function hasn't seen before — check the
		// cross-portal title/company dedup below.
	case err != nil:
		return "", false, false, err
	default:
		id = existingBySourceURL
	}

	if id == "" {
		var existingByTitleCompany string
		err = db.JobsDB.QueryRow(
			`SELECT id FROM jobs WHERE LOWER(TRIM(title)) = ? AND LOWER(TRIM(company)) = ?`,
			toLower(strings.TrimSpace(title)), toLower(strings.TrimSpace(company)),
		).Scan(&existingByTitleCompany)
		switch {
		case err == sql.ErrNoRows:
			id = uuid.NewString()
			created = true
		case err != nil:
			return "", false, false, err
		default:
			return existingByTitleCompany, false, true, nil
		}
	}

	_, err = db.JobsDB.Exec(
		`INSERT INTO jobs (id, source_url, title, company, portal_link_id, location, description, posted_at, crawled_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		 ON CONFLICT(source_url) DO UPDATE SET
			title = excluded.title,
			company = excluded.company,
			portal_link_id = excluded.portal_link_id,
			location = excluded.location,
			description = excluded.description,
			posted_at = excluded.posted_at,
			crawled_at = excluded.crawled_at`,
		id, sourceURL, title, company, portalLinkId, location, description, postedAt,
	)
	if err != nil {
		return "", false, false, err
	}
	return id, created, false, nil
}

// findPortalLinkURL is portal's own RequirePortalLinkExists SELECT,
// just also returning the row's own url — SavePortalJob needs both
// "does this portal link exist" and "what's its own crawl-target URL"
// in one round trip. A deliberately independent query against
// portal_links directly (rather than calling into the portal package,
// which would create the import cycle this file's own top comment
// explains) — nearly a dozen other call sites in the portal package
// itself only ever use its own RequirePortalLinkExists for the
// existence check and don't need the extra column for.
func findPortalLinkURL(id string) (string, error) {
	var linkURL string
	err := db.JobsDB.QueryRow(`SELECT url FROM portal_links WHERE id = ?`, id).Scan(&linkURL)
	switch {
	case err == sql.ErrNoRows:
		return "", errUnknownPortalLinkRef
	case err != nil:
		return "", err
	}
	return linkURL, nil
}

// normalizeJobURL resolves sourceURL against portalLinkURL (that
// job's own portal link — the URL that was actually crawled) whenever
// sourceURL doesn't already carry its own scheme, using the same
// standard RFC 3986 resolution net/url already implements (the same
// rule a browser uses to resolve a relative link found on a page
// against that page's own URL) — correct for a plain path
// ("/job/x?q=1"), a path without a leading slash, and a
// protocol-relative URL ("//example.com/x", which already names its
// own domain, just not the scheme) alike, with no hand-rolled string
// concatenation. Leaves an already-absolute sourceURL completely
// unchanged. Falls back to returning sourceURL as-is (rather than
// erroring) if either URL fails to parse or portalLinkURL itself isn't
// absolute — this is a best-effort correction, not a validator; a job
// this can't fix is no worse off than it was before this function
// existed.
func normalizeJobURL(sourceURL, portalLinkURL string) string {
	parsed, err := url.Parse(sourceURL)
	if err != nil || parsed.IsAbs() {
		return sourceURL
	}
	base, err := url.Parse(portalLinkURL)
	if err != nil || !base.IsAbs() {
		return sourceURL
	}
	return base.ResolveReference(parsed).String()
}

// toLower avoids pulling in strings just for this one call site's own
// ASCII-only need (search queries are expected to be plain job titles/
// companies, not text needing full Unicode case-folding).
func toLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// ClampLimit applies the default/ceiling this step's own design
// specifies — 0 or negative means "use the default," anything above
// the ceiling is clamped to it.
func ClampLimit(limit int) int {
	if limit <= 0 {
		return defaultJobsLimit
	}
	if limit > maxJobsLimit {
		return maxJobsLimit
	}
	return limit
}

// --- MCP registration ---

type saveJobArgs struct {
	SourceURL   string `json:"sourceUrl" jsonschema:"the posting's own canonical URL — used to detect an already-saved posting"`
	Title       string `json:"title" jsonschema:"the job title"`
	Company     string `json:"company,omitempty" jsonschema:"the hiring company"`
	Location    string `json:"location,omitempty" jsonschema:"where the job is located"`
	Description string `json:"description,omitempty" jsonschema:"the posting's own description text"`
	PostedAt    string `json:"postedAt,omitempty" jsonschema:"when the posting says it went up, as scraped (free text — formats vary too much per site to normalize)"`
}

func RegisterSaveJob(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "save_job",
		Description: "Save a crawled job posting. Saving the same sourceUrl again updates the existing entry instead of duplicating it.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args saveJobArgs) (*mcp.CallToolResult, any, error) {
		if args.SourceURL == "" || args.Title == "" {
			return db.ErrResult("sourceUrl and title are both required"), nil, nil
		}
		id, created, err := saveJob(args.SourceURL, args.Title, args.Company, args.Location, args.Description, args.PostedAt)
		if err != nil {
			return db.ErrResult(fmt.Sprintf("failed to save job: %v", err)), nil, nil
		}
		return db.JSONResult(map[string]any{"id": id, "created": created})
	})
}

type listJobsArgs struct {
	CompanyID    string `json:"companyId,omitempty" jsonschema:"restrict to jobs linked to this company (see link_job_to_company); omit for every job"`
	PortalLinkID string `json:"portalLinkId,omitempty" jsonschema:"restrict to jobs found via this specific portal link (see save_portal_job); omit for every job"`
	PortalID     string `json:"portalId,omitempty" jsonschema:"restrict to jobs found via ANY link belonging to this portal (see list_portals); omit for every job"`
	Limit        int    `json:"limit,omitempty" jsonschema:"max results to return (default 50, max 200)"`
	Offset       int    `json:"offset,omitempty" jsonschema:"how many matching results to skip, for paging"`
}

func RegisterListJobs(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_jobs",
		Description: "List saved job postings, newest first.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listJobsArgs) (*mcp.CallToolResult, any, error) {
		result, err := listJobs(args.CompanyID, args.PortalLinkID, args.PortalID, ClampLimit(args.Limit), args.Offset)
		if err != nil {
			return db.ErrResult(fmt.Sprintf("failed to list jobs: %v", err)), nil, nil
		}
		return db.JSONResult(result)
	})
}

type searchJobsArgs struct {
	Query        string `json:"query,omitempty" jsonschema:"matches against title/company/description"`
	Location     string `json:"location,omitempty" jsonschema:"restrict to postings whose location is exactly this value (case-sensitive, no partial match) — see list_jobs's own results for real values"`
	CompanyID    string `json:"companyId,omitempty" jsonschema:"restrict to jobs linked to this company (see link_job_to_company); omit for every matching job"`
	PortalLinkID string `json:"portalLinkId,omitempty" jsonschema:"restrict to jobs found via this specific portal link (see save_portal_job); omit for every matching job"`
	PortalID     string `json:"portalId,omitempty" jsonschema:"restrict to jobs found via ANY link belonging to this portal (see list_portals); omit for every matching job"`
	Limit        int    `json:"limit,omitempty" jsonschema:"max results to return (default 50, max 200)"`
	Offset       int    `json:"offset,omitempty" jsonschema:"how many matching results to skip, for paging"`
}

func RegisterSearchJobs(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_jobs",
		Description: "Search saved job postings by query, location, company, and/or portal (or a specific portal link). Omitting all is equivalent to list_jobs.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args searchJobsArgs) (*mcp.CallToolResult, any, error) {
		result, err := SearchJobs(args.Query, args.Location, args.CompanyID, args.PortalLinkID, args.PortalID, ClampLimit(args.Limit), args.Offset)
		if err != nil {
			return db.ErrResult(fmt.Sprintf("failed to search jobs: %v", err)), nil, nil
		}
		return db.JSONResult(result)
	})
}

type linkJobToCompanyArgs struct {
	JobID     string `json:"jobId" jsonschema:"the job's own id, from save_job/list_jobs/search_jobs"`
	CompanyID string `json:"companyId" jsonschema:"the company's own id, from create_company/list_companies; empty string unlinks the job"`
}

func RegisterLinkJobToCompany(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "link_job_to_company",
		Description: "Link a saved job posting to a company (or, with an empty companyId, unlink it).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args linkJobToCompanyArgs) (*mcp.CallToolResult, any, error) {
		if args.JobID == "" {
			return db.ErrResult("jobId is required"), nil, nil
		}
		if err := LinkJobToCompany(args.JobID, args.CompanyID); err != nil {
			if errors.Is(err, companies.ErrUnknownCompany) {
				return db.ErrResult(fmt.Sprintf("unknown company id %q", args.CompanyID)), nil, nil
			}
			return db.ErrResult(fmt.Sprintf("failed to link job to company: %v", err)), nil, nil
		}
		if args.CompanyID == "" {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "unlinked"}}}, nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "linked"}}}, nil, nil
	})
}

type savePortalJobArgs struct {
	PortalLinkID string `json:"portalLinkId" jsonschema:"the portal link this job was found via — must already exist (see add_portal_link/list_portals)"`
	SourceURL    string `json:"sourceUrl" jsonschema:"the posting's own canonical URL"`
	Title        string `json:"title" jsonschema:"the job title"`
	Company      string `json:"company" jsonschema:"the hiring company"`
	Location     string `json:"location,omitempty" jsonschema:"where the job is located"`
	Description  string `json:"description,omitempty" jsonschema:"the posting's own description text"`
	PostedAt     string `json:"postedAt,omitempty" jsonschema:"when the posting says it went up, as scraped (free text — not used for duplicate detection, see this tool's own description)"`
}

func RegisterSavePortalJob(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "save_portal_job",
		Description: "Save a job posting found via a portal link, with cross-portal duplicate detection: before " +
			"inserting, checks for an existing job with a case-insensitive, trimmed match on both title and " +
			"company (not sourceUrl, and not postedAt — free-text dates vary too much across portals to check " +
			"reliably) — if found, the existing job is left untouched and duplicate:true is returned instead of " +
			"inserting a second copy of the same real posting found via a different URL. Use this instead of " +
			"save_job for anything found through a portal link.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args savePortalJobArgs) (*mcp.CallToolResult, any, error) {
		if args.PortalLinkID == "" {
			return db.ErrResult("portalLinkId is required"), nil, nil
		}
		if args.SourceURL == "" || args.Title == "" || args.Company == "" {
			return db.ErrResult("sourceUrl, title, and company are all required"), nil, nil
		}
		id, created, duplicate, err := SavePortalJob(args.PortalLinkID, args.SourceURL, args.Title, args.Company, args.Location, args.Description, args.PostedAt)
		if err != nil {
			if errors.Is(err, errUnknownPortalLinkRef) {
				return db.ErrResult(fmt.Sprintf("unknown portal link id %q", args.PortalLinkID)), nil, nil
			}
			return db.ErrResult(fmt.Sprintf("failed to save portal job: %v", err)), nil, nil
		}
		return db.JSONResult(map[string]any{"id": id, "created": created, "duplicate": duplicate})
	})
}

// savePortalJobInput/savePortalJobsArgs (step 29) — the batch sibling
// of save_portal_job: the same required/optional fields, one entry
// per job. Exists specifically so a crawl finding N jobs on one page
// costs one tool call, not N — see this tool's own description below
// for why that matters (api/src/conversation/chat.go's own
// maxToolIterations, a real ceiling on tool round-trips per turn).
type savePortalJobInput struct {
	SourceURL   string `json:"sourceUrl" jsonschema:"the posting's own canonical URL"`
	Title       string `json:"title" jsonschema:"the job title"`
	Company     string `json:"company" jsonschema:"the hiring company"`
	Location    string `json:"location,omitempty" jsonschema:"where the job is located"`
	Description string `json:"description,omitempty" jsonschema:"the posting's own description text"`
	PostedAt    string `json:"postedAt,omitempty" jsonschema:"when the posting says it went up, as scraped (free text — not used for duplicate detection)"`
}

type savePortalJobsArgs struct {
	PortalLinkID string               `json:"portalLinkId" jsonschema:"the portal link every job in this batch was found via — must already exist"`
	Jobs         []savePortalJobInput `json:"jobs" jsonschema:"every job found on this crawl — pass them all in one call, never call save_portal_job job-by-job"`
}

func RegisterSavePortalJobs(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "save_portal_jobs",
		Description: "Save every job posting found on a crawl in ONE call — pass the whole batch in `jobs`, " +
			"never call save_portal_job (singular) once per job. Same cross-portal duplicate detection as " +
			"save_portal_job, applied per entry: a job matching an existing one (case-insensitive, trimmed " +
			"title+company) is left untouched and counted as a duplicate rather than inserted again. Use this " +
			"as the last step of a manual crawl, after crawl_paginated — this is what keeps a page listing many " +
			"jobs from needing many separate tool calls.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args savePortalJobsArgs) (*mcp.CallToolResult, any, error) {
		if args.PortalLinkID == "" {
			return db.ErrResult("portalLinkId is required"), nil, nil
		}
		if len(args.Jobs) == 0 {
			return db.ErrResult("jobs must have at least one entry"), nil, nil
		}

		var saved, updated, duplicates, skipped int
		for _, j := range args.Jobs {
			if j.SourceURL == "" || j.Title == "" || j.Company == "" {
				skipped++
				continue
			}
			_, created, duplicate, err := SavePortalJob(args.PortalLinkID, j.SourceURL, j.Title, j.Company, j.Location, j.Description, j.PostedAt)
			if err != nil {
				if errors.Is(err, errUnknownPortalLinkRef) {
					return db.ErrResult(fmt.Sprintf("unknown portal link id %q", args.PortalLinkID)), nil, nil
				}
				return db.ErrResult(fmt.Sprintf("failed to save portal job %q: %v", j.Title, err)), nil, nil
			}
			switch {
			case duplicate:
				duplicates++
			case created:
				saved++
			default:
				updated++
			}
		}
		return db.JSONResult(map[string]any{
			"jobsProcessed": len(args.Jobs),
			"saved":         saved,
			"updated":       updated,
			"duplicates":    duplicates,
			"skipped":       skipped,
		})
	})
}

type deleteJobArgs struct {
	ID string `json:"id" jsonschema:"the job's own id, from save_job/list_jobs/search_jobs"`
}

func RegisterDeleteJob(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_job",
		Description: "Delete one saved job posting.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args deleteJobArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return db.ErrResult("id is required"), nil, nil
		}
		if err := DeleteJob(args.ID); err != nil {
			return db.ErrResult(fmt.Sprintf("failed to delete job: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "deleted"}}}, nil, nil
	})
}

// ErrUnknownJob is a real, actionable error — unlike LinkJobToCompany
// above (a pre-existing silent no-op on an unknown jobId),
// SaveJobDetailExtraction's own caller (eventually a sub-agent spawned
// by crawl_urls_with_subagents, see plan/ai/tools/career/
// step-XX-job-detail-crawl-instructions.md) needs a real signal that
// its extraction had nowhere to go, not a silently-ignored write.
var ErrUnknownJob = errors.New("unknown job id")

func RequireJobExists(id string) error {
	var exists int
	err := db.JobsDB.QueryRow(`SELECT 1 FROM jobs WHERE id = ?`, id).Scan(&exists)
	switch {
	case err == sql.ErrNoRows:
		return ErrUnknownJob
	case err != nil:
		return err
	}
	return nil
}

// SaveJobDetailExtraction persists the output of a single job detail
// page's own crawl (step XX) — values is whatever
// job_detail_crawl_instructions' own fields produced, keyed by their
// own labels. Only values["description"] is ever written: the
// listing crawl (SavePortalJob) already owns title/company/location/
// postedAt, and job_detail_crawl_instructions' own required output
// key is description alone (see the portal package's own
// requiredJobDetailCrawlOutputKeys) — any other key present in values
// (e.g. a "requirements" field the instructions additionally declared)
// is accepted without error but not currently persisted anywhere.
//
// An empty/missing description records detail_crawl_status='failed'
// (step XX — the Jobs page's own Eye icon reads this to distinguish
// "crawled but got nothing" from "never crawled at all") without
// touching description/crawled_at — nothing to overwrite the existing
// (possibly listing-truncated) description with, and crawled_at stays
// whatever the listing crawl (or a previous successful detail crawl)
// last set it to.
func SaveJobDetailExtraction(jobId string, values map[string]string) (updated bool, err error) {
	if err := RequireJobExists(jobId); err != nil {
		return false, err
	}
	description := strings.TrimSpace(values["description"])
	if description == "" {
		return false, MarkJobDetailCrawlFailed(jobId)
	}
	_, err = db.JobsDB.Exec(
		`UPDATE jobs SET description = ?, crawled_at = datetime('now'), detail_crawl_status = 'success' WHERE id = ?`,
		description, jobId,
	)
	if err != nil {
		return false, err
	}
	return true, nil
}

// MarkJobDetailCrawlFailed records that a job-detail-page crawl
// attempt for jobId was made and did not produce a usable description
// — called both by SaveJobDetailExtraction above (extraction ran but
// came back empty) and directly by the crawl package's own
// runJobDetailCrawlNow for a harder failure that never even reached
// that function (the page fetch itself erroring, or no page extracted
// at all) — both cases look identical to the Jobs page's own Eye icon,
// which only needs to know "an attempt was made and it didn't work,"
// not why. Deliberately does not require the caller to re-check
// RequireJobExists — every call site already has a jobId it just
// successfully used moments earlier.
func MarkJobDetailCrawlFailed(jobId string) error {
	_, err := db.JobsDB.Exec(`UPDATE jobs SET detail_crawl_status = 'failed' WHERE id = ?`, jobId)
	return err
}

type saveJobDetailExtractionArgs struct {
	JobID  string            `json:"jobId" jsonschema:"the job's own id, from save_portal_job/list_jobs/search_jobs — this is the job whose detail page was just crawled"`
	Values map[string]string `json:"values" jsonschema:"the extracted values, keyed by the label declared in this job's own portal link job_detail_crawl_instructions (see set_portal_link_job_detail_crawl_instructions) — must include description"`
}

func RegisterSaveJobDetailExtraction(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "save_job_detail_extraction",
		Description: "Save the text extracted off one job's own detail page (per its portal link's own " +
			"job_detail_crawl_instructions) back onto that job's existing record — replacing its own " +
			"(often listing-truncated) description with the full text. Call this once per job, after " +
			"extracting its detail page; values must include a description key. Fails if jobId is unknown.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args saveJobDetailExtractionArgs) (*mcp.CallToolResult, any, error) {
		if args.JobID == "" {
			return db.ErrResult("jobId is required"), nil, nil
		}
		if len(args.Values) == 0 {
			return db.ErrResult("values is required"), nil, nil
		}
		updated, err := SaveJobDetailExtraction(args.JobID, args.Values)
		if err != nil {
			if errors.Is(err, ErrUnknownJob) {
				return db.ErrResult(fmt.Sprintf("unknown job id %q", args.JobID)), nil, nil
			}
			return db.ErrResult(fmt.Sprintf("failed to save job detail extraction: %v", err)), nil, nil
		}
		return db.JSONResult(map[string]any{"updated": updated})
	})
}
