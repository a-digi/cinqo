// Package applicationlog is the application protocol: the dated log of the communication of a job application (an email sent, a
// WhatsApp message received, a phone call, a planned interview), for one (profile, job) pair. Everything is entered by the person
// (or by the AI on request); nothing is read from a mailbox. Rows live in jobs.db next to the job and the board; profile ids and CV
// document ids are opaque ids into career.db, so the profile is checked to exist here. See
// plan/ai/tools/career/step-176-application-protocol.md.
package applicationlog

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"career-tool-backend/db"

	"github.com/google/uuid"
)

// The directions and channels an entry can have. The database repeats both lists as a CHECK.
const (
	DirectionSent     = "sent"
	DirectionReceived = "received"
	DirectionNote     = "note"
)

var (
	directions = []string{DirectionSent, DirectionReceived, DirectionNote}
	channels   = []string{"email", "platform", "sms", "whatsapp", "phone", "video", "in_person", "letter", "other"}
)

// The limits, in characters, checked before any write.
const (
	MaxShortText  = 300
	MaxBody       = 20000
	MaxURL        = 1000
	maxCVDocument = 100
)

var (
	ErrUnknownProfile   = errors.New("unknown profile id")
	ErrUnknownJob       = errors.New("unknown job id")
	ErrUnknownMessage   = errors.New("unknown message id")
	ErrUnknownRecruiter = errors.New("unknown recruiter id")
	ErrInvalidDirection = errors.New("direction must be one of sent, received, note")
	ErrInvalidChannel   = errors.New("channel must be one of email, platform, sms, whatsapp, phone, video, in_person, letter, other")
	ErrInvalidDate      = errors.New("occurredAt must be a date (2026-11-05) or a date and time (2026-11-05T14:30 or 2026-11-05T14:30:00Z), between 1970 and 2100")
	ErrTimeWithoutTime  = errors.New("hasTime cannot be true when occurredAt is a date without a time")
	ErrInvalidURL       = errors.New("externalUrl must be an http or https link")
	// ErrTooLong is wrapped by the refusal of a text over its limit; the message names the field and the limit.
	ErrTooLong = errors.New("too long")
)

// now is the clock for created_at and updated_at (UTC text). Tests replace it.
var now = func() string { return time.Now().UTC().Format("2006-01-02 15:04:05") }

// Message is one entry of the protocol. OccurredAt is a date ("2026-11-05") when HasTime is false (only the day is known, shown as that
// day whatever the time zone) and an ISO UTC date and time ("2026-11-05T14:30:00Z") when it is true.
type Message struct {
	ID                 string `json:"id"`
	ProfileID          string `json:"profileId"`
	JobID              string `json:"jobId"`
	Direction          string `json:"direction"`
	Channel            string `json:"channel"`
	ChannelLabel       string `json:"channelLabel"`
	OccurredAt         string `json:"occurredAt"`
	HasTime            bool   `json:"hasTime"`
	Subject            string `json:"subject"`
	Body               string `json:"body"`
	RecruiterID        string `json:"recruiterId"`
	CounterpartName    string `json:"counterpartName"`
	CounterpartContact string `json:"counterpartContact"`
	CVDocumentID       string `json:"cvDocumentId"`
	ExternalURL        string `json:"externalUrl"`
	CreatedAt          string `json:"createdAt"`
	UpdatedAt          string `json:"updatedAt,omitempty"`
}

// Input is what the person gives for a new entry. HasTime is optional: a date with a time defaults to true; false keeps only the day.
type Input struct {
	Direction          string `json:"direction"`
	Channel            string `json:"channel"`
	ChannelLabel       string `json:"channelLabel"`
	OccurredAt         string `json:"occurredAt"`
	HasTime            *bool  `json:"hasTime"`
	Subject            string `json:"subject"`
	Body               string `json:"body"`
	RecruiterID        string `json:"recruiterId"`
	CounterpartName    string `json:"counterpartName"`
	CounterpartContact string `json:"counterpartContact"`
	CVDocumentID       string `json:"cvDocumentId"`
	ExternalURL        string `json:"externalUrl"`
}

