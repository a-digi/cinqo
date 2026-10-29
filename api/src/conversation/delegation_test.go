package conversation

import (
	"reflect"
	"testing"
)

func TestEffectiveDelegateTools(t *testing.T) {
	ceiling := []string{"extract_from_url", "save_portal_jobs", "spawn_subagent"}
	cases := []struct {
		name      string
		requested []string
		want      []string
	}{
		{"no request grants the ceiling minus built-ins", nil, []string{"extract_from_url", "save_portal_jobs"}},
		{"request narrows", []string{"save_portal_jobs"}, []string{"save_portal_jobs"}},
		{"request cannot widen", []string{"delete_job", "extract_from_url"}, []string{"extract_from_url"}},
		{"built-in never granted", []string{"spawn_subagent"}, []string{}},
		{"nothing grantable", []string{"delete_job"}, []string{}},
	}
	for _, c := range cases {
		got := effectiveDelegateTools(ceiling, c.requested)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	if got := effectiveDelegateTools(nil, nil); len(got) != 0 {
		t.Errorf("empty ceiling must grant nothing, got %v", got)
	}
}

func TestParseDelegateAllowedTools(t *testing.T) {
	if got := parseDelegateAllowedTools(`["a","b"]`); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("got %v", got)
	}
	if got := parseDelegateAllowedTools("not json"); got != nil {
		t.Fatalf("malformed must yield nil, got %v", got)
	}
}
