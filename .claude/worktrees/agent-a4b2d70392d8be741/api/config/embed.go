// Package config exposes the on-disk configuration directory
// (migrations, routes YAML, config.json) to the rest of the
// codebase. os.DirFS-backed, not go:embed, so migrations stay
// visible/diffable on disk — mirrors coco-iam's and coco-mda's own
// api/config/embed.go exactly (an earlier draft of this file assumed
// go:embed; that was corrected here to match what the reference apps
// actually ship, since go:embed of a directory pattern fails to build
// while config/db/migrations has no non-dotfile content, which is the
// case until step 05 adds the first real migration).
package config

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// EnvVarConfigDir names the env var the operator can set to
// point at a non-default config directory.
const EnvVarConfigDir = "CINQO_CONFIG_DIR"

// candidateDefaults returns the paths we try (in order) when the operator
// hasn't set EnvVarConfigDir. First existing directory wins.
//
// The executable-relative candidate is checked FIRST and is what actually
// matters in production: under a flat deploy layout, the binary lives
// beside its config directory regardless of the process's working
// directory. The plain relative candidates stay as fallbacks for local
// dev (`go run`/`go test` from the repo root).
func candidateDefaults() []string {
	candidates := []string{"api/config", "config", "./config"}
	if exe, err := os.Executable(); err == nil {
		candidates = append([]string{filepath.Join(filepath.Dir(exe), "config")}, candidates...)
	}
	return candidates
}

type configDir struct{ root string }

func (c configDir) Open(name string) (fs.File, error) {
	return os.Open(c.realPath(name))
}

func (c configDir) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(c.realPath(name))
}

func (c configDir) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(c.realPath(name))
}

func (c configDir) realPath(name string) string {
	clean := filepath.FromSlash(strings.TrimPrefix(name, "/"))
	return filepath.Join(c.root, clean)
}

func (c configDir) Root() string { return c.root }

var (
	configFSOnce sync.Once
	configFSVal  configDir
)

// ConfigFS resolves to the on-disk config directory.
var ConfigFS = lazyConfigFS{}

type lazyConfigFS struct{}

func (lazyConfigFS) Open(name string) (fs.File, error)          { return resolved().Open(name) }
func (lazyConfigFS) ReadFile(name string) ([]byte, error)       { return resolved().ReadFile(name) }
func (lazyConfigFS) ReadDir(name string) ([]fs.DirEntry, error) { return resolved().ReadDir(name) }
func (lazyConfigFS) Root() string                               { return resolved().Root() }

func resolved() configDir {
	configFSOnce.Do(func() { configFSVal = initConfigDir() })
	return configFSVal
}

func initConfigDir() configDir {
	root := strings.TrimSpace(os.Getenv(EnvVarConfigDir))
	if root == "" {
		root = pickFirstExisting(candidateDefaults())
	}
	abs, err := filepath.Abs(root)
	if err == nil {
		root = abs
	}
	return configDir{root: root}
}

func pickFirstExisting(candidates []string) string {
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	return candidates[0]
}

// ReadConfigFile reads a file from the config directory.
func ReadConfigFile(name string) ([]byte, error) {
	return ConfigFS.ReadFile(name)
}

// MigrationsPath returns the on-disk path to cinqo's single database's
// migrations folder. cinqo starts with one SQLite database
// (data/db/cinqo.db) — a dedicated *MigrationsPath function per
// additional database, matching coco-mda's own accumulated pattern, gets
// added only once a specific feature's design calls for a second
// database (see the plan's "one database to start" decision).
func MigrationsPath() (string, error) {
	return migrationsSubdir("migrations")
}

// ConversationMigrationsPath returns the on-disk path to the
// conversation feature's own database's migrations folder
// (data/db/conversation.db) — a separate SQLite database from the
// main cinqo.db, per plan/ai/conversation/step-01's own decision. See
// MigrationsPath's own doc comment for why this is a dedicated
// function per database rather than a parameterized one.
func ConversationMigrationsPath() (string, error) {
	return migrationsSubdir("migrations-conversation")
}

func migrationsSubdir(name string) (string, error) {
	full := filepath.Join(resolved().Root(), "db", name)
	info, err := os.Stat(full)
	if err != nil {
		return "", fmt.Errorf("config: migrations folder %q not found (set %s to point at api/config or equivalent): %w",
			full, EnvVarConfigDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("config: %q exists but is not a directory", full)
	}
	return full, nil
}
