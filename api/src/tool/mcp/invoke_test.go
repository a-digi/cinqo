package mcp

import "testing"

func TestParseDelegate(t *testing.T) {
	good := map[string]any{"delegate": map[string]any{
		"message": "go",
		"tasks":   []any{map[string]any{"task": "a", "allowedTools": []any{"x"}, "maxAttempts": float64(2)}},
	}}
	d := parseDelegate(good)
	if d == nil || len(d.Tasks) != 1 || d.Tasks[0].Task != "a" || d.Tasks[0].AllowedTools[0] != "x" || d.Tasks[0].MaxAttempts != 2 || d.Message != "go" {
		t.Fatalf("unexpected: %+v", d)
	}
	for name, in := range map[string]any{
		"nil":       nil,
		"no key":    map[string]any{"other": 1},
		"no tasks":  map[string]any{"delegate": map[string]any{"tasks": []any{}}},
		"wrong typ": map[string]any{"delegate": "yes"},
	} {
		if parseDelegate(in) != nil {
			t.Errorf("%s: expected nil", name)
		}
	}
}
