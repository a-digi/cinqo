//go:build !unix

package system

// diskSpace is not read on this system: 0, 0 (unknown).
func diskSpace(string) (free, total uint64) { return 0, 0 }
