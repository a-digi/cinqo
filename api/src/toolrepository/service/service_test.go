package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/a-digi/cinqo/src/toolrepository/fetch"
	"github.com/a-digi/cinqo/src/toolrepository/status"
)

const sum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func repoFile(name, pkgURL string) string {
	return fmt.Sprintf(`{"formatVersion":1,"name":%q,"description":"d","tools":[{"id":"browser","currentVersionId":"v1","versions":[
	{"versionId":"v1","name":"Browser","version":"1.0.0","packages":[{"os":"any","arch":"any","url":%q,"sha256":%q}]}]}]}`, name, pkgURL, sum)
}

// fakeFetcher serves canned files and reports whether the server was local.
type fakeFetcher struct {
	files map[string]string
	local bool
	err   error
	calls []string
}

func (f *fakeFetcher) Get(_ context.Context, url string, _ int64, p fetch.LocalPolicy) (*fetch.Result, error) {
	f.calls = append(f.calls, url)
	if f.err != nil {
		return nil, f.err
	}
	body, ok := f.files[url]
	if !ok {
		return nil, errors.New("the server answered with status 404")
	}
	return &fetch.Result{Body: []byte(body), Local: f.local}, nil
}

func newService(t *testing.T, f *fakeFetcher) *Service {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	for _, name := range []string{"30_10_2026_10_00_00.sql", "30_10_2026_11_00_00.sql", "30_10_2026_12_00_00.sql"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "db", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, stmt := range strings.Split(string(raw), "/***Statement***/") {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				t.Fatal(err)
			}
		}
	}
	s := New(db, f)
	n := 0
	s.now = func() time.Time { n++; return time.Date(2026, 10, 30, 12, 0, n, 0, time.UTC) }
	return s
}

func kind(t *testing.T, err error, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("want %v, got %v", want, err)
	}
}

const link = "https://example.com/cinqo/tools.json"

func TestAddValidRepository(t *testing.T) {
	f := &fakeFetcher{files: map[string]string{link: repoFile("My tools", "a.zip")}}
	s := newService(t, f)
	repo, err := s.Add(context.Background(), " "+link+"#x ")
	if err != nil {
		t.Fatal(err)
	}
	if repo.URL != link || repo.Name != "My tools" || repo.Description != "d" || repo.LastStatus != "ok" || repo.LastFetchedAt == "" {
		t.Errorf("%+v", repo)
	}
	if !strings.Contains(repo.CatalogJSON, `"browser"`) {
		t.Error("catalog not stored verbatim")
	}
	list, _ := s.List()
	if len(list) != 1 {
		t.Fatalf("list: %d", len(list))
	}
}

func TestAddNameFallsBackToLinkHostAndFile(t *testing.T) {
	f := &fakeFetcher{files: map[string]string{link: strings.Replace(repoFile("", "a.zip"), `"name":"",`, "", 1)}}
	repo, err := newService(t, f).Add(context.Background(), link)
	if err != nil || repo.Name != "example.com/tools.json" {
		t.Fatalf("%v %+v", err, repo)
	}
}

func TestAddInvalidFileIsRejectedWithProblemsAndNothingStored(t *testing.T) {
	bad := `{"formatVersion":1,"tools":[{"id":"BAD","currentVersionId":"zz","versions":[]}]}`
	s := newService(t, &fakeFetcher{files: map[string]string{link: bad}})
	_, err := s.Add(context.Background(), link)
	kind(t, err, ErrInvalidFile)
	if !strings.Contains(err.Error(), "tools[0].id") || !strings.Contains(err.Error(), "currentVersionId") {
		t.Errorf("problems missing: %v", err)
	}
	if list, _ := s.List(); len(list) != 0 {
		t.Error("an invalid repository was stored")
	}
}

func TestAddFetchFailure(t *testing.T) {
	s := newService(t, &fakeFetcher{err: errors.New("the server took too long to answer")})
	_, err := s.Add(context.Background(), link)
	kind(t, err, ErrFetchFailed)
	if list, _ := s.List(); len(list) != 0 {
		t.Error("stored despite failure")
	}
}

