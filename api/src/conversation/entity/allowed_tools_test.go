package entity

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseAllowedTools(t *testing.T) {
	// Omitted means no restriction.
	if got, err := ParseAllowedTools(nil); got != nil || err != nil {
		t.Fatalf("nil: %v, %v", got, err)
	}
	// Trimmed, de-duplicated, first order kept.
	got, err := ParseAllowedTools([]string{" list_personas ", "get_persona_details", "list_personas", "list_profiles"})
	if err != nil || !reflect.DeepEqual(got, []string{"list_personas", "get_persona_details", "list_profiles"}) {
		t.Fatalf("got %v, %v", got, err)
	}
	tooMany := make([]string, MaxAllowedTools+1)
	for i := range tooMany {
		tooMany[i] = "tool_" + strings.Repeat("x", i%5) + string(rune('a'+i%26))
	}
	for name, raw := range map[string][]string{
		"empty list": {},
		"blank":      {"list_personas", "  "},
		"too long":   {strings.Repeat("x", 129)},
		"too many":   tooMany,
	} {
		if _, err := ParseAllowedTools(raw); !errors.Is(err, ErrInvalidAllowedTools) {
			t.Errorf("%s: %v, want ErrInvalidAllowedTools", name, err)
		}
	}
	// The limit itself is fine.
	max := make([]string, MaxAllowedTools)
	for i := range max {
		max[i] = strings.Repeat("t", 1+i%7) + string(rune('A'+i%26)) + strings.Repeat("_", i/26)
	}
	if got, err := ParseAllowedTools(max); err != nil || len(got) == 0 {
		t.Errorf("at the limit: %v, %v", len(got), err)
	}
}

func TestAllowedToolsRoundTripAndFailClosed(t *testing.T) {
	enc, err := EncodeAllowedTools([]string{"a", "b"})
	if err != nil || enc != `["a","b"]` {
		t.Fatalf("encode = %q, %v", enc, err)
	}
	if enc, _ := EncodeAllowedTools(nil); enc != "" {
		t.Fatalf("no list encodes to %q, want empty (NULL)", enc)
	}
	c := &Conversation{AllowedTools: enc}
	if got := c.AllowedToolNames(); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("names = %v", got)
	}
	// No list, and a missing conversation: no restriction (nil).
	if (&Conversation{}).AllowedToolNames() != nil || (*Conversation)(nil).AllowedToolNames() != nil {
		t.Fatal("no allowlist must read as nil (unrestricted)")
	}
	// A stored value that cannot be read must restrict to NOTHING, never to everything.
	for _, bad := range []string{"not json", `{"a":1}`, "null", `"a"`, `[1,2]`} {
		got := (&Conversation{AllowedTools: bad}).AllowedToolNames()
		if got == nil || len(got) != 0 {
			t.Errorf("%q read as %v, want an empty, non-nil list (fail closed)", bad, got)
		}
	}
}
