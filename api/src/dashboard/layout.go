package dashboard

// LayoutEntry is one widget in a user's own arrangement of the dashboard.
type LayoutEntry struct {
	WidgetID string
	Hidden   bool
}

// Placed is a widget definition together with whether this user has hidden it.
type Placed struct {
	Definition Definition
	Hidden     bool
}

// MaxLayoutEntries bounds a saved layout.
const MaxLayoutEntries = 200

// Arrange puts defs in the user's order. Widgets the user has arranged come first,
// in their saved order, carrying their hidden flag; widgets they have never seen
// (a freshly installed tool's, say) follow in the default order, visible. Saved
// entries for widgets that are not in defs — removed, disabled, or no longer
// allowed — are ignored. Nothing is invented and nothing is dropped.
func Arrange(defs []Definition, saved []LayoutEntry) []Placed {
	byID := make(map[string]Definition, len(defs))
	for _, d := range defs {
		byID[d.ID()] = d
	}
	placed := make([]Placed, 0, len(defs))
	used := make(map[string]struct{}, len(defs))
	for _, e := range saved {
		d, ok := byID[e.WidgetID]
		if !ok {
			continue
		}
		if _, dup := used[e.WidgetID]; dup {
			continue
		}
		used[e.WidgetID] = struct{}{}
		placed = append(placed, Placed{Definition: d, Hidden: e.Hidden})
	}
	for _, d := range defs {
		if _, ok := used[d.ID()]; !ok {
			placed = append(placed, Placed{Definition: d})
		}
	}
	return placed
}

// SanitizeLayout keeps what may be saved for a caller: entries for widgets they
// can see, once each, in the order given. An id that is unknown or not allowed is
// dropped without a word, so saving cannot be used to probe which widgets exist.
func SanitizeLayout(visible []Definition, submitted []LayoutEntry) []LayoutEntry {
	allowed := make(map[string]struct{}, len(visible))
	for _, d := range visible {
		allowed[d.ID()] = struct{}{}
	}
	out := make([]LayoutEntry, 0, len(submitted))
	seen := make(map[string]struct{}, len(submitted))
	for _, e := range submitted {
		if _, ok := allowed[e.WidgetID]; !ok {
			continue
		}
		if _, dup := seen[e.WidgetID]; dup {
			continue
		}
		seen[e.WidgetID] = struct{}{}
		out = append(out, e)
	}
	return out
}
