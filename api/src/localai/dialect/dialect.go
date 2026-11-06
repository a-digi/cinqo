// Package dialect is how the local AI teaches a model to use tools when the model's own chat template cannot do it the OpenAI
// way (the template of Phi-4-mini, for example, ignores the tools of a request). A Dialect is one such way, a Registry holds
// the ones a program knows by name, and the catalog says per model which name applies. Nothing here knows a concrete dialect:
// each one lives in its own package, is added with one Register call where the module is put together, and is removed or
// replaced by deleting or changing that call. A model whose dialect is not registered simply gets no tools.
package dialect

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// Dialect has two halves: Encode turns a request into the text format the model was trained on (the tools in the system
// message, the tool traffic as plain text), and Decode reads the model's text answer back into tool calls.
type Dialect interface {
	// Encode returns the messages to send; the tools are then not sent as a field. It never changes the caller's slice.
	Encode(messages []chatcompleter.Message, tools []chatcompleter.ToolDef) []chatcompleter.Message
	// Decode turns a text answer that holds tool calls into tool calls, only for the tools that were offered; anything else is
	// left as it is.
	Decode(result *chatcompleter.ChatCompletionResult, tools []chatcompleter.ToolDef)
}

// NativeToolsDialect is implemented by a dialect whose model's chat template renders the tools itself (the tools stay in the request) and
// that only has to read the model's answer back: Encode is then not used to teach the tools, and Decode still runs. A dialect without it
// teaches the tools in the messages and the request carries none.
type NativeToolsDialect interface {
	NativeTools() bool
}

// Lookup is what the rest of the module needs from a registry: the dialect for a name.
type Lookup interface {
	Lookup(name string) (Dialect, bool)
}

// NamePattern is what a dialect name looks like: lower-case words joined by hyphens, as written in the catalog.
var NamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// MaxNameLength bounds a name.
const MaxNameLength = 32

// ValidName reports whether name can be a dialect name.
func ValidName(name string) bool { return len(name) <= MaxNameLength && NamePattern.MatchString(name) }

// Registry holds dialects by name.
type Registry struct {
	mu sync.RWMutex
	m  map[string]Dialect
}

// NewRegistry is an empty registry.
func NewRegistry() *Registry { return &Registry{m: map[string]Dialect{}} }

// ErrInvalid is returned by Register for a name or dialect that cannot work.
var ErrInvalid = errors.New("dialect: invalid registration")

// Register adds a dialect. A name that is taken is refused: replacing a dialect means changing its one registration, not
// registering twice.
func (r *Registry) Register(name string, d Dialect) error {
	if !ValidName(name) || d == nil {
		return fmt.Errorf("%w: %q", ErrInvalid, name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, taken := r.m[name]; taken {
		return fmt.Errorf("%w: %q is already registered", ErrInvalid, name)
	}
	r.m[name] = d
	return nil
}

// Lookup is the dialect registered under name.
func (r *Registry) Lookup(name string) (Dialect, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.m[name]
	return d, ok
}

// Names are the registered names, sorted.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.m))
	for n := range r.m {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
