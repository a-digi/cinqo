// Package reviews is this tool's side of Cinqo's review engine. The reviews themselves (star ratings with an optional text) live in
// core, in this tool's own review database (career_review.db); the only thing the tool does is tell core when one of its entities is
// gone, so the reviews about it go too. See plan/ai/review/review-engine.md.
package reviews

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

// The entity types this tool declares in manifest.json's "reviews" (the frontend spells them in Shared/Reviews/reviewTypes.ts): keep them
// in step.
const CompanyType = "company"

// purgeTimeout bounds the call: it is made while a delete's event is delivered, and core retries a slow answer.
const purgeTimeout = 3 * time.Second

// RefusedError is core answering a purge with something other than 200: the reviews were not purged. A 4xx (an id core does not
// accept, a type the tool did not declare, a bad token) is final, asking again gets the same answer; a 5xx may pass.
type RefusedError struct {
	EntityType, EntityID string
	Status               int
}

func (e *RefusedError) Error() string {
	return fmt.Sprintf("core refused to purge reviews for %s %q: status %d", e.EntityType, e.EntityID, e.Status)
}

// Final reports whether asking again cannot help (a 4xx answer).
func (e *RefusedError) Final() bool { return e.Status >= 400 && e.Status < 500 }

// PurgeEntityStrict asks core to remove every review about one entity (it was deleted), authenticating as this tool with
// TOOL_SERVICE_TOKEN. It reports failure: an error when core is not configured, cannot be reached or does not answer 200 (the reviews
// are then still there). For the company-deleted consumer, whose 5xx makes core deliver the event once more. Purging twice is harmless.
func PurgeEntityStrict(entityType, entityID string) error {
	coreURL := os.Getenv("CORE_API_URL")
	token := os.Getenv("TOOL_SERVICE_TOKEN")
	if coreURL == "" || token == "" {
		return errors.New("CORE_API_URL or TOOL_SERVICE_TOKEN is not set")
	}
	body, err := json.Marshal(map[string]string{"entityType": entityType, "entityId": entityID})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), purgeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, coreURL+"/api/v1/reviews/purge", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to build purge request for %s %q: %w", entityType, entityID, err)
	}
	// ToolService, not Bearer: a tool's own token is never confused with a user's session.
	req.Header.Set("Authorization", "ToolService "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("purge request failed for %s %q: %w", entityType, entityID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &RefusedError{EntityType: entityType, EntityID: entityID, Status: resp.StatusCode}
	}
	return nil
}
