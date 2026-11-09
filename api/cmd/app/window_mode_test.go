package main

import (
	"strings"
	"testing"
)

func TestTheWindowModeComesFromTheFlagThenTheEnvironmentThenTheDefault(t *testing.T) {
	cases := []struct {
		flag, env string
		want      windowMode
	}{
		{"", "", windowModeApp},
		{"   ", "  ", windowModeApp},
		{"app", "", windowModeApp},
		{"browser", "", windowModeBrowser},
		{"", "browser", windowModeBrowser},
		{"", "app", windowModeApp},
		{"app", "browser", windowModeApp},     // the flag wins
		{"browser", "app", windowModeBrowser}, // the flag wins
		{" Browser ", "", windowModeBrowser},  // trimmed and case-insensitive
		{"APP", "", windowModeApp},
		{"", " BROWSER\n", windowModeBrowser},
	}
	for _, c := range cases {
		got, err := parseWindowMode(c.flag, c.env)
		if err != nil || got != c.want {
			t.Errorf("flag %q env %q: got %q %v, want %q", c.flag, c.env, got, err, c.want)
		}
	}
}

func TestAnUnknownWindowModeIsRefusedWithTheAcceptedValuesAndWhereItCameFrom(t *testing.T) {
	_, err := parseWindowMode("tablet", "browser")
	if err == nil || !strings.Contains(err.Error(), `--window="tablet"`) || !strings.Contains(err.Error(), `"app"`) || !strings.Contains(err.Error(), `"browser"`) {
		t.Errorf("a bad flag is refused, even when the environment is fine: %v", err)
	}
	_, err = parseWindowMode("", "kiosk")
	if err == nil || !strings.Contains(err.Error(), `CINQO_WINDOW="kiosk"`) {
		t.Errorf("a bad environment value names CINQO_WINDOW: %v", err)
	}
	for _, bad := range []string{"apps", "app mode", "0", "none", "--app"} {
		if _, err := parseWindowMode(bad, ""); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

func TestTheApplicationWindowArguments(t *testing.T) {
	got := chromeArgs("/tmp/profile-1", 9222, "http://localhost:7030", windowModeApp)
	want := []string{
		"--user-data-dir=/tmp/profile-1",
		"--no-first-run",
		"--no-default-browser-check",
		"--remote-debugging-port=9222",
		"--app=http://localhost:7030",
		"--window-size=1440,900",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTheBrowserWindowArgumentsAreExactlyWhatTheLauncherDidBefore(t *testing.T) {
	got := chromeArgs("/tmp/profile-1", 9222, "http://localhost:7030", windowModeBrowser)
	want := []string{
		"--user-data-dir=/tmp/profile-1",
		"--no-first-run",
		"--no-default-browser-check",
		"--remote-debugging-port=9222",
		"http://localhost:7030",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %q, want %q", got, want)
	}
	for _, a := range got {
		if strings.HasPrefix(a, "--app") || strings.HasPrefix(a, "--window-size") {
			t.Errorf("the normal window must not be an application window: %q", a)
		}
	}
}

func TestEveryModeKeepsTheFlagsTheWindowWatcherNeeds(t *testing.T) {
	for _, mode := range []windowMode{windowModeApp, windowModeBrowser} {
		args := strings.Join(chromeArgs("/p", 51234, "http://x", mode), " ")
		for _, must := range []string{"--user-data-dir=/p", "--remote-debugging-port=51234", "--no-first-run", "--no-default-browser-check"} {
			if !strings.Contains(args, must) {
				t.Errorf("%s: %q missing in %s", mode, must, args)
			}
		}
	}
}

func TestAnAddressWithSpecialCharactersIsOneArgumentAndNeverSplit(t *testing.T) {
	url := "http://localhost:7030/path with space?a=1&b=2#frag\"quote'"
	for _, mode := range []windowMode{windowModeApp, windowModeBrowser} {
		args := chromeArgs("/tmp/p q", 1, url, mode) // a profile folder with a space too
		if args[0] != "--user-data-dir=/tmp/p q" {
			t.Errorf("%s: %q", mode, args[0])
		}
		found := 0
		for _, a := range args {
			if strings.Contains(a, "path with space?a=1&b=2#frag\"quote'") {
				found++
			}
		}
		if found != 1 {
			t.Errorf("%s: the address must be exactly one argument, found in %d: %q", mode, found, args)
		}
	}
	if args := chromeArgs("/p", 1, url, windowModeApp); args[4] != "--app="+url {
		t.Errorf("%q", args[4])
	}
}

func TestAnUnknownModeFallsBackToTheApplicationWindowNeverToANoOp(t *testing.T) {
	// parseWindowMode refuses unknown words, so this cannot happen from the launcher; the function itself still gives a usable window
	args := chromeArgs("/p", 1, "http://x", windowMode("odd"))
	if !strings.Contains(strings.Join(args, " "), "--app=http://x") {
		t.Errorf("%q", args)
	}
}

func TestTheDefaultModeOfARunIsTheApplicationWindow(t *testing.T) {
	if launchWindowMode != windowModeApp {
		t.Errorf("the default is %q", launchWindowMode)
	}
	if appWindowWidth != 1440 || appWindowHeight != 900 {
		t.Errorf("the plan's starting size is 1440 x 900: %d x %d", appWindowWidth, appWindowHeight)
	}
}

// The command the launcher really runs follows the mode of this run, in both directions.
func TestTheLaunchedCommandFollowsTheModeOfTheRun(t *testing.T) {
	old := launchWindowMode
	t.Cleanup(func() { launchWindowMode = old })

	launchWindowMode = windowModeApp
	cmd := newChromeCommand("/chrome", "/p", 9222, "http://localhost:7030")
	if cmd.Path != "/chrome" || cmd.Args[0] != "/chrome" || !strings.Contains(strings.Join(cmd.Args, " "), "--app=http://localhost:7030") {
		t.Errorf("app mode: %q", cmd.Args)
	}
	launchWindowMode = windowModeBrowser
	cmd = newChromeCommand("/chrome", "/p", 9222, "http://localhost:7030")
	if strings.Contains(strings.Join(cmd.Args, " "), "--app") || cmd.Args[len(cmd.Args)-1] != "http://localhost:7030" {
		t.Errorf("browser mode: %q", cmd.Args)
	}
}
