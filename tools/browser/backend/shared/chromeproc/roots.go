package chromeproc

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// The profile folders of the Chromes this tool starts (plan 224, step 2). Every headless Chrome gets its own named folder under one
// root, so a Chrome that is ours can be told from every other one by its command line (`--user-data-dir=<our root>/...`), and nothing
// else is ever touched. The persistent profiles (the visible window, the headed fallback) and the login windows' folder keep their own
// places; their roots are added with AddOwnedRoot when they are first used.

var (
	rootsMu     sync.Mutex
	profileRoot string
	ownedRoots  []string
)

// ProfileRoot is the folder the headless Chromes' profiles live in. Without Configure it is a folder in the system's temporary folder, so a
// Chrome is still recognisable on a computer where the tool has no cache folder.
func ProfileRoot() string {
	rootsMu.Lock()
	defer rootsMu.Unlock()
	return profileRootLocked()
}

func profileRootLocked() string {
	if profileRoot == "" {
		return filepath.Join(os.TempDir(), "cinqo-browser-chrome", "profiles")
	}
	return profileRoot
}

func setProfileRoot(dir string) {
	rootsMu.Lock()
	profileRoot = dir
	rootsMu.Unlock()
}

// AddOwnedRoot says that Chromes started with a profile folder at or under dir are this tool's (a persistent profile or the folder the login
// windows' profiles are made in). Adding the same folder twice does nothing; an empty or relative folder is refused.
func AddOwnedRoot(dir string) {
	if dir == "" || !filepath.IsAbs(dir) {
		return
	}
	dir = filepath.Clean(dir)
	rootsMu.Lock()
	defer rootsMu.Unlock()
	for _, r := range ownedRoots {
		if r == dir {
			return
		}
	}
	ownedRoots = append(ownedRoots, dir)
}

// OwnedRoots is every folder whose Chromes are this tool's: the profile root and the added ones.
func OwnedRoots() []string {
	rootsMu.Lock()
	defer rootsMu.Unlock()
	return append([]string{profileRootLocked()}, ownedRoots...)
}

// NewProfileDir makes a new profile folder for a Chrome of this kind: <profile root>/<kind>-<8 random hex digits>, owner-only.
func NewProfileDir(kind Kind) (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("chromeproc: no random numbers: %w", err)
	}
	root := ProfileRoot()
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("chromeproc: cannot make the profile root: %w", err)
	}
	dir := filepath.Join(root, string(kind)+"-"+hex.EncodeToString(b[:]))
	if err := os.Mkdir(dir, 0o700); err != nil {
		return "", fmt.Errorf("chromeproc: cannot make a profile folder: %w", err)
	}
	return dir, nil
}

// RemoveProfileDir deletes a profile folder this package made. It refuses anything that is not directly inside the profile root (a safety
// net: a wrong path must never delete anything else), and a folder that is already gone is not an error.
func RemoveProfileDir(dir string) error {
	root := ProfileRoot()
	clean := filepath.Clean(dir)
	if filepath.Dir(clean) != root || clean == root {
		return errors.New("chromeproc: not a profile folder of this tool")
	}
	return os.RemoveAll(clean)
}

// commandUnderRoot says whether a command line starts a Chrome whose profile folder is root or inside it. It looks for the exact text
// `--user-data-dir=<root>` and requires the root to end there (a "/" follows, or a space, or the line ends), so /a/b never matches /a/bc;
// it does not parse the value, so a folder with spaces in its name still works.
func commandUnderRoot(cmd, root string) bool {
	needle := "--user-data-dir=" + root
	for from := 0; ; {
		i := strings.Index(cmd[from:], needle)
		if i < 0 {
			return false
		}
		end := from + i + len(needle)
		if end == len(cmd) || cmd[end] == '/' || cmd[end] == ' ' {
			return true
		}
		from = end
	}
}

// IsOurs says whether a command line belongs to a Chrome with a profile in one of this tool's folders.
func IsOurs(cmd string) bool {
	for _, r := range OwnedRoots() {
		if commandUnderRoot(cmd, r) {
			return true
		}
	}
	return false
}
