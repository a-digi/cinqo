package shared

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

// LogFileName is the tool's own log file under TOOL_LOGS_DIR, next to
// crawl_logs/ and challenge_logs/.
const LogFileName = "browser.log"

// maxLogFileBytes — at startup, a log file larger than this is rotated
// to browser.log.1 (replacing any previous one), so the file can never
// grow without bound across restarts.
const maxLogFileBytes = 10 << 20

// InitLogFile sends the standard logger to TOOL_LOGS_DIR/browser.log in
// addition to stderr. Needed because the host app starts this tool with
// stdout/stderr discarded (/dev/null — observed on a real install), so
// every log.Printf — the crawl-engine lines in particular — was lost.
// HTTP mode only: in --mcp mode stdout is the MCP transport. Call after
// InitDB (which validates TOOL_LOGS_DIR).
func InitLogFile() error {
	logsDir := os.Getenv("TOOL_LOGS_DIR")
	if logsDir == "" {
		return fmt.Errorf("TOOL_LOGS_DIR is not set")
	}
	path := filepath.Join(logsDir, LogFileName)
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogFileBytes {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", path, err)
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	return nil
}
