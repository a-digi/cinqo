package instruction

import (
	"fmt"
	"regexp"
	"sort"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

var (
	mu      sync.RWMutex
	engines = map[string]Engine{}
)

// Register adds an engine. It panics on an empty or malformed id or a second engine with the same id: that is a programming error found at
// start-up, never at run time. Called only from backend/engines.go (and from tests).
func Register(e Engine) {
	if e == nil {
		panic("instruction: Register(nil)")
	}
	id := e.ID()
	if !idPattern.MatchString(id) {
		panic(fmt.Sprintf("instruction: engine id %q must be lower case letters, digits and underscores", id))
	}
	mu.Lock()
	defer mu.Unlock()
	if _, dup := engines[id]; dup {
		panic(fmt.Sprintf("instruction: engine %q is registered twice", id))
	}
	engines[id] = e
}

// ForBrowserEngine returns the engine for the Browser's selected engine name; false when none is registered (an engine that was removed, an
// unknown name, or no reading): the shared code then judges nothing by an engine.
func ForBrowserEngine(name string) (Engine, bool) {
	mu.RLock()
	defer mu.RUnlock()
	e, ok := engines[name]
	return e, ok
}

// All returns every registered engine, sorted by id.
func All() []Engine {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Engine, 0, len(engines))
	for _, e := range engines {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// Reset removes every engine (tests only).
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	engines = map[string]Engine{}
}

// RegisterAllTools registers the AI tools of every registered engine on the MCP server, engines in id order. main.go calls it where the
// engines' tools used to be registered one by one.
func RegisterAllTools(server *mcp.Server) {
	for _, e := range All() {
		e.RegisterTools(server)
	}
}
