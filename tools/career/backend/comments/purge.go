// Package comments is this tool's side of Cinqo's comments engine. The comments
// themselves live in core (in this tool's own comments database); the only thing the
// tool does is tell core when one of its entities is gone, so the comments about it
// go too. See plan/ai/comments/comments-engine.md.
package comments

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

// The entity types this tool declares in manifest.json's "comments" (the frontend spells them in
// Shared/Comments/commentTypes.ts): keep the four in step.
const (
	JobType     = "job"
	ProfileType = "profile"
	PersonaType = "persona"
	CvType      = "cv"
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

// purge sends one purge request and reports whether core could be reached (any answer counts,
// a refusal included): false for no configuration or a network error.
func purge(entityType, entityID string) bool {
	coreURL := os.Getenv("CORE_API_URL")
	token := os.Getenv("TOOL_SERVICE_TOKEN")
	if coreURL == "" || token == "" {
		return false
	}

	body, err := json.Marshal(map[string]string{"entityType": entityType, "entityId": entityID})
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), purgeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, coreURL+"/api/v1/comments/purge", bytes.NewReader(body))
	if err != nil {
		log.Printf("comments: failed to build purge request for %s %q: %v", entityType, entityID, err)
		return false
	}
	req.Header.Set("Authorization", "ToolService "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("comments: purge request failed for %s %q: %v", entityType, entityID, err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("comments: core refused to purge comments for %s %q: status %d", entityType, entityID, resp.StatusCode)
	}
	return true
}
