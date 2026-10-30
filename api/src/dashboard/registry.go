package dashboard

import (
	"fmt"
	"sync"
)

// Registry holds the system's own (native) providers, registered once at boot.
// It is a Source.
type Registry struct {
	mu        sync.RWMutex
	providers []Provider
	ids       map[string]struct{}
}

func NewRegistry() *Registry {
	return &Registry{ids: map[string]struct{}{}}
}

// Register adds a provider. It fails on an invalid definition or a duplicate id.
func (r *Registry) Register(p Provider) error {
	def := p.Definition()
	if err := def.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.ids[def.ID()]; dup {
		return fmt.Errorf("widget %s is already registered", def.ID())
	}
	r.ids[def.ID()] = struct{}{}
	r.providers = append(r.providers, p)
	return nil
}

// Providers returns the registered providers in registration order.
func (r *Registry) Providers() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]Provider(nil), r.providers...)
}
