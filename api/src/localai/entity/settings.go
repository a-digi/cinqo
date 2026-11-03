// Package entity holds the local AI module's settings and the rules a settings change must satisfy. See
// plan/ai/small-llm-local/step-181-local-ai-small-models.md.
package entity

import (
	"errors"
	"fmt"
	"strings"

	"github.com/a-digi/cinqo/src/localai/catalog"
)

// Bounds of the numeric settings.
const (
	MinContextTokens = 512
	MaxContextTokens = 131072
	MaxIdleMinutes   = 1440
)

// Settings is the single global settings row (id always 1, enforced by the table's CHECK). The local AI is managed by
// Cinqo only: ModelID is the id of a catalog model, never free text.
type Settings struct {
	_             struct{} `table:"local_ai_settings"`
	ID            int      `db:"id" dbtype:"INTEGER" nullable:"false" json:"-"`
	Enabled       bool     `db:"enabled" dbtype:"INTEGER" nullable:"false" json:"enabled"`
	ModelID       string   `db:"model_id" dbtype:"TEXT" nullable:"false" json:"modelId"`
	ContextTokens int      `db:"context_tokens" dbtype:"INTEGER" nullable:"false" json:"contextTokens"`
	IdleMinutes   int      `db:"idle_minutes" dbtype:"INTEGER" nullable:"false" json:"idleMinutes"`
	UpdatedAt     *string  `db:"updated_at" dbtype:"TEXT" nullable:"true" json:"-"`
}

// Configured reports whether the settings are complete enough to answer a request: switched on and a model chosen. It is
// a pure check of the settings (no disk, no process), cheap enough to ask on every request. Whether the model is
// installed and running is the engine's business.
func (s Settings) Configured() bool { return s.Enabled && s.ModelID != "" }

// ErrInvalidSettings wraps every validation failure; the message is safe to show to the admin.
var ErrInvalidSettings = errors.New("invalid local AI settings")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidSettings, fmt.Sprintf(format, args...))
}

// Validate checks a requested settings change against the catalog and returns it normalised. cannotEnable is the reason
// the local AI cannot be switched on right now (nil when it can): while it is set, switching on is refused with that
// reason, and everything else may still be saved.
func Validate(in Settings, cat catalog.Catalog, cannotEnable error) (Settings, error) {
	out := in
	out.ID = 1
	out.ModelID = strings.TrimSpace(in.ModelID)

	if out.ContextTokens < MinContextTokens || out.ContextTokens > MaxContextTokens {
		return Settings{}, invalid("contextTokens must be between %d and %d", MinContextTokens, MaxContextTokens)
	}
	if out.IdleMinutes < 0 || out.IdleMinutes > MaxIdleMinutes {
		return Settings{}, invalid("idleMinutes must be between 0 and %d", MaxIdleMinutes)
	}
	if out.ModelID != "" {
		m, ok := cat.Find(out.ModelID)
		if !ok {
			return Settings{}, invalid("modelId must be one of the models Cinqo offers")
		}
		if out.ContextTokens > m.ContextTokens {
			return Settings{}, invalid("contextTokens is larger than this model can read (at most %d)", m.ContextTokens)
		}
	}
	if out.Enabled {
		if out.ModelID == "" {
			return Settings{}, invalid("choose a model to switch the local AI on")
		}
		if cannotEnable != nil {
			return Settings{}, invalid("%s", cannotEnable.Error())
		}
	}
	return out, nil
}
