package persistent

import (
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/a-digi/cinqo/src/organisation/entity"
)

// Errors a caller can tell apart.
var (
	ErrInvalidName   = errors.New("the name must be 1 to 120 characters")
	ErrInvalidStatus = errors.New("the status must be active or suspended")
	ErrNoSuchOrg     = errors.New("no such organisation")
	ErrInvalidUser   = errors.New("the user id must not be empty")
)

type OrganisationPersistentRepo struct {
	db  *sql.DB
	now func() time.Time
}

func NewOrganisationPersistentRepo(db *sql.DB) *OrganisationPersistentRepo {
	return &OrganisationPersistentRepo{db: db, now: time.Now}
}

// SetClock replaces the clock (tests).
func (r *OrganisationPersistentRepo) SetClock(now func() time.Time) { r.now = now }

func (r *OrganisationPersistentRepo) stamp() string { return r.now().UTC().Format(time.RFC3339) }

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > entity.MaxNameLength {
		return "", ErrInvalidName
	}
	return name, nil
}

// Create makes a new, active organisation and returns it.
func (r *OrganisationPersistentRepo) Create(name string) (*entity.Organisation, error) {
	name, err := cleanName(name)
	if err != nil {
		return nil, err
	}
	o := &entity.Organisation{ID: uuid.NewString(), Name: name, Status: entity.StatusActive, CreatedAt: r.stamp()}
	if _, err := r.db.Exec(`INSERT INTO organisations (id, name, status, created_at) VALUES (?, ?, ?, ?)`, o.ID, o.Name, o.Status, o.CreatedAt); err != nil {
		return nil, err
	}
	return o, nil
}

// Update changes the name and the status of an organisation (both are always given).
func (r *OrganisationPersistentRepo) Update(id, name, status string) error {
	name, err := cleanName(name)
	if err != nil {
		return err
	}
	if status != entity.StatusActive && status != entity.StatusSuspended {
		return ErrInvalidStatus
	}
	res, err := r.db.Exec(`UPDATE organisations SET name = ?, status = ?, updated_at = ? WHERE id = ?`, name, status, r.stamp(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoSuchOrg
	}
	return nil
}

// Assign puts a user in an organisation. A user belongs to exactly one: an earlier assignment is replaced, never added to.
func (r *OrganisationPersistentRepo) Assign(userID, orgID string) error {
	if strings.TrimSpace(userID) == "" {
		return ErrInvalidUser
	}
	var one int
	if err := r.db.QueryRow(`SELECT 1 FROM organisations WHERE id = ?`, orgID).Scan(&one); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNoSuchOrg
		}
		return err
	}
	_, err := r.db.Exec(`INSERT INTO user_organisations (user_id, org_id, created_at) VALUES (?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET org_id = excluded.org_id, created_at = excluded.created_at`, userID, orgID, r.stamp())
	return err
}

// AttachDefault puts a user who has no organisation yet into the default organisation (what every user of an existing install already is). A user who
// already has one is left alone. It reports whether it attached.
func (r *OrganisationPersistentRepo) AttachDefault(userID string) (bool, error) {
	if strings.TrimSpace(userID) == "" {
		return false, ErrInvalidUser
	}
	res, err := r.db.Exec(`INSERT OR IGNORE INTO user_organisations (user_id, org_id, created_at) VALUES (?, ?, ?)`, userID, entity.DefaultID, r.stamp())
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SetBackground switches unattended tool calls on or off for an organisation (off unless a row says on) and records who did it. An unknown organisation is
// refused, and switching to the state it already has is not an error.
func (r *OrganisationPersistentRepo) SetBackground(orgID string, enabled bool, by string) error {
	if strings.TrimSpace(by) == "" {
		return ErrInvalidUser
	}
	var one int
	if err := r.db.QueryRow(`SELECT 1 FROM organisations WHERE id = ?`, orgID).Scan(&one); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNoSuchOrg
		}
		return err
	}
	on := 0
	if enabled {
		on = 1
	}
	_, err := r.db.Exec(`INSERT INTO organisation_background (org_id, enabled, enabled_by, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(org_id) DO UPDATE SET enabled = excluded.enabled, enabled_by = excluded.enabled_by, updated_at = excluded.updated_at`, orgID, on, by, r.stamp())
	return err
}
