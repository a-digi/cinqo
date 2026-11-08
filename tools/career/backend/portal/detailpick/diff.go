package detailpick

import "strings"

// DiffSummary says what a recrawl would change in a job's description (a line-based comparison).
type DiffSummary struct {
	Unchanged bool `json:"unchanged"`
	Kept      int  `json:"kept"`
	Added     int  `json:"added"`
	Removed   int  `json:"removed"`
	OldChars  int  `json:"oldChars"`
	NewChars  int  `json:"newChars"`
	// FirstAdded and FirstRemoved are the first few changed lines (each cut to 120 characters), in page order.
	FirstAdded   []string `json:"firstAdded"`
	FirstRemoved []string `json:"firstRemoved"`
}

const (
	maxDiffLines = 3000
	maxDiffShown = 5
	maxDiffCells = 4_000_000
)

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	if len(out) > maxDiffLines {
		out = out[:maxDiffLines]
	}
	return out
}

func clip(s string) string {
	r := []rune(s)
	if len(r) > 120 {
		return string(r[:120])
	}
	return s
}

// Diff compares an old and a new description by lines (blank lines and surrounding spaces ignored). It is exact (longest common
// subsequence) up to maxDiffCells and falls back to comparing the sets of lines beyond that, so its cost is bounded.
func Diff(oldText, newText string) DiffSummary {
	a, b := lines(oldText), lines(newText)
	d := DiffSummary{OldChars: len([]rune(oldText)), NewChars: len([]rune(newText)), FirstAdded: []string{}, FirstRemoved: []string{}}
	keptA := make([]bool, len(a))
	keptB := make([]bool, len(b))
	if len(a)*len(b) <= maxDiffCells {
		// longest common subsequence over lines
		w := len(b) + 1
		table := make([]int32, (len(a)+1)*w)
		for i := len(a) - 1; i >= 0; i-- {
			for j := len(b) - 1; j >= 0; j-- {
				if a[i] == b[j] {
					table[i*w+j] = table[(i+1)*w+j+1] + 1
				} else if table[(i+1)*w+j] >= table[i*w+j+1] {
					table[i*w+j] = table[(i+1)*w+j]
				} else {
					table[i*w+j] = table[i*w+j+1]
				}
			}
		}
		for i, j := 0, 0; i < len(a) && j < len(b); {
			switch {
			case a[i] == b[j]:
				keptA[i], keptB[j] = true, true
				i++
				j++
			case table[(i+1)*w+j] >= table[i*w+j+1]:
				i++
			default:
				j++
			}
		}
	} else {
		inB := map[string]int{}
		for _, l := range b {
			inB[l]++
		}
		for i, l := range a {
			if inB[l] > 0 {
				inB[l]--
				keptA[i] = true
			}
		}
		inA := map[string]int{}
		for i, l := range a {
			if keptA[i] {
				inA[l]++
			}
		}
		for j, l := range b {
			if inA[l] > 0 {
				inA[l]--
				keptB[j] = true
			}
		}
	}
	for i, l := range a {
		if keptA[i] {
			d.Kept++
		} else {
			d.Removed++
			if len(d.FirstRemoved) < maxDiffShown {
				d.FirstRemoved = append(d.FirstRemoved, clip(l))
			}
		}
	}
	for j, l := range b {
		if !keptB[j] {
			d.Added++
			if len(d.FirstAdded) < maxDiffShown {
				d.FirstAdded = append(d.FirstAdded, clip(l))
			}
		}
	}
	d.Unchanged = d.Added == 0 && d.Removed == 0
	return d
}
