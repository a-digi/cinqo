package openrouter

import (
	"testing"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// OpenRouter speaks the OpenAI chat shape; a system-role message must
// reach the wire first, with its content intact, whether or not the
// model supports cache-control parts.
func TestSystemRoleReachesTheWireFirst(t *testing.T) {
	for _, cache := range []bool{false, true} {
		wire := toWireMessages([]chatcompleter.Message{
			{Role: "system", Content: "be a composer"},
			{Role: "user", Content: "hi", CacheBreakpoint: true},
		}, cache)
		if len(wire) != 2 || wire[0].Role != "system" || wire[0].Content != "be a composer" {
			t.Fatalf("cache=%v: system message lost or reordered: %+v", cache, wire)
		}
	}
}
