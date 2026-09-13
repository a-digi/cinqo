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

func (r *ConversationPersistentRepo) Insert(c *conversation_entity.Conversation) error {
	_, err := r.db.Exec(
		`INSERT INTO conversations (id, user_id, title, file_path) VALUES (?, ?, ?, ?)`,
		c.ID, c.UserID, c.Title, c.FilePath,
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
