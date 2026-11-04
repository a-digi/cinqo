package cinqo

import (
	"fmt"
	"testing"
)

// The composition root registers its providers without error, can be run again (the app restarts its services in tests), and logs through
// the function it is given.
func TestConfigureLiveLogsComposesTheModule(t *testing.T) {
	var lines []string
	logf := func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }
	for i := 0; i < 2; i++ {
		if err := configureLiveLogs(t.TempDir(), logf); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
}
