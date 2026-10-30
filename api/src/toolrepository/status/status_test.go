package status

import (
	"fmt"
	"testing"

	"github.com/a-digi/cinqo/src/toolrepository/catalog"
)

const sum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// tool builds a catalog tool whose current version is `current`, with older
// versions listed too.
func tool(t *testing.T, id, current, minApp string, platforms ...[2]string) *catalog.Tool {
	t.Helper()
	pk := ""
	for i, p := range platforms {
		if i > 0 {
			pk += ","
		}
		pk += fmt.Sprintf(`{"os":%q,"arch":%q,"url":"a.zip","sha256":%q}`, p[0], p[1], sum)
	}
	min := ""
	if minApp != "" {
		min = `,"minAppVersion":"` + minApp + `"`
	}
	data := fmt.Sprintf(`{"formatVersion":1,"tools":[{"id":%q,"currentVersionId":"cur","versions":[
		{"versionId":"old","name":"Old","version":"0.0.1","packages":[%s]},
		{"versionId":"cur","name":"Current","description":"desc","version":%q%s,"packages":[%s]}]}]}`, id, pk, current, min, pk)
	c, err := catalog.Parse([]byte(data), "https://e.com/r.json")
	if err != nil {
		t.Fatal(err)
	}
	return &c.Tools[0]
}

var darwin = [2]string{"darwin", "arm64"}

func env(installed string) Env {
	e := Env{OS: "darwin", Arch: "arm64", AppVersion: "0.0.100", Installed: map[string]string{}}
	if installed != "" {
		e.Installed["browser"] = installed
	}
	return e
}

func TestStatuses(t *testing.T) {
	cases := []struct {
		name      string
		installed string
		current   string
		want      string
	}{
		{"not installed", "", "1.0.0", NotInstalled},
		{"equal", "1.0.0", "1.0.0", UpToDate},
		{"older installed", "0.9.9", "1.0.0", UpdateAvailable},
		{"newer installed", "1.0.1", "1.0.0", NewerThanRepository},
		{"numeric not lexical", "0.0.9", "0.0.10", UpdateAvailable},
		{"two digit minor", "0.10.0", "0.9.0", NewerThanRepository},
		{"missing trailing part equals", "1.0", "1.0.0", UpToDate},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ForTool(tool(t, "browser", c.current, "", darwin), env(c.installed))
			if got.Status != c.want {
				t.Errorf("got %s want %s", got.Status, c.want)
			}
		})
	}
}

func TestRowFields(t *testing.T) {
	got := ForTool(tool(t, "browser", "1.0.0", "", darwin), env("0.5.0"))
	if got.Name != "Current" || got.Description != "desc" || got.InstalledVersion != "0.5.0" ||
		got.CurrentVersion != "1.0.0" || got.CurrentVersionID != "cur" || len(got.Versions) != 2 {
		t.Errorf("%+v", got)
	}
}

func TestUnavailableWhenNoPackageForThisPlatform(t *testing.T) {
	tl := tool(t, "browser", "1.0.0", "", [2]string{"windows", "amd64"})
	got := ForTool(tl, env(""))
	if got.Status != Unavailable || got.Reason == "" {
		t.Errorf("%+v", got)
	}
	got = ForTool(tl, env("0.5.0")) // would be an update, but cannot be installed
	if got.Status != Unavailable {
		t.Errorf("update without a package: %+v", got)
	}
	if got.Versions[0].Available || got.Versions[0].Reason == "" {
		t.Errorf("version availability: %+v", got.Versions[0])
	}
}

func TestPlatformFallbackToAny(t *testing.T) {
	got := ForTool(tool(t, "browser", "1.0.0", "", [2]string{"any", "any"}), env(""))
	if got.Status != NotInstalled {
		t.Errorf("%+v", got)
	}
}

func TestMinAppVersion(t *testing.T) {
	tl := tool(t, "browser", "1.0.0", "0.0.200", darwin)
	if got := ForTool(tl, env("")); got.Status != Unavailable {
		t.Errorf("fresh install needs newer app: %+v", got)
	}
	if got := ForTool(tl, env("0.5.0")); got.Status != Unavailable {
		t.Errorf("update needs newer app: %+v", got)
	}
	// an up-to-date tool stays up to date, whatever the app version
	if got := ForTool(tl, env("1.0.0")); got.Status != UpToDate {
		t.Errorf("%+v", got)
	}
	e := env("")
	e.AppVersion = "0.0.200"
	if got := ForTool(tl, e); got.Status != NotInstalled {
		t.Errorf("exactly the minimum must pass: %+v", got)
	}
	e.AppVersion = ""
	if got := ForTool(tl, e); got.Status != NotInstalled {
		t.Errorf("unknown app version skips the check: %+v", got)
	}
}

func TestUnparseableInstalledVersion(t *testing.T) {
	got := ForTool(tool(t, "browser", "1.0.0", "", darwin), env("dev-build"))
	if got.Status != Unavailable || got.Reason == "" {
		t.Errorf("%+v", got)
	}
}

func TestSameToolInTwoRepositoriesHasItsOwnStatus(t *testing.T) {
	a := tool(t, "browser", "1.0.0", "", darwin)
	b := tool(t, "browser", "2.0.0", "", darwin)
	e := env("1.0.0")
	if got := ForTool(a, e); got.Status != UpToDate {
		t.Errorf("a: %+v", got)
	}
	if got := ForTool(b, e); got.Status != UpdateAvailable {
		t.Errorf("b: %+v", got)
	}
}

func TestCounts(t *testing.T) {
	c := &catalog.Catalog{}
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		tl := tool(t, id, "1.0.0", "", darwin)
		c.Tools = append(c.Tools, *tl)
	}
	e := env("")
	e.Installed = map[string]string{"a": "1.0.0", "b": "0.9.0", "c": "0.9.0", "d": "2.0.0"}
	rows := ForCatalog(c, e)
	got := Count(rows)
	want := Counts{Tools: 5, Installed: 4, UpToDate: 1, Updatable: 2, NotInstalled: 1}
	if got != want {
		t.Errorf("got %+v want %+v", got, want)
	}
	if rows[0].ID != "a" || rows[4].ID != "e" {
		t.Error("order not kept")
	}
	if (Count(nil) != Counts{}) {
		t.Error("empty")
	}
}
