// Package query — turn_runs table reads. See
// plan/ai/conversation/step-23-detach-turn-execution-from-request.md.
package query

import (
	"database/sql"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
)

type TurnRunQueryRepo struct {
	db *sql.DB
}

func NewTurnRunQueryRepo(db *sql.DB) *TurnRunQueryRepo {
	return &TurnRunQueryRepo{db: db}
}

const turnRunColumns = `id, conversation_id, user_content, status, started_at, finished_at, log, cancel_requested`

func scanTurnRun(scan func(dest ...any) error) (*conversation_entity.TurnRun, error) {
	var t conversation_entity.TurnRun
	if err := scan(&t.ID, &t.ConversationID, &t.UserContent, &t.Status, &t.StartedAt, &t.FinishedAt, &t.Log, &t.CancelRequested); err != nil {
		return nil, err
	}
	return &t, nil
}

// FindActiveByConversationID returns the one running turn_runs row for
// conversationID, if any — sql.ErrNoRows (unwrapped) when nothing is
// running. The DB's own turn_runs_one_running_idx partial unique index
// guarantees there is never more than one to find.
func (r *TurnRunQueryRepo) FindActiveByConversationID(conversationID string) (*conversation_entity.TurnRun, error) {
	row := r.db.QueryRow(`SELECT `+turnRunColumns+` FROM turn_runs WHERE conversation_id = ? AND status = 'running' LIMIT 1`, conversationID)
	return scanTurnRun(row.Scan)
}

// FindMostRecentByConversationID returns conversationID's own most
// recently started turn_runs row, regardless of status — used by the
// polling endpoint (GetActiveTurnHandler), deliberately NOT restricted
// to status='running' the way FindActiveByConversationID is: a
// poller's in-flight request racing the exact moment a run finishes
// must still see that row (now terminal), never a 404 that would
// leave it unable to tell "finished" apart from "never existed" or
// "server restarted and lost it."
func (r *TurnRunQueryRepo) FindMostRecentByConversationID(conversationID string) (*conversation_entity.TurnRun, error) {
	row := r.db.QueryRow(`SELECT `+turnRunColumns+` FROM turn_runs WHERE conversation_id = ? ORDER BY started_at DESC LIMIT 1`, conversationID)
	return scanTurnRun(row.Scan)
}

// FindByID returns sql.ErrNoRows (unwrapped) if no row has this ID.
// Ownership is enforced by callers via the owning conversation (the
// same conversation_query.FindOwnedByID check every handler in this
// feature already does), not here — matches conversation_query.
// FindByID's own established split between an unfiltered lookup and
// an ownership-scoped one.
func (r *TurnRunQueryRepo) FindByID(id string) (*conversation_entity.TurnRun, error) {
	row := r.db.QueryRow(`SELECT `+turnRunColumns+` FROM turn_runs WHERE id = ? LIMIT 1`, id)
	return scanTurnRun(row.Scan)
}

// FindAllRunning returns every row still marked "running" — used once
// at boot to reconcile turns orphaned by a server restart. A goroutine
// has no PID to reattach to (unlike tool/manager.go's supervised OS
// subprocesses), so every such row is, by definition, dead the moment
// the process that ran it is gone.
func (r *TurnRunQueryRepo) FindAllRunning() ([]*conversation_entity.TurnRun, error) {
	rows, err := r.db.Query(`SELECT ` + turnRunColumns + ` FROM turn_runs WHERE status = 'running'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*conversation_entity.TurnRun, 0)
	for rows.Next() {
		t, err := scanTurnRun(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
