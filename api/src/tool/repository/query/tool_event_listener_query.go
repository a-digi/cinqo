package query

import (
	"database/sql"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
)

type ToolEventListenerQueryRepo struct {
	db *sql.DB
}

func NewToolEventListenerQueryRepo(db *sql.DB) *ToolEventListenerQueryRepo {
	return &ToolEventListenerQueryRepo{db: db}
}

func scanToolEventListener(scan func(dest ...any) error) (tool_entity.ToolEventListener, error) {
	var l tool_entity.ToolEventListener
	err := scan(&l.ToolID, &l.Topic, &l.PathSuffix)
	return l, err
}

// FindByTopic returns every listener declared for topic, belonging to a
// currently enabled AND running tool — the real delivery candidate set
// (plan/ai/domain-events/step-04-core-to-tool-delivery.md reads this).
// Mirrors ToolMCPToolQueryRepo.FindAllEnabled's own join shape, with the
// added status='running' filter: a listener belonging to a tool that's
// enabled but not currently running has nowhere to be delivered to.
func (r *ToolEventListenerQueryRepo) FindByTopic(topic string) ([]tool_entity.ToolEventListener, error) {
	rows, err := r.db.Query(
		`SELECT e.tool_id, e.topic, e.path_suffix
		FROM tool_event_listeners e
		JOIN tools t ON t.id = e.tool_id
		WHERE t.enabled = 1 AND t.status = 'running' AND e.topic = ?`,
		topic,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]tool_entity.ToolEventListener, 0)
	for rows.Next() {
		l, err := scanToolEventListener(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
