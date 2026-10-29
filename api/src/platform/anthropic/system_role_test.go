package anthropic

import (
	"testing"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// Anthropic has no system role inside messages: it must be lifted into
// the top-level system field, and never left among the turns.
func TestSystemRoleIsLiftedIntoTheSystemField(t *testing.T) {
	system, turns := toAnthropicMessages([]chatcompleter.Message{
		{Role: "system", Content: "be a composer"},
		{Role: "user", Content: "hi"},
	})
	if system != "be a composer" {
		t.Fatalf("system = %q", system)
	}
	if len(turns) != 1 || turns[0].Role != "user" {
		t.Fatalf("turns must hold only the user message: %+v", turns)
	}
}
