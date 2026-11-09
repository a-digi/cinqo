package query

import (
	"database/sql"
	"encoding/json"

	"github.com/a-digi/cinqo/src/system/runs/entity"
)

type RunsQueryRepo struct {
	db *sql.DB
}

func NewRunsQueryRepo(db *sql.DB) *RunsQueryRepo { return &RunsQueryRepo{db: db} }

const runColumns = `id, started_at, last_seen_at, ended_at, clean, pid, process_start, last_heartbeat`

func scanRun(sc interface{ Scan(...any) error }) (entity.Run, error) {
	var run entity.Run
	var ended sql.NullString
	var clean int
	if err := sc.Scan(&run.ID, &run.StartedAt, &run.LastSeenAt, &ended, &clean, &run.PID, &run.ProcessStart, &run.LastHeartbeat); err != nil {
		return run, err
	}
	if ended.Valid {
		s := ended.String
		run.EndedAt = &s
	}
	run.Clean = clean == 1
	return run, nil
}

// OpenRuns lists the runs that never got an end (still running, or stopped without a trace), oldest first.
func (r *RunsQueryRepo) OpenRuns() ([]entity.Run, error) {
	rows, err := r.db.Query(`SELECT ` + runColumns + ` FROM system_runs WHERE ended_at IS NULL ORDER BY started_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []entity.Run{}
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

// FindRun returns one run, or sql.ErrNoRows.
func (r *RunsQueryRepo) FindRun(id string) (entity.Run, error) {
	return scanRun(r.db.QueryRow(`SELECT `+runColumns+` FROM system_runs WHERE id = ?`, id))
}

// CountRuns is the number of runs kept.
func (r *RunsQueryRepo) CountRuns() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM system_runs`).Scan(&n)
	return n, err
}

// Events lists the events, newest first.
func (r *RunsQueryRepo) Events() ([]entity.Event, error) {
	rows, err := r.db.Query(`SELECT id, at, kind, verdict, evidence, acknowledged_at FROM system_events ORDER BY at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []entity.Event{}
	for rows.Next() {
		var e entity.Event
		var evidence string
		var ack sql.NullString
		if err := rows.Scan(&e.ID, &e.At, &e.Kind, &e.Verdict, &evidence, &ack); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(evidence), &e.Evidence); err != nil || e.Evidence == nil {
			e.Evidence = []entity.EvidenceItem{}
		}
		if ack.Valid {
			s := ack.String
			e.AcknowledgedAt = &s
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
