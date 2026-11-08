package livelog

import (
	"fmt"
	"regexp"
	"sort"
	"sync"
)

// idPattern is what a provider id looks like: lower-case words joined by hyphens, short enough for a URL.
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

const maxIDLength = 32

// Registry holds the providers by id. It is the only thing the HTTP side asks "which providers are there and may this caller read
// one"; it never names a concrete provider (the composition root registers them).
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

func NewRegistry() *Registry { return &Registry{providers: map[string]Provider{}} }

// Register adds a provider. An id that is not a valid name, a provider without a title or without a scope, and a duplicate id are
// all refused, so a mistake in the composition fails at start-up and not at the first request.
func (r *Registry) Register(p Provider) error {
	if p == nil {
		return fmt.Errorf("livelog: a nil provider")
	}
	id := p.ID()
	if len(id) > maxIDLength || !idPattern.MatchString(id) {
		return fmt.Errorf("livelog: %q is not a valid provider id (lower-case words joined by hyphens, at most %d characters)", id, maxIDLength)
	}
	if p.Title() == "" {
		return fmt.Errorf("livelog: provider %q needs a title", id)
	}
	if len(p.Scopes()) == 0 {
		return fmt.Errorf("livelog: provider %q needs at least one scope", id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.providers[id]; dup {
		return fmt.Errorf("livelog: provider %q is already registered", id)
	}
	r.providers[id] = p
	return nil
}

// Get is the provider with this id, whoever the caller is: the scope check is Allowed, done per request by the handler.
func (r *Registry) Get(id string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[id]
	return p, ok
}

// Allowed reports whether a caller holding callerScopes may read the provider: AdminScope always, otherwise any one of its scopes.
func Allowed(p Provider, callerScopes []string) bool {
	for _, have := range callerScopes {
		if have == AdminScope {
			return true
		}
		for _, need := range p.Scopes() {
			if have == need {
				return true
			}
		}
	}
	return false
}

// Info is what a caller is told about a provider it may read.
type Info struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// List is the providers the caller may read, by id. A caller with no scope for any gets an empty (not nil) list.
func (r *Registry) List(callerScopes []string) []Info {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []Info{}
	for _, p := range r.providers {
		if Allowed(p, callerScopes) {
			out = append(out, Info{ID: p.ID(), Title: p.Title()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
