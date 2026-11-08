package domainevent

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/a-digi/cinqo/src/tool/manager"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

// db is this package's own handle for looking up which tools declared
// an event_listeners entry for a given topic (tool_event_listeners,
// plan/ai/domain-events/step-03-tool-listener-manifest-and-cache.md) —
// set once, at boot, via SetDB, the same package-level-setter
// convention SetLogger (bus.go) already established. nil until then
// (e.g. in a unit test that never calls SetDB): dispatchToTools treats
// that as "no tool listeners to deliver to," not an error.
var db *sql.DB

// SetDB wires this package's own tool-listener lookup to the real
// database. Meant to be called once, at boot, from cinqo.Start.
func SetDB(database *sql.DB) {
	db = database
}

// toolDeliveryTimeout bounds a single HTTP delivery attempt to a tool —
// longer than listenerTimeout (bus.go, native listeners): a network
// call into a subprocess is inherently slower than an in-process call.
const toolDeliveryTimeout = 15 * time.Second

// toolDeliveryMaxRetries + toolDeliveryRetryBackoff absorb a tool
// mid-restart, without introducing a durable queue — real durability is
// plan/ai/domain-events/step-07-optional-durable-event-log.md,
// explicitly optional and not part of this step.
const (
	toolDeliveryMaxRetries   = 2
	toolDeliveryRetryBackoff = 500 * time.Millisecond
)

// dispatchToTools looks up every tool that declared an event_listeners
// entry for evt.Topic and launches one independent delivery goroutine
// per match. Mirrors dispatchOne's own snapshot-then-launch shape
// (bus.go) — the DB lookup itself runs synchronously (a fast local
// SQLite read, already filtered to enabled+running tools by
// FindByTopic), only the actual HTTP delivery is async.
func dispatchToTools(evt Event) {
	if db == nil {
		return
	}
	toolListeners, err := tool_query.NewToolEventListenerQueryRepo(db).FindByTopic(evt.Topic)
	if err != nil {
		logWarn("domainevent: failed to look up tool listeners for topic %q: %v", evt.Topic, err)
		return
	}
	for _, l := range toolListeners {
		go dispatchToTool(evt, l.ToolID, l.PathSuffix)
	}
}

// dispatchToTool delivers evt to one tool's own declared HTTP path,
// bounded and panic-safe like dispatchOne, plus a small fixed retry to
// absorb a tool mid-restart. Reuses the exact loopback trust boundary
// the existing /healthz probe already relies on (tool/manager.go) —
// core is the only realistic caller of a tool's own 127.0.0.1-bound
// HTTP server, so no additional auth is added for this direction. If
// every attempt fails, the delivery is logged and dropped — no
// dead-letter storage in this step.
func dispatchToTool(evt Event, toolID, pathSuffix string) {
	defer func() {
		if r := recover(); r != nil {
			logWarn("domainevent: tool delivery (tool %s, topic %q) panicked: %v", toolID, evt.Topic, r)
		}
	}()

	port, ok := manager.Port(toolID)
	if !ok {
		// A narrow race: FindByTopic already filtered to
		// status='running', but the tool could have stopped between
		// that read and this goroutine actually running. No retry
		// across "the tool is down" — see this step's own design doc.
		logWarn("domainevent: tool %s is not currently running, dropping delivery for topic %q", toolID, evt.Topic)
		return
	}

	body, err := json.Marshal(evt)
	if err != nil {
		logWarn("domainevent: failed to marshal event for tool delivery (tool %s, topic %q): %v", toolID, evt.Topic, err)
		return
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/%s", port, pathSuffix)

	var lastErr error
	for attempt := 0; attempt <= toolDeliveryMaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(toolDeliveryRetryBackoff)
		}
		if err := postEvent(url, body); err != nil {
			lastErr = err
			continue
		}
		return
	}
	logWarn("domainevent: delivery to tool %s (topic %q, path %q) failed after %d attempt(s): %v",
		toolID, evt.Topic, pathSuffix, toolDeliveryMaxRetries+1, lastErr)
}

// postEvent makes one bounded HTTP attempt, returning a non-nil error
// for a transport failure, a non-2xx response, or a timeout.
func postEvent(url string, body []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), toolDeliveryTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("tool responded with status %d", resp.StatusCode)
	}
	return nil
}
