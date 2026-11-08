package comments

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DBPath returns where an owner's comments database lives:
//
//	system: <dataDir>/db/comments.db
//	tool:   <dataDir>/db/tools/<slug>/comments.db   (inside the tool's own TOOL_DB_DIR)
//
// Each owner has its own file, so one owner's comments can never be mixed with
// another's. The result is checked to be inside dataDir.
func DBPath(dataDir, owner string) (string, error) {
	if err := checkOwner(owner); err != nil {
		return "", err
	}
	root, err := filepath.Abs(dataDir)
	if err != nil {
		return "", fmt.Errorf("resolve data dir: %w", err)
	}
	path := filepath.Join(root, "db", "comments.db")
	if owner != SystemOwner {
		path = filepath.Join(root, "db", "tools", owner, "comments.db")
	}
	if !strings.HasPrefix(path, root+string(filepath.Separator)) {
		return "", ErrInvalidOwner
	}
	return path, nil
}

// Manager hands out one Store per owner, opening each file the first time it is
// needed. Safe for concurrent use.
type Manager struct {
	dataDir string

	mu     sync.Mutex
	stores map[string]*Store

	now   func() time.Time
	newID func() string
}

func NewManager(dataDir string) *Manager {
	return &Manager{dataDir: dataDir, stores: map[string]*Store{}, now: time.Now, newID: newUUID}
}

// Store returns the owner's store, opening (and creating, with its schema) the
// database on first use.
func (m *Manager) Store(owner string) (*Store, error) {
	path, err := DBPath(m.dataDir, owner)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.stores[owner]; ok {
		return s, nil
	}
	s, err := openStore(path, m.now, m.newID)
	if err != nil {
		return nil, err
	}
	m.stores[owner] = s
	return s, nil
}

// Close closes every open database. The Manager can be used again afterwards;
// files are reopened on demand.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var first error
	for owner, s := range m.stores {
		if err := s.db.Close(); err != nil && first == nil {
			first = err
		}
		delete(m.stores, owner)
	}
	return first
}
