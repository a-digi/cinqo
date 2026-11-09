package crawler

import (
	"os"
	"testing"

	"browser-tool-backend/shared/chromeproc"
)

// The tests keep the profile folders of the Chromes they start in a folder of their own, and stop every engine's Chrome when they are done,
// so a run leaves nothing behind in the system's temporary folder (plan 224, step 2).
// testChromeDir is the folder of the tests' Chrome ledger and profiles.
var testChromeDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "crawler-chrome-")
	testChromeDir = dir
	if err == nil {
		_ = chromeproc.Configure(dir)
	}
	code := m.Run()
	StopRodEngine()
	StopKatanaEngine()
	StopKatanaWindow()
	if err == nil {
		_ = os.RemoveAll(dir)
	}
	os.Exit(code)
}
