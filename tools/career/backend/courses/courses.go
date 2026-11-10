// Package courses is the profile's courses (plan/ai/tools/career/step-231-hobbies-courses-languages.md): the short trainings and
// certificates of the job seeker (schools and degrees stay in education). It is global to the profile (many per profile, deleted with it)
// and a persona neither chooses nor changes it. Every check comes before the write: a title, a completed date of the shapes YYYY, YYYY-MM or
// YYYY-MM-DD (empty means not given), a certificate link that is empty or http(s), the lengths.
package courses

import (
	"database/sql"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"career-tool-backend/db"
)

var (
	ErrUnknownProfile = errors.New("unknown profile id")
	ErrUnknownCourse  = errors.New("unknown course id")
	ErrTitleRequired  = errors.New("a course needs a title")
	// ErrInvalidDate: a date that is not YYYY, YYYY-MM or YYYY-MM-DD (or not a real one).
	ErrInvalidDate = errors.New("the completed date must be YYYY, YYYY-MM or YYYY-MM-DD")
	// ErrInvalidURL: a certificate link that is not an http or https address.
	ErrInvalidURL = errors.New("the certificate link must be an http or https address")
	ErrTooLong    = errors.New("a text is too long (title and provider at most 200 characters, link at most 2000, description at most 2000)")
)

// The longest texts, in characters.
const (
	MaxShortLength = 200
	MaxLongLength  = 2000
)

// Course is one entry of a profile's courses.
type Course struct {
	ID            string `json:"id"`
	ProfileID     string `json:"profileId"`
	Title         string `json:"title"`
	Provider      string `json:"provider"`
	CompletedDate string `json:"completedDate"`
	CredentialURL string `json:"credentialUrl"`
	Description   string `json:"description"`
	CreatedAt     string `json:"createdAt"`
}

// Input is what a new entry is made of.
type Input struct {
	Title         string
	Provider      string
	CompletedDate string
	CredentialURL string
	Description   string
}

// Patch changes only the fields that are set.
type Patch struct {
	Title, Provider, CompletedDate, CredentialURL, Description *string
}

var dateShape = regexp.MustCompile(`^(\d{4})(?:-(\d{2})(?:-(\d{2}))?)?$`)

// validDate: empty, or YYYY, YYYY-MM or YYYY-MM-DD with a year of 1900 to 2100, a month of 1 to 12 and a day of 1 to 31 (the same rule as the
// education dates).
func validDate(text string) bool {
	if text == "" {
		return true
	}
	m := dateShape.FindStringSubmatch(text)
	if m == nil {
		return false
	}
	year, _ := strconv.Atoi(m[1])
	month, day := 1, 1
	if m[2] != "" {
		month, _ = strconv.Atoi(m[2])
	}
	if m[3] != "" {
		day, _ = strconv.Atoi(m[3])
	}
	return year >= 1900 && year <= 2100 && month >= 1 && month <= 12 && day >= 1 && day <= 31
}

