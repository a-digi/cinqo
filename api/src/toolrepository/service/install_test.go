package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/toolrepository/fetch"
	"github.com/a-digi/cinqo/src/toolrepository/status"
)

func hexSum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// installFile lists browser 0.0.1 ("old"), 0.0.5 ("mid") and 0.0.9 ("cur", the
// current one) for linux/amd64 only.
func installFile(pkgSum string, size int) string {
	pkg := func(u string) string {
		return fmt.Sprintf(`[{"os":"linux","arch":"amd64","url":%q,"sha256":%q,"sizeBytes":%d}]`, u, pkgSum, size)
	}
	return fmt.Sprintf(`{"formatVersion":1,"tools":[{"id":"browser","currentVersionId":"cur","versions":[
	{"versionId":"old","name":"B","version":"0.0.1","packages":%s},
	{"versionId":"mid","name":"B","version":"0.0.5","packages":%s},
	{"versionId":"cur","name":"B","version":"0.0.9","packages":%s}]},
	{"id":"winonly","currentVersionId":"w","versions":[{"versionId":"w","name":"W","version":"1.0.0",
	  "packages":[{"os":"windows","arch":"amd64","url":"w.zip","sha256":%q}]}]}]}`,
		pkg("old.zip"), pkg("mid.zip"), pkg("cur.zip"), pkgSum)
}

func envOf(installed string) status.Env {
	e := status.Env{OS: "linux", Arch: "amd64", Installed: map[string]string{}}
	if installed != "" {
		e.Installed["browser"] = installed
	}
	return e
}

func addRepo(t *testing.T, f *fakeFetcher, local bool) (*Service, string) {
	t.Helper()
	f.local = local
	s := newService(t, f)
	repo, err := s.Add(context.Background(), link)
	if err != nil {
		t.Fatal(err)
	}
	return s, repo.ID
}

func TestPlanInstallRules(t *testing.T) {
	f := &fakeFetcher{files: map[string]string{link: installFile(sum, 0)}}
	s, id := addRepo(t, f, false)

	p, err := s.PlanInstall(id, "browser", "", envOf(""))
	if err != nil || p.Version != "0.0.9" || p.VersionID != "cur" || p.URL != "https://example.com/cinqo/cur.zip" || p.SHA256 != sum {
		t.Fatalf("default is the current version: %v %+v", err, p)
	}
	if p, err := s.PlanInstall(id, "browser", "old", envOf("")); err != nil || p.Version != "0.0.1" {
		t.Errorf("a fresh install of an older version is allowed: %v %+v", err, p)
	}
	if p, err := s.PlanInstall(id, "browser", "", envOf("0.0.5")); err != nil || p.Version != "0.0.9" {
		t.Errorf("update to newer: %v %+v", err, p)
	}
	if p, err := s.PlanInstall(id, "browser", "cur", envOf("0.0.5")); err != nil || p.VersionID != "cur" {
		t.Errorf("%v", err)
	}

	for name, c := range map[string]struct{ version, installed string }{
		"same version":    {"cur", "0.0.9"},
		"older version":   {"old", "0.0.5"},
		"installed newer": {"", "1.0.0"},
	} {
		_, err := s.PlanInstall(id, "browser", c.version, envOf(c.installed))
		if !errors.Is(err, ErrNotInstallable) {
			t.Errorf("%s: %v", name, err)
		}
	}
	_, err = s.PlanInstall(id, "winonly", "", envOf(""))
	if !errors.Is(err, ErrNotInstallable) || !strings.Contains(err.Error(), "no package") {
		t.Errorf("missing platform package: %v", err)
	}
	_, err = s.PlanInstall(id, "nope", "", envOf(""))
	kind(t, err, ErrToolNotFound)
	_, err = s.PlanInstall(id, "browser", "zzz", envOf(""))
	kind(t, err, ErrVersionMissing)
	_, err = s.PlanInstall("nope", "browser", "", envOf(""))
	kind(t, err, ErrNotFound)
}

