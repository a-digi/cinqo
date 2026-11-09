package media

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

var pdfHead = []byte("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n1 0 obj")

func TestResolveContentType(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	zip := []byte("PK\x03\x04\x14\x00\x00\x00")
	cases := []struct {
		name, declared, ext string
		head                []byte
		want                string
	}{
		{"octet-stream pdf bytes", "application/octet-stream", ".pdf", pdfHead, "application/pdf"},
		{"empty declared pdf bytes", "", ".pdf", pdfHead, "application/pdf"},
		{"declared with parameters is still generic", "application/octet-stream; foo=bar", ".pdf", pdfHead, "application/pdf"},
		{"binary/octet-stream", "binary/octet-stream", ".bin", png, "image/png"},
		{"bytes beat a lying extension", "application/octet-stream", ".txt", pdfHead, "application/pdf"},
		{"a specific declaration is kept", "application/pdf", ".pdf", []byte("not a pdf"), "application/pdf"},
		{"a specific declaration is kept even if bytes differ", "image/jpeg", ".jpg", png, "image/jpeg"},
		{"text bytes take the extension", "application/octet-stream", ".json", []byte(`{"a":1}`), "application/json"},
		{"text bytes without a known extension stay text", "", ".zzz", []byte("hello"), "text/plain; charset=utf-8"},
		{"zip bytes take the extension", "", ".docx", zip, "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{"zip bytes without extension", "", ".zzz", zip, "application/zip"},
		{"unknown bytes with a pdf extension", "", ".PDF", []byte{0x00, 0x01, 0x02, 0x03}, "application/pdf"},
		{"nothing known", "", ".zzz", []byte{0x00, 0x01, 0x02}, "application/octet-stream"},
		{"empty file", "", "", nil, "text/plain; charset=utf-8"},
	}
	for _, c := range cases {
		if got := ResolveContentType(c.declared, c.ext, c.head); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPersistLocalFileStoresAPdfAsAPdf(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	files, _ := filepath.Glob(filepath.Join("..", "..", "config", "db", "migrations", "*.sql"))
	sort.Strings(files)
	for _, f := range files {
		raw, _ := os.ReadFile(f)
		for _, stmt := range strings.Split(string(raw), "/***Statement***/") {
			if strings.TrimSpace(stmt) != "" {
				if _, err := db.Exec(stmt); err != nil {
					t.Fatalf("%s: %v", filepath.Base(f), err)
				}
			}
		}
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "cv.pdf")
	if err := os.WriteFile(src, append(pdfHead, []byte(" more")...), 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := PersistLocalFile(db, dir, "career", "u1", "", src, "cv.pdf", "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	var ct string
	db.QueryRow(`SELECT content_type FROM media_files WHERE id = ?`, id).Scan(&ct)
	if ct != "application/pdf" {
		t.Fatalf("stored %q", ct)
	}
	// the migration repairs rows stored earlier, by extension, and leaves the rest alone
	for _, r := range [][3]string{{"a", "application/octet-stream", ".PDF"}, {"b", "application/octet-stream", ".zip"}, {"c", "text/plain", ".pdf"}} {
		if _, err := db.Exec(`INSERT INTO media_files (id, tool_slug, original_filename, extension, stored_path, content_type, size_bytes, uploaded_by_user_id, created_at, title) VALUES (?, 't', 'f', ?, '/x', ?, 1, 'u', 'now', 't')`, r[0], r[2], r[1]); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := os.ReadFile(filepath.Join("..", "..", "config", "db", "migrations", "31_10_2026_23_00_00.sql"))
	for _, stmt := range strings.Split(string(raw), "/***Statement***/") {
		if strings.TrimSpace(stmt) != "" {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatal(err)
			}
		}
	}
	got := map[string]string{}
	rows, _ := db.Query(`SELECT id, content_type FROM media_files WHERE id IN ('a','b','c')`)
	for rows.Next() {
		var i, c string
		rows.Scan(&i, &c)
		got[i] = c
	}
	if got["a"] != "application/pdf" || got["b"] != "application/octet-stream" || got["c"] != "text/plain" {
		t.Fatalf("%v", got)
	}
}
