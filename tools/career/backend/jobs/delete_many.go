package jobs

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"career-tool-backend/db"
	"career-tool-backend/jobs/deletedjobs"
)

// Deleting many jobs at once (step 149): the Jobs list's batch delete. It is the single delete (DeleteJob) done for a list, in the
// recipe of the cleanup's batches: remember the crawled jobs, read what each job's deleted event will say, delete them, and announce
// "career.job.deleted" for each job that really went. Unlike the cleanup it has no safety guards: a person deleting on purpose is not
// stopped by a board card or a CV, exactly as with the single delete.

// deleteBatchSize is how many jobs one transaction deletes, so the SQLite write lock is never held long while a crawl inserts.
const deleteBatchSize = 200

// DeleteManyResult says what DeleteMany did.
type DeleteManyResult struct {
	Deleted  int          // jobs whose row was removed
	NotFound int          // ids that no job had (already deleted, or never existed)
	Events   []DeletedJob // what the deleted events say, for the jobs that were removed (send them with PublishJobDeleted)
}

// DeleteMany deletes the jobs with these ids, deleteBatchSize per transaction. An id counts once however often it appears. A crawled
// job is remembered in the deleted-jobs database before it is deleted (so a crawl does not add it again) and the entry is taken
// back for any id whose delete removed nothing; a job added by hand is not remembered. Each batch is atomic, the whole is not: a
// failure stops at that batch, keeps the earlier ones and returns what was done with the error. No event is sent here: the caller
// sends res.Events (see PublishDeletedEvents), also after an error, for the batches that did commit.
func DeleteMany(ids []string) (DeleteManyResult, error) {
	var res DeleteManyResult
	seen := make(map[string]bool, len(ids))
	order := make([]string, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			order = append(order, id)
		}
	}
	for from := 0; from < len(order); from += deleteBatchSize {
		batch := order[from:min(len(order), from+deleteBatchSize)]
		if err := deleteBatchOf(batch, &res); err != nil {
			return res, err
		}
	}
	return res, nil
}

func deleteBatchOf(batch []string, res *DeleteManyResult) error {
	// The memory and the snapshots read JobsDB, which has one connection, so they are done before the transaction.
	forgets := make(map[string]func(), len(batch))
	snapshots := make(map[string]DeletedJob, len(batch))
	for _, id := range batch {
		forgets[id] = RememberBeforeDelete(id, deletedjobs.ReasonDeleted)
		if snap, ok := SnapshotForDeletedEvent(id); ok {
			snapshots[id] = snap
		}
	}
	takeBack := func(except map[string]bool) {
		for id, forget := range forgets {
			if !except[id] {
				forget()
			}
		}
	}

	tx, err := db.JobsDB.Begin()
	if err != nil {
		takeBack(nil)
		return err
	}
	gone := make(map[string]bool, len(batch))
	for _, id := range batch {
		r, err := tx.Exec(`DELETE FROM jobs WHERE id = ?`, id)
		if err != nil {
			_ = tx.Rollback()
			takeBack(nil)
			return err
		}
		if n, _ := r.RowsAffected(); n > 0 {
			gone[id] = true
		}
	}
	if err := tx.Commit(); err != nil {
		takeBack(nil) // nothing was deleted
		return err
	}
	takeBack(gone) // an id that removed nothing is not remembered either
	for _, id := range batch {
		if !gone[id] {
			res.NotFound++
			continue
		}
		res.Deleted++
		if snap, ok := snapshots[id]; ok {
			res.Events = append(res.Events, snap)
		}
	}
	return nil
}

const (
	// maxDeleteBody bounds the size of one request's JSON (about two hundred thousand ids): a safety against a broken client, not a
	// limit on a batch.
	maxDeleteBody = 8 << 20
	// deletePublishParallelism is how many deleted events are being published at once: each publish is a call to core.
	deletePublishParallelism = 4
)

// publishDeleted sends one deleted event; a seam so a test can see and hold the events.
var publishDeleted = PublishJobDeleted

// PublishDeletedEvents sends "career.job.deleted" for each job, a few at a time, and returns when all are sent. Best effort like
// every domain event.
func PublishDeletedEvents(events []DeletedJob) {
	sem := make(chan struct{}, deletePublishParallelism)
	var wg sync.WaitGroup
	for _, ev := range events {
		sem <- struct{}{}
		wg.Add(1)
		go func(ev DeletedJob) {
			defer wg.Done()
			defer func() { <-sem }()
			publishDeleted(ev)
		}(ev)
	}
	wg.Wait()
}

// DeleteManyHandler handles POST jobs/delete: {"jobIds": [...]} deletes those jobs (see DeleteMany) and answers 200
// {deleted, notFound}; after the answer is sent it announces each deleted job in the background. A failure part way answers 500
// {error, deleted, notFound} with what was done, and the events of what was deleted still go out.
func DeleteManyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		JobIDs []string `json:"jobIds"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxDeleteBody)).Decode(&body); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(body.JobIDs) == 0 {
		http.Error(w, "jobIds is required: at least one job id", http.StatusBadRequest)
		return
	}
	for _, id := range body.JobIDs {
		if id == "" {
			http.Error(w, "jobIds must not contain an empty id", http.StatusBadRequest)
			return
		}
	}
	res, err := DeleteMany(body.JobIDs)
	log.Printf("jobs: deleted %d job(s), %d not found (asked for %d)%s", res.Deleted, res.NotFound, len(body.JobIDs), errSuffix(err))
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to delete the jobs: " + err.Error(), "deleted": res.Deleted, "notFound": res.NotFound})
	} else {
		_ = json.NewEncoder(w).Encode(map[string]int{"deleted": res.Deleted, "notFound": res.NotFound})
	}
	go PublishDeletedEvents(res.Events)
}

func errSuffix(err error) string {
	if err == nil {
		return ""
	}
	return ": stopped by an error: " + err.Error()
}