// Patch is a change to an entry: only the fields that are present change. The profile and the job of an entry never change. A present
// OccurredAt replaces the date and the time (and HasTime is read again, as for a new entry); an empty RecruiterID, CVDocumentID or text
// clears that field.
type Patch struct {
	Direction          *string `json:"direction"`
	Channel            *string `json:"channel"`
	ChannelLabel       *string `json:"channelLabel"`
	OccurredAt         *string `json:"occurredAt"`
	HasTime            *bool   `json:"hasTime"`
	Subject            *string `json:"subject"`
	Body               *string `json:"body"`
	RecruiterID        *string `json:"recruiterId"`
	CounterpartName    *string `json:"counterpartName"`
	CounterpartContact *string `json:"counterpartContact"`
	CVDocumentID       *string `json:"cvDocumentId"`
	ExternalURL        *string `json:"externalUrl"`
}

// row is an entry as it is stored: the date already in its stored form.
type row struct {
	direction, channel, channelLabel    string
	occurredAt                          string // 'YYYY-MM-DD HH:MM:SS' UTC
	hasTime                             bool
	subject, body                       string
	recruiterID                         string
	counterpartName, counterpartContact string
	cvDocumentID, externalURL           string
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func tooLong(s string, max int) bool { return utf8.RuneCountInString(s) > max }

// parseDate reads what the person typed. A date alone has no time (hasTime false); a date and time without an offset is read as UTC.
func parseDate(text string) (stored string, dateOnly bool, err error) {
	text = strings.TrimSpace(text)
	if t, e := time.Parse("2006-01-02", text); e == nil {
		return finishDate(t, true)
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if t, e := time.Parse(layout, text); e == nil {
			return finishDate(t.UTC(), false)
		}
	}
	return "", false, ErrInvalidDate
}

func finishDate(t time.Time, dateOnly bool) (string, bool, error) {
	if t.Year() < 1970 || t.Year() > 2100 {
		return "", false, ErrInvalidDate
	}
	return t.Format("2006-01-02 15:04:05"), dateOnly, nil
}

// applyDate sets occurredAt and hasTime on r from what the person typed and the optional HasTime.
func applyDate(r *row, text string, hasTime *bool) error {
	stored, dateOnly, err := parseDate(text)
	if err != nil {
		return err
	}
	switch {
	case dateOnly:
		if hasTime != nil && *hasTime {
			return ErrTimeWithoutTime
		}
		r.hasTime = false
	case hasTime != nil && !*hasTime:
		stored = stored[:10] + " 00:00:00"
		r.hasTime = false
	default:
		r.hasTime = true
	}
	r.occurredAt = stored
	return nil
}

// check validates a whole entry as it would be stored; nothing is written when it fails.
func (r row) check() error {
	if !contains(directions, r.direction) {
		return ErrInvalidDirection
	}
	if !contains(channels, r.channel) {
		return ErrInvalidChannel
	}
	for name, v := range map[string]string{"channelLabel": r.channelLabel, "subject": r.subject, "counterpartName": r.counterpartName, "counterpartContact": r.counterpartContact} {
		if tooLong(v, MaxShortText) {
			return fmt.Errorf("%s: %w (at most %d characters)", name, ErrTooLong, MaxShortText)
		}
	}
	if tooLong(r.body, MaxBody) {
		return fmt.Errorf("body: %w (at most %d characters)", ErrTooLong, MaxBody)
	}
	if tooLong(r.cvDocumentID, maxCVDocument) {
		return fmt.Errorf("cvDocumentId: %w (at most %d characters)", ErrTooLong, maxCVDocument)
	}
	if r.externalURL != "" {
		if tooLong(r.externalURL, MaxURL) {
			return fmt.Errorf("externalUrl: %w (at most %d characters)", ErrTooLong, MaxURL)
		}
		if u, err := url.Parse(r.externalURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return ErrInvalidURL
		}
	}
	if r.recruiterID != "" {
		var one int
		switch err := db.JobsDB.QueryRow(`SELECT 1 FROM recruiters WHERE id = ?`, r.recruiterID).Scan(&one); {
		case err == sql.ErrNoRows:
			return ErrUnknownRecruiter
		case err != nil:
			return err
		}
	}
	return nil
}

func clean(r row) row {
	r.channelLabel = strings.TrimSpace(r.channelLabel)
	r.subject = strings.TrimSpace(r.subject)
	r.body = strings.TrimSpace(r.body)
	r.recruiterID = strings.TrimSpace(r.recruiterID)
	r.counterpartName = strings.TrimSpace(r.counterpartName)
	r.counterpartContact = strings.TrimSpace(r.counterpartContact)
	r.cvDocumentID = strings.TrimSpace(r.cvDocumentID)
	r.externalURL = strings.TrimSpace(r.externalURL)
	return r
}

func requirePair(profileID, jobID string) error {
	var one int
	switch err := db.CareerDB.QueryRow(`SELECT 1 FROM profiles WHERE id = ?`, profileID).Scan(&one); {
	case err == sql.ErrNoRows:
		return ErrUnknownProfile
	case err != nil:
		return err
	}
	switch err := db.JobsDB.QueryRow(`SELECT 1 FROM jobs WHERE id = ?`, jobID).Scan(&one); {
	case err == sql.ErrNoRows:
		return ErrUnknownJob
	case err != nil:
		return err
	}
	return nil
}

const columns = `id, profile_id, job_id, direction, channel, channel_label, occurred_at, has_time, subject, body,
	COALESCE(recruiter_id, ''), counterpart_name, counterpart_contact, COALESCE(cv_document_id, ''), external_url, created_at, COALESCE(updated_at, '')`

type scanner interface{ Scan(dest ...any) error }

func scan(s scanner) (Message, error) {
	var m Message
	var occurred string
	var hasTime int
	if err := s.Scan(&m.ID, &m.ProfileID, &m.JobID, &m.Direction, &m.Channel, &m.ChannelLabel, &occurred, &hasTime, &m.Subject, &m.Body,
		&m.RecruiterID, &m.CounterpartName, &m.CounterpartContact, &m.CVDocumentID, &m.ExternalURL, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return Message{}, err
	}
	m.HasTime = hasTime == 1
	if m.HasTime {
		m.OccurredAt = strings.Replace(occurred, " ", "T", 1) + "Z"
	} else {
		m.OccurredAt = occurred[:10]
	}
	return m, nil
}

// List returns the protocol of one (profile, job) pair, oldest first (a conversation reads top to bottom); entries at the same moment
// keep the order they were written in. Both ids must exist.
func List(profileID, jobID string) ([]Message, error) {
	if err := requirePair(profileID, jobID); err != nil {
		return nil, err
	}
	rows, err := db.JobsDB.Query(`SELECT `+columns+` FROM application_messages WHERE profile_id = ? AND job_id = ? ORDER BY occurred_at, rowid`, profileID, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Message{}
	for rows.Next() {
		m, err := scan(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

// ByID returns one entry, or ErrUnknownMessage.
func ByID(id string) (Message, error) {
	m, err := scan(db.JobsDB.QueryRow(`SELECT `+columns+` FROM application_messages WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return Message{}, ErrUnknownMessage
	}
	return m, err
}

// Add writes a new entry for a (profile, job) pair. The pair must exist and every field is checked first; nothing is written on a refusal.
func Add(profileID, jobID string, in Input) (Message, error) {
	r := clean(row{direction: in.Direction, channel: in.Channel, channelLabel: in.ChannelLabel, subject: in.Subject, body: in.Body,
		recruiterID: in.RecruiterID, counterpartName: in.CounterpartName, counterpartContact: in.CounterpartContact,
		cvDocumentID: in.CVDocumentID, externalURL: in.ExternalURL})
	if err := requirePair(profileID, jobID); err != nil {
		return Message{}, err
	}
	if err := applyDate(&r, in.OccurredAt, in.HasTime); err != nil {
		return Message{}, err
	}
	if err := r.check(); err != nil {
		return Message{}, err
	}
	id := uuid.NewString()
	_, err := db.JobsDB.Exec(`INSERT INTO application_messages
		(id, profile_id, job_id, direction, channel, channel_label, occurred_at, has_time, subject, body, recruiter_id, counterpart_name, counterpart_contact, cv_document_id, external_url, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, profileID, jobID, r.direction, r.channel, r.channelLabel, r.occurredAt, boolToInt(r.hasTime), r.subject, r.body,
		nullIfEmpty(r.recruiterID), r.counterpartName, r.counterpartContact, nullIfEmpty(r.cvDocumentID), r.externalURL, now())
	if err != nil {
		return Message{}, err
	}
	return ByID(id)
}

// Update changes the fields present in the patch. The whole entry is checked as it would be after the change; nothing is written on a
// refusal. A patch with no field present changes nothing.
func Update(id string, p Patch) (Message, error) {
	m, err := ByID(id)
	if err != nil {
		return Message{}, err
	}
	r := row{direction: m.Direction, channel: m.Channel, channelLabel: m.ChannelLabel, subject: m.Subject, body: m.Body, recruiterID: m.RecruiterID,
		counterpartName: m.CounterpartName, counterpartContact: m.CounterpartContact, cvDocumentID: m.CVDocumentID, externalURL: m.ExternalURL,
		hasTime: m.HasTime}
	r.occurredAt = strings.Replace(strings.TrimSuffix(m.OccurredAt, "Z"), "T", " ", 1)
	if !m.HasTime {
		r.occurredAt += " 00:00:00"
	}
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = *v
		}
	}
	set(&r.direction, p.Direction)
	set(&r.channel, p.Channel)
	set(&r.channelLabel, p.ChannelLabel)
	set(&r.subject, p.Subject)
	set(&r.body, p.Body)
	set(&r.recruiterID, p.RecruiterID)
	set(&r.counterpartName, p.CounterpartName)
	set(&r.counterpartContact, p.CounterpartContact)
	set(&r.cvDocumentID, p.CVDocumentID)
	set(&r.externalURL, p.ExternalURL)
	r = clean(r)
	switch {
	case p.OccurredAt != nil:
		if err := applyDate(&r, *p.OccurredAt, p.HasTime); err != nil {
			return Message{}, err
		}
	case p.HasTime != nil:
		// Only the flag changes: the day stays; the time is dropped, or a time is asked for that the date does not have.
		if *p.HasTime && !r.hasTime {
			return Message{}, ErrTimeWithoutTime
		}
		if !*p.HasTime {
			r.occurredAt = r.occurredAt[:10] + " 00:00:00"
			r.hasTime = false
		}
	}
	if err := r.check(); err != nil {
		return Message{}, err
	}
	if _, err := db.JobsDB.Exec(`UPDATE application_messages SET direction = ?, channel = ?, channel_label = ?, occurred_at = ?, has_time = ?,
		subject = ?, body = ?, recruiter_id = ?, counterpart_name = ?, counterpart_contact = ?, cv_document_id = ?, external_url = ?, updated_at = ? WHERE id = ?`,
		r.direction, r.channel, r.channelLabel, r.occurredAt, boolToInt(r.hasTime), r.subject, r.body, nullIfEmpty(r.recruiterID),
		r.counterpartName, r.counterpartContact, nullIfEmpty(r.cvDocumentID), r.externalURL, now(), id); err != nil {
		return Message{}, err
	}
	return ByID(id)
}

// Delete removes one entry, or returns ErrUnknownMessage.
func Delete(id string) error {
	res, err := db.JobsDB.Exec(`DELETE FROM application_messages WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUnknownMessage
	}
	return nil
}

// PurgeProfile removes every entry of a deleted profile. jobs.db cannot cascade from career.db, so profile.DeleteProfile calls this
// (wired in step 2).
func PurgeProfile(profileID string) error {
	_, err := db.JobsDB.Exec(`DELETE FROM application_messages WHERE profile_id = ?`, profileID)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
