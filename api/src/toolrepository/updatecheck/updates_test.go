package updatecheck

import (
	"fmt"
	"testing"

	repo_entity "github.com/a-digi/cinqo/src/toolrepository/entity"
	"github.com/a-digi/cinqo/src/toolrepository/status"
)

const sum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// catalogOf lists tool id -> current version, each with a package for linux/amd64
// (or for windows only when platform is "windows").
func catalogOf(tools map[string]string, platform string) string {
	body, sep := "", ""
	for id, ver := range tools {
		body += fmt.Sprintf(`%s{"id":%q,"currentVersionId":"c","versions":[{"versionId":"c","name":%q,"version":%q,
			"packages":[{"os":%q,"arch":"amd64","url":"https://example.com/%s.zip","sha256":%q}]}]}`, sep, id, "Tool "+id, ver, platform, id, sum)
		sep = ","
	}
	return `{"formatVersion":1,"tools":[` + body + `]}`
}

func repoWith(id, name string, native bool, catalog string) *repo_entity.Repository {
	return &repo_entity.Repository{ID: id, Name: name, URL: "https://example.com/" + id + "/repo.json", IsNative: native, CatalogJSON: catalog}
}

var env = status.Env{OS: "linux", Arch: "amd64", AppVersion: "0.0.100", Installed: map[string]string{
	"browser": "0.0.189", "career": "0.0.345", "pdf_tools": "0.0.100",
}}

func TestUpdatesListsOnlyInstalledToolsWithANewerVersion(t *testing.T) {
	r := repoWith("r1", "Cinqo", true, catalogOf(map[string]string{
		"browser":   "0.0.190", // update
		"career":    "0.0.345", // up to date
		"pdf_tools": "0.0.050", // older than installed
		"extra":     "9.9.9",   // not installed
	}, "linux"))
	got := Updates([]*repo_entity.Repository{r}, env)
	if len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	u := got[0]
	if u.Slug != "browser" || u.Name != "Tool browser" || u.InstalledVersion != "0.0.189" || u.LatestVersion != "0.0.190" ||
		u.VersionID != "c" || u.RepositoryID != "r1" || u.RepositoryName != "Cinqo" {
		t.Errorf("%+v", u)
	}
}

func TestUpdatesExcludesWhatCannotBeInstalledHere(t *testing.T) {
	r := repoWith("r1", "Cinqo", true, catalogOf(map[string]string{"browser": "0.0.190"}, "windows"))
	if got := Updates([]*repo_entity.Repository{r}, env); len(got) != 0 {
		t.Errorf("no package for this platform: %+v", got)
	}
	needsNewerApp := `{"formatVersion":1,"tools":[{"id":"browser","currentVersionId":"c","versions":[{"versionId":"c","name":"B","version":"0.0.190",
		"minAppVersion":"9.0.0","packages":[{"os":"linux","arch":"amd64","url":"https://example.com/b.zip","sha256":"` + sum + `"}]}]}]}`
	if got := Updates([]*repo_entity.Repository{repoWith("r1", "C", true, needsNewerApp)}, env); len(got) != 0 {
		t.Errorf("minAppVersion: %+v", got)
	}
}

func TestTheSameToolInTwoRepositoriesAppearsOnceWithTheHighestVersion(t *testing.T) {
	builtin := repoWith("native", "Cinqo", true, catalogOf(map[string]string{"browser": "0.0.190"}, "linux"))
	other := repoWith("other", "Other", false, catalogOf(map[string]string{"browser": "0.0.200"}, "linux"))
	got := Updates([]*repo_entity.Repository{builtin, other}, env)
	if len(got) != 1 || got[0].LatestVersion != "0.0.200" || got[0].RepositoryID != "other" {
		t.Fatalf("%+v", got)
	}
	// numeric, not text, ordering
	a := repoWith("a", "A", true, catalogOf(map[string]string{"browser": "0.0.9"}, "linux"))
	b := repoWith("b", "B", false, catalogOf(map[string]string{"browser": "0.0.10"}, "linux"))
	env2 := env
	env2.Installed = map[string]string{"browser": "0.0.1"}
	if got := Updates([]*repo_entity.Repository{a, b}, env2); got[0].LatestVersion != "0.0.10" {
		t.Errorf("%+v", got)
	}
}

func TestATieGoesToTheRepositoryListedFirst(t *testing.T) {
	first := repoWith("native", "Cinqo", true, catalogOf(map[string]string{"browser": "0.0.190"}, "linux"))
	second := repoWith("other", "Other", false, catalogOf(map[string]string{"browser": "0.0.190"}, "linux"))
	if got := Updates([]*repo_entity.Repository{first, second}, env); got[0].RepositoryID != "native" {
		t.Errorf("%+v", got)
	}
	if got := Updates([]*repo_entity.Repository{second, first}, env); got[0].RepositoryID != "other" {
		t.Errorf("list order decides: %+v", got)
	}
}

func TestUpdatesIsSortedNeverNilAndSkipsUnusableCatalogs(t *testing.T) {
	if got := Updates(nil, env); got == nil || len(got) != 0 {
		t.Errorf("%#v", got)
	}
	repos := []*repo_entity.Repository{
		repoWith("never", "Never loaded", false, ""),
		repoWith("bad", "Broken", false, "{not json"),
		repoWith("ok", "Ok", true, catalogOf(map[string]string{"pdf_tools": "0.0.200", "browser": "0.0.190"}, "linux")),
	}
	got := Updates(repos, env)
	if len(got) != 2 || got[0].Slug != "browser" || got[1].Slug != "pdf_tools" {
		t.Errorf("%+v", got)
	}
}

func TestLastCheckedAtIsTheNewestFetch(t *testing.T) {
	if LastCheckedAt(nil) != "" {
		t.Error("none")
	}
	repos := []*repo_entity.Repository{
		{LastFetchedAt: "2026-10-30 11:56:03"}, {LastFetchedAt: ""}, {LastFetchedAt: "2026-10-30 12:11:51"},
	}
	if got := LastCheckedAt(repos); got != "2026-10-30 12:11:51" {
		t.Errorf("%q", got)
	}
}
