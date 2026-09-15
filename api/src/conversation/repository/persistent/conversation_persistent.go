package persistent

import (
	"database/sql"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
)

type ConversationPersistentRepo struct {
	db *sql.DB
}

func NewConversationPersistentRepo(db *sql.DB) *ConversationPersistentRepo {
	return &ConversationPersistentRepo{db: db}
}

// Insert writes platform_id/model once, at creation — no method in
// this repo ever updates them afterward (see the entity's own doc
// comment on why that's a structural, not runtime, guarantee). hidden
// (step 30) is likewise set only here — nothing ever flips it after
// creation.
func (r *ConversationPersistentRepo) Insert(c *conversation_entity.Conversation) error {
	_, err := r.db.Exec(
		`INSERT INTO conversations (id, user_id, title, file_path, platform_id, model, hidden) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.UserID, c.Title, c.FilePath, c.PlatformID, c.Model, c.Hidden,
	)
	return err
}

// UpdateTitle renames a conversation. Ownership is the caller's own
// responsibility — the handler already verified it via
// FindOwnedByID before this is ever called.
func (r *ConversationPersistentRepo) UpdateTitle(id, title string) error {
	_, err := r.db.Exec(`UPDATE conversations SET title = ? WHERE id = ?`, title, id)
	return err
}

// Delete removes the conversations row — row only, no message table
// to cascade (step 1 moved content out of SQL entirely). Returns
// sql.ErrNoRows if no row matched.
func (r *ConversationPersistentRepo) Delete(id string) error {
	res, err := r.db.Exec(`DELETE FROM conversations WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
