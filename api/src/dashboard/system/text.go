package system

// truncate shortens s to at most max characters, ending in "…" when it cut.
// Providers shorten user-supplied text themselves: the engine rejects a payload
// with an over-long field outright, which would blank the whole widget.
func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
