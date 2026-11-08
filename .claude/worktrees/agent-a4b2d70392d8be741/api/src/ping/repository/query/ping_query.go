package query

import (
	"database/sql"

	ping_entity "github.com/a-digi/cinqo/src/ping/entity"
)

type PingQueryRepo struct {
	db *sql.DB
}

func NewPingQueryRepo(db *sql.DB) *PingQueryRepo {
	return &PingQueryRepo{db: db}
}

func scanPing(scan func(dest ...any) error) (*ping_entity.Ping, error) {
	var p ping_entity.Ping
	var createdAt sql.NullString

	if err := scan(&p.ID, &p.Message, &createdAt); err != nil {
		return nil, err
	}

	p.CreatedAt = createdAt.String

	return &p, nil
}

func (r *PingQueryRepo) FindByID(id string) (*ping_entity.Ping, error) {
	row := r.db.QueryRow(
		`SELECT id, message, created_at FROM pings WHERE id = ? LIMIT 1`, id,
	)
	return scanPing(row.Scan)
}

// List returns every ping, most recent first.
func (r *PingQueryRepo) List() ([]*ping_entity.Ping, error) {
	rows, err := r.db.Query(`SELECT id, message, created_at FROM pings ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// make(..., 0), not var (nil slice) — a nil slice marshals to JSON
	// `null` for an empty table, not `[]`, which crashes frontend code
	// that does `.map()` on the response without a null-check.
	pings := make([]*ping_entity.Ping, 0)
	for rows.Next() {
		p, err := scanPing(rows.Scan)
		if err != nil {
			return nil, err
		}
		pings = append(pings, p)
	}
	return pings, rows.Err()
}
