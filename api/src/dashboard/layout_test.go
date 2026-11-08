package dashboard

import (
	"reflect"
	"testing"
)

func defs(keys ...string) []Definition {
	out := make([]Definition, len(keys))
	for i, k := range keys {
		out[i] = statDef(k, "")
	}
	return out
}

func idsOf(p []Placed) []string {
	out := make([]string, len(p))
	for i, x := range p {
		out[i] = x.Definition.ID()
	}
	return out
}

func TestArrangeWithoutALayoutKeepsTheDefaultOrder(t *testing.T) {
	got := Arrange(defs("a", "b", "c"), nil)
	if want := []string{"test.a", "test.b", "test.c"}; !reflect.DeepEqual(idsOf(got), want) {
		t.Errorf("order = %v", idsOf(got))
	}
	for _, p := range got {
		if p.Hidden {
			t.Errorf("%s hidden by default", p.Definition.ID())
		}
	}
}

func TestArrangeAppliesSavedOrderAndHiddenFlags(t *testing.T) {
	got := Arrange(defs("a", "b", "c"), []LayoutEntry{{"test.c", false}, {"test.a", true}, {"test.b", false}})
	if want := []string{"test.c", "test.a", "test.b"}; !reflect.DeepEqual(idsOf(got), want) {
		t.Fatalf("order = %v", idsOf(got))
	}
	if got[0].Hidden || !got[1].Hidden || got[2].Hidden {
		t.Errorf("hidden flags = %v %v %v", got[0].Hidden, got[1].Hidden, got[2].Hidden)
	}
}

func TestArrangePutsNewWidgetsLastAndVisible(t *testing.T) {
	got := Arrange(defs("a", "b", "new1", "new2"), []LayoutEntry{{"test.b", true}, {"test.a", false}})
	if want := []string{"test.b", "test.a", "test.new1", "test.new2"}; !reflect.DeepEqual(idsOf(got), want) {
		t.Fatalf("order = %v", idsOf(got))
	}
	if got[2].Hidden || got[3].Hidden {
		t.Error("a widget the user has never arranged must be visible")
	}
}

func TestArrangeIgnoresStaleAndDuplicateEntries(t *testing.T) {
	got := Arrange(defs("a", "b"), []LayoutEntry{{"test.gone", true}, {"test.b", true}, {"test.b", false}, {"test.a", false}})
	if want := []string{"test.b", "test.a"}; !reflect.DeepEqual(idsOf(got), want) {
		t.Fatalf("order = %v", idsOf(got))
	}
	if !got[0].Hidden {
		t.Error("the first entry for a duplicate id must win")
	}
}

func TestSanitizeLayoutKeepsOnlyVisibleWidgetsOnce(t *testing.T) {
	got := SanitizeLayout(defs("a", "b"), []LayoutEntry{
		{"test.b", true}, {"test.secret", false}, {"test.a", false}, {"test.b", false}, {"", false},
	})
	want := []LayoutEntry{{"test.b", true}, {"test.a", false}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got := SanitizeLayout(defs("a"), nil); got == nil || len(got) != 0 {
		t.Errorf("empty input must give an empty, non-nil list: %#v", got)
	}
}
