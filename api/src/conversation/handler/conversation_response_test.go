package handler

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
)

// A restricted conversation says so, so a caller can check its restriction took effect; an
// unrestricted one carries no such field at all.
func TestConversationResponseEchoesTheAllowlist(t *testing.T) {
	restricted := toConversationResponse(&conversation_entity.Conversation{ID: "c1", AllowedTools: `["list_personas","get_persona_details"]`})
	if !reflect.DeepEqual(restricted.AllowedTools, []string{"list_personas", "get_persona_details"}) {
		t.Fatalf("restricted = %v", restricted.AllowedTools)
	}
	raw, _ := json.Marshal(restricted)
	if !strings.Contains(string(raw), `"allowedTools":["list_personas","get_persona_details"]`) {
		t.Fatalf("json = %s", raw)
	}

	open, _ := json.Marshal(toConversationResponse(&conversation_entity.Conversation{ID: "c2"}))
	if strings.Contains(string(open), "allowedTools") {
		t.Fatalf("an unrestricted conversation must not carry the field: %s", open)
	}
}
