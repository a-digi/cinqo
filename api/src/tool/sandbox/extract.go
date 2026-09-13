// Package sandbox extracts an uploaded tool package into a staging
// directory under hard security guards — verified directly against
// coco-mda's real plugin_sandbox.go rather than assumed. See
// plan/ai/tools/step-02-manifest-and-safe-zip-extraction.md.
package sandbox

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// MaxEntries caps the number of entries a package may contain —
	// a zip-bomb defense independent of the byte-size cap below (many
	// tiny entries can also be used to exhaust resources).
	MaxEntries = 1000
	// MaxUncompressedBytes caps the cumulative uncompressed size of
	// every entry combined, checked incrementally during extraction —
	// not only against the final total — so a bomb is caught mid-way,
	// not after it's already been fully written to disk.
	MaxUncompressedBytes = 200 * 1024 * 1024

	// unixSymlinkModeBits is the Unix S_IFLNK file-type bits within a
	// zip entry's external attributes (bits 16-31). Checked in addition
	// to f.Mode()&os.ModeSymlink — archive/zip only decodes Mode() this
	// way when the entry's CreatorVersion indicates a Unix host;
	// checking the raw bits too is defense in depth against a package
	// crafted to dodge that decoding, matching the reference's own
	// dual check exactly.
	unixSymlinkModeBits = 0xA000
)

// ExtractZip extracts every entry of r into destDir — a fresh,
// caller-provided staging directory, never the final install
// location — rejecting zip-slip path escapes and symlink entries, and
// enforcing the entry-count/cumulative-size caps above.
func ExtractZip(r *zip.Reader, destDir string) error {
	if len(r.File) > MaxEntries {
		return fmt.Errorf("package has %d entries, exceeding the %d-entry limit", len(r.File), MaxEntries)
	}

	cleanDest, err := filepath.Abs(filepath.Clean(destDir))
	if err != nil {
		return err
	}

	var totalUncompressed uint64
	for _, f := range r.File {
		if isSymlink(f) {
			return fmt.Errorf("entry %q is a symlink, which is not allowed", f.Name)
		}

		target, err := resolveEntryPath(cleanDest, f.Name)
		if err != nil {
			return err
		}

		totalUncompressed += f.UncompressedSize64
		if totalUncompressed > MaxUncompressedBytes {
			return fmt.Errorf("package's uncompressed size exceeds the %d-byte limit", MaxUncompressedBytes)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := extractFile(f, target); err != nil {
			return err
		}
	}

	return nil
}

func isSymlink(f *zip.File) bool {
	if f.Mode()&os.ModeSymlink != 0 {
		return true
	}
	return (f.ExternalAttrs>>16)&0xF000 == unixSymlinkModeBits
}

// resolveEntryPath rejects zip-slip: an entry name that, once joined
// to destDir, resolves outside it (via "../" or an absolute path).
func resolveEntryPath(destDir, name string) (string, error) {
	cleanName := filepath.Clean(strings.TrimPrefix(name, "/"))
	target := filepath.Join(destDir, cleanName)

	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if absTarget != destDir && !strings.HasPrefix(absTarget, destDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("entry %q escapes the extraction directory", name)
	}
	return absTarget, nil
}

func extractFile(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, rc); err != nil {
		return err
	}
	return nil
}