func TestAddDuplicateLinkDoesNotFetch(t *testing.T) {
	f := &fakeFetcher{files: map[string]string{link: repoFile("A", "a.zip")}}
	s := newService(t, f)
	if _, err := s.Add(context.Background(), link); err != nil {
		t.Fatal(err)
	}
	f.calls = nil
	_, err := s.Add(context.Background(), link+"#other")
	kind(t, err, ErrDuplicate)
	if len(f.calls) != 0 {
		t.Error("fetched a duplicate")
	}
}

func TestAddRefusesBadLinks(t *testing.T) {
	s := newService(t, &fakeFetcher{})
	for _, l := range []string{"", "  ", "example.com/tools.json", "/tmp/tools.json", "file:///tmp/tools.json",
		"ftp://example.com/tools.json", "http://example.com/tools.json", "https://u:p@example.com/tools.json",
		"https://example.com", "https://example.com/", "https://example.com/cinqo/"} {
		_, err := s.Add(context.Background(), l)
		kind(t, err, ErrInvalidLink)
	}
	if _, err := NormalizeLink("http://localhost:9000/repo/tools.json"); err != nil {
		t.Errorf("local http link refused: %v", err)
	}
	if got, err := NormalizeLink("https://example.com/a/tools?v=1"); err != nil || got != "https://example.com/a/tools?v=1" {
		t.Errorf("any file name must do: %q %v", got, err)
	}
}

func TestFolderLinkMessage(t *testing.T) {
	_, err := NormalizeLink("https://example.com/cinqo/")
	if err == nil || err.Error() != "this link must point to the JSON file" {
		t.Errorf("%v", err)
	}
}

func TestPublicRepositoryMayNotPointAtLocalAddress(t *testing.T) {
	file := repoFile("Evil", "http://192.168.1.10/a.zip")
	s := newService(t, &fakeFetcher{files: map[string]string{link: file}, local: false})
	_, err := s.Add(context.Background(), link)
	kind(t, err, ErrInvalidFile)
	if !strings.Contains(err.Error(), "must not point at a local address") {
		t.Errorf("%v", err)
	}
}

func TestLocalRepositoryMayPointAtLocalAddress(t *testing.T) {
	local := "http://192.168.1.10:8080/repo/tools.json"
	s := newService(t, &fakeFetcher{files: map[string]string{local: repoFile("Lan", "http://192.168.1.10:8080/a.zip")}, local: true})
	if _, err := s.Add(context.Background(), local); err != nil {
		t.Fatal(err)
	}
}

func TestTooManyRepositories(t *testing.T) {
	f := &fakeFetcher{files: map[string]string{}}
	s := newService(t, f)
	for i := 0; i < MaxRepositories; i++ {
		l := fmt.Sprintf("https://example.com/r%d.json", i)
		f.files[l] = repoFile("r", "a.zip")
		if _, err := s.Add(context.Background(), l); err != nil {
			t.Fatal(err)
		}
	}
	f.files["https://example.com/extra.json"] = repoFile("r", "a.zip")
	_, err := s.Add(context.Background(), "https://example.com/extra.json")
	kind(t, err, ErrTooMany)
}

func TestRefreshUpdatesCatalog(t *testing.T) {
	f := &fakeFetcher{files: map[string]string{link: repoFile("Old", "a.zip")}}
	s := newService(t, f)
	repo, _ := s.Add(context.Background(), link)
	f.files[link] = repoFile("New", "b.zip")
	got, err := s.Refresh(context.Background(), repo.ID)
	if err != nil || got.Name != "New" || got.LastStatus != "ok" || !strings.Contains(got.CatalogJSON, "b.zip") {
		t.Fatalf("%v %+v", err, got)
	}
	if got.LastFetchedAt == repo.LastFetchedAt {
		t.Error("lastFetchedAt not advanced")
	}
}

