package localai

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type entry struct {
	name   string
	typ    byte
	body   string
	link   string
	mode   int64
	size   int64 // overrides len(body) for a declared size (zip-bomb tests)
	repeat int   // writes body this many times (a big file without a big test)
}

func buildArchive(t *testing.T, entries ...entry) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		n := int64(len(e.body))
		if e.repeat > 0 {
			n = int64(len(e.body)) * int64(e.repeat)
		}
		hdr := &tar.Header{Name: e.name, Typeflag: typ, Mode: mode, Linkname: e.link}
		if typ == tar.TypeReg {
			hdr.Size = n
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg {
			if e.repeat > 0 {
				for i := 0; i < e.repeat; i++ {
					tw.Write([]byte(e.body))
				}
			} else {
				tw.Write([]byte(e.body))
			}
		}
	}
	tw.Close()
	gz.Close()
	p := filepath.Join(t.TempDir(), "a.tar.gz")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func dest(t *testing.T) string { return filepath.Join(t.TempDir(), "runtime", "rt1") }

func noLeftovers(t *testing.T, d string) {
	t.Helper()
	parent := filepath.Dir(d)
	entries, _ := os.ReadDir(parent)
	for _, e := range entries {
		t.Errorf("leftover next to the destination: %s", e.Name())
	}
}

func TestAnArchiveShapedLikeTheRealOneIsUnpacked(t *testing.T) {
	a := buildArchive(t,
		entry{name: "llama-b1/", typ: tar.TypeDir, mode: 0o755},
		entry{name: "llama-b1/llama-server", body: "#!binary", mode: 0o755},
		entry{name: "llama-b1/libllama.0.dylib", body: "lib", mode: 0o755},
		entry{name: "llama-b1/libllama.dylib", typ: tar.TypeSymlink, link: "libllama.0.dylib"},
		entry{name: "llama-b1/LICENSE", body: "MIT"},
		entry{name: "llama-b1/sub/deeper/file.txt", body: "x"},
	)
	d := dest(t)
	if err := extractTarGz(a, d); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(d, "llama-b1", "llama-server"))
	if err != nil || st.Mode().Perm() != 0o755 || !st.Mode().IsRegular() {
		t.Errorf("server program: %v %v", st, err)
	}
	if st, _ := os.Stat(filepath.Join(d, "llama-b1", "LICENSE")); st.Mode().Perm() != 0o644 {
		t.Errorf("a plain file must be 0644, is %v", st.Mode().Perm())
	}
	link, err := os.Readlink(filepath.Join(d, "llama-b1", "libllama.dylib"))
	if err != nil || link != "libllama.0.dylib" {
		t.Errorf("symlink = %q, %v", link, err)
	}
	if b, err := os.ReadFile(filepath.Join(d, "llama-b1", "libllama.dylib")); err != nil || string(b) != "lib" {
		t.Errorf("the link must resolve to the library: %q %v", b, err)
	}
	if b, _ := os.ReadFile(filepath.Join(d, "llama-b1", "sub", "deeper", "file.txt")); string(b) != "x" {
		t.Error("files in folders that have no entry of their own must still be unpacked")
	}
	noLeftovers2(t, d)
}

func noLeftovers2(t *testing.T, d string) {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Dir(d))
	if len(entries) != 1 || entries[0].Name() != filepath.Base(d) {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("the parent folder must hold only the destination, has %v", names)
	}
}

