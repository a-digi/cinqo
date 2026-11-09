package persistent

import (
	"database/sql"
	"encoding/json"

	"github.com/a-digi/cinqo/src/system/runs/entity"
)

type RunsPersistentRepo struct {
	db *sql.DB
}

func NewRunsPersistentRepo(db *sql.DB) *RunsPersistentRepo { return &RunsPersistentRepo{db: db} }

// InsertRun records a run that has just started.
func (r *RunsPersistentRepo) InsertRun(run entity.Run) error {
	_, err := r.db.Exec(`INSERT INTO system_runs (id, started_at, last_seen_at, ended_at, clean, pid, process_start, last_heartbeat) VALUES (?, ?, ?, NULL, 0, ?, ?, ?)`,
		run.ID, run.StartedAt, run.LastSeenAt, run.PID, run.ProcessStart, run.LastHeartbeat)
	return err
}

// Heartbeat updates the one row of a living run: when it was last seen and what it knew then.
func (r *RunsPersistentRepo) Heartbeat(id, at, heartbeatJSON string) error {
	_, err := r.db.Exec(`UPDATE system_runs SET last_seen_at = ?, last_heartbeat = ? WHERE id = ? AND ended_at IS NULL`, at, heartbeatJSON, id)
	return err
}

// CloseRun ends a run: cleanly (the app said it was stopping) or not (found dead at the next start, ended when it was last seen).
func (r *RunsPersistentRepo) CloseRun(id, endedAt string, clean bool) error {
	c := 0
	if clean {
		c = 1
	}
	_, err := r.db.Exec(`UPDATE system_runs SET ended_at = ?, clean = ? WHERE id = ? AND ended_at IS NULL`, endedAt, c, id)
	return err
}

// InsertEvent stores an event with its evidence.
func (r *RunsPersistentRepo) InsertEvent(e entity.Event) error {
	ev := e.Evidence
	if ev == nil {
		ev = []entity.EvidenceItem{}
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`INSERT INTO system_events (id, at, kind, verdict, evidence, acknowledged_at) VALUES (?, ?, ?, ?, ?, NULL)`, e.ID, e.At, e.Kind, e.Verdict, string(raw))
	return err
}

// Acknowledge marks an event as seen. found is false for an id that does not exist; marking an event that is already seen changes nothing and is not an
// error (the first time it was seen is kept).
func (r *RunsPersistentRepo) Acknowledge(id, at string) (found bool, err error) {
	res, err := r.db.Exec(`UPDATE system_events SET acknowledged_at = COALESCE(acknowledged_at, ?) WHERE id = ?`, at, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// Prune keeps the newest keepRuns finished runs (a run that is still open is never removed) and the newest keepEvents events.
func (r *RunsPersistentRepo) Prune(keepRuns, keepEvents int) error {
	if _, err := r.db.Exec(`DELETE FROM system_runs WHERE ended_at IS NOT NULL AND id NOT IN (SELECT id FROM system_runs WHERE ended_at IS NOT NULL ORDER BY started_at DESC, id DESC LIMIT ?)`, keepRuns); err != nil {
		return err
	}
	_, err := r.db.Exec(`DELETE FROM system_events WHERE id NOT IN (SELECT id FROM system_events ORDER BY at DESC, id DESC LIMIT ?)`, keepEvents)
	return err
}
