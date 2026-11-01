package jobs

import (
	"database/sql"
	"errors"
	"log"

	"career-tool-backend/db"
	"career-tool-backend/deletedjobs"
)

// Remembering deleted jobs (step 133). Before a crawled job is deleted, a snapshot of it is written to the deleted-jobs
// database, found again later by the hash of its URL. The memory is written first and the job deleted second: the two
// are separate files and cannot share a transaction, and a crash between them leaves an entry for a job that still
// exists, which is harmless, while the other order could lose the entry. See
// plan/ai/tools/career/step-133-deleted-jobs-filter.md.

// ErrDeletedBefore is returned by a crawl's save (saveJob, SavePortalJob) for a job whose URL was deleted before and is
// remembered: nothing is inserted, no event is sent. Callers count it as "ignored", not as a failure.
var ErrDeletedBefore = errors.New("this job was deleted before and is not added again")

// rememberedDeleted says whether the crawler must skip a job it is about to insert. It is only asked for a URL that has
// no row yet: a live job is always refreshed. A memory that cannot be read counts as "nothing remembered" (and is
// logged), because a missing safety net must never stop jobs from being saved.
func rememberedDeleted(sourceURL string) bool {
	found, err := deletedjobs.Has(sourceURL)
	if err != nil {
		if !errors.Is(err, deletedjobs.ErrNoMemory) {
			log.Printf("deleted jobs: could not look up %q: %v", sourceURL, err)
		}
		return false
	}
	return found
}

// forgetDeleted takes a URL out of the memory because a person added the job by hand (best effort, logged).
func forgetDeleted(sourceURL string) {
	if err := deletedjobs.Forget(sourceURL); err != nil && !errors.Is(err, deletedjobs.ErrNoMemory) && !errors.Is(err, deletedjobs.ErrNoKey) {
		log.Printf("deleted jobs: could not forget %q: %v", sourceURL, err)
	}
}

// RememberBeforeDelete records the job in the deleted-jobs database with the reason, and returns forget, which takes the
// entry back again if (and only if) this call created it: for a delete that then removes nothing. It never fails the
// delete: a job that is not worth remembering (one added by hand, one that no longer exists) or any problem with the
// memory (the database is unavailable, a write fails) gives a forget that does nothing, and a problem is logged. It
// must be called outside a JobsDB transaction (it reads JobsDB, which has a single connection).
func RememberBeforeDelete(jobID, reason string) (forget func()) {
	nothing := func() {}
	entry, ok, err := snapshotForMemory(jobID, reason)
	if err != nil {
		log.Printf("deleted jobs: could not read job %s to remember it: %v", jobID, err)
		return nothing
	}
	if !ok {
		return nothing
	}
	created, err := deletedjobs.Remember(entry)
	if err != nil {
		if !errors.Is(err, deletedjobs.ErrNoMemory) {
			log.Printf("deleted jobs: could not remember job %s: %v", jobID, err)
		}
		return nothing
	}
	if !created {
		return nothing
	}
	return func() {
		if err := deletedjobs.Forget(entry.SourceURL); err != nil {
			log.Printf("deleted jobs: could not take back the entry for job %s: %v", jobID, err)
		}
	}
}

// snapshotForMemory reads what is remembered about a job. ok is false for a job that is not remembered: it does not
// exist, or it was added by hand (a person typed it in, a crawl cannot bring it back).
func snapshotForMemory(jobID, reason string) (deletedjobs.Entry, bool, error) {
	var e deletedjobs.Entry
	var source string
	var score sql.NullInt64
	err := db.JobsDB.QueryRow(
		`SELECT j.source_url, j.title, COALESCE(j.company, ''), COALESCE(j.location, ''), COALESCE(j.posted_at, ''), j.source,
			COALESCE(p.name, ''), jm.score
		 FROM jobs j
		 LEFT JOIN portal_links pl ON pl.id = j.portal_link_id
		 LEFT JOIN portals p ON p.id = pl.portal_id
		 LEFT JOIN job_matches jm ON jm.job_id = j.id
		 WHERE j.id = ?`, jobID,
	).Scan(&e.SourceURL, &e.Title, &e.Company, &e.Location, &e.PostedAt, &source, &e.PortalName, &score)
	if errors.Is(err, sql.ErrNoRows) {
		return deletedjobs.Entry{}, false, nil
	}
	if err != nil {
		return deletedjobs.Entry{}, false, err
	}
	if source != SourceCrawled {
		return deletedjobs.Entry{}, false, nil
	}
	if score.Valid {
		v := int(score.Int64)
		e.MatchScore = &v
	}
	e.Reason = reason
	return e, true, nil
}
