package jsonview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestCache(t *testing.T, lim CacheLimits) (*Cache, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "cache")
	c, err := NewCache(dir, lim)
	if err != nil {
		t.Fatal(err)
	}
	return c, dir
}

func writeSource(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func isUnder(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}

func filesIn(t *testing.T, dir string) []string {
	t.Helper()
	items, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, it := range items {
		names = append(names, it.Name())
	}
	return names
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const sample = `{"a":[1,2,{"b":"c"}],"d":null}`

func TestEnsureMakesTheIndentedCopyOnceAndKeepsIt(t *testing.T) {
	c, dir := newTestCache(t, CacheLimits{})
	src := writeSource(t, t.TempDir(), "x.json", sample)
	d, err := c.Ensure(context.Background(), "tool:browser:crawl-logs|x.json", src)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := readFile(t, d.Path), oracle(t, []byte(sample)); got != want {
		t.Fatalf("copy %q, want %q", got, want)
	}
	if !isUnder(dir, d.Path) || !regexp.MustCompile(`^[0-9a-f]{64}\.txt$`).MatchString(filepath.Base(d.Path)) {
		t.Errorf("copy is not a hashed file inside the cache folder: %s", d.Path)
	}
	if d.SourceBytes != int64(len(sample)) || d.Bytes != int64(len(readFile(t, d.Path))) {
		t.Errorf("sizes %+v", d)
	}
	d2, err := c.Ensure(context.Background(), "tool:browser:crawl-logs|x.json", src)
	if err != nil || d2.Path != d.Path || c.conversions.Load() != 1 {
		t.Fatalf("second ask: %+v %v after %d conversions", d2, err, c.conversions.Load())
	}
	if n, _ := c.Stats(); n != 1 {
		t.Errorf("%d copies kept", n)
	}
	st, _ := os.Stat(d.Path)
	if st.Mode().Perm() != 0o600 {
		t.Errorf("copy mode %v", st.Mode().Perm())
	}
	ds, _ := os.Stat(dir)
	if ds.Mode().Perm() != 0o700 {
		t.Errorf("folder mode %v", ds.Mode().Perm())
	}
}

func TestAChangedFileGetsANewCopy(t *testing.T) {
	c, _ := newTestCache(t, CacheLimits{})
	src := writeSource(t, t.TempDir(), "x.json", sample)
	first, _ := c.Ensure(context.Background(), "id", src)
	if err := os.WriteFile(src, []byte(`{"changed":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := c.Ensure(context.Background(), "id", src)
	if err != nil || second.Path == first.Path || !strings.Contains(readFile(t, second.Path), `"changed": true`) {
		t.Fatalf("%+v %v", second, err)
	}
	// same size, different time: still a new copy
	same := writeSource(t, t.TempDir(), "y.json", `{"n":1}`)
	a, _ := c.Ensure(context.Background(), "id2", same)
	later := time.Now().Add(time.Hour)
	if err := os.WriteFile(same, []byte(`{"n":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(same, later, later)
	b, err := c.Ensure(context.Background(), "id2", same)
	if err != nil || b.Path == a.Path || !strings.Contains(readFile(t, b.Path), `"n": 2`) {
		t.Fatalf("same size, new time: %+v %v", b, err)
	}
}

func TestTheSameFileUnderTwoIdentitiesHasTwoCopies(t *testing.T) {
	c, _ := newTestCache(t, CacheLimits{})
	src := writeSource(t, t.TempDir(), "x.json", sample)
	a, _ := c.Ensure(context.Background(), "tool:a:logs|x.json", src)
	b, _ := c.Ensure(context.Background(), "tool:b:logs|x.json", src)
	if a.Path == b.Path || c.conversions.Load() != 2 {
		t.Fatalf("shared a copy across identities: %s %s", a.Path, b.Path)
	}
}

func TestManyAsksForOneFileMakeOneCopy(t *testing.T) {
	c, _ := newTestCache(t, CacheLimits{})
	src := writeSource(t, t.TempDir(), "x.json", `[`+strings.Repeat(`{"k":"vvvvvvvvvvvv"},`, 20000)+`1]`)
	release := make(chan struct{})
	c.hookBefore = func(context.Context) { <-release }
	var wg sync.WaitGroup
	paths := make([]string, 8)
	errs := make([]error, 8)
	for i := range paths {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d, err := c.Ensure(context.Background(), "id", src)
			paths[i], errs[i] = d.Path, err
		}(i)
	}
	time.Sleep(100 * time.Millisecond) // everyone is waiting on the one conversion
	close(release)
	wg.Wait()
	for i := range paths {
		if errs[i] != nil || paths[i] != paths[0] {
			t.Fatalf("asker %d: %q %v", i, paths[i], errs[i])
		}
	}
	if c.conversions.Load() != 1 {
		t.Fatalf("%d conversions for one file", c.conversions.Load())
	}
}

func TestAnInvalidFileKeepsNothing(t *testing.T) {
	c, dir := newTestCache(t, CacheLimits{})
	src := writeSource(t, t.TempDir(), "bad.json", `{"a":1,}`)
	_, err := c.Ensure(context.Background(), "id", src)
	var se *SyntaxError
	if !errors.As(err, &se) || se.Offset != 7 {
		t.Fatalf("got %v", err)
	}
	if names := filesIn(t, dir); len(names) != 0 {
		t.Fatalf("left behind: %v", names)
	}
	if n, b := c.Stats(); n != 0 || b != 0 {
		t.Fatalf("stats %d %d", n, b)
	}
	// not cached as a failure either: asking again tries again (the file may have been fixed)
	if _, err := c.Ensure(context.Background(), "id", src); err == nil || c.conversions.Load() != 2 {
		t.Fatalf("second ask: %v after %d", err, c.conversions.Load())
	}
}

func TestAFileOverTheSizeLimitIsRefusedWithoutReadingIt(t *testing.T) {
	c, dir := newTestCache(t, CacheLimits{Format: Limits{MaxBytes: 100}})
	src := writeSource(t, t.TempDir(), "big.json", `["`+strings.Repeat("x", 200)+`"]`)
	if _, err := c.Ensure(context.Background(), "id", src); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("got %v", err)
	}
	if c.conversions.Load() != 0 || len(filesIn(t, dir)) != 0 {
		t.Fatalf("a conversion started or left files: %d %v", c.conversions.Load(), filesIn(t, dir))
	}
}

func TestAnIndentingBombLeavesNothingBehind(t *testing.T) {
	c, dir := newTestCache(t, CacheLimits{Format: Limits{MaxOutputBytes: 1 << 20}})
	doc := strings.Repeat("[", 50) + strings.Repeat("1,", 200000) + "1" + strings.Repeat("]", 50)
	src := writeSource(t, t.TempDir(), "bomb.json", doc)
	if _, err := c.Ensure(context.Background(), "id", src); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("got %v", err)
	}
	if names := filesIn(t, dir); len(names) != 0 {
		t.Fatalf("left behind: %v", names)
	}
}

func TestNotARegularFileOrMissing(t *testing.T) {
	c, _ := newTestCache(t, CacheLimits{})
	if _, err := c.Ensure(context.Background(), "id", t.TempDir()); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("a directory: %v", err)
	}
	if _, err := c.Ensure(context.Background(), "id", filepath.Join(t.TempDir(), "none.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a missing file: %v", err)
	}
}

func TestTheLeastRecentlyUsedCopiesGoFirst(t *testing.T) {
	c, dir := newTestCache(t, CacheLimits{MaxFiles: 3})
	srcDir := t.TempDir()
	var derived []Derived
	for i := 0; i < 3; i++ {
		d, err := c.Ensure(context.Background(), "id"+string(rune('a'+i)), writeSource(t, srcDir, string(rune('a'+i))+".json", sample))
		if err != nil {
			t.Fatal(err)
		}
		derived = append(derived, d)
		time.Sleep(5 * time.Millisecond)
	}
	// use the oldest again so it is the newest, then add two more
	if _, err := c.Ensure(context.Background(), "ida", filepath.Join(srcDir, "a.json")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	for i := 3; i < 5; i++ {
		if _, err := c.Ensure(context.Background(), "id"+string(rune('a'+i)), writeSource(t, srcDir, string(rune('a'+i))+".json", sample)); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if n, _ := c.Stats(); n != 3 || len(filesIn(t, dir)) != 3 {
		t.Fatalf("kept %d, on disk %v", n, filesIn(t, dir))
	}
	if _, err := os.Stat(derived[0].Path); err != nil {
		t.Errorf("the copy used most recently was evicted: %v", err)
	}
	for _, d := range derived[1:] {
		if _, err := os.Stat(d.Path); err == nil {
			t.Errorf("an old copy survived: %s", d.Path)
		}
	}
}

func TestTheByteBoundEvictsToo(t *testing.T) {
	one := int64(len(oracle(t, []byte(sample))))
	c, _ := newTestCache(t, CacheLimits{MaxBytes: one*2 + one/2})
	srcDir := t.TempDir()
	for i := 0; i < 5; i++ {
		if _, err := c.Ensure(context.Background(), "id"+string(rune('a'+i)), writeSource(t, srcDir, string(rune('a'+i))+".json", sample)); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if n, b := c.Stats(); n != 2 || b > one*2+one/2 {
		t.Fatalf("kept %d copies, %d bytes (bound %d)", n, b, one*2+one/2)
	}
}

func TestStartUpEmptiesOldCopiesAndOnlyThose(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	if err := os.MkdirAll(filepath.Join(dir, "keepdir"), 0o700); err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("ab", 32)
	for name, content := range map[string]string{hash + ".txt": "stale", hash + ".123.tmp": "half", "notes.txt": "mine", "keepdir/inner.txt": "inner", "short.txt": "x"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := NewCache(dir, CacheLimits{}); err != nil {
		t.Fatal(err)
	}
	left := map[string]bool{}
	for _, n := range filesIn(t, dir) {
		left[n] = true
	}
	if left[hash+".txt"] || left[hash+".123.tmp"] {
		t.Errorf("old copies were not removed: %v", left)
	}
	if !left["notes.txt"] || !left["keepdir"] || !left["short.txt"] {
		t.Errorf("something that is not a copy was removed: %v", left)
	}
	if _, err := os.Stat(filepath.Join(dir, "keepdir", "inner.txt")); err != nil {
		t.Errorf("a subfolder was touched: %v", err)
	}
}

func TestAConversionThatCannotGetATurnAnswersBusy(t *testing.T) {
	c, _ := newTestCache(t, CacheLimits{MaxConcurrent: 1, MaxWait: 60 * time.Millisecond})
	srcDir := t.TempDir()
	one := writeSource(t, srcDir, "1.json", sample)
	two := writeSource(t, srcDir, "2.json", `{"two":2}`)
	hold, started := make(chan struct{}), make(chan struct{}, 1)
	c.hookBefore = func(context.Context) {
		select {
		case started <- struct{}{}:
			<-hold
		default:
		}
	}
	done := make(chan error, 1)
	go func() { _, err := c.Ensure(context.Background(), "a", one); done <- err }()
	<-started
	begin := time.Now()
	if _, err := c.Ensure(context.Background(), "b", two); !errors.Is(err, ErrBusy) {
		t.Fatalf("got %v", err)
	}
	if time.Since(begin) > time.Second {
		t.Fatalf("waited %s for a turn", time.Since(begin))
	}
	close(hold)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := c.Ensure(context.Background(), "b", two); err != nil {
		t.Fatalf("after the turn was free: %v", err)
	}
}

func TestAConversionOverTheTimeLimitIsSlow(t *testing.T) {
	c, dir := newTestCache(t, CacheLimits{ConvertTimeout: 30 * time.Millisecond})
	c.hookBefore = func(ctx context.Context) { <-ctx.Done() } // a conversion that outlasts its budget
	src := writeSource(t, t.TempDir(), "x.json", sample)
	if _, err := c.Ensure(context.Background(), "id", src); !errors.Is(err, ErrSlow) {
		t.Fatalf("got %v", err)
	}
	if len(filesIn(t, dir)) != 0 {
		t.Fatalf("left behind: %v", filesIn(t, dir))
	}
}

func TestACancelledAskStopsAndLeavesNothing(t *testing.T) {
	c, dir := newTestCache(t, CacheLimits{})
	src := writeSource(t, t.TempDir(), "x.json", sample)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Ensure(ctx, "id", src); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if len(filesIn(t, dir)) != 0 {
		t.Fatalf("left behind: %v", filesIn(t, dir))
	}
}

func TestWhenTheConverterIsCancelledAnotherAskerTakesOver(t *testing.T) {
	c, _ := newTestCache(t, CacheLimits{})
	src := writeSource(t, t.TempDir(), "x.json", sample)
	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	inHook := make(chan struct{})
	first := true
	var mu sync.Mutex
	c.hookBefore = func(ctx context.Context) {
		mu.Lock()
		isFirst := first
		first = false
		mu.Unlock()
		if isFirst {
			close(inHook)
			<-ctx.Done() // the leader is stuck until its own context ends
		}
	}
	leaderErr := make(chan error, 1)
	go func() { _, err := c.Ensure(leaderCtx, "id", src); leaderErr <- err }()
	<-inHook
	followerRes := make(chan error, 1)
	var got Derived
	go func() {
		d, err := c.Ensure(context.Background(), "id", src)
		got = d
		followerRes <- err
	}()
	time.Sleep(50 * time.Millisecond) // the follower is waiting on the leader's flight
	cancelLeader()
	if err := <-leaderErr; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader: %v", err)
	}
	if err := <-followerRes; err != nil || got.Path == "" {
		t.Fatalf("follower: %v %+v", err, got)
	}
	if !strings.Contains(readFile(t, got.Path), "\"a\": [") {
		t.Fatal("the follower's copy is wrong")
	}
}

func TestAFileThatChangesWhileFormattedIsNotKept(t *testing.T) {
	c, dir := newTestCache(t, CacheLimits{})
	src := writeSource(t, t.TempDir(), "x.json", sample)
	c.hookAfter = func() {
		later := time.Now().Add(time.Hour)
		_ = os.Chtimes(src, later, later)
	}
	if _, err := c.Ensure(context.Background(), "id", src); !errors.Is(err, ErrChanged) {
		t.Fatalf("got %v", err)
	}
	if len(filesIn(t, dir)) != 0 {
		t.Fatalf("left behind: %v", filesIn(t, dir))
	}
	if n, _ := c.Stats(); n != 0 {
		t.Fatalf("kept %d", n)
	}
}

func TestACopyDeletedFromTheDiskIsMadeAgain(t *testing.T) {
	c, _ := newTestCache(t, CacheLimits{})
	src := writeSource(t, t.TempDir(), "x.json", sample)
	first, _ := c.Ensure(context.Background(), "id", src)
	if err := os.Remove(first.Path); err != nil {
		t.Fatal(err)
	}
	again, err := c.Ensure(context.Background(), "id", src)
	if err != nil || readFile(t, again.Path) != oracle(t, []byte(sample)) || c.conversions.Load() != 2 {
		t.Fatalf("%+v %v after %d conversions", again, err, c.conversions.Load())
	}
	if n, b := c.Stats(); n != 1 || b != again.Bytes {
		t.Fatalf("stats %d %d", n, b)
	}
}

func TestACopyIsExactlyWhatFormatMakes(t *testing.T) {
	c, _ := newTestCache(t, CacheLimits{})
	raw, _ := json.Marshal(map[string]any{"s": "é>", "n": []any{1, 2.5, nil, true}, "o": map[string]any{}})
	src := writeSource(t, t.TempDir(), "x.json", string(raw))
	d, err := c.Ensure(context.Background(), "id", src)
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	if _, err := Format(context.Background(), &want, bytes.NewReader(raw), Limits{}); err != nil {
		t.Fatal(err)
	}
	if readFile(t, d.Path) != want.String() {
		t.Fatal("the copy differs from Format's output")
	}
}

func TestNewCacheNeedsAFolder(t *testing.T) {
	if _, err := NewCache("", CacheLimits{}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestAFileRewrittenToAnotherSizeUnderTheSameTimeGetsANewCopy(t *testing.T) {
	c, _ := newTestCache(t, CacheLimits{})
	src := writeSource(t, t.TempDir(), "x.json", `{"n":1}`)
	st, _ := os.Stat(src)
	first, err := c.Ensure(context.Background(), "id", src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte(`{"n":1,"more":[1,2,3]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(src, st.ModTime(), st.ModTime()); err != nil { // a tool that keeps the file's time
		t.Fatal(err)
	}
	second, err := c.Ensure(context.Background(), "id", src)
	if err != nil || second.Path == first.Path || !strings.Contains(readFile(t, second.Path), `"more"`) {
		t.Fatalf("the size alone must tell a new version: %+v %v", second, err)
	}
}

func TestACopyBiggerThanTheByteBoundIsStillReturnedAndGoesWhenAnotherComes(t *testing.T) {
	c, _ := newTestCache(t, CacheLimits{MaxBytes: 1})
	srcDir := t.TempDir()
	a, err := c.Ensure(context.Background(), "a", writeSource(t, srcDir, "a.json", sample))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.Path); err != nil {
		t.Fatalf("the copy just made must not be evicted by its own arrival: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	b, err := c.Ensure(context.Background(), "b", writeSource(t, srcDir, "b.json", `{"b":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(b.Path); err != nil {
		t.Fatalf("the newest copy is gone: %v", err)
	}
	if _, err := os.Stat(a.Path); err == nil {
		t.Fatal("the older copy should have been evicted")
	}
}
