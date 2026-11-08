package detailpick

import (
	"strings"
	"unicode"
)

// norm lower-cases s and reduces it to words: letters and digits, single spaces.
func norm(s string) string {
	var b strings.Builder
	space := true
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
			continue
		}
		if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

func words(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(norm(s)) {
		out[w] = true
	}
	return out
}

// overlap is |A∩B| / min(|A|,|B|) over the words of a and b (0 when either has none): how much of the shorter text the longer one says.
func overlap(a, b string) float64 {
	wa, wb := words(a), words(b)
	if len(wa) == 0 || len(wb) == 0 {
		return 0
	}
	n := 0
	for w := range wa {
		if wb[w] {
			n++
		}
	}
	m := len(wa)
	if len(wb) < m {
		m = len(wb)
	}
	return float64(n) / float64(m)
}

// agrees reports whether two texts say the same thing: equal, or one contains the other (at least 5 letters or digits).
func agrees(a, b string) bool {
	na, nb := norm(a), norm(b)
	if na == "" || nb == "" {
		return false
	}
	if na == nb {
		return true
	}
	if len(na) >= 5 && strings.Contains(nb, na) {
		return true
	}
	return len(nb) >= 5 && strings.Contains(na, nb)
}

func runeLen(s string) int { return len([]rune(s)) }
