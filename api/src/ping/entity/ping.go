package entity

// Ping is a reference/demo domain proving the scope-enforcement pattern
// end to end — not a real cinqo feature. Delete this whole src/ping/
// package (and its route file/handler map entries) once cinqo's first
// real domain lands; the migration that created the `pings` table stays
// (append-only), superseded by a later migration that drops it.
type Ping struct {
	_         struct{} `table:"pings"`
	ID        string   `db:"id" dbtype:"UUID" nullable:"false" json:"id"`
	Message   string   `db:"message" dbtype:"TEXT" nullable:"false" json:"message"`
	CreatedAt string   `db:"created_at" dbtype:"DATETIME" nullable:"true" json:"created_at"`
}
