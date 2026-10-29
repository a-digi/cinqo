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
