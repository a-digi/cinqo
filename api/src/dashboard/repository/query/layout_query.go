// Package query reads the Home dashboard's per-user layout.
package query

import (
	"database/sql"

	"github.com/a-digi/cinqo/src/dashboard"
)

type LayoutQueryRepo struct {
	db *sql.DB
}

func NewLayoutQueryRepo(db *sql.DB) *LayoutQueryRepo { return &LayoutQueryRepo{db: db} }

// Find returns the user's saved arrangement in order.
func (r *LayoutQueryRepo) Find(userID string) ([]dashboard.LayoutEntry, error) {
	rows, err := r.db.Query(
		`SELECT widget_id, hidden FROM dashboard_layout WHERE user_id = ? ORDER BY position ASC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]dashboard.LayoutEntry, 0)
	for rows.Next() {
		var e dashboard.LayoutEntry
		var hidden int
		if err := rows.Scan(&e.WidgetID, &hidden); err != nil {
			return nil, err
		}
		e.Hidden = hidden != 0
		out = append(out, e)
	}
	return out, rows.Err()
}
