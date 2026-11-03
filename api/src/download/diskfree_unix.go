//go:build unix

package download

import (
	"errors"
	"syscall"
)

// diskFree is the number of bytes a normal user can still write in the directory's file system.
func diskFree(dir string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, err
	}
	if st.Bsize <= 0 {
		return 0, errors.New("unknown block size")
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}
