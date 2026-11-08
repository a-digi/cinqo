package jobs

import (
	"database/sql"
	"errors"
	"log"

	"career-tool-backend/db"
	"career-tool-backend/domainevent"
)

// DeletedJob is what a job-deleted event says about the job: read before the delete, because the row is gone by the time
// anything listens. CompanyID is the link to the companies table (empty when the job was never linked); Company is the
// free text of the crawl.
type DeletedJob struct {
	ID        string
	Title     string
	Company   string
	CompanyID string
	Source    string
}

// SnapshotForDeletedEvent reads what the event will say about a job that is about to be deleted. ok is false for a job
// that does not exist (nothing will be deleted, so nothing will be announced). A failed read is logged and also gives
// ok false: announcing the delete is best effort and must never fail it. It must be called outside a JobsDB transaction
// (JobsDB has a single connection).
func SnapshotForDeletedEvent(jobID string) (job DeletedJob, ok bool) {
	job.ID = jobID
	err := db.JobsDB.QueryRow(
		`SELECT title, COALESCE(company, ''), COALESCE(company_id, ''), source FROM jobs WHERE id = ?`, jobID,
	).Scan(&job.Title, &job.Company, &job.CompanyID, &job.Source)
	if errors.Is(err, sql.ErrNoRows) {
		return DeletedJob{}, false
	}
	if err != nil {
		log.Printf("job deleted event: could not read job %s: %v", jobID, err)
		return DeletedJob{}, false
	}
	return job, true
}

// PublishJobDeleted announces "career.job.deleted" for a job whose row was really removed (step 139). Best effort like
// every domain event, and synchronous: domainevent.Publish's own short timeout bounds it. Callers send it only after the
// delete succeeded and affected a row.
func PublishJobDeleted(job DeletedJob) {
	domainevent.Publish("career.job.deleted", map[string]string{
		"job_id": job.ID, "title": job.Title, "company": job.Company, "company_id": job.CompanyID, "source": job.Source,
	})
}
