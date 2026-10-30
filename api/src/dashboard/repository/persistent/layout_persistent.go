// Package persistent writes the Home dashboard's per-user layout.
package persistent

import (
	"database/sql"

	"github.com/a-digi/cinqo/src/dashboard"
)

type LayoutPersistentRepo struct {
	db *sql.DB
}

func NewLayoutPersistentRepo(db *sql.DB) *LayoutPersistentRepo { return &LayoutPersistentRepo{db: db} }

// ReplaceAll wholesale-replaces one user's layout, in one transaction. An empty
// list clears it, which returns the user to the default arrangement.
func (r *LayoutPersistentRepo) ReplaceAll(userID string, entries []dashboard.LayoutEntry) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM dashboard_layout WHERE user_id = ?`, userID); err != nil {
		return err
	}
	for i, e := range entries {
		hidden := 0
		if e.Hidden {
			hidden = 1
		}
		if _, err := tx.Exec(
			`INSERT INTO dashboard_layout (user_id, widget_id, position, hidden) VALUES (?, ?, ?, ?)`,
			userID, e.WidgetID, i, hidden,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}
