package localai

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/repository/query"
	"github.com/a-digi/cinqo/src/platform"
	platform_entity "github.com/a-digi/cinqo/src/platform/entity"
)

// PlatformID is the id of the platform this module registers.
const PlatformID = "local"

// Entry builds the platform entry for the registry. Every seam reads the settings at call time, so an admin's change
// takes effect at once and nothing is cached.
func Entry(db *sql.DB) platform.Entry {
	repo := query.NewSettingsQueryRepo(db)
	load := func() (entity.Settings, bool) {
		s, err := repo.Load()
		return s, err == nil
	}
	return platform.Entry{
		Info:      platform_entity.Platform{ID: PlatformID, Name: "Local AI"},
		Completer: completer{settings: repo},
		// Credentials only says whether the engine can answer now. There is no key and no address to hand over: the
		// completer reaches the engine itself.
		Credentials: func(ctx context.Context) (string, string, error) {
			s, ok := load()
			if !ok {
				return "", "", fmt.Errorf("could not read the local AI settings")
			}
			if !s.Configured() {
				return "", "", ErrNotAvailable
			}
			return "", "", engine.Ready(ctx)
		},
		Models: func() []string {
			if s, ok := load(); ok && s.Configured() {
				return []string{s.ModelID}
			}
			return nil
		},
		Available: func() bool {
			s, ok := load()
			return ok && s.Configured()
		},
		// Tools only for a catalog model that was measured to call them (step 8), and only the ones a conversation names:
		// never every tool, and never in composer mode.
		ToolsFor: func(model string) platform.ToolPolicy {
			if toolsUsable(model) {
				return platform.ToolsExplicit
			}
			return platform.ToolsNone
		},
		// A small local model needs the work in short steps (measured on the CV import).
		ShortSteps: true,
		// What a conversation can hold at once is the context size of the settings (0 while the local AI is not configured).
		ContextTokens: func() int {
			if s, ok := load(); ok && s.Configured() {
				return s.ContextTokens
			}
			return 0
		},
		ToolHint: toolHint,
	}
}

// toolHint is told to a small model that has tools: it answers about the user's data from nothing unless it is told to look
// (measured in step 8: with a question it never called a tool).
const toolHint = "You have tools. For any question about the user's own data, always call the matching tool first and answer only from its result; never guess or invent data. Use a tool only when it is needed."