func TestEverythingUnsafeRefusesTheWholeArchiveAndLeavesNothing(t *testing.T) {
	cases := map[string][]entry{
		"an absolute path":               {{name: "/etc/evil", body: "x"}},
		"a path going up":                {{name: "../evil", body: "x"}},
		"a path going up inside":         {{name: "a/../../evil", body: "x"}},
		"a path going up and down":       {{name: "a/../b", body: "x"}},
		"an absolute symlink":            {{name: "l", typ: tar.TypeSymlink, link: "/etc/passwd"}},
		"a symlink going up":             {{name: "l", typ: tar.TypeSymlink, link: "../outside"}},
		"a symlink going up from a dir":  {{name: "d/l", typ: tar.TypeSymlink, link: "../../outside"}},
		"a symlink to the parent":        {{name: "l", typ: tar.TypeSymlink, link: ".."}},
		"an empty symlink target":        {{name: "l", typ: tar.TypeSymlink, link: ""}},
		"a hard link":                    {{name: "a", body: "x"}, {name: "b", typ: tar.TypeLink, link: "a"}},
		"a character device":             {{name: "dev", typ: tar.TypeChar}},
		"a pipe":                         {{name: "p", typ: tar.TypeFifo}},
		"a file below a symlinked dir":   {{name: "d", typ: tar.TypeSymlink, link: "e"}, {name: "d/x", body: "x"}},
		"a dir below a symlink":          {{name: "d", typ: tar.TypeSymlink, link: "e"}, {name: "d/sub/", typ: tar.TypeDir}},
		"a link below a symlink":         {{name: "d", typ: tar.TypeSymlink, link: "e"}, {name: "d/l", typ: tar.TypeSymlink, link: "x"}},
		"a good file and then a bad one": {{name: "ok", body: "fine"}, {name: "../bad", body: "x"}},
	}
	for name, entries := range cases {
		a := buildArchive(t, entries...)
		d := dest(t)
		err := extractTarGz(a, d)
		if !errors.Is(err, errUnsafeArchive) {
			t.Errorf("%s: err = %v, want errUnsafeArchive", name, err)
		}
		if _, statErr := os.Stat(d); statErr == nil {
			t.Errorf("%s: a refused archive must leave no destination", name)
		}
		noLeftovers(t, d)
	}
}

func TestALinkThatStaysInsideIsAccepted(t *testing.T) {
	a := buildArchive(t,
		entry{name: "d/", typ: tar.TypeDir},
		entry{name: "d/real", body: "x"},
		entry{name: "d/up", typ: tar.TypeSymlink, link: "../d/real"},
		entry{name: "d/same", typ: tar.TypeSymlink, link: "./real"},
	)
	if err := extractTarGz(a, dest(t)); err != nil {
		t.Errorf("links that stay inside the tree must be accepted: %v", err)
	}
}

func TestPermissionsAreReducedToPlainOnes(t *testing.T) {
	a := buildArchive(t,
		entry{name: "setuid", body: "x", mode: 0o4777},
		entry{name: "writable", body: "x", mode: 0o666},
		entry{name: "exec", body: "x", mode: 0o700},
		entry{name: "none", body: "x", mode: 0o000 + 0o001},
	)
	d := dest(t)
	if err := extractTarGz(a, d); err != nil {
		t.Fatal(err)
	}
	want := map[string]os.FileMode{"setuid": 0o755, "writable": 0o644, "exec": 0o755, "none": 0o755}
	for name, mode := range want {
		st, err := os.Stat(filepath.Join(d, name))
		if err != nil || st.Mode() != mode {
			t.Errorf("%s: mode %v (%v), want %v with no special bits", name, st.Mode(), err, mode)
		}
	}
}

