//go:build windows

package filetail

import "os"

// identityOf has no device and inode to use here: the cursor's check of the file's first bytes and of its size still notice a rotation.
func identityOf(st os.FileInfo) string { return "" }
