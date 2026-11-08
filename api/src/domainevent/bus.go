package domainevent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// listeners is package-level, mutex-protected state — no struct
// instance threaded through DI — the same idiom already established in
// this codebase by tool/manager.go's own process registry and
// conversation/runner.go's own cancel registry, for the same reason
// runner.go's own doc comment gives: api/config/di.ContextBag (a flat
// map[string]interface{}) has no mechanism for a long-lived service
// with its own lifecycle.
var (
	mu        sync.Mutex
	listeners = map[string][]func(context.Context, Event) error{}
)

// listenerTimeout bounds a single listener invocation
// (plan/ai/domain-events/step-02-async-dispatch.md). Short, since a
// native (in-process) listener should be fast — a tool-delivery
// listener (a later step) defines its own, longer constant, since a
// network call into a subprocess is inherently slower.
const listenerTimeout = 10 * time.Second

// logWarn is this package's own warning sink — a no-op until SetLogger
// wires it to the real logger at boot, so the package stays safe to use
// (Publish never panics on a nil func) even before that wiring runs.
// Matches this codebase's own established convention of background/
// package-level code taking a `warn func(format string, args ...any)`
// closure instead of importing coco-logger directly (tool/manager.go,
// conversation/runner.go) — implemented here as a package-level setter
// rather than a per-call parameter (the shape those two use) because
// Publish, unlike their own one-shot boot-time calls, runs continuously
// from many call sites; threading a logger through every single Publish
// call would be significant, avoidable friction for no benefit.
var logWarn = func(format string, args ...any) {}

// SetLogger wires this package's own warning output to the real
// logger. Meant to be called once, at boot, from cinqo.Start — the same
// place every other one-time package wiring in this codebase happens.
// A nil warn is ignored (keeps the safe no-op default) rather than
// panicking later.
func SetLogger(warn func(format string, args ...any)) {
	if warn == nil {
		return
	}
	logWarn = warn
}

// Subscribe registers handler to run whenever Publish is called with
// exactly this topic — no wildcard/prefix matching in this step (see
// plan/ai/domain-events/step-01-core-event-bus.md's own open
// questions). Meant to be called once, at boot, by whichever core
// package wants to react to an event — e.g. from cinqo.Start, the same
// place every other package-level registration in this codebase
// happens.
func Subscribe(topic string, handler func(ctx context.Context, evt Event) error) {
	mu.Lock()
	defer mu.Unlock()
	listeners[topic] = append(listeners[topic], handler)
}

// Publish marshals payload, builds the Event envelope with
// SourceKind "core"/SourceID "" (a core-native event), and dispatches
// it to every listener subscribed to topic. ctx bounds only Publish's
// own synchronous work above (payload marshaling); it is deliberately
// NOT propagated into listener dispatch — see publish's own doc
// comment.
func Publish(ctx context.Context, topic string, payload any) (Event, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Event{}, fmt.Errorf("marshal event payload for topic %q: %w", topic, err)
	}
	return publish(topic, raw, "core", ""), nil
}

// PublishFromTool is Publish's tool-originated counterpart
// (plan/ai/domain-events/step-05-tool-publish-endpoint.md), called only
// from domainevent/handler's own PublishHandler — never directly by
// other core code — once that handler has authenticated the calling
// tool via its own service_token and validated that topic starts with
// that tool's own slug. This function does NOT re-validate the
// namespace itself: toolSlug is trusted as already-established caller
// identity by the time this is called, exactly like mcpTool.ToolID is
// already-established by the time invokeToolCall's own scope check
// runs. payload is already-raw JSON here (the handler's own request
// body) — unlike Publish's `any`, which core code hands over
// unmarshaled — so there's nothing to re-marshal.
func PublishFromTool(topic string, payload json.RawMessage, toolSlug string) (Event, error) {
	return publish(topic, payload, "tool", toolSlug), nil
}

// publish is Publish's and PublishFromTool's shared core: build the
// envelope, dispatch to every matching native listener, and hand off to
// dispatchToTools (tool_delivery.go) for every matching tool listener.
//
// Dispatch is fire-and-forget: publish snapshots the matching native
// listener list, launches one independent goroutine per listener, and
// returns as soon as they're launched — it never waits for a single one
// to finish. Each dispatched goroutine roots its own bounded context in
// context.Background(), never the caller's own ctx (see dispatchOne) —
// mirroring conversation/runner.go's own explicit rationale for
// rooting detached work this way, so an event fired during a request
// isn't dropped just because that request's own client disconnects
// first.
func publish(topic string, raw json.RawMessage, sourceKind, sourceID string) Event {
	evt := Event{
		ID:         uuid.NewString(),
		Topic:      topic,
		SourceKind: sourceKind,
		SourceID:   sourceID,
		Payload:    raw,
		OccurredAt: time.Now().UTC(),
	}

	mu.Lock()
	handlers := append([]func(context.Context, Event) error(nil), listeners[topic]...)
	mu.Unlock()

	for _, h := range handlers {
		go dispatchOne(evt, h)
	}

	// Tool listeners (plan/ai/domain-events/step-04-core-to-tool-delivery.md)
	// — a manifest-declared, DB-cached counterpart to the in-process
	// listeners above, delivered over HTTP instead of a direct Go call.
	// See tool_delivery.go.
	dispatchToTools(evt)

	return evt
}

// dispatchOne runs a single listener, bounded and panic-safe — the
// guarantee this step adds. Verified directly that no existing
// background-execution code in this codebase (tool/manager.go's
// supervise, conversation/runner.go's runDetachedTurn) recovers from a
// panic; this is new, not a copied convention, added here because a
// later step turns "listener" into "an HTTP call to a tool process,"
// exactly the kind of unreliable operation this guards against.
func dispatchOne(evt Event, handler func(context.Context, Event) error) {
	defer func() {
		if r := recover(); r != nil {
			logWarn("domainevent: listener for topic %q panicked: %v", evt.Topic, r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), listenerTimeout)
	defer cancel()

	if err := handler(ctx, evt); err != nil {
		logWarn("domainevent: listener for topic %q returned an error: %v", evt.Topic, err)
	}
}
