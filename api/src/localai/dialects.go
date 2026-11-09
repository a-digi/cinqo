package localai

import (
	"sync"

	"github.com/a-digi/cinqo/src/localai/dialect"
	"github.com/a-digi/cinqo/src/localai/models"
)

// The registry of tool dialects the module knows. It is set once by Register (from dialects_default.go, the one place that
// names concrete dialects) and read by the completer and the platform entry. Without one, or with a name nobody registered,
// a model gets no tools: a missing dialect never breaks anything else.
var (
	dialectsMu sync.RWMutex
	dialects   dialect.Lookup
)

func setDialects(l dialect.Lookup) {
	dialectsMu.Lock()
	dialects = l
	dialectsMu.Unlock()
}

func currentDialects() dialect.Lookup {
	dialectsMu.RLock()
	defer dialectsMu.RUnlock()
	return dialects
}

// dialectOf is the tool dialect the catalog names for model: nil for a model that takes tools natively, and for one whose
// dialect is not registered (see toolsUsable).
func dialectOf(model string) dialect.Dialect {
	l := currentDialects()
	if l == nil {
		return nil
	}
	cat, err := models.Current()
	if err != nil {
		return nil
	}
	m, ok := cat.Find(model)
	if !ok {
		return nil
	}
	d, _ := l.Lookup(m.ToolDialect) // "" is never registered: a model without a dialect gets none
	return d
}

// toolsUsable says whether conversations on model may be given tools: the catalog says the model was measured to call them,
// and, when that needs a dialect, the dialect is registered. A model whose dialect was removed simply has no tools.
func toolsUsable(model string) bool {
	cat, err := models.Current()
	if err != nil {
		return false
	}
	m, ok := cat.Find(model)
	if !ok || !m.Tools {
		return false
	}
	if m.ToolDialect == "" {
		return true
	}
	l := currentDialects()
	if l == nil {
		return false
	}
	_, registered := l.Lookup(m.ToolDialect)
	return registered
}
