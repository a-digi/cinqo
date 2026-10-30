package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/a-digi/cinqo/src/toolrepository/status"
)

const nativeLink = "https://raw.githubusercontent.com/a-digi/cinqo/0.0.1/tools/repository.json"

func nativeOf(t *testing.T, s *Service) (id, url string, ok bool) {
	t.Helper()
	n, err := s.query.FindNative()
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return n.ID, n.URL, true
}

func rowCount(t *testing.T, s *Service) int {
	t.Helper()
	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	return len(list)
}

func TestSyncInsertsTheNativeRepositoryWithoutTouchingTheNetwork(t *testing.T) {
	f := &fakeFetcher{}
	s := newService(t, f)
	if err := s.SyncNative(nativeLink); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 {
		t.Errorf("sync must not fetch: %v", f.calls)
	}
	list, _ := s.List()
	if len(list) != 1 {
		t.Fatalf("%d rows", len(list))
	}
	r := list[0]
	if !r.IsNative || r.URL != nativeLink || r.LastStatus != "never" || r.CatalogJSON != "" || r.LastFetchedAt != "" || r.Name != "raw.githubusercontent.com/repository.json" {
		t.Errorf("%+v", r)
	}
}

func TestSyncTwiceIsANoOp(t *testing.T) {
	s := newService(t, &fakeFetcher{})
	for i := 0; i < 3; i++ {
		if err := s.SyncNative(nativeLink); err != nil {
			t.Fatal(err)
		}
	}
	id1, _, _ := nativeOf(t, s)
	if rowCount(t, s) != 1 || id1 == "" {
		t.Errorf("rows: %d", rowCount(t, s))
	}
}

