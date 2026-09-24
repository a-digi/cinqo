// Package query holds conversations table reads. FindByID (unfiltered)
// was created ahead of plan/ai/conversation/step-03-conversation-api.md
// out of strict necessity: step 2's chat.go needs to look up a
// conversation's file_path by ID before it can do anything, and
// SendMessage deliberately has no ownership concept of its own (that
// belongs at the request-layer boundary). FindOwnedByID/ListOwnedBy
// (step 3) are the ownership-scoped forms every HTTP handler in this
// feature actually uses — wrong owner and nonexistent both read as
// the same zero rows, the same anti-enumeration convention this
// codebase already uses elsewhere.
package query

import (
	"database/sql"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
)

type ConversationQueryRepo struct {
	db *sql.DB
}

func NewConversationQueryRepo(db *sql.DB) *ConversationQueryRepo {
	return &ConversationQueryRepo{db: db}
}

const conversationColumns = `id, user_id, title, started_at, file_path, platform_id, model, hidden, tool_slug`

func scanConversation(scan func(dest ...any) error) (*conversation_entity.Conversation, error) {
	var c conversation_entity.Conversation
	var toolSlug sql.NullString
	if err := scan(&c.ID, &c.UserID, &c.Title, &c.StartedAt, &c.FilePath, &c.PlatformID, &c.Model, &c.Hidden, &toolSlug); err != nil {
		return nil, err
	}
	c.ToolSlug = toolSlug.String
	return &c, nil
}

// FindByID returns sql.ErrNoRows (unwrapped, matching database/sql's
// own convention) when no conversation has this ID. Unfiltered by
// owner — used only by step 2's chat.go, never by an HTTP handler.
func (r *ConversationQueryRepo) FindByID(id string) (*conversation_entity.Conversation, error) {
	row := r.db.QueryRow(`SELECT `+conversationColumns+` FROM conversations WHERE id = ? LIMIT 1`, id)
	return scanConversation(row.Scan)
}

// FindOwnedByID returns the conversation with this ID, scoped to
// userID — sql.ErrNoRows (unwrapped) for both "doesn't exist" and
// "belongs to someone else," identical either way. Every HTTP handler
// in this feature (step 3) uses this, never the unfiltered FindByID.
func (r *ConversationQueryRepo) FindOwnedByID(id, userID string) (*conversation_entity.Conversation, error) {
	row := r.db.QueryRow(`SELECT `+conversationColumns+` FROM conversations WHERE id = ? AND user_id = ? LIMIT 1`, id, userID)
	return scanConversation(row.Scan)
}

// ListOwnedBy returns userID's own non-hidden conversations, most
// recently started first. No "last activity" sort available — see
// step 3's own design note on why. Excludes hidden = 1 rows (step 30)
// — a conversation created with hidden:true never shows up here, but
// remains fully reachable via FindOwnedByID/FindByID by anything that
// already has its ID.
func (r *ConversationQueryRepo) ListOwnedBy(userID string) ([]*conversation_entity.Conversation, error) {
	rows, err := r.db.Query(`SELECT `+conversationColumns+` FROM conversations WHERE user_id = ? AND hidden = 0 ORDER BY started_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*conversation_entity.Conversation, 0)
	for rows.Next() {
		c, err := scanConversation(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
