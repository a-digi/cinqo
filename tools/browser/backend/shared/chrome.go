// chrome.go (step 53) — the ONE place that decides where Chrome/Chromium is,
// and the GET /chrome-status route that reports it. Every engine that drives
// a local Chrome uses it: Rod (crawler/rod_engine.go) and the chromedp
// sessions (session.go) are both handed this exact path, so the Chromium page
// can never say "installed" while the built-in fallback cannot start. (Before
// this, Rod and chromedp each searched their own, different, lists — Rod also
// finds Microsoft Edge, chromedp does not.) See
// plan/ai/tools/browser/step-53-chromium-page-and-gate.md.
//
// The Playwright engine is separate: it launches Playwright's own downloaded
// Chromium and does not use this path.
package shared

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// ChromePathEnv is the operator override for the Chrome location.
const ChromePathEnv = "BROWSER_TOOL_CHROME_PATH"

// ErrChromeNotFound: no Chrome or Chromium binary was found. A plain error,
// never a Cloudflare verdict — callers fall back or fail cleanly.
var ErrChromeNotFound = errors.New("no Chrome/Chromium binary found")

// ChromePathError: ChromePathEnv is set but does not point at a usable file.
// An explicit override is never silently ignored in favour of another browser.
type ChromePathError struct {
	Path string
	Err  error
}

func (e *ChromePathError) Error() string {
	return fmt.Sprintf("%s %q is not usable: %v", ChromePathEnv, e.Path, e.Err)
}

func (e *ChromePathError) Unwrap() error { return e.Err }

// LookChrome searches the per-OS list of Chrome and Chromium locations. A
// var so a test can simulate a machine with no browser (or a particular one).
var LookChrome = lookChromeOnThisMachine

// chromeCandidates is the fixed list of Chrome and Chromium locations for an
// OS, best first. Chrome and Chromium only — Microsoft Edge is deliberately
// not here. Pure (everything it needs is passed in) so every OS's list is
// testable on any machine.
func chromeCandidates(goos string, getenv func(string) string, home string) []string {
	switch goos {
	case "darwin":
		list := []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
		if home != "" {
			list = append(list,
				path.Join(home, "Applications", "Google Chrome.app", "Contents", "MacOS", "Google Chrome"),
				path.Join(home, "Applications", "Chromium.app", "Contents", "MacOS", "Chromium"),
			)
		}
		return list
	case "windows":
		list := []string{"chrome", "chrome.exe"}
		for _, root := range []string{getenv("ProgramFiles"), getenv("ProgramFiles(x86)"), getenv("LocalAppData")} {
			if root == "" {
				continue
			}
			root = strings.TrimRight(root, `\`)
			// Explicit backslashes, not filepath.Join: this list is built
			// for Windows whichever OS computes it (so it can be tested
			// anywhere).
			list = append(list,
				root+`\Google\Chrome\Application\chrome.exe`,
				root+`\Chromium\Application\chrome.exe`,
			)
		}
		return list
	default: // linux and other unix-likes
		return []string{
			"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome",
			"/usr/bin/google-chrome", "/usr/bin/google-chrome-stable", "/usr/bin/chromium", "/usr/bin/chromium-browser",
			"/snap/bin/chromium", "/usr/local/bin/chrome", "/opt/google/chrome/chrome",
		}
	}
}

// firstExecutable returns the first candidate that resolves to an
// executable (names via PATH, absolute paths directly).
func firstExecutable(candidates []string) (string, bool) {
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if found, err := exec.LookPath(c); err == nil && found != "" {
			return found, true
		}
	}
	return "", false
}

func lookChromeOnThisMachine() (string, bool) {
	home, _ := os.UserHomeDir()
	return firstExecutable(chromeCandidates(runtime.GOOS, os.Getenv, home))
}

// ChromePath resolves the Chrome binary: ChromePathEnv if set (it must exist
// and be a file, else *ChromePathError — never a silent fallback), otherwise
// the per-OS search (ErrChromeNotFound when it finds nothing). The result is
// never empty on success — an empty path would make rod's launcher DOWNLOAD
// a Chromium of its own, which this tool must never do.
func ChromePath() (string, error) {
	if p := strings.TrimSpace(os.Getenv(ChromePathEnv)); p != "" {
		info, err := os.Stat(p)
		if err != nil {
			return "", &ChromePathError{Path: p, Err: err}
		}
		if info.IsDir() {
			return "", &ChromePathError{Path: p, Err: errors.New("it is a directory")}
		}
		return p, nil
	}
	if p, ok := LookChrome(); ok && p != "" {
		return p, nil
	}
	return "", ErrChromeNotFound
}

// ChromeLaunchPath is the path the chromedp sessions should launch, and
// whether to pass one at all. An explicit ChromePathEnv is passed through
// as-is even if unusable (so a wrong override still fails loudly instead of
// quietly starting some other browser); otherwise the resolved path; and when
// nothing is found, no path (chromedp's own search then fails exactly as it
// always did).
func ChromeLaunchPath() (string, bool) {
	if p := strings.TrimSpace(os.Getenv(ChromePathEnv)); p != "" {
		return p, true
	}
	if p, err := ChromePath(); err == nil {
		return p, true
	}
	return "", false
}

// chromeVersionTimeout bounds the one `--version` call. A var so a test can
// shrink it.
var chromeVersionTimeout = 3 * time.Second

// printableVersion accepts what a real Chrome prints ("Google Chrome
// 156.0.8078.73", "Chromium 120.0.6099.109 built on Debian") and nothing
// else; a binary that answers with anything else gets no version rather than
// having its output shown.
var printableVersion = regexp.MustCompile(`^[A-Za-z0-9 ._()\-]{1,80}$`)

// ChromeVersion returns the browser's own version line, or "". Only on macOS
// and Linux: on Windows `chrome.exe --version` is unreliable (it can open a
// window), so it is skipped there. A convenience for the status page, never a
// condition for "installed".
func ChromeVersion(path string) string {
	if runtime.GOOS == "windows" || path == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), chromeVersionTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if !printableVersion.MatchString(line) {
		return ""
	}
	return line
}

// ChromeStatus is the body of GET /chrome-status. OS is the SERVER's
// operating system (where Cinqo and this tool run), so the page can show the
// right install instructions. Detail is a fixed sentence plus a path, never
// raw process output.
type ChromeStatus struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path"`
	Version   string `json:"version"`
	OS        string `json:"os"`
	Detail    string `json:"detail"`
}

// CurrentChromeStatus computes the status now (never cached, so installing
// Chrome is noticed on the next check, with no restart).
func CurrentChromeStatus() ChromeStatus {
	st := ChromeStatus{OS: runtime.GOOS}
	path, err := ChromePath()
	var pathErr *ChromePathError
	switch {
	case err == nil:
		st.Installed, st.Path, st.Version = true, path, ChromeVersion(path)
		st.Detail = "Chrome or Chromium found at " + path
	case errors.As(err, &pathErr):
		st.Detail = fmt.Sprintf("%s is set but not usable (%s)", ChromePathEnv, pathErr.Path)
	default:
		st.Detail = "Chrome or Chromium was not found"
	}
	return st
}

// ChromeStatusHandler handles GET /chrome-status.
func ChromeStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(CurrentChromeStatus())
}
