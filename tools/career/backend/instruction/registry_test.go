package instruction_test

import (
	"strings"
	"testing"

	"career-tool-backend/instruction"
	"career-tool-backend/instruction/enginetest"
)

func TestRegistry(t *testing.T) {
	instruction.Reset()
	t.Cleanup(instruction.Reset)
	instruction.Register(goodFake("b"))
	instruction.Register(goodFake("a"))
	all := instruction.All()
	if len(all) != 2 || all[0].ID() != "a" || all[1].ID() != "b" {
		t.Fatalf("All() not sorted by id: %v", all)
	}
	if _, ok := instruction.ForBrowserEngine("a"); !ok {
		t.Error("a missing")
	}
	if _, ok := instruction.ForBrowserEngine("gone"); ok {
		t.Error("unknown engine must not resolve")
	}
}

func TestRegisterPanics(t *testing.T) {
	instruction.Reset()
	t.Cleanup(instruction.Reset)
	instruction.Register(goodFake("a"))
	for name, fn := range map[string]func(){
		"nil":       func() { instruction.Register(nil) },
		"duplicate": func() { instruction.Register(goodFake("a")) },
		"bad id":    func() { instruction.Register(goodFake("Bad-Id")) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: want panic", name)
				}
			}()
			fn()
		}()
	}
}

func TestContractAcceptsGoodEngine(t *testing.T) { enginetest.Run(t, goodFake("good")) }

func TestContractCatchesBadEngines(t *testing.T) {
	cases := map[string]struct {
		mutate func(*fake)
		want   string
	}{
		"unprefixed new tool": {func(f *fake) { f.tools[0].Name = "other_one"; f.register = []string{"other_one"} }, "must start with"},
		"no scope":            {func(f *fake) { f.tools[0].Scope = "" }, "no scope"},
		"unregistered tool":   {func(f *fake) { f.register = nil }, "does not register"},
		"undeclared tool":     {func(f *fake) { f.register = append(f.register, "bad_extra") }, "does not declare"},
		"bad policy":          {func(f *fake) { f.written = "nobody" }, "WrittenBy"},
		"sql semicolon":       {func(f *fake) { f.sql = "x=1;" }, "ListingReadySQL"},
		"empty sql":           {func(f *fake) { f.sql = "" }, "ListingReadySQL"},
		"bad topic":           {func(f *fake) { f.events[0].Topic = "nope" }, "event topic"},
		"consumed no suffix":  {func(f *fake) { f.events = []instruction.EventSpec{{Topic: "career.a.b", Role: instruction.Consumes}} }, "no path suffix"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := goodFake("good")
			c.mutate(f)
			got := strings.Join(enginetest.Check(f), "\n")
			if !strings.Contains(got, c.want) {
				t.Errorf("want a problem containing %q, got %q", c.want, got)
			}
		})
	}
	t.Run("legacy tool may be unprefixed", func(t *testing.T) {
		f := goodFake("good")
		f.tools[0] = instruction.ToolSpec{Name: "old_tool", Scope: "career:write", Legacy: true}
		f.register = []string{"old_tool"}
		if p := enginetest.Check(f); len(p) != 0 {
			t.Errorf("unexpected: %v", p)
		}
	})
}

func TestCheckRegistry(t *testing.T) {
	a, b := goodFake("a"), goodFake("b")
	if p := enginetest.CheckRegistry([]instruction.Engine{a, b}); len(p) != 0 {
		t.Errorf("unexpected: %v", p)
	}
	b.tools[0].Name = "a_one"
	if p := enginetest.CheckRegistry([]instruction.Engine{a, b}); len(p) == 0 {
		t.Error("shared tool name not caught")
	}
}
