package templates

import (
	"regexp"
	"strings"
	"testing"
)

// The order of the template picker (plan 254 and 255): the designs with two columns and a photo first, the ATS-friendly designs (meant to have no photo) last, every
// other design between them, each group in the registry order.

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

func TestTheDesignsAreInThreeGroupsAndEachGroupKeepsTheRegistryOrder(t *testing.T) {
	got := ListTemplates()
	first, last := len(twoColumnPhotoDesigns), 0
	for _, m := range metas {
		if m.Category == atsCategory {
			last++
		}
	}
	if first != 21 || last != 15 {
		t.Fatalf("%d two-column photo designs and %d ATS-friendly ones, want the 21 and 15 agreed after looking at every design", first, last)
	}
	middle := len(got) - first - last
	if middle != 74 {
		t.Fatalf("%d designs between the groups, want 74", middle)
	}
	var wantFirst, wantMiddle, wantLast []string
	for _, m := range metas {
		switch {
		case twoColumnPhotoDesigns[m.ID]:
			wantFirst = append(wantFirst, m.ID)
		case m.Category == atsCategory:
			wantLast = append(wantLast, m.ID)
		default:
			wantMiddle = append(wantMiddle, m.ID)
		}
	}
	ids := idsOf(got)
	for name, c := range map[string]struct{ got, want []string }{
		"the two-column photo designs":    {ids[:first], wantFirst},
		"the one-column designs":          {ids[first : first+middle], wantMiddle},
		"the ATS-friendly designs (last)": {ids[first+middle:], wantLast},
	} {
		if strings.Join(c.got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s are not where they belong or not in registry order:\n got  %v\n want %v", name, c.got, c.want)
		}
	}
	if ids[len(ids)-1] != wantLast[len(wantLast)-1] {
		t.Errorf("the last design is %s, want %s (the last ATS-friendly design of the registry)", ids[len(ids)-1], wantLast[len(wantLast)-1])
	}
}

// The ats-* designs are the ones meant to have no photo: the category and the intent must not drift apart.
func TestTheAtsFriendlyGroupIsExactlyTheAtsDesigns(t *testing.T) {
	for _, m := range metas {
		if isAts := strings.HasPrefix(m.ID, "ats-"); isAts != (m.Category == atsCategory) {
			t.Errorf("%s: id starts with ats- = %v, category %q", m.ID, isAts, m.Category)
		}
	}
	got := idsOf(ListTemplates())
	for i, id := range got[len(got)-15:] {
		if !strings.HasPrefix(id, "ats-") {
			t.Errorf("position %d from the end is %s, not an ats design", 15-i, id)
		}
	}
	for _, id := range got[:len(got)-15] {
		if strings.HasPrefix(id, "ats-") {
			t.Errorf("%s is not among the last 15", id)
		}
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

func TestThePartitionNeverDropsOrDoublesADesignAndRanksThemByGroup(t *testing.T) {
	m := func(id, cat string) TemplateMeta { return TemplateMeta{ID: id, Category: cat} }
	all := []TemplateMeta{m("ats1", "ATS-Friendly"), m("a", "Modern"), m("two1", "Modern"), m("ats2", "ATS-Friendly"), m("b", "Classic"), m("two2", "ATS-Friendly"), m("c", "Something new")}
	for _, c := range []struct {
		twoColumn map[string]bool
		want      string
	}{
		{nil, "a,two1,b,c,ats1,ats2,two2"},
		{map[string]bool{"two1": true, "two2": true}, "two1,two2,a,b,c,ats1,ats2"},
		{map[string]bool{"c": true}, "c,a,two1,b,ats1,ats2,two2"},
		{map[string]bool{"unknown": true}, "a,two1,b,c,ats1,ats2,two2"},
	} {
		if got := strings.Join(idsOf(partitionForPicker(all, c.twoColumn)), ","); got != c.want {
			t.Errorf("two-column %v: %s, want %s", c.twoColumn, got, c.want)
		}
	}
	if len(partitionForPicker(nil, nil)) != 0 {
		t.Error("an empty list stays empty")
	}
	// a design that is two-column and ATS-friendly is in the first group, one with an unknown category is in the middle
	if r := pickerRank(m("x", "ATS-Friendly"), map[string]bool{"x": true}); r != 0 {
		t.Errorf("rank = %d, want 0", r)
	}
	if r := pickerRank(m("y", "Brand new"), nil); r != 1 {
		t.Errorf("rank = %d, want 1", r)
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
