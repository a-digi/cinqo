package query

import "database/sql"

// CommentTypeScopes is a commentable entity type's scopes.
type CommentTypeScopes struct {
	ReadScope  string
	WriteScope string
}

type ToolCommentTypeQueryRepo struct {
	db *sql.DB
}

func NewToolCommentTypeQueryRepo(db *sql.DB) *ToolCommentTypeQueryRepo {
	return &ToolCommentTypeQueryRepo{db: db}
}

// FindEnabled returns the scopes of one entity type of the tool with this slug,
// only while that tool is ENABLED (a disabled tool's comments are not reachable;
// its data and its declaration are kept). sql.ErrNoRows when there is no such
// tool, it is disabled, or it declared no such type.
func (r *ToolCommentTypeQueryRepo) FindEnabled(slug, entityType string) (CommentTypeScopes, error) {
	var s CommentTypeScopes
	err := r.db.QueryRow(
		`SELECT c.read_scope, c.write_scope
		FROM tool_comment_types c
		JOIN tools t ON t.id = c.tool_id
		WHERE t.slug = ? AND t.enabled = 1 AND c.entity_type = ?`,
		slug, entityType,
	).Scan(&s.ReadScope, &s.WriteScope)
	return s, err
}
