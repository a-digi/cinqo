package manifest

import (
	"fmt"
	"strings"
	"testing"
)

func browserWith(routes ...string) map[string]InstalledToolInfo {
	set := map[string]struct{}{}
	for _, r := range routes {
		set[r] = struct{}{}
	}
	return map[string]InstalledToolInfo{"browser": {Version: "1.0.0", Enabled: true, BackgroundRoutes: set}}
}

func bgManifest(decls ...BackgroundAccessDecl) Manifest {
	return Manifest{Slug: "namako", RequiresTools: []RequiredToolDecl{{Slug: "browser"}}, BackgroundAccess: decls}
}

func ask(tool string, routes ...string) BackgroundAccessDecl {
	d := BackgroundAccessDecl{Tool: tool}
	for _, r := range routes {
		method, path, _ := strings.Cut(r, " ")
		d.Routes = append(d.Routes, BackgroundRouteDecl{Method: method, PathSuffix: path})
	}
	return d
}

func TestABackgroundAccessDeclarationIsAcceptedWhenTheTargetAllowsEveryRoute(t *testing.T) {
	in := browserWith("POST list-links", "POST fetch-frames", "GET other")
	if err := ValidateBackground(bgManifest(ask("browser", "POST list-links", "POST fetch-frames")), in, true); err != nil {
		t.Errorf("%v", err)
	}
	if err := ValidateBackground(bgManifest(), in, false); err != nil {
		t.Errorf("a manifest that asks for nothing needs no backend: %v", err)
	}
	if err := ValidateBackground(Manifest{Slug: "x"}, nil, false); err != nil {
		t.Errorf("%v", err)
	}
}

