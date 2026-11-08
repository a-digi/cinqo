package livelog_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/livelog"
	"github.com/a-digi/cinqo/src/livelog/livelogtest"
)

func prov(id, title string, scopes ...string) *livelogtest.Provider {
	return &livelogtest.Provider{IDValue: id, TitleValue: title, ScopeList: scopes}
}

func TestRegisterRefusesWhatWouldFailLaterAtARequest(t *testing.T) {
	r := livelog.NewRegistry()
	if err := r.Register(prov("ai-trace", "AI trace logs", "cinqo:super:admin")); err != nil {
		t.Fatalf("a valid provider: %v", err)
	}
	for name, p := range map[string]livelog.Provider{
		"duplicate id":       prov("ai-trace", "again", "x"),
		"empty id":           prov("", "t", "x"),
		"upper case":         prov("AiTrace", "t", "x"),
		"space":              prov("ai trace", "t", "x"),
		"slash":              prov("ai/trace", "t", "x"),
		"leading digit":      prov("1log", "t", "x"),
		"double hyphen":      prov("a--b", "t", "x"),
		"trailing hyphen":    prov("a-", "t", "x"),
		"too long":           prov(strings.Repeat("a", 33), "t", "x"),
		"no title":           prov("no-title", "", "x"),
		"no scope":           prov("no-scope", "t"),
		"nil provider":       nil,
		"nil typed provider": livelog.Provider(nil),
	} {
		if err := r.Register(p); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if err := r.Register(prov(strings.Repeat("a", 32), "t", "x")); err != nil {
		t.Errorf("a 32 character id is the longest allowed: %v", err)
	}
	if err := r.Register(prov("a1-b2", "t", "x")); err != nil {
		t.Errorf("digits and single hyphens are fine: %v", err)
	}
}

func TestGetFindsByIdOnly(t *testing.T) {
	r := livelog.NewRegistry()
	p := prov("ai-trace", "AI", "s")
	_ = r.Register(p)
	if got, ok := r.Get("ai-trace"); !ok || got != livelog.Provider(p) {
		t.Fatalf("Get = %v %v", got, ok)
	}
	for _, id := range []string{"", "AI-TRACE", "ai-trace ", "other"} {
		if _, ok := r.Get(id); ok {
			t.Errorf("Get(%q) found something", id)
		}
	}
}

func TestAllowedByScopeAndAdminAlways(t *testing.T) {
	p := prov("sys", "System", "tool:x:read", "tool:y:read")
	for _, c := range []struct {
		scopes []string
		want   bool
	}{
		{nil, false},
		{[]string{}, false},
		{[]string{"cinqo:conversation:use"}, false},
		{[]string{"tool:x:read"}, true},
		{[]string{"a", "tool:y:read"}, true},
		{[]string{livelog.AdminScope}, true},
		{[]string{"tool:x:rea"}, false},
		{[]string{"TOOL:X:READ"}, false},
	} {
		if got := livelog.Allowed(p, c.scopes); got != c.want {
			t.Errorf("Allowed(%v) = %v, want %v", c.scopes, got, c.want)
		}
	}
}

func TestListShowsOnlyWhatTheCallerMayReadSortedAndNeverNull(t *testing.T) {
	r := livelog.NewRegistry()
	_ = r.Register(prov("zeta", "Z", "tool:z:read"))
	_ = r.Register(prov("alpha", "A", livelog.AdminScope))
	_ = r.Register(prov("mid", "M", "tool:m:read"))
	if got := r.List([]string{livelog.AdminScope}); !reflect.DeepEqual(got, []livelog.Info{{ID: "alpha", Title: "A"}, {ID: "mid", Title: "M"}, {ID: "zeta", Title: "Z"}}) {
		t.Errorf("admin: %v", got)
	}
	if got := r.List([]string{"tool:m:read", "tool:z:read"}); !reflect.DeepEqual(got, []livelog.Info{{ID: "mid", Title: "M"}, {ID: "zeta", Title: "Z"}}) {
		t.Errorf("scoped: %v", got)
	}
	got := r.List([]string{"nothing"})
	if got == nil || len(got) != 0 {
		t.Errorf("no access must be an empty, non-nil list: %#v", got)
	}
	if got := livelog.NewRegistry().List(nil); got == nil || len(got) != 0 {
		t.Errorf("an empty registry must give an empty, non-nil list: %#v", got)
	}
}
