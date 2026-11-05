// Package jobcleanup deletes jobs nobody needs any more: jobs older than a number of days and jobs that match
// no profile well. This file holds its settings (the singleton job_cleanup_settings row in jobs.db) and the
// GET/PUT /settings/cleanup handler. See plan/ai/tools/career/step-131-job-cleanup.md.
package jobcleanup

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/db"
)

// The bounds are repeated as CHECKs in the schema (db.go).
const (
	MinOlderThanDays   = 1
	MaxOlderThanDays   = 3650
	MinMatchPercentMin = 1
	MinMatchPercentMax = 100
	MinIntervalHours   = 1
	MaxIntervalHours   = 168
)

// timeFormat is the text format of every timestamp this package writes (UTC, what SQLite's datetime('now') makes).
const timeFormat = "2006-01-02 15:04:05"

// ErrInvalid is a caller mistake (a 400), never a server failure.
var ErrInvalid = errors.New("olderThanDays must be 1 to 3650, minMatchPercent 1 to 100 and intervalHours 1 to 168")

// Settings is what the Settings page reads and writes. The LastRun fields are written by the cleanup itself and
// ignored on a PUT.
type Settings struct {
	Enabled         bool   `json:"enabled"`
	OlderEnabled    bool   `json:"olderEnabled"`
	OlderThanDays   int    `json:"olderThanDays"`
	LowMatchEnabled bool   `json:"lowMatchEnabled"`
	MinMatchPercent int    `json:"minMatchPercent"`
	IncludeManual   bool   `json:"includeManual"`
	IntervalHours   int    `json:"intervalHours"`
	LastRunAt       string `json:"lastRunAt"`
	LastRunDeleted  int    `json:"lastRunDeleted"`
	LastRunSummary  string `json:"lastRunSummary"`
}

// Validate refuses a value outside its bounds. A threshold is checked even when its rule is off, so a stored
// row is always valid.
func (s Settings) Validate() error {
	if s.OlderThanDays < MinOlderThanDays || s.OlderThanDays > MaxOlderThanDays ||
		s.MinMatchPercent < MinMatchPercentMin || s.MinMatchPercent > MinMatchPercentMax ||
		s.IntervalHours < MinIntervalHours || s.IntervalHours > MaxIntervalHours {
		return ErrInvalid
	}
	return nil
}

// GetSettings reads the singleton row (seeded by the schema, so it always exists).
func GetSettings() (Settings, error) {
	var s Settings
	var lastAt, lastSummary *string
	var lastDeleted *int
	err := db.JobsDB.QueryRow(
		`SELECT enabled, older_enabled, older_than_days, low_match_enabled, min_match_percent, include_manual,
			interval_hours, last_run_at, last_run_deleted, last_run_summary
		 FROM job_cleanup_settings WHERE id = 1`,
	).Scan(&s.Enabled, &s.OlderEnabled, &s.OlderThanDays, &s.LowMatchEnabled, &s.MinMatchPercent, &s.IncludeManual,
		&s.IntervalHours, &lastAt, &lastDeleted, &lastSummary)
	if err != nil {
		return Settings{}, err
	}
	if lastAt != nil {
		s.LastRunAt = *lastAt
	}
	if lastDeleted != nil {
		s.LastRunDeleted = *lastDeleted
	}
	if lastSummary != nil {
		s.LastRunSummary = *lastSummary
	}
	return s, nil
}

// UpdateSettings stores the editable fields and returns the saved row (with the last-run fields as they are).
func UpdateSettings(in Settings) (Settings, error) {
	if err := in.Validate(); err != nil {
		return Settings{}, err
	}
	if _, err := db.JobsDB.Exec(
		`UPDATE job_cleanup_settings SET enabled = ?, older_enabled = ?, older_than_days = ?, low_match_enabled = ?,
			min_match_percent = ?, include_manual = ?, interval_hours = ?, updated_at = ? WHERE id = 1`,
		in.Enabled, in.OlderEnabled, in.OlderThanDays, in.LowMatchEnabled, in.MinMatchPercent, in.IncludeManual, in.IntervalHours,
		now().UTC().Format(timeFormat),
	); err != nil {
		return Settings{}, err
	}
	return GetSettings()
}

// SettingsHandler handles GET and PUT /settings/cleanup. A PUT must carry every editable field: a missing
// number would otherwise read as 0 and be refused as out of range, which is the clearer answer than a silent
// default.
func SettingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s, err := GetSettings()
		if err != nil {
			http.Error(w, "failed to read the cleanup settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, s)
	case http.MethodPut:
		r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
		dec := json.NewDecoder(r.Body)
		var in Settings
		if err := dec.Decode(&in); err != nil {
			http.Error(w, "body must be the cleanup settings as JSON", http.StatusBadRequest)
			return
		}
		saved, err := UpdateSettings(in)
		if errors.Is(err, ErrInvalid) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "failed to save the cleanup settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, saved)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
