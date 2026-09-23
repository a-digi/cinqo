package media

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// existsTimeout is deliberately shorter than setTitleTimeout — this
// call sits directly in a user-facing GET request's own response path
// (jobs.go's queryJobs/GetJobByID, checking whether a job's own CV is
// still really downloadable), not a fire-and-forget background call
// after an already-completed action.
const existsTimeout = 2 * time.Second

// Exists reports whether mediaFileID is a real Media row this tool
// owns, authenticating with this process's own TOOL_SERVICE_TOKEN.
// Returns false ONLY on a definitive 404 from core's own
// GET .../exists route — every other outcome (missing
// CORE_API_URL/TOOL_SERVICE_TOKEN, a network error, a timeout, an
// unexpected status) FAILS OPEN (true): none of those say the file is
// actually gone, and a transient core hiccup must never hide a
// possibly-real CV download link. See
// plan/ai/media/step-11-media-exists-check.md.
func Exists(mediaFileID string) bool {
	coreURL := os.Getenv("CORE_API_URL")
	token := os.Getenv("TOOL_SERVICE_TOKEN")
	if coreURL == "" || token == "" {
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), existsTimeout)
	defer cancel()

	url := fmt.Sprintf("%s/api/v1/media/%s/exists", coreURL, mediaFileID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.Printf("media: failed to build exists request for %q: %v", mediaFileID, err)
		return true
	}
	req.Header.Set("Authorization", "ToolService "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("media: exists request failed for %q: %v", mediaFileID, err)
		return true
	}
	defer resp.Body.Close()

	return resp.StatusCode != http.StatusNotFound
}
