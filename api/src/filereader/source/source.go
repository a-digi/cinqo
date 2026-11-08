// Package source decides WHICH files the file reader may open. A client never sends a path: it names a
// source and a reference ("ai-trace" + "<conversationId>/<turnRunId>"), and the source turns that into a
// file under a root the server owns, or refuses. See plan/ai/file-reader/file-reader-engine.md.
package source

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ErrNotFound is the one answer for every way a reference can fail to name a readable file (malformed,
// escaping its root, missing, not a regular file), so a caller learns nothing about what exists.
var ErrNotFound = errors.New("source: not found")

// AdminScope satisfies every source's scope check, as everywhere else in Cinqo.
const AdminScope = "cinqo:super:admin"

type Source interface {
	ID() string
	// Scopes lists the scopes that may read this source (any one of them is enough); AdminScope always may.
	Scopes() []string
	// Resolve turns a reference into the absolute path of a regular file inside the source's root.
	Resolve(ref string) (string, error)
}

// Resolver finds a source that is not registered up front (an installed tool's, which comes and goes with the
// tool) by its id.
type Resolver func(id string) (Source, bool)

type prefixed struct {
	prefix  string
	resolve Resolver
}

// Registry holds the sources by id: the ones registered at boot, and the ones a resolver finds for an id with
// its prefix.
type Registry struct {
	mu        sync.RWMutex
	sources   map[string]Source
	resolvers []prefixed
}

func NewRegistry() *Registry { return &Registry{sources: map[string]Source{}} }

// AddResolver lets ids that start with prefix ("tool:") be looked up on demand.
func (r *Registry) AddResolver(prefix string, resolve Resolver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolvers = append(r.resolvers, prefixed{prefix: prefix, resolve: resolve})
}

func (r *Registry) Register(s Source) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.ID() == "" || len(s.Scopes()) == 0 {
		return fmt.Errorf("source: %q needs an id and at least one scope", s.ID())
	}
	if _, dup := r.sources[s.ID()]; dup {
		return fmt.Errorf("source: %q is already registered", s.ID())
	}
	r.sources[s.ID()] = s
	return nil
}

func (r *Registry) Get(id string) (Source, bool) {
	r.mu.RLock()
	s, ok := r.sources[id]
	resolvers := r.resolvers
	r.mu.RUnlock()
	if ok {
		return s, true
	}
	for _, p := range resolvers {
		if strings.HasPrefix(id, p.prefix) {
			if s, ok := p.resolve(id); ok {
				return s, true
			}
		}
	}
	return nil, false
}

// Allowed reports whether a caller holding callerScopes may read the source.
func Allowed(s Source, callerScopes []string) bool {
	for _, have := range callerScopes {
		if have == AdminScope {
			return true
		}
		for _, need := range s.Scopes() {
			if have == need {
				return true
			}
		}
	}
	return false
}

// ResolveUnder joins the relative path (already validated by the source, as single path segments) under
// root and returns the real path of the regular file it names. Symlinks are resolved first and the result
// must still lie inside the (resolved) root, so a link that points out of it is refused.
func ResolveUnder(root string, segments ...string) (string, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", ErrNotFound
	}
	real, err := filepath.EvalSymlinks(filepath.Join(append([]string{realRoot}, segments...)...))
	if err != nil {
		return "", ErrNotFound
	}
	if !strings.HasPrefix(real, realRoot+string(os.PathSeparator)) {
		return "", ErrNotFound
	}
	st, err := os.Stat(real)
	if err != nil || !st.Mode().IsRegular() {
		return "", ErrNotFound
	}
	return real, nil
}
