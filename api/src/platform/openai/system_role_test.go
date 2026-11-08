package openai

import (
	"encoding/json"
	"testing"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// The composer orchestrator and its workers lead with a system-role
// message; OpenAI's chat completions accept that role natively, so it
// must reach the wire as the first message, unchanged.
func TestSystemRoleReachesTheWireFirst(t *testing.T) {
	wire := toWireMessages([]chatcompleter.Message{
		{Role: "system", Content: "be a composer"},
		{Role: "user", Content: "hi"},
	})
	raw, _ := json.Marshal(wire)
	if len(wire) != 2 || wire[0].Role != "system" || wire[0].Content != "be a composer" {
		t.Fatalf("system message lost or reordered: %s", raw)
	}
}
