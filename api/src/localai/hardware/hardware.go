// Package hardware reports what the machine can run: its system, its processor family, its memory and its cores. The
// numbers come from the operating system (never guessed), and a value that cannot be read is reported as unknown (0).
package hardware

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Info is what Detect found.
type Info struct {
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	RAMBytes uint64 `json:"ramBytes"` // 0 = unknown
	CPUs     int    `json:"cpus"`
}

// The readers are variables so the tests can replace them.
var (
	goos    = runtime.GOOS
	goarch  = runtime.GOARCH
	numCPU  = runtime.NumCPU
	readOS  = os.ReadFile
	sysctlN = func(name string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		// An absolute path: the program never looks this tool up on a search path.
		return exec.CommandContext(ctx, "/usr/sbin/sysctl", "-n", name).Output()
	}
)

// Detect reads the machine's system, architecture, memory and core count.
func Detect() Info {
	return Info{OS: goos, Arch: goarch, RAMBytes: totalRAM(), CPUs: numCPU()}
}

// Supported reports whether Cinqo's managed local AI is meant to run here: macOS on Apple Silicon and Linux on amd64
// (decision 7 of the plan). Anything else is "not supported yet".
func (i Info) Supported() bool {
	return (i.OS == "darwin" && i.Arch == "arm64") || (i.OS == "linux" && i.Arch == "amd64")
}

func totalRAM() uint64 {
	switch goos {
	case "darwin":
		out, err := sysctlN("hw.memsize")
		if err != nil {
			return 0
		}
		v, ok := parseBytes(string(out))
		if !ok {
			return 0
		}
		return v
	case "linux":
		raw, err := readOS("/proc/meminfo")
		if err != nil {
			return 0
		}
		v, _ := parseMeminfo(raw)
		return v
	}
	return 0
}

// parseBytes reads a plain unsigned number such as sysctl prints it.
func parseBytes(s string) (uint64, bool) {
	v, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	if err != nil || v == 0 {
		return 0, false
	}
	return v, true
}

// parseMeminfo finds "MemTotal:  16384000 kB" in the text of /proc/meminfo and returns it in bytes.
func parseMeminfo(raw []byte) (uint64, bool) {
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 3 && fields[0] == "MemTotal:" && fields[2] == "kB" {
			kb, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil || kb == 0 || kb > (1<<64-1)/1024 {
				return 0, false
			}
			return kb * 1024, true
		}
	}
	return 0, false
}
