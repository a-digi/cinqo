package localai

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/a-digi/cinqo/src/localai/catalog"
	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/hardware"
	"github.com/a-digi/cinqo/src/localai/repository/query"
	"github.com/a-digi/cinqo/src/platform"
	platform_entity "github.com/a-digi/cinqo/src/platform/entity"
)

// PlatformID is the id of the platform this module registers.
const PlatformID = "local"

// offeredModels are the models a new conversation may choose (plan 206): every model that is ready and fits, the saved default first,
// for an engine that can list them; the saved model alone, when it is ready, for one that cannot. A few file checks, cheap enough for
// every question about availability. Empty: the local AI cannot answer yet.
func offeredModels(defaultID string) []string {
	if l, ok := engine.(modelLister); ok {
		return l.ReadyModels(defaultID)
	}
	if engine.Ready(context.Background()) == nil {
		return []string{defaultID}
	}
	return nil
}

// engineAnswers says why the local AI cannot answer at all right now (nil when at least one model can). The model of a conversation is
// checked again when it asks (Chat), so a conversation on a model that is gone gets that model's own reason.
func engineAnswers(ctx context.Context, defaultID string) error {
	if l, ok := engine.(modelLister); ok && len(l.ReadyModels(defaultID)) > 0 {
		return nil
	}
	return engine.Ready(ctx)
}

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
			return "", "", engineAnswers(ctx, s.ModelID)
		},
		Models: func() []string {
			if s, ok := load(); ok && s.Configured() {
				return offeredModels(s.ModelID)
			}
			return nil
		},
		// A readable name for each model, so a tool's picker can say "Qwen2.5-Coder-3B-Instruct, 2.0 GiB" instead of the file id (plan 206).
		ModelNames: modelNames,
		// The tasks a model was measured not to manage (plan 207), so Career can warn before a CV import on the 1.5B model.
		ModelUnsuitable: modelUnsuitable,
		// How a long task is run for a model (Phi-4-mini: in code, with small questions, plan 209).
		ModelDriver: modelDrivers,
		// Offered only when it can really answer (plan 205): switched on AND the program and at least one model are on disk and fit.
		// While they are still being downloaded, or after they were deleted, "Local AI" is not offered anywhere, exactly as when it is
		// off. The models offered are all the ready ones, the saved default first (plan 206).
		Available: func() bool {
			s, ok := load()
			return ok && s.Configured() && len(offeredModels(s.ModelID)) > 0
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
		// A small model repeats a call it already has the answer to (the same document read again and again).
		AnswerRepeatedCalls: true,
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

// modelNames is the readable name of every catalog model: its name and its download size ("Qwen2.5-Coder-3B-Instruct, 2.0 GiB").
func modelNames() map[string]string {
	cat, err := catalog.Default()
	if err != nil {
		return nil
	}
	out := make(map[string]string, len(cat.Models))
	for _, m := range cat.Models {
		out[m.ID] = fmt.Sprintf("%s, %s", m.Name, hardware.GiB(uint64(m.SizeBytes)))
	}
	return out
}

// modelDrivers is the driver of every catalog model that has one.
func modelDrivers() map[string]string {
	cat, err := catalog.Default()
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, m := range cat.Models {
		if m.Driver != "" {
			out[m.ID] = m.Driver
		}
	}
	return out
}

// modelUnsuitable is, for every catalog model that is marked, the tasks it cannot do.
func modelUnsuitable() map[string][]string {
	cat, err := catalog.Default()
	if err != nil {
		return nil
	}
	out := map[string][]string{}
	for _, m := range cat.Models {
		if len(m.UnsuitableFor) > 0 {
			out[m.ID] = append([]string(nil), m.UnsuitableFor...)
		}
	}
	return out
}
