//go:build !unix

package runs

// Without these readers every earlier open run counts as dead (nothing can say otherwise).
func Alive(int) bool       { return false }
func StartTime(int) string { return "" }
