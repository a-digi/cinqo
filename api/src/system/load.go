package system

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// The readers are variables so the tests can replace them.
var (
	loadGoos   = runtime.GOOS
	loadCPUs   = runtime.NumCPU
	loadReadOS = os.ReadFile
	// An absolute path with a fixed argument: nothing the caller sends reaches the operating system.
	loadSysctl = func() ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, "/usr/sbin/sysctl", "-n", "vm.loadavg").Output()
	}
)

// readLoad returns the load average of the last minute and the number of cores; the load is -1 when the system does not say.
func readLoad() (load1 float64, cpus int) {
	cpus = loadCPUs()
	load1 = -1
	switch loadGoos {
	case "darwin":
		if out, err := loadSysctl(); err == nil {
			load1 = parseLoad(strings.Trim(strings.TrimSpace(string(out)), "{}"))
		}
	case "linux":
		if raw, err := loadReadOS("/proc/loadavg"); err == nil {
			load1 = parseLoad(string(raw))
		}
	}
	return load1, cpus
}

// parseLoad reads the first number of "1.52 1.80 1.90 ..." (both /proc/loadavg and sysctl's vm.loadavg without its braces); -1 when it is none.
func parseLoad(s string) float64 {
	f := strings.Fields(s)
	if len(f) == 0 {
		return -1
	}
	v, err := strconv.ParseFloat(f[0], 64)
	if err != nil || v < 0 {
		return -1
	}
	return v
}
