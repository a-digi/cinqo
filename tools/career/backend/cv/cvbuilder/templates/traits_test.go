package templates

import (
	"regexp"
	"strings"
	"testing"
)

// The order of the template picker (plan 254): the designs with two columns and a photo first, every other design after them, each group in the registry order.

func idsOf(list []TemplateMeta) []string {
	out := make([]string, len(list))
	for i, m := range list {
		out[i] = m.ID
	}
	return out
}

func TestTheListHasEveryDesignOnceInTheNewOrder(t *testing.T) {
	got := ListTemplates()
	if len(got) != len(metas) || len(got) < 100 {
		t.Fatalf("%d designs, want the %d of the registry", len(got), len(metas))
	}
	seen := map[string]int{}
	for _, m := range got {
		seen[m.ID]++
	}
	for _, m := range metas {
		if seen[m.ID] != 1 {
			t.Errorf("%s appears %d times, want once", m.ID, seen[m.ID])
		}
	}
}

func TestTheTwoColumnPhotoDesignsComeFirstAndEachGroupKeepsTheRegistryOrder(t *testing.T) {
	got := idsOf(ListTemplates())
	first := len(twoColumnPhotoDesigns)
	if first != 21 {
		t.Fatalf("%d designs in the first group, want the 21 agreed after looking at every design", first)
	}
	for i, id := range got {
		if inFirst := twoColumnPhotoDesigns[id]; inFirst != (i < first) {
			t.Errorf("position %d is %s: in the first group = %v, but the first group is positions 0 to %d", i, id, inFirst, first-1)
		}
	}
	var wantFirst, wantRest []string
	for _, m := range metas {
		if twoColumnPhotoDesigns[m.ID] {
			wantFirst = append(wantFirst, m.ID)
		} else {
			wantRest = append(wantRest, m.ID)
		}
	}
	if strings.Join(got[:first], ",") != strings.Join(wantFirst, ",") {
		t.Errorf("the first group is not in registry order:\n got  %v\n want %v", got[:first], wantFirst)
	}
	if strings.Join(got[first:], ",") != strings.Join(wantRest, ",") {
		t.Errorf("the other designs moved: they must keep the registry order")
	}
}

func TestTheOrderIsTheSameEveryTimeAndACallerCannotChangeIt(t *testing.T) {
	a := ListTemplates()
	b := ListTemplates()
	if strings.Join(idsOf(a), ",") != strings.Join(idsOf(b), ",") {
		t.Fatal("two calls gave two orders")
	}
	a[0], a[len(a)-1] = a[len(a)-1], a[0]
	a[1].ID = "changed"
	c := ListTemplates()
	if strings.Join(idsOf(c), ",") != strings.Join(idsOf(b), ",") {
		t.Error("a caller reordered or renamed the shared list")
	}
}

func TestTheDesignsOfTheFirstGroupExistAndShowThePhoto(t *testing.T) {
	for id := range twoColumnPhotoDesigns {
		if !IsKnownTemplate(id) {
			t.Errorf("%s is in the first group but is not a design", id)
			continue
		}
		html, err := templateFS.ReadFile(id + "/template.html")
		if err != nil {
			t.Errorf("%s: %v", id, err)
			continue
		}
		if !strings.Contains(string(html), "{{.PhotoDataURI}}") {
			t.Errorf("%s is in the first group but its template has no place for the photo", id)
		}
	}
}

// A design that has a sidebar in its markup has two columns, so it cannot be forgotten in the first group.
var secondColumn = regexp.MustCompile(`class="(sidebar|aside)"`)

func TestEveryDesignWithASecondColumnIsInTheFirstGroup(t *testing.T) {
	var found int
	for _, m := range metas {
		html, err := templateFS.ReadFile(m.ID + "/template.html")
		if err != nil {
			t.Fatal(err)
		}
		has := secondColumn.Match(html)
		if has {
			found++
		}
		if has && !twoColumnPhotoDesigns[m.ID] {
			t.Errorf("%s has a sidebar or an aside in its markup but is not in the first group", m.ID)
		}
		if !has && twoColumnPhotoDesigns[m.ID] {
			t.Errorf("%s is in the first group but has no sidebar or aside in its markup", m.ID)
		}
	}
	if found != 21 {
		t.Errorf("%d designs have a second column, want 21", found)
	}
}

func TestThePartitionNeverDropsOrDoublesADesignWhateverIsInTheFirstGroup(t *testing.T) {
	all := []TemplateMeta{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}}
	for _, c := range []struct {
		first map[string]bool
		want  string
	}{
		{nil, "a,b,c,d"},
		{map[string]bool{"c": true}, "c,a,b,d"},
		{map[string]bool{"d": true, "b": true}, "b,d,a,c"},
		{map[string]bool{"a": true, "b": true, "c": true, "d": true}, "a,b,c,d"},
		{map[string]bool{"unknown": true}, "a,b,c,d"},
	} {
		if got := strings.Join(idsOf(partitionTwoColumnFirst(all, c.first)), ","); got != c.want {
			t.Errorf("first %v: %s, want %s", c.first, got, c.want)
		}
	}
	if len(partitionTwoColumnFirst(nil, nil)) != 0 {
		t.Error("an empty list stays empty")
	}
}

func TestTheFirstPageOfThePickerHoldsTwelveOfTheTwoColumnDesigns(t *testing.T) {
	got := ListTemplates()[:12]
	for _, m := range got {
		if !twoColumnPhotoDesigns[m.ID] {
			t.Errorf("%s is on the first page of the picker (12 designs) but is not a two-column photo design", m.ID)
		}
	}
	if got[0].ID != "modern-sidebar" {
		t.Errorf("the first design is %s, want modern-sidebar (the first of the group in the registry)", got[0].ID)
	}
}
