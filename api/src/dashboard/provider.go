package dashboard

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Caller is who a widget's data is being fetched for.
type Caller struct {
	UserID string
	Scopes []string
	// Token is the caller's own access token. A tool provider forwards it as a
	// bearer credential, so the tool only ever acts with what that user may do.
	Token string
}

// HasScope reports whether the caller holds scope, or cinqo:super:admin, which
// satisfies every scope check (the same convention as the tool proxy).
func (c Caller) HasScope(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope || s == AdminScope {
			return true
		}
	}
	return false
}

// AdminScope is the scope that satisfies every other scope.
const AdminScope = "cinqo:super:admin"

// ErrUnavailable is what a provider returns when its data source is down
// (a stopped tool, say). The engine reports it as state "unavailable", not
// "error", so the page can say "not running" instead of "broken".
var ErrUnavailable = errors.New("widget data source is unavailable")

// Definition describes one widget. Provider is the source's name — "cinqo" for
// the system, the tool's slug for a tool — and Key is unique within it, so the
// widget's id is "<provider>.<key>".
type Definition struct {
	Provider       string
	Key            string
	Title          string
	Description    string
	Kind           Kind
	Size           Size
	RefreshSeconds int
	// RequiredScope gates who sees the widget at all. Empty means every
	// signed-in user.
	RequiredScope string
}

// ID is "<provider>.<key>".
func (d Definition) ID() string { return d.Provider + "." + d.Key }

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Validate checks a definition is registrable.
func (d Definition) Validate() error {
	if !namePattern.MatchString(d.Provider) {
		return fmt.Errorf("widget provider %q must match %s", d.Provider, namePattern)
	}
	if !namePattern.MatchString(d.Key) {
		return fmt.Errorf("widget key %q must match %s", d.Key, namePattern)
	}
	if strings.TrimSpace(d.Title) == "" {
		return fmt.Errorf("widget %s: title is required", d.ID())
	}
	if _, ok := ParseKind(string(d.Kind)); !ok {
		return fmt.Errorf("widget %s: unknown kind %q", d.ID(), d.Kind)
	}
	if d.Size.W < MinWidth || d.Size.W > MaxWidth || d.Size.H < MinHeight || d.Size.H > MaxHeight {
		return fmt.Errorf("widget %s: size %dx%d outside %dx%d..%dx%d", d.ID(), d.Size.W, d.Size.H, MinWidth, MinHeight, MaxWidth, MaxHeight)
	}
	if d.RefreshSeconds < MinRefreshSeconds || d.RefreshSeconds > MaxRefreshSeconds {
		return fmt.Errorf("widget %s: refresh %ds outside %d..%d", d.ID(), d.RefreshSeconds, MinRefreshSeconds, MaxRefreshSeconds)
	}
	return nil
}

// Provider supplies one widget's data. Data must honour ctx (the engine bounds
// it) and return a payload for the definition's kind; the engine validates it,
// so a provider never has to trust itself nor be trusted.
type Provider interface {
	Definition() Definition
	Data(ctx context.Context, caller Caller) ([]byte, error)
}

// Source yields the providers that exist right now. The system's own registry is
// one; installed tools' widgets (a later step) are another. It is asked afresh on
// every request, so a tool installed or removed shows up without a restart.
type Source interface {
	Providers() []Provider
}
