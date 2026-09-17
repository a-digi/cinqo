// challenge_log.go is a Debug + Log HTML-only diagnostic aid for the
// Cloudflare human-wait loop (waitForHumanToClearCloudflare, crawl.go)
// — dumps the exact HTML that loop's own expected-content/Cloudflare
// checks are about to evaluate on each tick, one file per tick, so a
// still-stuck human-wait can be debugged from disk without guessing
// what the page actually looked like at each retry. A single crawl
// session (one requestID) can produce many of these across a long
// wait, so each session gets its own subfolder rather than one flat
// directory shared across every concurrent/historical crawl.
package crawler

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"browser-tool-backend/shared"
)

// logChallengeDetectorHTML best-effort writes html to its own file
// under shared.ChallengeLogsDir/{requestID}/ — a no-op when requestID
// is empty (the AI's own MCP-driven calls never set one, same opt-in
// convention crawl_status.go/crawl_cancel.go already establish; there
// is no session to fold a log under in that case). A write failure
// (disk full, permissions) is never worth failing or interrupting the
// wait loop itself over — this is pure observability, matching
// saveCrawlLog/fetchCacheStore's own established "best-effort" logging
// convention.
func logChallengeDetectorHTML(requestID string, attempt int, html string) {
	if requestID == "" {
		return
	}

	dir := filepath.Join(shared.ChallengeLogsDir, requestID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	fileName := fmt.Sprintf("challenge_detector_%d_%d.txt", time.Now().Unix(), attempt)
	_ = os.WriteFile(filepath.Join(dir, fileName), []byte(html), 0o644)
}