func TestSyncDoesNotResetAFetchedNativeRepositoryWithTheSameLink(t *testing.T) {
	f := &fakeFetcher{files: map[string]string{nativeLink: repoFile("Cinqo", "a.zip")}}
	s := newService(t, f)
	s.SyncNative(nativeLink)
	id, _, _ := nativeOf(t, s)
	if _, err := s.Refresh(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	s.SyncNative(nativeLink) // a restart
	n, _ := s.query.FindNative()
	if n.LastStatus != "ok" || n.CatalogJSON == "" || n.Name != "Cinqo" {
		t.Errorf("a restart must keep what was fetched: %+v", n)
	}
}

func TestSyncAdoptsAHandAddedRepositoryWithTheSameLink(t *testing.T) {
	f := &fakeFetcher{files: map[string]string{nativeLink: repoFile("Mine", "a.zip")}}
	s := newService(t, f)
	added, err := s.Add(context.Background(), nativeLink)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SyncNative(nativeLink); err != nil {
		t.Fatal(err)
	}
	id, _, ok := nativeOf(t, s)
	if !ok || id != added.ID || rowCount(t, s) != 1 {
		t.Errorf("must adopt, not duplicate: id %s want %s, rows %d", id, added.ID, rowCount(t, s))
	}
	n, _ := s.query.FindNative()
	if n.CatalogJSON == "" || n.LastStatus != "ok" {
		t.Error("an adopted repository keeps what it already fetched")
	}
}

func TestSyncChangedLinkResetsTheNativeRepository(t *testing.T) {
	f := &fakeFetcher{files: map[string]string{nativeLink: repoFile("Cinqo", "a.zip")}}
	s := newService(t, f)
	s.SyncNative(nativeLink)
	id, _, _ := nativeOf(t, s)
	s.Refresh(context.Background(), id)

	other := "https://example.com/other/tools.json"
	if err := s.SyncNative(other); err != nil {
		t.Fatal(err)
	}
	n, err := s.query.FindNative()
	if err != nil {
		t.Fatal(err)
	}
	if n.ID != id || n.URL != other || n.LastStatus != "never" || n.CatalogJSON != "" || n.Description != "" || n.LastFetchedAt != "" || n.IsLocal {
		t.Errorf("a new link is a different repository: %+v", n)
	}
	if rowCount(t, s) != 1 {
		t.Errorf("rows: %d", rowCount(t, s))
	}
}

func TestSyncChangedLinkAdoptsAHandAddedRepositoryThatAlreadyHasIt(t *testing.T) {
	other := "https://example.com/other/tools.json"
	f := &fakeFetcher{files: map[string]string{other: repoFile("Other", "a.zip")}}
	s := newService(t, f)
	s.SyncNative(nativeLink)
	oldID, _, _ := nativeOf(t, s)
	added, _ := s.Add(context.Background(), other)

	if err := s.SyncNative(other); err != nil {
		t.Fatal(err)
	}
	id, url, _ := nativeOf(t, s)
	if id != added.ID || url != other {
		t.Errorf("native is now %s %s", id, url)
	}
	old, err := s.query.FindByID(oldID)
	if err != nil || old.IsNative {
		t.Errorf("the previous native row must remain, as an ordinary repository: %v %+v", err, old)
	}
	if rowCount(t, s) != 2 {
		t.Errorf("rows: %d", rowCount(t, s))
	}
}

func TestSyncEmptyLinkDemotesTheNativeRepositoryAndDeletesNothing(t *testing.T) {
	s := newService(t, &fakeFetcher{})
	s.SyncNative(nativeLink)
	id, _, _ := nativeOf(t, s)

	if err := s.SyncNative(""); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := nativeOf(t, s); ok {
		t.Error("still native")
	}
	r, err := s.query.FindByID(id)
	if err != nil || r.IsNative {
		t.Errorf("it must remain, as an ordinary repository: %v %+v", err, r)
	}
	if err := s.Remove(id); err != nil {
		t.Errorf("now removable: %v", err)
	}
	if err := s.SyncNative(""); err != nil { // nothing to demote
		t.Fatal(err)
	}
}

func TestOnlyOneNativeRepositoryCanExist(t *testing.T) {
	s := newService(t, &fakeFetcher{})
	s.SyncNative(nativeLink)
	_, err := s.db.Exec(`INSERT INTO tool_repositories (id, url, is_native) VALUES ('second', 'https://example.com/x.json', 1)`)
	if err == nil {
		t.Fatal("the database must refuse a second native repository")
	}
}

func TestNativeRepositoryCannotBeRemoved(t *testing.T) {
	s := newService(t, &fakeFetcher{})
	s.SyncNative(nativeLink)
	id, _, _ := nativeOf(t, s)
	err := s.Remove(id)
	kind(t, err, ErrNativeRemoval)
	if !errors.As(err, new(*Error)) || err.Error() != "the built-in repository cannot be removed" {
		t.Errorf("%v", err)
	}
	if rowCount(t, s) != 1 {
		t.Error("removed anyway")
	}
}

func TestNativeRepositoryDoesNotCountTowardsTheLimit(t *testing.T) {
	f := &fakeFetcher{files: map[string]string{}}
	s := newService(t, f)
	s.SyncNative(nativeLink)
	for i := 0; i < MaxRepositories; i++ {
		l := fmt.Sprintf("https://example.com/r%d.json", i)
		f.files[l] = repoFile("r", "a.zip")
		if _, err := s.Add(context.Background(), l); err != nil {
			t.Fatalf("repository %d: %v", i+1, err)
		}
	}
	f.files["https://example.com/extra.json"] = repoFile("r", "a.zip")
	_, err := s.Add(context.Background(), "https://example.com/extra.json")
	kind(t, err, ErrTooMany)
}

func TestNativeRepositoryIsListedFirstAndMarkedInTheJSON(t *testing.T) {
	f := &fakeFetcher{files: map[string]string{"https://example.com/a.json": repoFile("A", "a.zip")}}
	s := newService(t, f)
	s.Add(context.Background(), "https://example.com/a.json")
	s.SyncNative(nativeLink)
	sums, err := s.Summaries(status.Env{OS: "linux", Arch: "amd64"})
	if err != nil || len(sums) != 2 {
		t.Fatalf("%v %d", err, len(sums))
	}
	if !sums[0].IsNative || sums[1].IsNative {
		t.Errorf("native first: %v %v", sums[0].IsNative, sums[1].IsNative)
	}
}

func TestNativeRepositoryRefreshesLikeAnyOther(t *testing.T) {
	f := &fakeFetcher{files: map[string]string{nativeLink: repoFile("Cinqo tools", "a.zip")}}
	s := newService(t, f)
	s.SyncNative(nativeLink)
	id, _, _ := nativeOf(t, s)
	r, err := s.Refresh(context.Background(), id)
	if err != nil || r.LastStatus != "ok" || r.Name != "Cinqo tools" || !r.IsNative {
		t.Fatalf("%v %+v", err, r)
	}
}
