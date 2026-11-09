//go:build unix

package runs

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Alive says whether a process with this id exists (signal 0 asks only that; EPERM means it exists but is someone else's).
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// StartTime is the operating system's start time of a process as one comparable string (`ps -o lstart=`), "" when it cannot be read. With the id
// it tells a process from another that was given the same id later.
func StartTime(pid int) string {
	if pid <= 0 {
		return ""
	}
	for _, ps := range []string{"/bin/ps", "/usr/bin/ps"} {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		out, err := exec.CommandContext(ctx, ps, "-o", "lstart=", "-p", strconv.Itoa(pid)).Output()
		cancel()
		if err == nil {
			return strings.Join(strings.Fields(string(out)), " ")
		}
	}
	return ""
}
