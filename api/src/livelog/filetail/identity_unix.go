//go:build !windows

package filetail

import (
	"fmt"
	"os"
	"syscall"
)

// identityOf names the file itself (device and inode), so a log that was rotated, and a new file put at the same path, is noticed even
// when the new one is already longer than the old cursor.
func identityOf(st os.FileInfo) string {
	if sys, ok := st.Sys().(*syscall.Stat_t); ok {
		return fmt.Sprintf("%d.%d", sys.Dev, sys.Ino)
	}
	return ""
}
