package query

import (
	"database/sql"

	platform_entity "github.com/a-digi/cinqo/src/platform/entity"
)

type KeyQueryRepo struct {
	db *sql.DB
}

func NewKeyQueryRepo(db *sql.DB) *KeyQueryRepo {
	return &KeyQueryRepo{db: db}
}

const keyColumns = `id, label, platform, encrypted_key, created_by, created_at`

func scanKey(scan func(dest ...any) error) (*platform_entity.Key, error) {
	var k platform_entity.Key
	if err := scan(&k.ID, &k.Label, &k.Platform, &k.EncryptedKey, &k.CreatedBy, &k.CreatedAt); err != nil {
		return nil, err
	}
	return &k, nil
}

// List returns every registered key, newest first — matching the
// reference's own "most recently created row is the current key"
// convention for a given platform (step 2).
func (r *KeyQueryRepo) List() ([]*platform_entity.Key, error) {
	rows, err := r.db.Query(`SELECT ` + keyColumns + ` FROM platform_keys ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := make([]*platform_entity.Key, 0)
	for rows.Next() {
		k, err := scanKey(rows.Scan)
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// FindByID returns sql.ErrNoRows (unwrapped, matching database/sql's
// own convention) when no key has this ID.
func (r *KeyQueryRepo) FindByID(id string) (*platform_entity.Key, error) {
	row := r.db.QueryRow(`SELECT `+keyColumns+` FROM platform_keys WHERE id = ? LIMIT 1`, id)
	return scanKey(row.Scan)
}
