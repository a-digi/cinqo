package detailpick

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

func TestDiff(t *testing.T) {
	d := Diff("a\nb\nc", "a\nb\nc")
	if !d.Unchanged || d.Kept != 3 || d.Added != 0 || d.Removed != 0 {
		t.Fatalf("same: %+v", d)
	}
	d = Diff("a\nb\nc", "a\nx\nc\nd")
	if d.Unchanged || d.Kept != 2 || d.Added != 2 || d.Removed != 1 || d.FirstAdded[0] != "x" || d.FirstAdded[1] != "d" || d.FirstRemoved[0] != "b" {
		t.Fatalf("changed: %+v", d)
	}
	d = Diff("  a  \n\n\nb", "a\nb")
	if !d.Unchanged {
		t.Errorf("blank lines and surrounding spaces do not count: %+v", d)
	}
	d = Diff("", "a\nb")
	if d.Added != 2 || d.Removed != 0 || d.OldChars != 0 || d.NewChars != 3 {
		t.Errorf("from nothing: %+v", d)
	}
	d = Diff("a\nb", "")
	if d.Removed != 2 || d.Added != 0 {
		t.Errorf("to nothing: %+v", d)
	}
	d = Diff("a\nb\nc", "c\nb\na")
	if d.Unchanged || d.Kept != 1 {
		t.Errorf("a reorder is a change (longest common subsequence): %+v", d)
	}
	d = Diff("x\nx\nx", "x")
	if d.Kept != 1 || d.Removed != 2 {
		t.Errorf("repeated lines are counted: %+v", d)
	}
}

func TestDiffShowsOnlyTheFirstFewLinesAndClipsThem(t *testing.T) {
	var a, b []string
	for i := 0; i < 20; i++ {
		b = append(b, fmt.Sprintf("new line %d %s", i, strings.Repeat("z", 200)))
	}
	d := Diff(strings.Join(a, "\n"), strings.Join(b, "\n"))
	if d.Added != 20 || len(d.FirstAdded) != maxDiffShown || len([]rune(d.FirstAdded[0])) != 120 {
		t.Errorf("%+v", d)
	}
}

func TestDiffIsBoundedOnHugeInputs(t *testing.T) {
	var a, b []string
	for i := 0; i < 5000; i++ {
		a = append(a, fmt.Sprintf("line %d", i))
		b = append(b, fmt.Sprintf("line %d", i+1))
	}
	d := Diff(strings.Join(a, "\n"), strings.Join(b, "\n")) // 3,000 lines each: over the exact limit, so the fallback runs
	if d.Kept == 0 || d.Kept+d.Removed != maxDiffLines || d.Kept+d.Added != maxDiffLines {
		t.Errorf("%+v", d)
	}
}

// Kept + Removed is the old line count, Kept + Added the new one, whatever the input.
func TestDiffCountsAddUp(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	pool := []string{"a", "b", "c", "d", "e", "", "  ", "Similar jobs", "x y z"}
	gen := func() string {
		var l []string
		for i := 0; i < r.Intn(30); i++ {
			l = append(l, pool[r.Intn(len(pool))])
		}
		return strings.Join(l, "\n")
	}
	for i := 0; i < 2000; i++ {
		a, b := gen(), gen()
		d := Diff(a, b)
		if d.Kept+d.Removed != len(lines(a)) || d.Kept+d.Added != len(lines(b)) {
			t.Fatalf("counts do not add up for %q -> %q: %+v", a, b, d)
		}
		if d.Unchanged != (strings.Join(lines(a), "\n") == strings.Join(lines(b), "\n")) {
			t.Fatalf("unchanged wrong for %q -> %q: %+v", a, b, d)
		}
	}
}

func FuzzDiff(f *testing.F) {
	f.Add("a\nb", "b\na")
	f.Add("", "\n\n")
	f.Fuzz(func(t *testing.T, a, b string) {
		d := Diff(a, b)
		if d.Kept < 0 || d.Added < 0 || d.Removed < 0 || d.Kept+d.Removed != len(lines(a)) || d.Kept+d.Added != len(lines(b)) {
			t.Fatalf("%+v", d)
		}
	})
}
