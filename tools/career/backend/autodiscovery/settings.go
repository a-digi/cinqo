// settings.go is the auto-discovery feature's own configuration
// surface — the singleton auto_discovery_settings row (db.go's own
// jobsSchema), read/written here, and its plain HTTP mirror
// (GET/PUT /portal-links/auto-discovery). No manager reads this yet —
// that's a later step; toggling this setting has no visible effect on
// its own until then. See
// plan/ai/tools/career/step-84-auto-discovery-scheduled-crawling.md.
package autodiscovery

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"career-tool-backend/db"
)

// errInvalidInterval is a caller mistake (400), never a server failure.
var errInvalidInterval = errors.New("intervalMinutes must be one of 15, 30, 60, 180, 360, 720, 1440")

// Settings is the singleton row's own shape — LastRunAt is unix
// seconds (nil means auto-discovery has never actually run), matching
// this feature's own explicit "store as unix" requirement directly, no
// RFC3339 conversion needed on this one field.
type Settings struct {
	Enabled         bool   `json:"enabled"`
	IntervalMinutes int    `json:"intervalMinutes"`
	LastRunAt       *int64 `json:"lastRunAt"`
}

// GetSettings reads the singleton row — seeded once at schema creation
// (db.go's own jobsSchema always inserts id = 1), so sql.ErrNoRows
// never happens in practice.
func GetSettings() (Settings, error) {
	var s Settings
	var lastRunAt sql.NullInt64
	err := db.JobsDB.QueryRow(
		`SELECT enabled, interval_minutes, last_run_at FROM auto_discovery_settings WHERE id = 1`,
	).Scan(&s.Enabled, &s.IntervalMinutes, &lastRunAt)
	if err != nil {
		return Settings{}, err
	}
	if lastRunAt.Valid {
		s.LastRunAt = &lastRunAt.Int64
	}
	return s, nil
}

// UpdateSettings validates intervalMinutes against allowedIntervalMinutes
// before writing — errInvalidInterval otherwise. Never touches
// last_run_at — that's the scheduler manager's own field (a later
// step), not something a settings write should ever reset.
func UpdateSettings(enabled bool, intervalMinutes int) error {
	if !db.AllowedAutoDiscoveryIntervalMinutes[intervalMinutes] {
		return errInvalidInterval
	}
	_, err := db.JobsDB.Exec(
		`UPDATE auto_discovery_settings SET enabled = ?, interval_minutes = ?, updated_at = datetime('now') WHERE id = 1`,
		enabled, intervalMinutes,
	)
	return err
}

// recordSweepStart sets last_run_at to now (unix seconds) — called by
// the manager (manager.go) at the START of a sweep, not its end: a
// sweep that visits many links can itself take longer than one tick,
// and measuring "time since the last sweep began" (not "since it last
// finished") is what actually prevents two sweeps overlapping when
// combined with the manager's own in-process sweepRunning guard. Never
// called from SettingsHandler's own PUT — a user changing the interval
// or toggling enabled must never reset how overdue the current cycle
// already is.
func recordSweepStart() error {
	_, err := db.JobsDB.Exec(
		`UPDATE auto_discovery_settings SET last_run_at = ?, updated_at = datetime('now') WHERE id = 1`,
		time.Now().Unix(),
	)
	return err
}

type updateSettingsRequest struct {
	Enabled         bool `json:"enabled"`
	IntervalMinutes int  `json:"intervalMinutes"`
}

// SettingsHandler handles GET/PUT /portal-links/auto-discovery.
func SettingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s, err := GetSettings()
		if err != nil {
			http.Error(w, "failed to load auto-discovery settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, s)

	case http.MethodPut:
		var body updateSettingsRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := UpdateSettings(body.Enabled, body.IntervalMinutes); err != nil {
			if errors.Is(err, errInvalidInterval) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, "failed to update auto-discovery settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s, err := GetSettings()
		if err != nil {
			http.Error(w, "settings saved but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, s)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