func TestATooLargeOrTooManyEntriesIsRefused(t *testing.T) {
	oldBytes, oldFiles := maxArchiveBytes, maxArchiveFiles
	defer func() { maxArchiveBytes, maxArchiveFiles = oldBytes, oldFiles }()
	maxArchiveBytes = 3 << 20
	big := buildArchive(t, entry{name: "a", body: strings.Repeat("a", 1<<20), repeat: 2}, entry{name: "b", body: strings.Repeat("b", 1<<20), repeat: 2})
	d := dest(t)
	if err := extractTarGz(big, d); !errors.Is(err, errUnsafeArchive) {
		t.Errorf("an archive that unpacks to more than the limit: err = %v", err)
	}
	exact := buildArchive(t, entry{name: "a", body: strings.Repeat("a", 1<<20), repeat: 3})
	if err := extractTarGz(exact, dest(t)); err != nil {
		t.Errorf("exactly the limit must be accepted: %v", err)
	}
	maxArchiveBytes = oldBytes
	if _, err := os.Stat(d); err == nil {
		t.Error("nothing may be left behind")
	}
	noLeftovers(t, d)

	var many []entry
	for i := 0; i <= maxArchiveFiles; i++ {
		many = append(many, entry{name: "f" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + "/" + itoa(i), body: "x"})
	}
	if err := extractTarGz(buildArchive(t, many...), dest(t)); !errors.Is(err, errUnsafeArchive) {
		t.Errorf("too many entries: err = %v", err)
	}
	// Exactly at the limit is fine.
	if err := extractTarGz(buildArchive(t, many[:maxArchiveFiles]...), dest(t)); err != nil {
		t.Errorf("exactly %d entries must be accepted: %v", maxArchiveFiles, err)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func TestAnythingThatIsNotAGzipTarIsRefusedWithoutDetails(t *testing.T) {
	dir := t.TempDir()
	notGz := filepath.Join(dir, "x.tar.gz")
	os.WriteFile(notGz, []byte("this is not an archive"), 0o644)
	var plainTar bytes.Buffer
	tw := tar.NewWriter(&plainTar)
	tw.WriteHeader(&tar.Header{Name: "a", Size: 1, Mode: 0o644})
	tw.Write([]byte("x"))
	tw.Close()
	plain := filepath.Join(dir, "plain.tar.gz")
	os.WriteFile(plain, plainTar.Bytes(), 0o644)
	var cut bytes.Buffer
	gz := gzip.NewWriter(&cut)
	gz.Write([]byte("garbage, not tar"))
	gz.Close()
	garbage := filepath.Join(dir, "garbage.tar.gz")
	os.WriteFile(garbage, cut.Bytes(), 0o644)
	good := buildArchive(t, entry{name: "d/", typ: tar.TypeDir}, entry{name: "d/f", body: strings.Repeat("y", 100000)})
	raw, _ := os.ReadFile(good)
	truncated := filepath.Join(dir, "trunc.tar.gz")
	os.WriteFile(truncated, raw[:len(raw)/2], 0o644)

	for name, p := range map[string]string{"not gzip": notGz, "a plain tar": plain, "gzip of garbage": garbage, "a truncated archive": truncated, "a missing file": filepath.Join(dir, "missing")} {
		d := dest(t)
		err := extractTarGz(p, d)
		if err == nil {
			t.Errorf("%s: want an error", name)
			continue
		}
		if strings.Contains(err.Error(), dir) || strings.Contains(err.Error(), "EOF") {
			t.Errorf("%s: the message must not show paths or internals: %v", name, err)
		}
		if _, statErr := os.Stat(d); statErr == nil {
			t.Errorf("%s: no destination may remain", name)
		}
		noLeftovers(t, d)
	}
}

func TestAnOlderCopyIsReplacedOnlyByACompleteNewOne(t *testing.T) {
	d := dest(t)
	if err := extractTarGz(buildArchive(t, entry{name: "v1", body: "old"}), d); err != nil {
		t.Fatal(err)
	}
	// A bad archive leaves the old copy as it was.
	if err := extractTarGz(buildArchive(t, entry{name: "v2", body: "new"}, entry{name: "../x", body: "x"}), d); err == nil {
		t.Fatal("want an error")
	}
	if b, err := os.ReadFile(filepath.Join(d, "v1")); err != nil || string(b) != "old" {
		t.Errorf("the old copy must survive a failed update: %q %v", b, err)
	}
	// A good one replaces it completely.
	if err := extractTarGz(buildArchive(t, entry{name: "v2", body: "new"}), d); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(d, "v1")); err == nil {
		t.Error("the old files must be gone after a successful update")
	}
	if b, _ := os.ReadFile(filepath.Join(d, "v2")); string(b) != "new" {
		t.Error("the new file must be there")
	}
	noLeftovers2(t, d)
}

func TestTheRootEntryAndDotPrefixedNamesWork(t *testing.T) {
	a := buildArchive(t, entry{name: "./", typ: tar.TypeDir}, entry{name: "./bin/", typ: tar.TypeDir}, entry{name: "./bin/tool", body: "x", mode: 0o755})
	d := dest(t)
	if err := extractTarGz(a, d); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(filepath.Join(d, "bin", "tool")); err != nil || st.Mode().Perm() != 0o755 {
		t.Errorf("%v %v", st, err)
	}
}

func TestTheSameFileTwiceInOneArchiveIsRefused(t *testing.T) {
	a := buildArchive(t, entry{name: "llama-b1/llama-server", body: "first", mode: 0o755}, entry{name: "llama-b1/llama-server", body: "second", mode: 0o755})
	d := dest(t)
	if err := extractTarGz(a, d); err == nil {
		t.Error("an archive that puts two files at one path must be refused: the later one would silently replace the first")
	}
	if _, err := os.Stat(d); err == nil {
		t.Error("nothing may be left behind")
	}
	noLeftovers(t, d)
}
