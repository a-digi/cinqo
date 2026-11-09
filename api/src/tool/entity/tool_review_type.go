package entity

// ToolReviewType is one kind of entity a tool declared in its manifest's reviews[] as reviewable (star ratings), cached at
// install/update time (same convention as ToolCommentType). The reviews themselves are not here: they live in the tool's own
// review database. See plan/ai/review/review-engine.md.
type ToolReviewType struct {
	_          struct{} `table:"tool_review_types"`
	ToolID     string   `db:"tool_id" dbtype:"TEXT" nullable:"false" json:"tool_id"`
	EntityType string   `db:"entity_type" dbtype:"TEXT" nullable:"false" json:"entity_type"`
	Scale      int      `db:"scale" dbtype:"INTEGER" nullable:"false" json:"scale"`
	HalfStars  bool     `db:"half_stars" dbtype:"INTEGER" nullable:"false" json:"half_stars"`
	ReadScope  string   `db:"read_scope" dbtype:"TEXT" nullable:"false" json:"read_scope"`
	WriteScope string   `db:"write_scope" dbtype:"TEXT" nullable:"false" json:"write_scope"`
}
