//go:build unix

package localai

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// setProcAttr puts the child in its own process group, so it and anything it starts can be stopped together.
func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// signalGroup sends sig to the whole process group led by pid (the child is its own group leader).
func signalGroup(pid int, sig syscall.Signal) error {
	if pid <= 1 {
		return errors.New("refusing to signal pid " + strconv.Itoa(pid))
	}
	return syscall.Kill(-pid, sig)
}

func terminateGroup(pid int) error { return signalGroup(pid, syscall.SIGTERM) }
func killGroup(pid int) error      { return signalGroup(pid, syscall.SIGKILL) }

// processAlive says whether a process with this pid exists.
func processAlive(pid int) bool {
	if pid <= 1 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// commandOf returns the command line of a process, read with ps by absolute path.
func commandOf(pid int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/bin/ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
