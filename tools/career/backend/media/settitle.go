// Package media is Career's own small client for cinqo core's Media
// PATCH .../title service route — mirrors domainevent.Publish's own
// shape closely (same env vars, same short timeout), since this is the
// exact same "a Tool's backend acting as itself, not as any end user"
// problem domainevent.Publish already solved. See
// tools/career/backend/domainevent/publish.go and
// plan/ai/media/step-10-obligatory-title.md.
package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// setTitleTimeout mirrors domainevent.Publish's own publishTimeout —
// short, so an unreachable core never meaningfully delays whatever
// already-completed action a call to SetTitle follows.
const setTitleTimeout = 5 * time.Second

// ErrMediaNotOwned is SetTitle's ONE non-nil, actionable return value —
// a 404 or 403 from core's own PATCH .../title route definitively
// means mediaFileID is either not a real Media row at all, or not one
// this tool owns. Unlike every other failure mode below (which stays
// best-effort/log-and-ignore, since it says nothing about whether
// mediaFileID itself is valid), this one is a real signal a caller
// should act on — see cv_pdf.go's own use of it to reject a
// never-actually-promoted pdfResource BEFORE recording it on a job,
// rather than discovering it later via a dead download link (the
// live-observed bug this constant was added to fix).
var ErrMediaNotOwned = errors.New("media file not found or not owned by this tool")

// SetTitle sets mediaFileID's own title, authenticating with this
// process's own TOOL_SERVICE_TOKEN. Returns ErrMediaNotOwned on a
// definitive 404/403 (see above); every other failure (missing
// CORE_API_URL/TOOL_SERVICE_TOKEN, a marshal failure, an unreachable
// core, an unexpected non-2xx status) is logged and returns nil —
// none of those say anything conclusive about mediaFileID's own
// validity, so a caller must never treat them as a reason to reject an
// otherwise-legitimate id.
func SetTitle(mediaFileID, title string) error {
	coreURL := os.Getenv("CORE_API_URL")
	token := os.Getenv("TOOL_SERVICE_TOKEN")
	if coreURL == "" || token == "" {
		return nil
	}

	body, err := json.Marshal(map[string]string{"title": title})
	if err != nil {
		log.Printf("media: failed to marshal title request for %q: %v", mediaFileID, err)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), setTitleTimeout)
	defer cancel()

	url := fmt.Sprintf("%s/api/v1/media/%s/title", coreURL, mediaFileID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("media: failed to build title request for %q: %v", mediaFileID, err)
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "ToolService "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("media: title request failed for %q: %v", mediaFileID, err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		return ErrMediaNotOwned
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("media: title request for %q responded with status %d", mediaFileID, resp.StatusCode)
	}
	return nil
}
