//go:build unix

package system

import "syscall"

// diskSpace is the free and the total space of the volume a folder is on, in bytes; 0, 0 when it cannot be read.
func diskSpace(dir string) (free, total uint64) {
	if dir == "" {
		return 0, 0
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, 0
	}
	bs := uint64(st.Bsize) //nolint:gosec // a block size is never negative
	return uint64(st.Bavail) * bs, uint64(st.Blocks) * bs
}
