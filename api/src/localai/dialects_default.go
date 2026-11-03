package localai

import (
	"github.com/a-digi/cinqo/src/localai/dialect"
	"github.com/a-digi/cinqo/src/localai/dialect/phijson"
)

// defaultDialects is the one place that names concrete tool dialects. Adding one is a package and a line here; replacing one
// is changing its line; removing one is deleting its line (and its package): a model that names a dialect nobody registered
// has no tools, nothing else changes.
func defaultDialects() *dialect.Registry {
	r := dialect.NewRegistry()
	// The names are constants of valid shape and the registry is new, so these cannot fail; a test checks every name the
	// catalog uses is registered.
	_ = r.Register(phijson.Name, phijson.New())
	return r
}
