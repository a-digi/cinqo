package localai

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Limits of what an archive may contain, so a hostile or broken file cannot fill the disk or the file table. Variables only
// so a test can lower them.
var (
	maxArchiveFiles       = 2000
	maxArchiveBytes int64 = 1 << 30 // 1 GiB unpacked
)

var errUnsafeArchive = errors.New("the archive contains something that is not allowed")

func unsafeEntry(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errUnsafeArchive, fmt.Sprintf(format, args...))
}

// extractTarGz unpacks a .tar.gz into destDir. The tree is built in a temporary folder next to destDir and moved into
// place only when everything was unpacked, so destDir is either the complete tree or absent (an older destDir is replaced).
// Only folders, regular files and relative symbolic links that stay inside the tree are accepted: an absolute path, a ".."
// anywhere, a link that points outside, a file written through a link, a hard link, a device or a pipe refuses the whole
// archive. File modes are reduced to plain 0644 or 0755 (no setuid, no write for others).
func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return errors.New("the archive could not be opened")
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return errors.New("the file is not a valid .tar.gz archive")
	}
	defer gz.Close()

	if err := os.MkdirAll(filepath.Dir(destDir), 0o755); err != nil {
		return errors.New("the folder for the program could not be created")
	}
	tmp, err := os.MkdirTemp(filepath.Dir(destDir), filepath.Base(destDir)+".tmp-")
	if err != nil {
		return errors.New("the folder for the program could not be created")
	}
	defer os.RemoveAll(tmp) // gone after a success (it was moved) and after any failure

	tr := tar.NewReader(gz)
	files := 0
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.New("the archive is damaged")
		}
		files++
		if files > maxArchiveFiles {
			return unsafeEntry("more than %d entries", maxArchiveFiles)
		}
		name, err := cleanEntryName(hdr.Name)
		if err != nil {
			return err
		}
		if name == "" { // the root folder itself ("./")
			continue
		}
		target := filepath.Join(tmp, filepath.FromSlash(name))
		if err := noSymlinkAbove(tmp, target); err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return errors.New("a folder of the archive could not be created")
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return errors.New("a folder of the archive could not be created")
			}
			mode := os.FileMode(0o644)
			if hdr.FileInfo().Mode().Perm()&0o111 != 0 {
				mode = 0o755
			}
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
			if err != nil {
				return errors.New("a file of the archive could not be created")
			}
			n, cerr := io.Copy(out, io.LimitReader(tr, maxArchiveBytes-total+1))
			total += n
			if err := out.Close(); err != nil && cerr == nil {
				cerr = err
			}
			if total > maxArchiveBytes {
				return unsafeEntry("more than %d bytes when unpacked", maxArchiveBytes)
			}
			if cerr != nil {
				return errors.New("the archive is damaged")
			}
			if err := os.Chmod(target, mode); err != nil {
				return errors.New("a file of the archive could not be created")
			}
		case tar.TypeSymlink:
			if err := checkLink(name, hdr.Linkname); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return errors.New("a folder of the archive could not be created")
			}
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return errors.New("a link of the archive could not be created")
			}
		default:
			return unsafeEntry("%q is a hard link, a device or another special entry", hdr.Name)
		}
	}

	if err := os.RemoveAll(destDir); err != nil {
		return errors.New("an older copy of the program could not be removed")
	}
	if err := os.Rename(tmp, destDir); err != nil {
		return errors.New("the program could not be put in place")
	}
	return nil
}

// cleanEntryName returns the entry's path relative to the tree ("" for the root), or an error for a name that is absolute or
// leaves the tree.
func cleanEntryName(raw string) (string, error) {
	if raw == "" || strings.HasPrefix(raw, "/") || strings.ContainsRune(raw, 0) {
		return "", unsafeEntry("the path %q is not relative", raw)
	}
	for _, part := range strings.Split(raw, "/") {
		if part == ".." {
			return "", unsafeEntry("the path %q goes up a folder", raw)
		}
	}
	cleaned := path.Clean(raw)
	if cleaned == "." {
		return "", nil
	}
	return cleaned, nil
}

// checkLink accepts a symbolic link only if its target is relative and, resolved from the link's own folder, stays inside
// the tree.
func checkLink(name, target string) error {
	if target == "" || strings.HasPrefix(target, "/") || strings.ContainsRune(target, 0) {
		return unsafeEntry("the link %q points to an absolute or empty target", name)
	}
	resolved := path.Join(path.Dir(name), target)
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return unsafeEntry("the link %q points outside the folder", name)
	}
	return nil
}

// noSymlinkAbove refuses to create anything below a symbolic link, so a link can never redirect later entries elsewhere.
func noSymlinkAbove(root, target string) error {
	rel, err := filepath.Rel(root, filepath.Dir(target))
	if err != nil || rel == "." {
		return nil
	}
	cur := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		st, err := os.Lstat(cur)
		if err != nil {
			return nil // not created yet
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return unsafeEntry("an entry is placed below the link %q", part)
		}
	}
	return nil
}
