package crawler

import (
	"os"
	"testing"

	"browser-tool-backend/shared/chromeproc"
)

// The tests keep the profile folders of the Chromes they start in a folder of their own, and stop every engine's Chrome when they are done,
// so a run leaves nothing behind in the system's temporary folder (plan 224, step 2).
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "crawler-chrome-")
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
