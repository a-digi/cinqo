// Package entity holds the local AI module's settings and the rules a settings change must satisfy. See
// plan/ai/small-llm-local/step-181-local-ai-small-models.md.
package entity

import (
	"errors"
	"fmt"
	"strings"

	"github.com/a-digi/cinqo/src/localai/catalog"
	"github.com/a-digi/cinqo/src/localai/hardware"
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

// RecommendedToolContext is the context size recommended for a model that can use tools: a tool task reads documents and hands
// results over in pieces, all in one context. Measured on a real CV import with Phi-4-mini (2026-11-03): the first two requests
// alone were 6,232 tokens, so 16,384 is the least that works for an ordinary CV. (The Career tool's own minimum is the same number.)
const RecommendedToolContext = 16384

// ContextSizes are the context sizes the Local AI page offers (and the ones the program suggests), in tokens, ascending.
var ContextSizes = []int{2048, 4096, 8192, 16384, 32768, 65536, 131072}

// CheckMemory refuses a context size the machine does not have the memory for: the model, its cache for that many tokens and the
// reserve for the system. It says what the size needs and the largest size that fits. It judges only what the context adds: a
// machine that cannot hold the model at all is the business of the "needs more memory" rule (so choosing such a model while the
// local AI is off keeps working), and nothing is refused when the machine's memory, the model's cache size or the model is not
// known (an unknown is never held against the admin).
func CheckMemory(s Settings, cat catalog.Catalog, ramBytes uint64) error {
	m, ok := cat.Find(s.ModelID)
	if !ok {
		return nil
	}
	largest, known := hardware.LargestContext(m.SizeBytes, m.KVBytesPerToken, ramBytes, SizesUpTo(m.ContextTokens))
	required := hardware.RequiredBytes(m.SizeBytes, m.KVBytesPerToken, s.ContextTokens)
	if !known || required <= ramBytes || largest == 0 {
		return nil
	}
	return invalid("a context of %s tokens needs about %s of memory (the model, its cache and a reserve for the system); this computer has %s. Choose at most %s tokens",
		hardware.Thousands(s.ContextTokens), hardware.GiB(required), hardware.GiB(ramBytes), hardware.Thousands(largest))
}

// SizesUpTo is the offered sizes the model can read (all of them when its maximum is not known).
func SizesUpTo(modelMax int) []int {
	out := make([]int, 0, len(ContextSizes))
	for _, size := range ContextSizes {
		if modelMax <= 0 || size <= modelMax {
			out = append(out, size)
		}
	}
	return out
}
