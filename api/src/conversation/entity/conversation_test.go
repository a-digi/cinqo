package entity

import "testing"

func TestParseOrchestrationMode(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"", OrchestrationModeDirect, true},
		{"direct", OrchestrationModeDirect, true},
		{"composer", OrchestrationModeComposer, true},
		{"Composer", "", false},
		{" composer", "", false},
		{"swarm", "", false},
	}
	for _, c := range cases {
		got, ok := ParseOrchestrationMode(c.in)
		if got != c.want || ok != c.wantOK {
			t.Errorf("ParseOrchestrationMode(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

func TestValidToolIterationLimit(t *testing.T) {
	ptr := func(n int) *int { return &n }
	for name, c := range map[string]struct {
		in   *int
		want bool
	}{
		"absent":   {nil, true},
		"one":      {ptr(1), true},
		"ten":      {ptr(10), true},
		"ceiling":  {ptr(MaxToolIterationLimit), true},
		"zero":     {ptr(0), false},
		"negative": {ptr(-1), false},
		"above":    {ptr(MaxToolIterationLimit + 1), false},
	} {
		if got := ValidToolIterationLimit(c.in); got != c.want {
			t.Errorf("%s: got %v, want %v", name, got, c.want)
		}
	}
}