func TestPlanInstallRespectsMinAppVersion(t *testing.T) {
	file := strings.Replace(installFile(sum, 0), `"version":"0.0.9"`, `"version":"0.0.9","minAppVersion":"9.0.0"`, 1)
	s, id := addRepo(t, &fakeFetcher{files: map[string]string{link: file}}, false)
	e := envOf("")
	e.AppVersion = "0.0.100"
	_, err := s.PlanInstall(id, "browser", "", e)
	if !errors.Is(err, ErrNotInstallable) || !strings.Contains(err.Error(), "9.0.0") {
		t.Errorf("%v", err)
	}
}

type downloadFake struct {
	body   []byte
	err    error
	policy fetch.LocalPolicy
	url    string
}

func (d *downloadFake) Get(_ context.Context, url string, _ int64, p fetch.LocalPolicy) (*fetch.Result, error) {
	d.url, d.policy = url, p
	if d.err != nil {
		return nil, d.err
	}
	return &fetch.Result{Body: d.body}, nil
}

func TestDownloadVerifiesChecksumAndSize(t *testing.T) {
	pkg := []byte("zip bytes")
	f := &fakeFetcher{files: map[string]string{link: installFile(hexSum(pkg), len(pkg))}}
	s, id := addRepo(t, f, false)
	plan, err := s.PlanInstall(id, "browser", "", envOf(""))
	if err != nil {
		t.Fatal(err)
	}

	dl := &downloadFake{body: pkg}
	s.WithDownloader(dl)
	got, err := s.Download(context.Background(), plan)
	if err != nil || string(got) != "zip bytes" || dl.url != plan.URL {
		t.Fatalf("%v", err)
	}

	dl.body = []byte("tampered and longer")
	_, err = s.Download(context.Background(), plan)
	kind(t, err, ErrDownload)
	if !strings.Contains(err.Error(), "size") {
		t.Errorf("wanted a size complaint, got: %v", err)
	}
	dl.body = []byte("zip bytez") // same size, wrong checksum
	_, err = s.Download(context.Background(), plan)
	if !errors.Is(err, ErrDownload) || !strings.Contains(err.Error(), "checksum") {
		t.Errorf("%v", err)
	}
	dl.body, dl.err = nil, errors.New("the server could not be reached")
	_, err = s.Download(context.Background(), plan)
	if !errors.Is(err, ErrDownload) || !strings.Contains(err.Error(), "could not be reached") {
		t.Errorf("%v", err)
	}
}

func TestDownloadWithoutListedSizeStillChecksChecksum(t *testing.T) {
	pkg := []byte("zip bytes")
	f := &fakeFetcher{files: map[string]string{link: installFile(hexSum(pkg), 0)}}
	s, id := addRepo(t, f, false)
	plan, _ := s.PlanInstall(id, "browser", "", envOf(""))
	s.WithDownloader(&downloadFake{body: pkg})
	if _, err := s.Download(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	s.WithDownloader(&downloadFake{body: []byte("other")})
	if _, err := s.Download(context.Background(), plan); !errors.Is(err, ErrDownload) {
		t.Errorf("%v", err)
	}
}

func TestDownloadPolicyFollowsTheRepository(t *testing.T) {
	pkg := []byte("zip bytes")
	for _, local := range []bool{false, true} {
		f := &fakeFetcher{files: map[string]string{link: installFile(hexSum(pkg), 0)}}
		s, id := addRepo(t, f, local)
		plan, _ := s.PlanInstall(id, "browser", "", envOf(""))
		dl := &downloadFake{body: pkg}
		s.WithDownloader(dl)
		if _, err := s.Download(context.Background(), plan); err != nil {
			t.Fatal(err)
		}
		want := fetch.LocalForbidden
		if local {
			want = fetch.LocalAllowed
		}
		if dl.policy != want {
			t.Errorf("local=%v: policy %v", local, dl.policy)
		}
	}
}
