package conversation

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProduced(t *testing.T, dataDir, slug, name string) string {
	t.Helper()
	dir := filepath.Join(dataDir, "uploads", "tools", slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLocatePromotedFile(t *testing.T) {
	d := t.TempDir()
	want := writeProduced(t, d, "pdf_tools", "abc-123.pdf")

	for _, id := range []string{"abc-123", "abc-123.pdf"} {
		got, err := locatePromotedFile(d, "pdf_tools", id)
		if err != nil || got != want {
			t.Errorf("id %q: got %q, %v", id, got, err)
		}
	}
	if _, err := locatePromotedFile(d, "pdf_tools", "nope.pdf"); err == nil || !strings.HasPrefix(err.Error(), promotionFailedPrefix) {
		t.Errorf("a missing file must give the model-facing error, got %v", err)
	}
}

func TestLocatePromotedFileRefusesTraversal(t *testing.T) {
	d := t.TempDir()
	writeProduced(t, d, "pdf_tools", "abc.pdf")
	secret := filepath.Join(d, "secret.txt")
	if err := os.WriteFile(secret, []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := [][2]string{
		{"pdf_tools", "../../../secret"},
		{"pdf_tools", "..%2f..%2fsecret"},
		{"pdf_tools", "../../../secret.txt"},
		{"..", "x"},
		{"../..", "secret"},
		{"pdf_tools", "*"},
		{"pdf_tools", "ab?"},
		{"pdf_tools", "abc.pdf.pdf"},
		{"pdf_tools", ""},
	}
	for _, c := range cases {
		got, err := locatePromotedFile(d, c[0], c[1])
		if err == nil {
			t.Errorf("slug %q id %q resolved to %q", c[0], c[1], got)
			continue
		}
		if strings.Contains(err.Error(), d) {
			t.Errorf("the error leaks a local path: %v", err)
		}
	}
}

func TestPromotionFailureIsCountedAsToolFailure(t *testing.T) {
	_, err := locatePromotedFile(t.TempDir(), "pdf_tools", "missing")
	if !isToolFailure(err.Error()) {
		t.Errorf("not counted: %v", err)
	}
}

func TestResolvePromoteMediaArgumentAcceptsAnAppendedExtensionAndExplainsAFailure(t *testing.T) {
	d := t.TempDir()
	writeProduced(t, d, "pdf_tools", "abc-123.pdf")
	db := openMainDB(t)

	out, err := resolvePromoteMediaArgument(db, d, "career", "u1", "conv", []byte(`{"pdfResource":"/api/v1/tools/pdf_tools/proxy/files?id=abc-123.pdf"}`), "pdfResource")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "proxy/files") {
		t.Errorf("the link was not replaced by a media id: %s", out)
	}

	_, err = resolvePromoteMediaArgument(db, d, "career", "u1", "conv", []byte(`{"pdfResource":"/api/v1/tools/pdf_tools/proxy/files?id=gone.pdf"}`), "pdfResource")
	var pe *promotionError
	if !errors.As(err, &pe) {
		t.Fatalf("a missing file must be a promotionError the caller can show, got %v", err)
	}
}
