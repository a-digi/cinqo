package persistent

import (
	"database/sql"

	ping_entity "github.com/a-digi/cinqo/src/ping/entity"
)

type PingPersistentRepo struct {
	db *sql.DB
}

func NewPingPersistentRepo(db *sql.DB) *PingPersistentRepo {
	return &PingPersistentRepo{db: db}
}

func (r *PingPersistentRepo) Insert(p *ping_entity.Ping) error {
	_, err := r.db.Exec(
		`INSERT INTO pings (id, message, created_at) VALUES (?, ?, CURRENT_TIMESTAMP)`,
		p.ID, p.Message,
	)
	return err
}
