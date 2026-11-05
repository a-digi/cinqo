package crawlcriteria

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
)

// timeFormat is the text format of every timestamp this package writes (UTC, what SQLite's datetime('now') makes).
const timeFormat = "2006-01-02 15:04:05"

// now is the clock, a var so a test can fix it.
var now = time.Now

// ErrInvalidSettings is a caller mistake (a 400), never a server failure.
var ErrInvalidSettings = errors.New("minMatchPercent must be 1 to 100")

// Settings is what the Job Crawler settings page reads and writes: the master switch "Ignore jobs" and the rules under it. Today there
// is one rule; a later matcher adds its own fields here and a builder in matcherBuilders.
type Settings struct {
	IgnoreEnabled   bool   `json:"ignoreEnabled"`
	MinMatchEnabled bool   `json:"minMatchEnabled"`
	MinMatchPercent int    `json:"minMatchPercent"`
	UpdatedAt       string `json:"updatedAt"`
}

// Validate refuses a value outside its bounds; the limit is checked even when its rule is off, so a stored row is always valid.
func (s Settings) Validate() error {
	if s.MinMatchPercent < MinPercent || s.MinMatchPercent > MaxPercent {
		return ErrInvalidSettings
	}
	return nil
}

// GetSettings reads the singleton row (seeded by the schema, so it always exists).
func GetSettings() (Settings, error) {
	var s Settings
	var updated *string
	err := db.JobsDB.QueryRow(`SELECT ignore_enabled, min_match_enabled, min_match_percent, updated_at FROM crawler_settings WHERE id = 1`).
		Scan(&s.IgnoreEnabled, &s.MinMatchEnabled, &s.MinMatchPercent, &updated)
	if err != nil {
		return Settings{}, err
	}
	if updated != nil {
		s.UpdatedAt = *updated
	}
	return s, nil
}

// UpdateSettings stores the editable fields and returns the saved row.
func UpdateSettings(in Settings) (Settings, error) {
	if err := in.Validate(); err != nil {
		return Settings{}, err
	}
	if _, err := db.JobsDB.Exec(
		`UPDATE crawler_settings SET ignore_enabled = ?, min_match_enabled = ?, min_match_percent = ?, updated_at = ? WHERE id = 1`,
		in.IgnoreEnabled, in.MinMatchEnabled, in.MinMatchPercent, now().UTC().Format(timeFormat),
	); err != nil {
		return Settings{}, err
	}
	return GetSettings()
}

// SettingsHandler handles GET and PUT /settings/crawler. A PUT must carry every editable field: a missing number would otherwise read
// as 0 and be refused as out of range, the clearer answer than a silent default.
func SettingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s, err := GetSettings()
		if err != nil {
			http.Error(w, "failed to read the crawler settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, s)
	case http.MethodPut:
		r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
		var in Settings
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "body must be the crawler settings as JSON", http.StatusBadRequest)
			return
		}
		saved, err := UpdateSettings(in)
		if errors.Is(err, ErrInvalidSettings) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "failed to save the crawler settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, saved)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// matcherBuilders is the list of rules the settings can switch on: each reads the settings and returns its matcher, or nil when its
// rule is off. A later matcher adds one entry here (and its fields to Settings).
var matcherBuilders = []func(Settings) (jobs.CriteriaMatcher, error){
	func(s Settings) (jobs.CriteriaMatcher, error) {
		if !s.MinMatchEnabled {
			return nil, nil
		}
		return NewMinProfileMatchFromData(s.MinMatchPercent)
	},
}

// Source is the jobs.CriteriaSource of the tool: it reads the settings and builds the matchers that are switched on, once per crawl.
// With "Ignore jobs" off it builds none, so every job is kept.
type Source struct{}

// Matchers implements jobs.CriteriaSource.
func (Source) Matchers() ([]jobs.CriteriaMatcher, error) {
	s, err := GetSettings()
	if err != nil {
		return nil, err
	}
	if !s.IgnoreEnabled {
		return nil, nil
	}
	var out []jobs.CriteriaMatcher
	for _, build := range matcherBuilders {
		m, err := build(s)
		if err != nil {
			return nil, err
		}
		if m != nil {
			out = append(out, m)
		}
	}
	return out, nil
}

var _ jobs.CriteriaSource = Source{}
