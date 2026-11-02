package persona

import (
	"database/sql"
	"errors"
	"strings"

	"career-tool-backend/db"
)

// The summary and the goals of a persona (plan 174): the profile holds one value of each (profile_details), a persona FOLLOWS it unless
// it has its own (the nullable *_override columns of persona_details: NULL follows, any value, also "" or 0, is its own). Headline and
// location are always the persona's. These are the names of the five values a persona can follow, as the routes and tools spell them.
const (
	FieldSummary          = "summary"
	FieldDesiredTitles    = "desiredTitles"
	FieldDesiredLocations = "desiredLocations"
	FieldMinSalary        = "minSalary"
	FieldRemotePreference = "remotePreference"
)

var detailFields = []string{FieldSummary, FieldDesiredTitles, FieldDesiredLocations, FieldMinSalary, FieldRemotePreference}

// ErrUnknownFollowField is a followProfile entry that is not one of the five values.
var ErrUnknownFollowField = errors.New("followProfile can only name summary, desiredTitles, desiredLocations, minSalary or remotePreference")

// ErrSetAndFollow is a value sent together with the request to follow the profile for the same field.
var ErrSetAndFollow = errors.New("a field cannot be set and sent back to following the profile in one request")

type rowQueryer interface {
	QueryRow(query string, args ...any) *sql.Row
}

// detailsRow is a persona's own persona_details row: headline and location, and the five optional own values.
type detailsRow struct {
	exists                     bool
	headline, location         string
	summary, titles, locations sql.NullString
	remote                     sql.NullString
	salary                     sql.NullInt64
}

// profileValues are the profile's own summary and goals (zero values when the profile has no row).
type profileValues struct {
	summary, titles, locations, remote string
	salary                             int64
}

func (v profileValues) empty() bool {
	return v.summary == "" && v.titles == "" && v.locations == "" && v.remote == "" && v.salary == 0
}

func readDetailsRow(q rowQueryer, personaID string) (detailsRow, error) {
	var r detailsRow
	err := q.QueryRow(
		`SELECT headline, location, summary_override, desired_titles_override, desired_locations_override, min_salary_override, remote_preference_override
		 FROM persona_details WHERE persona_id = ?`, personaID,
	).Scan(&r.headline, &r.location, &r.summary, &r.titles, &r.locations, &r.salary, &r.remote)
	switch {
	case err == sql.ErrNoRows:
		return detailsRow{}, nil
	case err != nil:
		return detailsRow{}, err
	}
	r.exists = true
	return r, nil
}

// readProfileValues reads the summary and goals of a profile; a profile without a row has none.
func readProfileValues(q rowQueryer, profileID string) (profileValues, error) {
	var v profileValues
	var salary sql.NullInt64
	err := q.QueryRow(
		`SELECT summary, desired_titles, desired_locations, min_salary, remote_preference FROM profile_details WHERE profile_id = ?`, profileID,
	).Scan(&v.summary, &v.titles, &v.locations, &salary, &v.remote)
	switch {
	case err == sql.ErrNoRows:
		return profileValues{}, nil
	case err != nil:
		return profileValues{}, err
	}
	v.salary = salary.Int64
	return v, nil
}

func profileValuesOfPersona(q rowQueryer, personaID string) (profileValues, error) {
	var profileID string
	if err := q.QueryRow(`SELECT profile_id FROM personas WHERE id = ?`, personaID).Scan(&profileID); err != nil {
		return profileValues{}, err
	}
	return readProfileValues(q, profileID)
}

// effectiveDetails is what the persona shows: its own value where it has one, else the profile's; Overridden names the own ones.
func effectiveDetails(personaID string, r detailsRow, pv profileValues) personaDetails {
	d := personaDetails{
		PersonaID:        personaID,
		Headline:         r.headline,
		Location:         r.location,
		Summary:          pv.summary,
		DesiredTitles:    pv.titles,
		DesiredLocations: pv.locations,
		MinSalary:        pv.salary,
		RemotePreference: pv.remote,
		Overridden:       []string{},
	}
	if r.summary.Valid {
		d.Summary = r.summary.String
		d.Overridden = append(d.Overridden, FieldSummary)
	}
	if r.titles.Valid {
		d.DesiredTitles = r.titles.String
		d.Overridden = append(d.Overridden, FieldDesiredTitles)
	}
	if r.locations.Valid {
		d.DesiredLocations = r.locations.String
		d.Overridden = append(d.Overridden, FieldDesiredLocations)
	}
	if r.salary.Valid {
		d.MinSalary = r.salary.Int64
		d.Overridden = append(d.Overridden, FieldMinSalary)
	}
	if r.remote.Valid {
		d.RemotePreference = r.remote.String
		d.Overridden = append(d.Overridden, FieldRemotePreference)
	}
	return d
}

