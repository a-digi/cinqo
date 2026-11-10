// Package languages is the profile's languages (plan/ai/tools/career/step-231-hobbies-courses-languages.md): the languages the job seeker
// speaks or writes, with an optional level on the CEFR scale (or native). It is global to the profile (many per profile, deleted with it)
// and a persona neither chooses nor changes it. Every check comes before the write: a name, at least one of spoken or written, a level
// from the list, one entry per language per profile.
package languages

import (
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"career-tool-backend/db"
)

var (
	ErrUnknownProfile  = errors.New("unknown profile id")
	ErrUnknownLanguage = errors.New("unknown language id")
	ErrNameRequired    = errors.New("a language needs a name")
	ErrTooLong         = errors.New("the language name is too long (at most 200 characters)")
	ErrInvalidLevel    = errors.New("the level must be empty, native, c2, c1, b2, b1, a2 or a1")
	ErrSpokenOrWritten = errors.New("a language must be spoken, written or both")
	ErrDuplicate       = errors.New("this language is already listed for the profile")
)

// MaxNameLength is the longest language name, in characters.
const MaxNameLength = 200

// Levels are the accepted levels, best first: native, then the CEFR scale from C2 down to A1. An empty level is also accepted (not given).
var Levels = []string{"native", "c2", "c1", "b2", "b1", "a2", "a1"}

// Language is one entry of a profile's languages.
type Language struct {
	ID        string `json:"id"`
	ProfileID string `json:"profileId"`
	Language  string `json:"language"`
	Level     string `json:"level"`
	CanSpeak  bool   `json:"canSpeak"`
	CanWrite  bool   `json:"canWrite"`
	CreatedAt string `json:"createdAt"`
}

// Input is what a new entry is made of.
type Input struct {
	Language string
	Level    string
	CanSpeak bool
	CanWrite bool
}

// Patch changes only the fields that are set.
type Patch struct {
	Language *string
	Level    *string
	CanSpeak *bool
	CanWrite *bool
}

func clean(in Input) Input {
	in.Language = strings.TrimSpace(in.Language)
	in.Level = strings.ToLower(strings.TrimSpace(in.Level))
	return in
}

func validLevel(level string) bool {
	if level == "" {
		return true
	}
	for _, l := range Levels {
		if l == level {
			return true
		}
	}
	return false
}

func validate(in Input) error {
	switch {
	case in.Language == "":
		return ErrNameRequired
	case utf8.RuneCountInString(in.Language) > MaxNameLength:
		return ErrTooLong
	case !validLevel(in.Level):
		return ErrInvalidLevel
	case !in.CanSpeak && !in.CanWrite:
		return ErrSpokenOrWritten
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

// listed says whether the profile already has the language (ignoring the case of plain letters), apart from the entry named except.
func listed(profileID, language, except string) (bool, error) {
	var one int
	switch err := db.CareerDB.QueryRow(
		`SELECT 1 FROM profile_languages WHERE profile_id = ? AND language = ? COLLATE NOCASE AND id != ?`, profileID, language, except).Scan(&one); {
	case err == sql.ErrNoRows:
		return false, nil
	case err != nil:
		return false, err
	}
	return true, nil
}

// List is the profile's languages: native first, then C2 down to A1, then those without a level, each group by name. An unknown profile has none.
func List(profileID string) ([]Language, error) {
	rows, err := db.CareerDB.Query(
		`SELECT id, profile_id, language, level, can_speak, can_write, created_at FROM profile_languages WHERE profile_id = ?
		 ORDER BY CASE level WHEN 'native' THEN 0 WHEN 'c2' THEN 1 WHEN 'c1' THEN 2 WHEN 'b2' THEN 3 WHEN 'b1' THEN 4 WHEN 'a2' THEN 5 WHEN 'a1' THEN 6 ELSE 7 END ASC,
		 language COLLATE NOCASE ASC, id ASC`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Language{}
	for rows.Next() {
		var l Language
		var speak, write int
		if err := rows.Scan(&l.ID, &l.ProfileID, &l.Language, &l.Level, &speak, &write, &l.CreatedAt); err != nil {
			return nil, err
		}
		l.CanSpeak, l.CanWrite = speak != 0, write != 0
		out = append(out, l)
	}
	return out, rows.Err()
}

// ByID is one entry, or ErrUnknownLanguage.
func ByID(id string) (Language, error) {
	var l Language
	var speak, write int
	switch err := db.CareerDB.QueryRow(
		`SELECT id, profile_id, language, level, can_speak, can_write, created_at FROM profile_languages WHERE id = ?`, id).
		Scan(&l.ID, &l.ProfileID, &l.Language, &l.Level, &speak, &write, &l.CreatedAt); {
	case err == sql.ErrNoRows:
		return Language{}, ErrUnknownLanguage
	case err != nil:
		return Language{}, err
	}
	l.CanSpeak, l.CanWrite = speak != 0, write != 0
	return l, nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Add adds a language to a profile. Every field is checked first and the profile must exist (ErrUnknownProfile); a language already listed is
// ErrDuplicate. Nothing is written on a refusal.
func Add(profileID string, in Input) (Language, error) {
	in = clean(in)
	if err := validate(in); err != nil {
		return Language{}, err
	}
	if err := requireProfile(profileID); err != nil {
		return Language{}, err
	}
	if dup, err := listed(profileID, in.Language, ""); err != nil {
		return Language{}, err
	} else if dup {
		return Language{}, ErrDuplicate
	}
	id := uuid.NewString()
	if _, err := db.CareerDB.Exec(
		`INSERT INTO profile_languages (id, profile_id, language, level, can_speak, can_write) VALUES (?, ?, ?, ?, ?, ?)`,
		id, profileID, in.Language, in.Level, b2i(in.CanSpeak), b2i(in.CanWrite)); err != nil {
		return Language{}, err
	}
	return ByID(id)
}

// Update changes the fields that are set; the whole entry as it would be is checked before the write. An unknown entry is
// ErrUnknownLanguage; renaming to a language already listed is ErrDuplicate.
func Update(id string, p Patch) (Language, error) {
	current, err := ByID(id)
	if err != nil {
		return Language{}, err
	}
	in := Input{Language: current.Language, Level: current.Level, CanSpeak: current.CanSpeak, CanWrite: current.CanWrite}
	if p.Language != nil {
		in.Language = *p.Language
	}
	if p.Level != nil {
		in.Level = *p.Level
	}
	if p.CanSpeak != nil {
		in.CanSpeak = *p.CanSpeak
	}
	if p.CanWrite != nil {
		in.CanWrite = *p.CanWrite
	}
	in = clean(in)
	if err := validate(in); err != nil {
		return Language{}, err
	}
	if dup, err := listed(current.ProfileID, in.Language, id); err != nil {
		return Language{}, err
	} else if dup {
		return Language{}, ErrDuplicate
	}
	if _, err := db.CareerDB.Exec(
		`UPDATE profile_languages SET language = ?, level = ?, can_speak = ?, can_write = ? WHERE id = ?`,
		in.Language, in.Level, b2i(in.CanSpeak), b2i(in.CanWrite), id); err != nil {
		return Language{}, err
	}
	return ByID(id)
}

// Delete removes an entry, or ErrUnknownLanguage.
func Delete(id string) error {
	res, err := db.CareerDB.Exec(`DELETE FROM profile_languages WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUnknownLanguage
	}
	return nil
}
