package persistent

import (
	"database/sql"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
)

type ToolEventListenerPersistentRepo struct {
	db *sql.DB
}

func NewToolEventListenerPersistentRepo(db *sql.DB) *ToolEventListenerPersistentRepo {
	return &ToolEventListenerPersistentRepo{db: db}
}

// ReplaceAll wholesale-replaces one tool's cached event_listeners
// declarations — called at install/update time. Structurally identical
// to ToolMCPToolPersistentRepo.ReplaceAll. Not cleared on disable, same
// reasoning as that one: a disabled tool's cached rows are simply never
// read, since step 4's delivery query filters on the parent tool's own
// enabled/running state.
func (r *ToolEventListenerPersistentRepo) ReplaceAll(toolID string, listeners []tool_entity.ToolEventListener) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM tool_event_listeners WHERE tool_id = ?`, toolID); err != nil {
		return err
	}
	for _, l := range listeners {
		if _, err := tx.Exec(
			`INSERT INTO tool_event_listeners (tool_id, topic, path_suffix) VALUES (?, ?, ?)`,
			toolID, l.Topic, l.PathSuffix,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}
