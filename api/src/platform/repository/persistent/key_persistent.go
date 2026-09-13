package persistent

import (
	"database/sql"

	platform_entity "github.com/a-digi/cinqo/src/platform/entity"
)

type KeyPersistentRepo struct {
	db *sql.DB
}

func NewKeyPersistentRepo(db *sql.DB) *KeyPersistentRepo {
	return &KeyPersistentRepo{db: db}
}

// Insert adds a new key row. No Update — rotating a key means adding a
// new row (step 2).
func (r *KeyPersistentRepo) Insert(k *platform_entity.Key) error {
	_, err := r.db.Exec(
		`INSERT INTO platform_keys (id, label, platform, encrypted_key, created_by, created_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		k.ID, k.Label, k.Platform, k.EncryptedKey, k.CreatedBy,
	)
	return err
}

// Delete hard-removes a key row — no soft-delete flag, matching this
// table's own small, low-stakes nature (step 5). Returns sql.ErrNoRows
// if no row matched, so callers can distinguish "not found" from a
// real error the same way query repos do.
func (r *KeyPersistentRepo) Delete(id string) error {
	res, err := r.db.Exec(`DELETE FROM platform_keys WHERE id = ?`, id)
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
