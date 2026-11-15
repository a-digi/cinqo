package query

import (
	"database/sql"

	"github.com/a-digi/cinqo/src/organisation/entity"
)

type OrganisationQueryRepo struct {
	db *sql.DB
}

func NewOrganisationQueryRepo(db *sql.DB) *OrganisationQueryRepo {
	return &OrganisationQueryRepo{db: db}
}

const orgColumns = `id, name, status, created_at, updated_at`

func scanOrganisation(scan func(dest ...any) error) (*entity.Organisation, error) {
	var o entity.Organisation
	var updated sql.NullString
	if err := scan(&o.ID, &o.Name, &o.Status, &o.CreatedAt, &updated); err != nil {
		return nil, err
	}
	o.UpdatedAt = updated.String
	return &o, nil
}

// FindByID returns sql.ErrNoRows (unwrapped) when there is no such organisation.
func (r *OrganisationQueryRepo) FindByID(id string) (*entity.Organisation, error) {
	return scanOrganisation(r.db.QueryRow(`SELECT `+orgColumns+` FROM organisations WHERE id = ?`, id).Scan)
}

// List returns every organisation, by name then id.
func (r *OrganisationQueryRepo) List() ([]entity.Organisation, error) {
	rows, err := r.db.Query(`SELECT ` + orgColumns + ` FROM organisations ORDER BY lower(name), id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []entity.Organisation{}
	for rows.Next() {
		o, err := scanOrganisation(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, rows.Err()
}

// FindByUser returns the one organisation of a user, or sql.ErrNoRows (unwrapped) when the user belongs to none yet.
func (r *OrganisationQueryRepo) FindByUser(userID string) (*entity.Organisation, error) {
	return scanOrganisation(r.db.QueryRow(`SELECT o.id, o.name, o.status, o.created_at, o.updated_at FROM organisations o
		JOIN user_organisations u ON u.org_id = o.id WHERE u.user_id = ?`, userID).Scan)
}

// Members returns the users of an organisation, oldest first.
func (r *OrganisationQueryRepo) Members(orgID string) ([]entity.Member, error) {
	rows, err := r.db.Query(`SELECT user_id, org_id, created_at FROM user_organisations WHERE org_id = ? ORDER BY created_at, user_id`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []entity.Member{}
	for rows.Next() {
		var m entity.Member
		if err := rows.Scan(&m.UserID, &m.OrgID, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MemberCounts returns how many users each organisation has (an organisation without users is absent from the map).
func (r *OrganisationQueryRepo) MemberCounts() (map[string]int, error) {
	rows, err := r.db.Query(`SELECT org_id, COUNT(*) FROM user_organisations GROUP BY org_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// BackgroundEnabled reports whether unattended tool calls are on for an organisation: false for an organisation nobody switched on, and for one that does not exist.
func (r *OrganisationQueryRepo) BackgroundEnabled(orgID string) (bool, error) {
	var on int
	err := r.db.QueryRow(`SELECT enabled FROM organisation_background WHERE org_id = ?`, orgID).Scan(&on)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return on == 1, err
}

// BackgroundOrganisations lists the ids of the active organisations that have unattended tool calls switched on, in a fixed order. A suspended organisation is
// never listed, whatever its switch says.
func (r *OrganisationQueryRepo) BackgroundOrganisations() ([]string, error) {
	rows, err := r.db.Query(`SELECT b.org_id FROM organisation_background b JOIN organisations o ON o.id = b.org_id WHERE b.enabled = 1 AND o.status = 'active' ORDER BY b.org_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// BackgroundStates returns the unattended-use switch of every organisation that has a row (an organisation without one is off and is absent from the map).
func (r *OrganisationQueryRepo) BackgroundStates() (map[string]bool, error) {
	rows, err := r.db.Query(`SELECT org_id, enabled FROM organisation_background`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		var on int
		if err := rows.Scan(&id, &on); err != nil {
			return nil, err
		}
		out[id] = on == 1
	}
	return out, rows.Err()
}