// validLink: empty, or an absolute http or https address with a host and no spaces (never javascript:, data: or a relative address).
func validLink(link string) bool {
	if link == "" {
		return true
	}
	if strings.ContainsAny(link, " \t\r\n") {
		return false
	}
	u, err := url.Parse(link)
	if err != nil || u.Host == "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

func clean(in Input) Input {
	in.Title = strings.TrimSpace(in.Title)
	in.Provider = strings.TrimSpace(in.Provider)
	in.CompletedDate = strings.TrimSpace(in.CompletedDate)
	in.CredentialURL = strings.TrimSpace(in.CredentialURL)
	return in
}

func tooLong(s string, max int) bool { return utf8.RuneCountInString(s) > max }

func validate(in Input) error {
	switch {
	case in.Title == "":
		return ErrTitleRequired
	case tooLong(in.Title, MaxShortLength), tooLong(in.Provider, MaxShortLength), tooLong(in.CredentialURL, MaxLongLength), tooLong(in.Description, MaxLongLength):
		return ErrTooLong
	case !validDate(in.CompletedDate):
		return ErrInvalidDate
	case !validLink(in.CredentialURL):
		return ErrInvalidURL
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

const columns = `id, profile_id, title, provider, completed_date, credential_url, description, created_at`

type scanner interface{ Scan(dest ...any) error }

func scan(s scanner) (Course, error) {
	var c Course
	var date, description sql.NullString
	if err := s.Scan(&c.ID, &c.ProfileID, &c.Title, &c.Provider, &date, &c.CredentialURL, &description, &c.CreatedAt); err != nil {
		return Course{}, err
	}
	c.CompletedDate, c.Description = date.String, description.String
	return c, nil
}

// List is the profile's courses, the latest completed first, those without a date last (newest added first among equals). An unknown
// profile has none.
func List(profileID string) ([]Course, error) {
	rows, err := db.CareerDB.Query(
		`SELECT `+columns+` FROM profile_courses WHERE profile_id = ?
		 ORDER BY CASE WHEN COALESCE(completed_date, '') = '' THEN 1 ELSE 0 END ASC, COALESCE(completed_date, '') DESC, created_at DESC, id ASC`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Course{}
	for rows.Next() {
		c, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ByID is one entry, or ErrUnknownCourse.
func ByID(id string) (Course, error) {
	c, err := scan(db.CareerDB.QueryRow(`SELECT `+columns+` FROM profile_courses WHERE id = ?`, id))
	switch {
	case err == sql.ErrNoRows:
		return Course{}, ErrUnknownCourse
	case err != nil:
		return Course{}, err
	}
	return c, nil
}

// Add adds a course to a profile. Every field is checked first and the profile must exist (ErrUnknownProfile); nothing is written on a refusal.
func Add(profileID string, in Input) (Course, error) {
	in = clean(in)
	if err := validate(in); err != nil {
		return Course{}, err
	}
	if err := requireProfile(profileID); err != nil {
		return Course{}, err
	}
	id := uuid.NewString()
	if _, err := db.CareerDB.Exec(
		`INSERT INTO profile_courses (id, profile_id, title, provider, completed_date, credential_url, description) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, profileID, in.Title, in.Provider, in.CompletedDate, in.CredentialURL, in.Description); err != nil {
		return Course{}, err
	}
	return ByID(id)
}

// Update changes the fields that are set; the whole entry as it would be is checked before the write. An unknown entry is ErrUnknownCourse.
func Update(id string, p Patch) (Course, error) {
	current, err := ByID(id)
	if err != nil {
		return Course{}, err
	}
	in := Input{Title: current.Title, Provider: current.Provider, CompletedDate: current.CompletedDate, CredentialURL: current.CredentialURL, Description: current.Description}
	if p.Title != nil {
		in.Title = *p.Title
	}
	if p.Provider != nil {
		in.Provider = *p.Provider
	}
	if p.CompletedDate != nil {
		in.CompletedDate = *p.CompletedDate
	}
	if p.CredentialURL != nil {
		in.CredentialURL = *p.CredentialURL
	}
	if p.Description != nil {
		in.Description = *p.Description
	}
	in = clean(in)
	if err := validate(in); err != nil {
		return Course{}, err
	}
	if _, err := db.CareerDB.Exec(
		`UPDATE profile_courses SET title = ?, provider = ?, completed_date = ?, credential_url = ?, description = ? WHERE id = ?`,
		in.Title, in.Provider, in.CompletedDate, in.CredentialURL, in.Description, id); err != nil {
		return Course{}, err
	}
	return ByID(id)
}

// Delete removes an entry, or ErrUnknownCourse.
func Delete(id string) error {
	res, err := db.CareerDB.Exec(`DELETE FROM profile_courses WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUnknownCourse
	}
	return nil
}
