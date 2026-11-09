package shared

import (
	"os"
	"testing"

	"browser-tool-backend/shared/chromeproc"
)

// See crawler/testmain_test.go: the tests' Chrome profile folders live in a folder of their own and every browser is stopped at the end.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "shared-chrome-")
	if err == nil {
		_ = chromeproc.Configure(dir)
	}
	code := m.Run()
	StopSharedSession()
	StopHeadedBrowser()
	if err == nil {
		_ = os.RemoveAll(dir)
	}
	os.Exit(code)
}