func TestRefreshFailureKeepsLastGoodCatalog(t *testing.T) {
	f := &fakeFetcher{files: map[string]string{link: repoFile("Good", "a.zip")}}
	s := newService(t, f)
	repo, _ := s.Add(context.Background(), link)

	f.files[link] = `{"formatVersion":1,"tools":[{"id":"BAD"}]}`
	got, err := s.Refresh(context.Background(), repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastStatus != "error" || !strings.Contains(got.LastError, "not valid") {
		t.Errorf("%+v", got)
	}
	if got.Name != "Good" || got.CatalogJSON != repo.CatalogJSON {
		t.Error("last good catalog lost")
	}

	f.err = errors.New("the server could not be reached")
	got, _ = s.Refresh(context.Background(), repo.ID)
	if got.LastStatus != "error" || !strings.Contains(got.LastError, "could not be reached") || got.CatalogJSON != repo.CatalogJSON {
		t.Errorf("%+v", got)
	}

	f.err = nil
	f.files[link] = repoFile("Good", "a.zip")
	got, _ = s.Refresh(context.Background(), repo.ID)
	if got.LastStatus != "ok" || got.LastError != "" {
		t.Errorf("error not cleared: %+v", got)
	}
}

func TestRefreshAndRemoveUnknownID(t *testing.T) {
	s := newService(t, &fakeFetcher{})
	_, err := s.Refresh(context.Background(), "nope")
	kind(t, err, ErrNotFound)
	kind(t, s.Remove("nope"), ErrNotFound)
}

func TestRemove(t *testing.T) {
	f := &fakeFetcher{files: map[string]string{link: repoFile("A", "a.zip")}}
	s := newService(t, f)
	repo, _ := s.Add(context.Background(), link)
	if err := s.Remove(repo.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.List(); len(list) != 0 {
		t.Error("not removed")
	}
	if _, err := s.Add(context.Background(), link); err != nil {
		t.Errorf("link cannot be re-added after removal: %v", err)
	}
}

func TestRepositoryJSONDoesNotExposeCatalog(t *testing.T) {
	f := &fakeFetcher{files: map[string]string{link: repoFile("A", "a.zip")}}
	repo, _ := newService(t, f).Add(context.Background(), link)
	out, _ := json.Marshal(repo)
	if strings.Contains(string(out), "browser") || strings.Contains(string(out), "catalog") {
		t.Errorf("catalog leaked: %s", out)
	}
}

func TestSummariesAndDetailReflectInstalledTools(t *testing.T) {
	f := &fakeFetcher{files: map[string]string{link: repoFile("A", "a.zip")}}
	s := newService(t, f)
	repo, _ := s.Add(context.Background(), link)

	env := status.Env{OS: "linux", Arch: "amd64", Installed: map[string]string{}}
	sums, err := s.Summaries(env)
	if err != nil || len(sums) != 1 || sums[0].Counts.NotInstalled != 1 || sums[0].Counts.Tools != 1 {
		t.Fatalf("%v %+v", err, sums)
	}

	env.Installed["browser"] = "0.5.0"
	d, err := s.Detail(repo.ID, env)
	if err != nil || d.Counts.Updatable != 1 || len(d.Tools) != 1 || d.Tools[0].Status != status.UpdateAvailable {
		t.Fatalf("%v %+v", err, d)
	}

	env.Installed["browser"] = "1.0.0"
	d, _ = s.Detail(repo.ID, env)
	if d.Counts.UpToDate != 1 || d.Tools[0].Status != status.UpToDate {
		t.Fatalf("%+v", d)
	}

	if _, err := s.Detail("nope", env); !errors.Is(err, ErrNotFound) {
		t.Errorf("%v", err)
	}
}

func TestDetailJSONShape(t *testing.T) {
	f := &fakeFetcher{files: map[string]string{link: repoFile("A", "a.zip")}}
	s := newService(t, f)
	repo, _ := s.Add(context.Background(), link)
	d, _ := s.Detail(repo.ID, status.Env{OS: "linux", Arch: "amd64"})
	out, _ := json.Marshal(d)
	for _, want := range []string{`"id":"` + repo.ID + `"`, `"url"`, `"counts"`, `"tools":[`, `"status":"not_installed"`, `"versions":[`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
	if strings.Contains(string(out), "catalog") {
		t.Error("catalog leaked")
	}
}

func TestRepositoryWithoutCatalogHasNoTools(t *testing.T) {
	s := newService(t, &fakeFetcher{})
	if _, err := s.db.Exec(`INSERT INTO tool_repositories (id, url) VALUES ('x','https://e.com/r.json')`); err != nil {
		t.Fatal(err)
	}
	d, err := s.Detail("x", status.Env{})
	if err != nil || len(d.Tools) != 0 || d.Counts.Tools != 0 {
		t.Fatalf("%v %+v", err, d)
	}
	out, _ := json.Marshal(d)
	if !strings.Contains(string(out), `"tools":[]`) {
		t.Errorf("tools must be an empty array: %s", out)
	}
}

func TestTheSameLinkIsNeverAddedTwice(t *testing.T) {
	canonical := "https://example.com/a/tools.json"
	f := &fakeFetcher{files: map[string]string{canonical: repoFile("A", "a.zip")}}
	s := newService(t, f)
	if _, err := s.Add(context.Background(), "HTTPS://Example.COM:443/a/tools.json#section"); err != nil {
		t.Fatal(err)
	}
	same := []string{
		canonical, "  " + canonical + "  ", "https://EXAMPLE.com/a/tools.json", "https://example.com:443/a/tools.json",
		"https://example.com./a/tools.json", "https://example.com/a/tools.json#other", "https://example.com/a/tools.json?",
	}
	for _, l := range same {
		f.calls = nil
		_, err := s.Add(context.Background(), l)
		kind(t, err, ErrDuplicate)
		if len(f.calls) != 0 {
			t.Errorf("%q: a duplicate must not be fetched", l)
		}
	}
	if list, _ := s.List(); len(list) != 1 || list[0].URL != canonical {
		t.Errorf("one row, stored in the canonical form: %+v", list)
	}
}

func TestDifferentLinksAreNotDuplicates(t *testing.T) {
	canonical := "https://example.com/a/tools.json"
	f := &fakeFetcher{files: map[string]string{}}
	s := newService(t, f)
	f.files[canonical] = repoFile("A", "a.zip")
	if _, err := s.Add(context.Background(), canonical); err != nil {
		t.Fatal(err)
	}
	for _, l := range []string{
		"https://example.com/A/tools.json", // the path is case-sensitive
		"https://example.com/a/Tools.json",
		"https://example.com:8443/a/tools.json", // another port
		"https://example.com/a/tools.json?v=2",  // another query
		"https://example.org/a/tools.json",      // another host
		"https://other.example.com/a/tools.json",
	} {
		f.files[mustNormalize(t, l)] = repoFile("B", "a.zip")
		if _, err := s.Add(context.Background(), l); err != nil {
			t.Errorf("%q must be accepted: %v", l, err)
		}
	}
}

func mustNormalize(t *testing.T, l string) string {
	t.Helper()
	n, err := NormalizeLink(l)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestNormalizeLinkSpellings(t *testing.T) {
	cases := map[string]string{
		"HTTPS://Example.COM/a/tools.json":        "https://example.com/a/tools.json",
		"https://example.com:443/a/tools.json":    "https://example.com/a/tools.json",
		"http://localhost:80/t.json":              "http://localhost/t.json",
		"http://localhost:8099/t.json":            "http://localhost:8099/t.json",
		"https://example.com./t.json":             "https://example.com/t.json",
		"https://example.com/t.json?":             "https://example.com/t.json",
		"https://example.com/t.json?a=1#x":        "https://example.com/t.json?a=1",
		"http://[::1]:8099/t.json":                "http://[::1]:8099/t.json",
		"https://example.com/Path/With/Case.json": "https://example.com/Path/With/Case.json",
	}
	for in, want := range cases {
		if got, err := NormalizeLink(in); err != nil || got != want {
			t.Errorf("%q: got %q, %v; want %q", in, got, err, want)
		}
	}
}

// raceFetcher stores the same link while Add is fetching it, like a second
// request that got past the existence check first.
type raceFetcher struct {
	inner *fakeFetcher
	s     *Service
	url   string
}

func (r *raceFetcher) Get(ctx context.Context, url string, max int64, p fetch.LocalPolicy) (*fetch.Result, error) {
	if _, err := r.s.db.Exec(`INSERT INTO tool_repositories (id, url) VALUES ('raced', ?)`, r.url); err != nil {
		return nil, err
	}
	return r.inner.Get(ctx, url, max, p)
}

// Two adds racing past the existence check hit the database's unique index; the
// loser gets the same answer as any duplicate, not a server error.
func TestDuplicateInsertRaceIsReportedAsADuplicate(t *testing.T) {
	inner := &fakeFetcher{files: map[string]string{link: repoFile("A", "a.zip")}}
	s := newService(t, inner)
	s.fetcher = &raceFetcher{inner: inner, s: s, url: link}
	_, err := s.Add(context.Background(), link)
	kind(t, err, ErrDuplicate)
	if list, _ := s.List(); len(list) != 1 || list[0].ID != "raced" {
		t.Errorf("only the first insert may remain: %+v", list)
	}
}
