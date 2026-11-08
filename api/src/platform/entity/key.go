package entity

// Key is one registered API key for a platform. EncryptedKey and
// CreatedBy are never serialized — a key's plaintext must never reach
// a response, and no per-key access-control feature exists in this
// design to need "who added this" surfaced. See
// plan/ai/platform/step-02-ai-key-data-model-and-encryption.md.
type Key struct {
	_            struct{} `table:"platform_keys"`
	ID           string   `db:"id" dbtype:"UUID" nullable:"false" json:"id"`
	Label        string   `db:"label" dbtype:"TEXT" nullable:"false" json:"label"`
	Platform     string   `db:"platform" dbtype:"TEXT" nullable:"false" json:"platform"`
	EncryptedKey string   `db:"encrypted_key" dbtype:"TEXT" nullable:"false" json:"-"`
	CreatedBy    string   `db:"created_by" dbtype:"TEXT" nullable:"false" json:"-"`
	CreatedAt    string   `db:"created_at" dbtype:"DATETIME" nullable:"false" json:"created_at"`
}
