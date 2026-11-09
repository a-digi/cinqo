package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// The kind of Chrome window the app opens for itself (plan 227). "app" is Chrome's application window (--app=<url>): no tab strip, no address
// bar, no bookmarks bar, just the page and the window frame. "browser" is the normal window with a tab, which is what the launcher did before.
type windowMode string

const (
	windowModeApp     windowMode = "app"
	windowModeBrowser windowMode = "browser"
)

// The size an application window starts at. Every launch uses a fresh temporary profile, so Chrome remembers nothing about the last size.
const (
	appWindowWidth  = 1440
	appWindowHeight = 900
)

// launchWindowMode is the mode this run uses, set once at start-up from the flag and the environment (see parseWindowMode).
var launchWindowMode = windowModeApp

// parseWindowMode picks the window mode: the --window flag when it is set, else CINQO_WINDOW, else "app". The words are case-insensitive and
// trimmed; anything else is refused with a message that names the accepted values (a typo must never silently open the wrong kind of window).
func parseWindowMode(flagValue, envValue string) (windowMode, error) {
	raw, source := strings.TrimSpace(flagValue), "--window"
	if raw == "" {
		raw, source = strings.TrimSpace(envValue), "CINQO_WINDOW"
	}
	if raw == "" {
		return windowModeApp, nil
	}
	switch windowMode(strings.ToLower(raw)) {
	case windowModeApp:
		return windowModeApp, nil
	case windowModeBrowser:
		return windowModeBrowser, nil
	}
	return "", fmt.Errorf("%s=%q is not a window mode: use %q (an application window without an address bar) or %q (a normal browser window)", source, raw, windowModeApp, windowModeBrowser)
}

// chromeArgs is the command line of the private Chrome the app starts: its own throw-away profile (a separate Chrome instance, with its own PID),
// no first-run or default-browser questions, a debug port the launcher watches to learn when the window is gone, and then the page, as an
// application window or as a tab of a normal window.
func chromeArgs(profileDir string, debugPort int, url string, mode windowMode) []string {
	args := []string{
		"--user-data-dir=" + profileDir,
		"--no-first-run",
		"--no-default-browser-check",
		fmt.Sprintf("--remote-debugging-port=%d", debugPort),
	}
	if mode == windowModeBrowser {
		return append(args, url)
	}
	return append(args, "--app="+url, fmt.Sprintf("--window-size=%d,%d", appWindowWidth, appWindowHeight))
}

// newChromeCommand is the command that starts the private Chrome, using the window mode this run was started with.
func newChromeCommand(chromePath, profileDir string, debugPort int, url string) *exec.Cmd {
	return exec.Command(chromePath, chromeArgs(profileDir, debugPort, url, launchWindowMode)...)
}
