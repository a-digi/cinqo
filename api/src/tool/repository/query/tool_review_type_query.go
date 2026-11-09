package query

import "database/sql"

// ReviewTypeSpec is a reviewable entity type's scale and scopes.
type ReviewTypeSpec struct {
	Scale      int
	HalfStars  bool
	ReadScope  string
	WriteScope string
}

type ToolReviewTypeQueryRepo struct {
	db *sql.DB
}

func NewToolReviewTypeQueryRepo(db *sql.DB) *ToolReviewTypeQueryRepo {
	return &ToolReviewTypeQueryRepo{db: db}
}

// FindEnabled returns one entity type of the tool with this slug, only while that tool is ENABLED (a disabled tool's reviews are
// not reachable; its data and its declaration are kept). sql.ErrNoRows when there is no such tool, it is disabled, or it declared no
// such type.
func (r *ToolReviewTypeQueryRepo) FindEnabled(slug, entityType string) (ReviewTypeSpec, error) {
	var s ReviewTypeSpec
	var half int
	err := r.db.QueryRow(
		`SELECT c.scale, c.half_stars, c.read_scope, c.write_scope
		FROM tool_review_types c
		JOIN tools t ON t.id = c.tool_id
		WHERE t.slug = ? AND t.enabled = 1 AND c.entity_type = ?`,
		slug, entityType,
	).Scan(&s.Scale, &half, &s.ReadScope, &s.WriteScope)
	s.HalfStars = half != 0
	return s, err
}
