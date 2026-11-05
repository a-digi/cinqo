package jobdeletedconsumers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
	"career-tool-backend/match/personamatch"
)

func deliverPersona(method, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	PersonaMatchJobDeletedHandler(rec, httptest.NewRequest(method, "/events/job-deleted-persona-matches", strings.NewReader(body)))
	return rec
}

func personaRows(t *testing.T, jobID string) int {
	t.Helper()
	return rows(t, `SELECT COUNT(*) FROM job_persona_matches WHERE job_id = ?`, jobID)
}

func seedRows(t *testing.T, jobID string) {
	t.Helper()
	exec(t, `INSERT INTO jobs (id, source_url, title) VALUES (?, ?, 'T')`, jobID, "https://x.example/"+jobID)
	if err := personamatch.Replace(jobID, []personamatch.Row{{PersonaID: "pa", ProfileID: "A", Score: 40}, {PersonaID: "pb", ProfileID: "B", Score: 80}}); err != nil {
		t.Fatal(err)
	}
}

// The safety net: a row that survived its job (here forced by switching the foreign key off for one insert) is removed
// by the consumer, and only that job's rows.
func TestARowThatSurvivedItsJobIsRemovedByTheConsumer(t *testing.T) {
	setupDB(t)
	seedRows(t, "other") // another job's rows must stay
	// an orphan: a job id that does not exist, inserted with the cascade's foreign key off
	exec(t, `PRAGMA foreign_keys = OFF`)
	exec(t, `INSERT INTO job_persona_matches (job_id, persona_id, profile_id, score) VALUES ('gone-job', 'pa', 'A', 50)`)
	exec(t, `PRAGMA foreign_keys = ON`)
	if personaRows(t, "gone-job") != 1 {
		t.Fatal("the orphan row was not created")
	}
	if rec := deliverPersona(http.MethodPost, eventFor("gone-job", "")); rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if personaRows(t, "gone-job") != 0 {
		t.Error("the orphan row is still there")
	}
	if personaRows(t, "other") != 2 {
		t.Errorf("another job's rows = %d, want 2", personaRows(t, "other"))
	}
}

// The real chain: the job's own delete already removes the rows by the cascade, the event then finds nothing and answers 200.
func TestDeletingTheJobRemovesTheRowsAndTheConsumerFindsNothingLeft(t *testing.T) {
	setupDB(t)
	fakeCoreEvents(t)
	seedRows(t, "j1")
	seedRows(t, "j2")
	if err := jobs.DeleteJob("j1"); err != nil {
		t.Fatal(err)
	}
	if personaRows(t, "j1") != 0 {
		t.Error("the cascade must have removed the rows with the job")
	}
	if rec := deliverPersona(http.MethodPost, eventFor("j1", "")); rec.Code != http.StatusOK {
		t.Errorf("status %d", rec.Code)
	}
	if personaRows(t, "j2") != 2 {
		t.Errorf("the other job's rows = %d, want 2", personaRows(t, "j2"))
	}
}

func TestADeliveryTwiceAndAnUnknownJobAreNoOps(t *testing.T) {
	setupDB(t)
	for i := 0; i < 2; i++ {
		if rec := deliverPersona(http.MethodPost, eventFor("never-existed", "c1")); rec.Code != http.StatusOK {
			t.Errorf("delivery %d: status %d", i, rec.Code)
		}
	}
}

func TestPersonaMatchJobDeletedRejectsBadEventsAndMethods(t *testing.T) {
	setupDB(t)
	seedRows(t, "j1")
	for name, body := range map[string]string{
		"not json":     `nope`,
		"no payload":   `{"topic":"career.job.deleted"}`,
		"empty job id": `{"payload":{"job_id":"","company_id":"c1"}}`,
	} {
		if rec := deliverPersona(http.MethodPost, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, rec.Code)
		}
	}
	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		if rec := deliverPersona(m, eventFor("j1", "")); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status %d, want 405", m, rec.Code)
		}
	}
	if personaRows(t, "j1") != 2 {
		t.Error("nothing may be deleted for bad requests")
	}
}

func TestPersonaMatchJobDeletedDatabaseFailureIs500SoCoreRetries(t *testing.T) {
	setupDB(t)
	db.JobsDB.Close() // every query now fails
	if rec := deliverPersona(http.MethodPost, eventFor("j1", "")); rec.Code != http.StatusInternalServerError {
		t.Errorf("status %d, want 500", rec.Code)
	}
}