// follow sends the named fields back to following the profile (the override becomes NULL); an unknown name is ErrUnknownFollowField.
func (r *detailsRow) follow(fields []string) error {
	for _, f := range fields {
		switch f {
		case FieldSummary:
			r.summary = sql.NullString{}
		case FieldDesiredTitles:
			r.titles = sql.NullString{}
		case FieldDesiredLocations:
			r.locations = sql.NullString{}
		case FieldMinSalary:
			r.salary = sql.NullInt64{}
		case FieldRemotePreference:
			r.remote = sql.NullString{}
		default:
			return ErrUnknownFollowField
		}
	}
	return nil
}

// PublishProfileDetailsChanged announces "career.persona.changed" (change "details") for every persona of the profile: the profile's
// summary or goals changed, so every persona that follows them shows something else and the matcher must hear of it. Called after the
// write succeeded, only for a change that really changed something.
func PublishProfileDetailsChanged(profileID string) {
	rows, err := db.CareerDB.Query(`SELECT id FROM personas WHERE profile_id = ?`, profileID)
	if err != nil {
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		publishPersonaChanged(id, ChangeDetails)
	}
}

// insertAppliedDetails writes the details of a persona created from an accepted AI proposal (plan 174): a proposed summary or goal
// fills the profile's value where the profile has none yet, and the persona follows it; where the profile already has a value, the
// persona follows it when the proposal says the same, and keeps the proposed value as its own when it differs. A proposed value that
// is empty ("no opinion") is never an own value: the persona follows. The proposal carries no remote preference: it follows.
// Everything runs in the caller's transaction.
func insertAppliedDetails(tx *sql.Tx, profileID, personaID string, p ProposedDetails) error {
	pv, err := readProfileValues(tx, profileID)
	if err != nil {
		return err
	}
	filled := false
	var own detailsRow

	text := func(proposed string, profile *string) sql.NullString {
		value := strings.TrimSpace(proposed)
		switch {
		case value == "":
			return sql.NullString{}
		case *profile == "":
			*profile = value
			filled = true
			return sql.NullString{}
		case value == *profile:
			return sql.NullString{}
		}
		return sql.NullString{String: proposed, Valid: true}
	}
	own.summary = text(p.Summary, &pv.summary)
	own.titles = text(p.DesiredTitles, &pv.titles)
	own.locations = text(p.DesiredLocations, &pv.locations)
	switch {
	case p.MinSalary <= 0:
	case pv.salary == 0:
		pv.salary = int64(p.MinSalary)
		filled = true
	case pv.salary != int64(p.MinSalary):
		own.salary = sql.NullInt64{Int64: int64(p.MinSalary), Valid: true}
	}

	if filled {
		var salary any
		if pv.salary > 0 {
			salary = pv.salary
		}
		if _, err := tx.Exec(
			`INSERT INTO profile_details (profile_id, summary, desired_titles, desired_locations, min_salary, remote_preference, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, datetime('now'))
			 ON CONFLICT(profile_id) DO UPDATE SET summary = excluded.summary, desired_titles = excluded.desired_titles,
			   desired_locations = excluded.desired_locations, min_salary = excluded.min_salary, updated_at = excluded.updated_at`,
			profileID, pv.summary, pv.titles, pv.locations, salary, pv.remote,
		); err != nil {
			return err
		}
	}
	_, err = tx.Exec(
		`INSERT INTO persona_details (persona_id, headline, location, summary_override, desired_titles_override, desired_locations_override,
		   min_salary_override, remote_preference_override, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
		personaID, p.Headline, p.Location, own.summary, own.titles, own.locations, own.salary, own.remote,
	)
	return err
}
