package crawler

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sync"
	"time"
)

// conversation_engine.go (plan 197, step 5): which engine reads the pages an AI conversation asks for. The Browser tool has ONE selected
// engine, but a Career link can be set to Rod while Katana is selected (the site stops Katana's Chrome). The AI that instructs such a link
// reads its pages with fetch_page_html, a tool with no link and no engine argument (and Rod's AI tools must stay as they are), so under a global
// Katana that read would go through the very browser that was stopped. The one channel a tool call already has is the id of its conversation
// (aiContextId, plan 183, set by core, never by the model): Career's page tells the Browser, once, when it creates the conversation of a Rod
// link, which engine that conversation reads with; POST /crawl then uses it for that conversation's single-page reads.
//
// It is in memory only (nothing persists, a restart forgets it), expires after an hour (a hidden Generate conversation is short), is limited
// in size, is set by a person's signed-in session through the ordinary tool proxy (route PUT /conversation-engine, scope tool:browser:crawl) and
// is never an MCP tool, so the AI cannot change which engine reads for it. A conversation with no entry, or an expired one, reads with the
// Browser's selected engine exactly as before.

const (
	conversationEngineTTL = time.Hour
	conversationEngineMax = 1000
)

var conversationIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)

type conversationEngineEntry struct {
	engine  string
	expires time.Time
}

var conversationEngines = struct {
	mu  sync.Mutex
	m   map[string]conversationEngineEntry
	now func() time.Time
}{m: map[string]conversationEngineEntry{}, now: time.Now}

// setConversationEngine remembers the engine for a conversation. An unknown engine or a malformed id is refused (false).
func setConversationEngine(conversationID, engine string) bool {
	if !conversationIDPattern.MatchString(conversationID) || (engine != fetchEngineRod && engine != fetchEngineKatana) {
		return false
	}
	conversationEngines.mu.Lock()
	defer conversationEngines.mu.Unlock()
	now := conversationEngines.now()
	for id, e := range conversationEngines.m { // forget the expired ones while we hold the lock
		if !e.expires.After(now) {
			delete(conversationEngines.m, id)
		}
	}
	if _, exists := conversationEngines.m[conversationID]; !exists && len(conversationEngines.m) >= conversationEngineMax {
		return false // full of live entries: refuse rather than grow
	}
	conversationEngines.m[conversationID] = conversationEngineEntry{engine: engine, expires: now.Add(conversationEngineTTL)}
	return true
}

// conversationEngine is the engine remembered for a conversation, "" when there is none or it expired.
func conversationEngine(conversationID string) string {
	if conversationID == "" {
		return ""
	}
	conversationEngines.mu.Lock()
	defer conversationEngines.mu.Unlock()
	e, ok := conversationEngines.m[conversationID]
	if !ok {
		return ""
	}
	if !e.expires.After(conversationEngines.now()) {
		delete(conversationEngines.m, conversationID)
		return ""
	}
	return e.engine
}

type fetchEngineCtxKey struct{}

// withFetchEngine makes fetchWithEngine use engine for the call this context belongs to ("" changes nothing).
func withFetchEngine(ctx context.Context, engine string) context.Context {
	if engine == "" {
		return ctx
	}
	return context.WithValue(ctx, fetchEngineCtxKey{}, engine)
}

func fetchEngineFrom(ctx context.Context) string {
	e, _ := ctx.Value(fetchEngineCtxKey{}).(string)
	return e
}

type conversationEngineRequest struct {
	ConversationID string `json:"conversationId"`
	Engine         string `json:"engine"`
}

// ConversationEngineHandler handles PUT /conversation-engine {conversationId, engine}: 204 when remembered, 400 for a bad id or engine.
func ConversationEngineHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body conversationEngineRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil || !setConversationEngine(body.ConversationID, body.Engine) {
		http.Error(w, `conversationId (letters, digits, - and _) and engine ("rod" or "katana") are required`, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