func TestABackgroundAccessDeclarationIsRefusedWhenAnythingIsOff(t *testing.T) {
	in := browserWith("POST list-links", "POST fetch-frames")
	many := make([]string, 0, MaxBackgroundRoutesPerTool+1)
	extra := map[string]struct{}{}
	for i := 0; i <= MaxBackgroundRoutesPerTool; i++ {
		r := fmt.Sprintf("GET r%d", i)
		many = append(many, r)
		extra[r] = struct{}{}
	}
	cases := map[string]struct {
		m       Manifest
		in      map[string]InstalledToolInfo
		backend bool
		want    string
	}{
		"the target does not allow that route":   {bgManifest(ask("browser", "POST login-window")), in, true, "does not allow background use of POST login-window"},
		"the method must match too":              {bgManifest(ask("browser", "GET list-links")), in, true, "does not allow background use of GET list-links"},
		"target not installed":                   {bgManifest(ask("browser", "POST list-links")), map[string]InstalledToolInfo{}, true, "does not allow"},
		"not a required tool":                    {Manifest{Slug: "namako", BackgroundAccess: []BackgroundAccessDecl{ask("browser", "POST list-links")}}, in, true, "requires_tools"},
		"itself":                                 {Manifest{Slug: "namako", RequiresTools: []RequiredToolDecl{{Slug: "browser"}}, BackgroundAccess: []BackgroundAccessDecl{ask("namako", "GET x")}}, in, true, "own routes"},
		"bad slug":                               {bgManifest(ask("Bad-Slug", "GET x")), in, true, "must match"},
		"a tool twice":                           {bgManifest(ask("browser", "POST list-links"), ask("browser", "POST fetch-frames")), in, true, "twice"},
		"no routes":                              {bgManifest(BackgroundAccessDecl{Tool: "browser"}), in, true, "at least one route"},
		"too many routes":                        {bgManifest(ask("browser", many...)), map[string]InstalledToolInfo{"browser": {Enabled: true, BackgroundRoutes: extra}}, true, "at most"},
		"a route twice":                          {bgManifest(ask("browser", "POST list-links", "POST list-links")), in, true, "twice"},
		"no backend to call with":                {bgManifest(ask("browser", "POST list-links")), in, false, "backend executable"},
		"method not allowed":                     {bgManifest(ask("browser", "TRACE list-links")), in, true, "method"},
		"lower case method":                      {bgManifest(ask("browser", "post list-links")), in, true, "method"},
		"empty path":                             {bgManifest(BackgroundAccessDecl{Tool: "browser", Routes: []BackgroundRouteDecl{{Method: "GET"}}}), in, true, "path_suffix"},
		"leading slash":                          {bgManifest(ask("browser", "GET /list-links")), in, true, "path_suffix"},
		"dot dot":                                {bgManifest(ask("browser", "GET a/../b")), in, true, "path_suffix"},
		"a query in the path":                    {bgManifest(ask("browser", "GET a?x=1")), in, true, "path_suffix"},
		"a fragment in the path":                 {bgManifest(ask("browser", "GET a#x")), in, true, "path_suffix"},
		"a backslash in the path":                {bgManifest(ask("browser", `GET a\b`)), in, true, "path_suffix"},
		"a space in the path":                    {bgManifest(ask("browser", "GET a b")), in, true, "path_suffix"},
		"a path of 201 characters":               {bgManifest(BackgroundAccessDecl{Tool: "browser", Routes: []BackgroundRouteDecl{{Method: "GET", PathSuffix: strings.Repeat("a", 201)}}}), in, true, "path_suffix"},
		"capability token and background on one": {Manifest{Slug: "x", Routes: []RouteDecl{{Method: "GET", PathSuffix: "a", AllowBackground: true, AllowCapabilityToken: true}}}, nil, false, "cannot be combined"},
	}
	for name, c := range cases {
		err := ValidateBackground(c.m, c.in, c.backend)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
}

func TestThePathLimitsAreExact(t *testing.T) {
	ok := strings.Repeat("a", maxBackgroundPathLength)
	in := map[string]InstalledToolInfo{"browser": {Enabled: true, BackgroundRoutes: map[string]struct{}{"GET " + ok: {}}}}
	if err := ValidateBackground(bgManifest(BackgroundAccessDecl{Tool: "browser", Routes: []BackgroundRouteDecl{{Method: "GET", PathSuffix: ok}}}), in, true); err != nil {
		t.Errorf("exactly the limit is fine: %v", err)
	}
	fifty := make([]string, MaxBackgroundRoutesPerTool)
	set := map[string]struct{}{}
	for i := range fifty {
		fifty[i] = fmt.Sprintf("GET r%d", i)
		set[fifty[i]] = struct{}{}
	}
	if err := ValidateBackground(bgManifest(ask("browser", fifty...)), map[string]InstalledToolInfo{"browser": {Enabled: true, BackgroundRoutes: set}}, true); err != nil {
		t.Errorf("exactly 50 routes is fine: %v", err)
	}
}

func TestValidateRunsTheBackgroundCheckAfterTheRequirements(t *testing.T) {
	m := metaManifest(func(m *Manifest) {
		m.Slug = "namako"
		m.RequiresTools = []RequiredToolDecl{{Slug: "browser"}}
		m.BackgroundAccess = []BackgroundAccessDecl{ask("browser", "POST login-window")}
	})
	_, err := Validate(m, ValidationInput{HasBackendExecutable: true, CurrentAppVersion: "1.0.0", InstalledTools: browserWith("POST list-links")})
	if err == nil || !strings.Contains(err.Error(), "does not allow background use") {
		t.Errorf("Validate must refuse it: %v", err)
	}
	m.BackgroundAccess = []BackgroundAccessDecl{ask("browser", "POST list-links")}
	if _, err := Validate(m, ValidationInput{HasBackendExecutable: true, CurrentAppVersion: "1.0.0", InstalledTools: browserWith("POST list-links")}); err != nil {
		t.Errorf("%v", err)
	}
}

func TestTheManifestFieldsReadFromJSON(t *testing.T) {
	m, err := Parse([]byte(`{"slug":"namako","routes":[{"method":"GET","path_suffix":"a","required_scope":"s","allow_background":true}],
		"background_access":[{"tool":"browser","routes":[{"method":"POST","path_suffix":"list-links"}]}]}`))
	if err != nil || !m.Routes[0].AllowBackground || len(m.BackgroundAccess) != 1 || m.BackgroundAccess[0].Tool != "browser" || m.BackgroundAccess[0].Routes[0].PathSuffix != "list-links" {
		t.Errorf("%+v %v", m, err)
	}
}
