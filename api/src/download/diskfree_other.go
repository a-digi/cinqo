//go:build !unix

package download

import "errors"

// diskFree is unknown on this system: the free-space check is then skipped.
func diskFree(string) (uint64, error) {
	return 0, errors.New("free disk space is not known on this system")
}
