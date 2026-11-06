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

// Plan 195, step 4: a link that needs a login with a login kept in the Browser tool is read by the engine's own method when the engine uses a
// kept login (Katana); every other login link keeps the AI instructions and the login window.
type keptFake struct{ *fake }

func (keptFake) UsesKeptLogin() bool { return true }

func TestListingMethodForLinkWithAKeptLogin(t *testing.T) {
	instruction.Reset()
	t.Cleanup(instruction.Reset)
	k := goodFake("katana")
	k.method = instruction.MethodLinks
	instruction.Register(keptFake{k})
	instruction.Register(goodFake("rod")) // Rod does not use a kept login
	cases := []struct {
		engine        string
		login, kept   bool
		want          string
		usesKeptLogin bool
	}{
		{"katana", true, true, instruction.MethodLinks, true},
		{"katana", true, false, instruction.MethodInstructions, true}, // no kept login: the window
		{"katana", false, false, instruction.MethodLinks, true},
		{"katana", false, true, instruction.MethodLinks, true}, // a kept login on a link that needs none changes nothing
		{"rod", true, true, instruction.MethodInstructions, false},
		{"", true, true, instruction.MethodInstructions, false},
		{"unknown", true, true, instruction.MethodInstructions, false},
	}
	for _, c := range cases {
		if got := instruction.ListingMethodForLink(c.engine, c.login, c.kept); got != c.want {
			t.Errorf("ListingMethodForLink(%q, login=%v, kept=%v) = %q, want %q", c.engine, c.login, c.kept, got, c.want)
		}
		if got := instruction.KeptLoginUsedBy(c.engine); got != c.usesKeptLogin {
			t.Errorf("KeptLoginUsedBy(%q) = %v", c.engine, got)
		}
	}
	// the old function never knew a kept login
	if got := instruction.ListingMethodFor("katana", true); got != instruction.MethodInstructions {
		t.Errorf("ListingMethodFor with a login link: %q", got)
	}
}
