package instruction_test

import (
	"testing"

	"career-tool-backend/instruction"
)

// The cases of the old portal.ListingMethodFor test (plan 190), now through the registry with a Katana-like and a Rod-like engine.
func TestListingMethodFor(t *testing.T) {
	instruction.Reset()
	t.Cleanup(instruction.Reset)
	k := goodFake("katana")
	k.method = instruction.MethodLinks
	instruction.Register(k)
	instruction.Register(goodFake("rod"))
	cases := []struct {
		engine string
		login  bool
		want   string
	}{
		{"katana", false, instruction.MethodLinks},
		{" katana ", false, instruction.MethodLinks}, // surrounding space is not a different engine
		{"rod", false, instruction.MethodInstructions},
		{"", false, instruction.MethodInstructions},         // an older Browser sends none
		{"Katana", false, instruction.MethodInstructions},   // exact, not case-folded
		{"selenium", false, instruction.MethodInstructions}, // any other name
		{"katana", true, instruction.MethodInstructions},    // a login link is always read with its AI instructions
		{"rod", true, instruction.MethodInstructions},
	}
	for _, c := range cases {
		if got := instruction.ListingMethodFor(c.engine, c.login); got != c.want {
			t.Errorf("ListingMethodFor(%q, login=%v) = %q, want %q", c.engine, c.login, got, c.want)
		}
	}
	instruction.Reset() // a removed Katana: its name falls back to the AI instructions
	if got := instruction.ListingMethodFor("katana", false); got != instruction.MethodInstructions {
		t.Errorf("unregistered engine: %q", got)
	}
}
