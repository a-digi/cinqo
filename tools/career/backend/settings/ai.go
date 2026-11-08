// Package settings holds the Career tool's own settings. Today that is one thing: the
// AI platform and model the whole tool uses. The row is a singleton (db.go's ai_settings)
// read and written here and mirrored by GET/PUT /settings/ai.
package settings

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"career-tool-backend/db"
)

const maxFieldRunes = 200

// ErrInvalid is a caller mistake (a 400), never a server failure.
var ErrInvalid = errors.New("platformId and model must be plain text of at most 200 characters")

// AI is the chosen platform and model. Both empty means "not chosen".
type AI struct {
	PlatformID string `json:"platformId"`
	Model      string `json:"model"`
}

// GetAI reads the singleton row (seeded by the schema, so it always exists).
func GetAI() (AI, error) {
	var a AI
	err := db.JobsDB.QueryRow(`SELECT platform_id, model FROM ai_settings WHERE id = 1`).Scan(&a.PlatformID, &a.Model)
	return a, err
}

func clean(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if strings.ContainsRune(s, 0) || utf8.RuneCountInString(s) > maxFieldRunes {
		return "", false
	}
	return s, true
}

// UpdateAI stores the choice. A model without a platform makes no sense, so it is
// refused; clearing both returns the tool to "not chosen".
func UpdateAI(in AI) (AI, error) {
	platform, ok1 := clean(in.PlatformID)
	model, ok2 := clean(in.Model)
	if !ok1 || !ok2 || (platform == "" && model != "") {
		return AI{}, ErrInvalid
	}
	if _, err := db.JobsDB.Exec(
		`UPDATE ai_settings SET platform_id = ?, model = ?, updated_at = datetime('now') WHERE id = 1`, platform, model,
	); err != nil {
		return AI{}, err
	}
	return AI{PlatformID: platform, Model: model}, nil
}

// AIHandler handles GET and PUT /settings/ai.
func AIHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a, err := GetAI()
		if err != nil {
			http.Error(w, "failed to read the AI settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, a)
	case http.MethodPut:
		r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
		var in AI
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, `body must be {"platformId": "…", "model": "…"}`, http.StatusBadRequest)
			return
		}
		saved, err := UpdateAI(in)
		if errors.Is(err, ErrInvalid) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "failed to save the AI settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, saved)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
