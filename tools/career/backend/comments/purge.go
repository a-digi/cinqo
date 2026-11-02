// Package comments is this tool's side of Cinqo's comments engine. The comments
// themselves live in core (in this tool's own comments database); the only thing the
// tool does is tell core when one of its entities is gone, so the comments about it
// go too. See plan/ai/comments/comments-engine.md.
package comments

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

// The entity types this tool declares in manifest.json's "comments" (the frontend spells them in
// Shared/Comments/commentTypes.ts): keep them in step.
const (
	JobType     = "job"
	ProfileType = "profile"
	PersonaType = "persona"
	CvType      = "cv"
	CompanyType = "company"
	// RecruiterType (plan 168, step 7): the comments about a recruiter, read and written with the companies scopes.
	RecruiterType = "recruiter"
)

// Entity is one thing whose comments are to be purged.
type Entity struct {
	Type string
	ID   string
}

// purgeTimeout bounds the call: it sits in the response path of a user's delete.
const purgeTimeout = 3 * time.Second

// PurgeEntity asks core to remove every comment about one entity (it was deleted),
// authenticating as this tool with TOOL_SERVICE_TOKEN. Best effort: a missing
// CORE_API_URL/TOOL_SERVICE_TOKEN, a network error or a refusal is logged and
// otherwise ignored, because failing to tidy up comments must never fail the
// delete itself (the orphaned comments are unreachable, since nothing shows them).
func PurgeEntity(entityType, entityID string) {
	purge(entityType, entityID)
}

// PurgeMany purges several entities (a delete that cascaded). It stops at the first call that could
// not reach core, so an unreachable core costs one timeout, not one per entity.
func PurgeMany(items []Entity) {
	for _, e := range items {
		if !purge(e.Type, e.ID) {
			return
		}
	}
}

// RefusedError is core answering a purge with something other than 200: the comments were not purged. A 4xx (an id core
// does not accept, a type the tool did not declare, a bad token) is final, asking again gets the same answer; a 5xx may pass.
type RefusedError struct {
	EntityType, EntityID string
	Status               int
}

func (e *RefusedError) Error() string {
	return fmt.Sprintf("core refused to purge comments for %s %q: status %d", e.EntityType, e.EntityID, e.Status)
}

// Final reports whether asking again cannot help (a 4xx answer).
func (e *RefusedError) Final() bool { return e.Status >= 400 && e.Status < 500 }

// PurgeEntityStrict is PurgeEntity that reports failure: an error when core is not configured, cannot be reached or does
// not answer 200 (the comments are then still there). For a caller that can be asked again, such as the job-deleted
// consumer, whose 5xx makes core deliver the event once more.
func PurgeEntityStrict(entityType, entityID string) error {
	_, err := send(entityType, entityID)
	return err
}

// purge sends one purge request and reports whether core could be reached (any answer counts,
// a refusal included): false for no configuration or a network error. Failures are logged.
func purge(entityType, entityID string) bool {
	reached, err := send(entityType, entityID)
	if err != nil {
		log.Printf("comments: %v", err)
	}
	return reached
}

// send is one purge request: whether core could be reached (any answer counts) and, when the comments were not purged,
// why.
func send(entityType, entityID string) (reached bool, err error) {
	coreURL := os.Getenv("CORE_API_URL")
	token := os.Getenv("TOOL_SERVICE_TOKEN")
	if coreURL == "" || token == "" {
		return false, errors.New("CORE_API_URL or TOOL_SERVICE_TOKEN is not set")
	}

	body, err := json.Marshal(map[string]string{"entityType": entityType, "entityId": entityID})
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), purgeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, coreURL+"/api/v1/comments/purge", bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("failed to build purge request for %s %q: %w", entityType, entityID, err)
	}
	req.Header.Set("Authorization", "ToolService "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("purge request failed for %s %q: %w", entityType, entityID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return true, &RefusedError{EntityType: entityType, EntityID: entityID, Status: resp.StatusCode}
	}
	return true, nil
}
