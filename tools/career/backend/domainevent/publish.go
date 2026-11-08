// Package domainevent is Career's own small client for cinqo core's
// Domain Events engine (api/src/domainevent) — this backend is a
// separate Go module and cannot import that package directly, so this
// is Career's own minimal Publish function, shared by every one of
// this tool's own publish call sites (jobs/cv_pdf.go's
// publishCvGeneratedEvent, portal's set_portal_link_crawl_instructions
// handler, crawl's own runCrawlNow) instead of each one duplicating the
// same env-var-read-and-POST boilerplate independently. See
// plan/ai/domain-events/step-05-tool-publish-endpoint.md (the core
// side) and plan/ai/tools/career/step-66-career-event-publishers.md
// (this package's own introduction).
package domainevent

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

// publishTimeout bounds a single publish attempt — short, so an
// unreachable core never meaningfully delays whatever already-completed
// action a call to Publish follows. See each call site's own doc
// comment for why running synchronously (not a detached goroutine) is
// safe there.
const publishTimeout = 5 * time.Second

// Publish best-effort-publishes topic with payload into core's own
// Domain Events engine, authenticating with this process's own
// TOOL_SERVICE_TOKEN. A missing CORE_API_URL/TOOL_SERVICE_TOKEN, a
// marshal failure, an unreachable core, or a non-2xx response are all
// logged and otherwise ignored — publishing an event must never turn an
// already-successful action into a reported failure at any call site.
func Publish(topic string, payload any) {
	coreURL := os.Getenv("CORE_API_URL")
	token := os.Getenv("TOOL_SERVICE_TOKEN")
	if coreURL == "" || token == "" {
		return
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		log.Printf("domainevent: failed to marshal payload for topic %q: %v", topic, err)
		return
	}
	body, err := json.Marshal(map[string]any{
		"topic":   topic,
		"payload": json.RawMessage(rawPayload),
	})
	if err != nil {
		log.Printf("domainevent: failed to marshal request body for topic %q: %v", topic, err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, coreURL+"/api/v1/events/publish", bytes.NewReader(body))
	if err != nil {
		log.Printf("domainevent: failed to build request for topic %q: %v", topic, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "ToolService "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("domainevent: publish request failed for topic %q: %v", topic, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("domainevent: publish for topic %q responded with status %d", topic, resp.StatusCode)
	}
}
