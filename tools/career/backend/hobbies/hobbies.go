// Package hobbies is the profile's hobbies (plan/ai/tools/career/step-231-hobbies-courses-languages.md): what the job seeker does outside
// work. It is global to the profile (many per profile, deleted with it) and a persona neither chooses nor changes it. Every check comes
// before the write: a name, the lengths, one entry per name per profile.
package hobbies

import (
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"career-tool-backend/db"
)

var (
	ErrUnknownProfile = errors.New("unknown profile id")
	ErrUnknownHobby   = errors.New("unknown hobby id")
	ErrNameRequired   = errors.New("a hobby needs a name")
	ErrNameTooLong    = errors.New("the hobby name is too long (at most 200 characters)")
	ErrTextTooLong    = errors.New("the description is too long (at most 2000 characters)")
	ErrDuplicate      = errors.New("this hobby is already listed for the profile")
)

// The longest name and description, in characters.
const (
	MaxNameLength        = 200
	MaxDescriptionLength = 2000
)

// Hobby is one entry of a profile's hobbies.
type Hobby struct {
	ID          string `json:"id"`
	ProfileID   string `json:"profileId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   string `json:"createdAt"`
}

// Input is what a new entry is made of.
type Input struct {
	Name        string
	Description string
}

// Patch changes only the fields that are set.
type Patch struct {
	Name, Description *string
}

func clean(in Input) Input {
	in.Name = strings.TrimSpace(in.Name)
	return in
}

func validate(in Input) error {
	switch {
	case in.Name == "":
		return ErrNameRequired
	case utf8.RuneCountInString(in.Name) > MaxNameLength:
		return ErrNameTooLong
	case utf8.RuneCountInString(in.Description) > MaxDescriptionLength:
		return ErrTextTooLong
	}
	return nil
}

func requireProfile(id string) error {
	var one int
	switch err := db.CareerDB.QueryRow(`SELECT 1 FROM profiles WHERE id = ?`, id).Scan(&one); {
	case err == sql.ErrNoRows:
		return ErrUnknownProfile
	case err != nil:
		return err
	}
	return nil
}

func listed(profileID, name, except string) (bool, error) {
	var one int
	switch err := db.CareerDB.QueryRow(
		`SELECT 1 FROM profile_hobbies WHERE profile_id = ? AND name = ? COLLATE NOCASE AND id != ?`, profileID, name, except).Scan(&one); {
	case err == sql.ErrNoRows:
		return false, nil
	case err != nil:
		return false, err
	}
	return true, nil
}

// List is the profile's hobbies by name. An unknown profile has none.
func List(profileID string) ([]Hobby, error) {
	rows, err := db.CareerDB.Query(
		`SELECT id, profile_id, name, description, created_at FROM profile_hobbies WHERE profile_id = ? ORDER BY name COLLATE NOCASE ASC, id ASC`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Hobby{}
	for rows.Next() {
		var h Hobby
		var description sql.NullString
		if err := rows.Scan(&h.ID, &h.ProfileID, &h.Name, &description, &h.CreatedAt); err != nil {
			return nil, err
		}
		h.Description = description.String
		out = append(out, h)
	}
	return out, rows.Err()
}

// ByID is one entry, or ErrUnknownHobby.
func ByID(id string) (Hobby, error) {
	var h Hobby
	var description sql.NullString
	switch err := db.CareerDB.QueryRow(`SELECT id, profile_id, name, description, created_at FROM profile_hobbies WHERE id = ?`, id).
		Scan(&h.ID, &h.ProfileID, &h.Name, &description, &h.CreatedAt); {
	case err == sql.ErrNoRows:
		return Hobby{}, ErrUnknownHobby
	case err != nil:
		return Hobby{}, err
	}
	h.Description = description.String
	return h, nil
}

// Add adds a hobby to a profile (ErrUnknownProfile, ErrNameRequired, the length errors, ErrDuplicate); nothing is written on a refusal.
func Add(profileID string, in Input) (Hobby, error) {
	in = clean(in)
	if err := validate(in); err != nil {
		return Hobby{}, err
	}
	if err := requireProfile(profileID); err != nil {
		return Hobby{}, err
	}
	if dup, err := listed(profileID, in.Name, ""); err != nil {
		return Hobby{}, err
	} else if dup {
		return Hobby{}, ErrDuplicate
	}
	id := uuid.NewString()
	if _, err := db.CareerDB.Exec(`INSERT INTO profile_hobbies (id, profile_id, name, description) VALUES (?, ?, ?, ?)`,
		id, profileID, in.Name, in.Description); err != nil {
		return Hobby{}, err
	}
	return ByID(id)
}

// Update changes the fields that are set; the whole entry as it would be is checked before the write. An unknown entry is ErrUnknownHobby.
func Update(id string, p Patch) (Hobby, error) {
	current, err := ByID(id)
	if err != nil {
		return Hobby{}, err
	}
	in := Input{Name: current.Name, Description: current.Description}
	if p.Name != nil {
		in.Name = *p.Name
	}
	if p.Description != nil {
		in.Description = *p.Description
	}
	in = clean(in)
	if err := validate(in); err != nil {
		return Hobby{}, err
	}
	if dup, err := listed(current.ProfileID, in.Name, id); err != nil {
		return Hobby{}, err
	} else if dup {
		return Hobby{}, ErrDuplicate
	}
	if _, err := db.CareerDB.Exec(`UPDATE profile_hobbies SET name = ?, description = ? WHERE id = ?`, in.Name, in.Description, id); err != nil {
		return Hobby{}, err
	}
	return ByID(id)
}

// Delete removes an entry, or ErrUnknownHobby.
func Delete(id string) error {
	res, err := db.CareerDB.Exec(`DELETE FROM profile_hobbies WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUnknownHobby
	}
	return nil
}
