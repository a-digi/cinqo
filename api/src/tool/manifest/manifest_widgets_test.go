package manifest

import (
	"strings"
	"testing"
)

func widgetManifest(w ...WidgetDecl) Manifest {
	return Manifest{
		Info: testInfo(),
		Slug: "demo", Name: "Demo", Version: "1.0.0",
		Scopes: []ScopeDecl{{Scope: "tool:demo:read"}},
		Routes: []RouteDecl{
			{Method: "GET", PathSuffix: "dashboard/jobs", RequiredScope: "tool:demo:read"},
			{Method: "POST", PathSuffix: "dashboard/post", RequiredScope: "tool:demo:read"},
			{Method: "GET", PathSuffix: "dashboard/open", RequiredScope: ""},
		},
		Widgets: w,
	}
}

func okWidget() WidgetDecl {
	return WidgetDecl{Key: "jobs", Title: "Jobs", Kind: "stat", DataPath: "dashboard/jobs"}
}

func TestValidateWidgets(t *testing.T) {
	in := ValidationInput{HasBackendExecutable: true, CurrentAppVersion: "1.0.0"}
	if _, err := Validate(widgetManifest(okWidget()), in); err != nil {
		t.Fatalf("valid widget rejected: %v", err)
	}
	custom := okWidget()
	custom.Kind = "custom"
	if _, err := Validate(widgetManifest(custom), in); err != nil {
		t.Fatalf("a custom widget must be declarable: %v", err)
	}
	full := okWidget()
	full.SchemaVersion, full.Width, full.Height, full.RefreshSeconds = 1, 4, 3, 3600
	if _, err := Validate(widgetManifest(full), in); err != nil {
		t.Fatalf("fully specified widget rejected: %v", err)
	}
	if _, err := Validate(widgetManifest(), in); err != nil {
		t.Fatalf("no widgets rejected: %v", err)
	}

	for name, mutate := range map[string]func(*WidgetDecl){
		"bad key":         func(w *WidgetDecl) { w.Key = "Bad-Key" },
		"no title":        func(w *WidgetDecl) { w.Title = "" },
		"unknown kind":    func(w *WidgetDecl) { w.Kind = "table" },
		"too wide":        func(w *WidgetDecl) { w.Width = 5 },
		"too tall":        func(w *WidgetDecl) { w.Height = 4 },
		"fast refresh":    func(w *WidgetDecl) { w.RefreshSeconds = 5 },
		"slow refresh":    func(w *WidgetDecl) { w.RefreshSeconds = 100000 },
		"schema version":  func(w *WidgetDecl) { w.SchemaVersion = 2 },
		"unknown route":   func(w *WidgetDecl) { w.DataPath = "dashboard/nope" },
		"non-GET route":   func(w *WidgetDecl) { w.DataPath = "dashboard/post" },
		"route no scope":  func(w *WidgetDecl) { w.DataPath = "dashboard/open" },
		"empty data_path": func(w *WidgetDecl) { w.DataPath = "" },
	} {
		w := okWidget()
		mutate(&w)
		if _, err := Validate(widgetManifest(w), in); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	if _, err := Validate(widgetManifest(okWidget(), okWidget()), in); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("duplicate key: %v", err)
	}
	noBackend := ValidationInput{HasFrontendBundle: true, CurrentAppVersion: "1.0.0"}
	if _, err := Validate(widgetManifest(okWidget()), noBackend); err == nil {
		t.Error("widgets without a backend accepted")
	}
}

func TestWidgetDefinitionDefaults(t *testing.T) {
	def := WidgetDefinition("demo", okWidget())
	if def.ID() != "demo.jobs" || def.Size.W != 1 || def.Size.H != 1 || def.RefreshSeconds != 60 {
		t.Errorf("defaults: %+v", def)
	}
}
