// Package dashboard is the Home dashboard's widget contract and engine.
// A widget is a definition (what it is), a Kind (its expected data schema
// and, in the frontend, its display UI) and a provider (who supplies the
// data). This file holds the shared vocabulary; kinds.go holds the kind
// schemas and their validators. See plan/ai/dashboard/home-dashboard.md.
package dashboard

import "encoding/json"

// Kind names a core-owned widget data schema. A provider (the system or a
// tool) picks one; it cannot invent one — each kind has a Go validator here
// and a React renderer in the frontend.
type Kind string

const (
	KindStat  Kind = "stat"
	KindList  Kind = "list"
	KindChart Kind = "chart"
	// KindCustom is for a widget whose display a tool draws itself (the bridge's
	// registerDashboardWidget). Its data is a free-form JSON object: core checks
	// that it is an object, not too deep and not too big, and leaves the shape to
	// the tool's own renderer. See kinds.go.
	KindCustom Kind = "custom"
)

// SchemaVersion is the version of every kind's schema in this release.
// Widgets declare the version they were written against; a version the
// engine does not know is rejected rather than guessed at.
const SchemaVersion = 1

// Kinds lists every known kind.
func Kinds() []Kind {
	return []Kind{KindStat, KindList, KindChart, KindCustom}
}

// ParseKind reports whether s names a known kind.
func ParseKind(s string) (Kind, bool) {
	for _, k := range Kinds() {
		if string(k) == s {
			return k, true
		}
	}
	return "", false
}

// State is what a widget's data request resolved to. A provider failure is a
// state, never an HTTP error: one failing widget must not break the page.
type State string

const (
	StateOK          State = "ok"
	StateEmpty       State = "empty"
	StateUnavailable State = "unavailable" // the tool is stopped or timed out
	StateError       State = "error"       // the provider failed or sent invalid data
)

// Size is a widget's footprint on the dashboard grid, in cells.
type Size struct {
	W int `json:"w"`
	H int `json:"h"`
}

// Grid and refresh bounds shared by every provider's definition.
const (
	MinWidth, MaxWidth                   = 1, 4
	MinHeight, MaxHeight                 = 1, 3
	MinRefreshSeconds, MaxRefreshSeconds = 15, 3600
	DefaultRefreshSeconds                = 60
)

// Data is the envelope returned for one widget's data request.
type Data struct {
	State         State           `json:"state"`
	Kind          Kind            `json:"kind"`
	SchemaVersion int             `json:"schemaVersion"`
	Data          json.RawMessage `json:"data,omitempty"`
	FetchedAt     string          `json:"fetchedAt"`
}
