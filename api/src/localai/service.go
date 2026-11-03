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
		NoTools: true,
	}
}
