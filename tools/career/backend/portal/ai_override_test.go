package portal

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"career-tool-backend/db"
)

func setupPortalDB(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatalf("InitDatabases: %v", err)
	}
	t.Cleanup(func() {
		db.JobsDB.Close()
		db.CareerDB.Close()
	})
}

func TestCleanAIOverride(t *testing.T) {
	cases := []struct {
		name    string
		in      AIOverride
		want    AIOverride
		invalid bool
	}{
		{"both empty clears", AIOverride{}, AIOverride{}, false},
		{"platform and model", AIOverride{"openrouter", "gpt"}, AIOverride{"openrouter", "gpt"}, false},
		{"platform without model", AIOverride{"openrouter", ""}, AIOverride{"openrouter", ""}, false},
		{"trimmed", AIOverride{"  openrouter ", " gpt\t"}, AIOverride{"openrouter", "gpt"}, false},
		{"model without platform", AIOverride{"", "gpt"}, AIOverride{}, true},
		{"blank platform with model", AIOverride{"   ", "gpt"}, AIOverride{}, true},
		{"platform 200 runes", AIOverride{strings.Repeat("a", 200), ""}, AIOverride{strings.Repeat("a", 200), ""}, false},
		{"platform 201 runes", AIOverride{strings.Repeat("a", 201), ""}, AIOverride{}, true},
		{"model 201 runes", AIOverride{"p", strings.Repeat("é", 201)}, AIOverride{}, true},
		{"NUL in platform", AIOverride{"a\x00b", ""}, AIOverride{}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := cleanAIOverride(c.in)
			if c.invalid {
				if !errors.Is(err, ErrInvalidAIOverride) {
					t.Fatalf("err = %v, want ErrInvalidAIOverride", err)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("got %+v, %v; want %+v", got, err, c.want)
			}
		})
	}
}

func listed(t *testing.T) (AIOverride, map[string]AIOverride) {
	t.Helper()
	portals, err := ListPortals()
	if err != nil {
		t.Fatal(err)
	}
	links := map[string]AIOverride{}
	var p AIOverride
	for _, po := range portals {
		p = AIOverride{po.AIPlatformID, po.AIModel}
		for _, l := range po.Links {
			links[l.ID] = AIOverride{l.AIPlatformID, l.AIModel}
		}
	}
	return p, links
}

func TestSetPortalAndLinkAI(t *testing.T) {
	setupPortalDB(t)
	pid, err := CreatePortal("Stepstone")
	if err != nil {
		t.Fatal(err)
	}
	l1, err := AddPortalLink(pid, "https://a.example", "A")
	if err != nil {
		t.Fatal(err)
	}
	l2, err := AddPortalLink(pid, "https://b.example", "B")
	if err != nil {
		t.Fatal(err)
	}

	// Untouched: no override anywhere.
	if p, links := listed(t); p != (AIOverride{}) || links[l1] != (AIOverride{}) || links[l2] != (AIOverride{}) {
		t.Fatalf("fresh rows carry an override: %+v %+v", p, links)
	}

	// Set the portal's, then one link's; they are independent.
	if err := SetPortalAI(pid, AIOverride{"openrouter", "m1"}); err != nil {
		t.Fatal(err)
	}
	if err := SetLinkAI(l1, AIOverride{"anthropic", "m2"}); err != nil {
		t.Fatal(err)
	}
	p, links := listed(t)
	if p != (AIOverride{"openrouter", "m1"}) || links[l1] != (AIOverride{"anthropic", "m2"}) || links[l2] != (AIOverride{}) {
		t.Fatalf("after set: %+v %+v", p, links)
	}

	// Replace, then clear.
	if err := SetLinkAI(l1, AIOverride{"openrouter", ""}); err != nil {
		t.Fatal(err)
	}
	if err := SetPortalAI(pid, AIOverride{}); err != nil {
		t.Fatal(err)
	}
	p, links = listed(t)
	if p != (AIOverride{}) || links[l1] != (AIOverride{"openrouter", ""}) {
		t.Fatalf("after replace/clear: %+v %+v", p, links)
	}
	if err := SetLinkAI(l1, AIOverride{}); err != nil {
		t.Fatal(err)
	}
	if _, links = listed(t); links[l1] != (AIOverride{}) {
		t.Fatalf("link not cleared: %+v", links)
	}

	// Other fields survive a change of the override.
	portals, _ := ListPortals()
	if portals[0].Name != "Stepstone" || len(portals[0].Links) != 2 || portals[0].Links[0].Title != "A" {
		t.Fatalf("other fields changed: %+v", portals[0])
	}
}

func TestSetAI_RefusesBadInputAndUnknownIds(t *testing.T) {
	setupPortalDB(t)
	pid, _ := CreatePortal("P")
	lid, _ := AddPortalLink(pid, "https://a.example", "")

	if err := SetPortalAI(pid, AIOverride{"", "m"}); !errors.Is(err, ErrInvalidAIOverride) {
		t.Fatalf("model without platform: %v", err)
	}
	if err := SetLinkAI(lid, AIOverride{"", "m"}); !errors.Is(err, ErrInvalidAIOverride) {
		t.Fatalf("link model without platform: %v", err)
	}
	if err := SetPortalAI("nope", AIOverride{"p", ""}); !errors.Is(err, ErrUnknownPortal) {
		t.Fatalf("unknown portal: %v", err)
	}
	if err := SetLinkAI("nope", AIOverride{"p", ""}); !errors.Is(err, ErrUnknownPortalLink) {
		t.Fatalf("unknown link: %v", err)
	}
	// A refused call changed nothing.
	if p, links := listed(t); p != (AIOverride{}) || links[lid] != (AIOverride{}) {
		t.Fatalf("refused call changed data: %+v %+v", p, links)
	}
}

// The override fields are always in the JSON, empty when unset, never missing: the
// frontend must not meet an absent field (the blank Portals page came from one).
func TestListPortals_JSONAlwaysCarriesOverrideFields(t *testing.T) {
	setupPortalDB(t)
	pid, _ := CreatePortal("P")
	if _, err := AddPortalLink(pid, "https://a.example", ""); err != nil {
		t.Fatal(err)
	}
	portals, err := ListPortals()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(portals)
	if err != nil {
		t.Fatal(err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	check := func(where string, m map[string]any) {
		for _, key := range []string{"aiPlatformId", "aiModel"} {
			v, ok := m[key]
			if !ok || v != "" {
				t.Errorf("%s: %s = %v (present=%v), want present and empty", where, key, v, ok)
			}
		}
	}
	check("portal", decoded[0])
	links := decoded[0]["links"].([]any)
	check("link", links[0].(map[string]any))
}
