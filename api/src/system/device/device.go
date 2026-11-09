// Package device says what the computer is: its model, chip, system and system version. The facts are static, so they are read once at
// start-up (in the background: system_profiler takes about a second) and kept. Only an allow-list of lines is read: no serial number, no
// hardware UUID, no user name, no host name, no file path. A fact that cannot be read is empty.
package device

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

// Info is what the computer is.
type Info struct {
	Model     string `json:"model"`   // "MacBook Air"
	ModelID   string `json:"modelId"` // "MacBookAir10,1"
	Chip      string `json:"chip"`    // "Apple M1"
	OS        string `json:"os"`      // "macOS"
	OSVersion string `json:"osVersion"`
	Arch      string `json:"arch"`
	Cores     int    `json:"cores"`
}

// The readers are variables so the tests can replace them.
var (
	goos    = runtime.GOOS
	goarch  = runtime.GOARCH
	numCPU  = runtime.NumCPU
	readOS  = os.ReadFile
	runProg = func(timeout time.Duration, path string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		// An absolute path with fixed arguments: nothing a caller sends reaches the operating system.
		return exec.CommandContext(ctx, path, args...).Output()
	}
)

// Read asks the computer. It never fails: what cannot be read stays empty.
func Read() Info {
	i := Info{Arch: goarch, Cores: numCPU()}
	switch goos {
	case "darwin":
		i.OS = "macOS"
		if out, err := runProg(2*time.Second, "/usr/bin/sw_vers", "-productVersion"); err == nil {
			i.OSVersion = strings.TrimSpace(string(out))
		}
		if out, err := runProg(5*time.Second, "/usr/sbin/system_profiler", "SPHardwareDataType"); err == nil {
			i.Model, i.ModelID, i.Chip = parseProfiler(out)
		}
		if i.ModelID == "" {
			if out, err := runProg(2*time.Second, "/usr/sbin/sysctl", "-n", "hw.model"); err == nil {
				i.ModelID = strings.TrimSpace(string(out))
			}
		}
		if i.Chip == "" {
			if out, err := runProg(2*time.Second, "/usr/sbin/sysctl", "-n", "machdep.cpu.brand_string"); err == nil {
				i.Chip = strings.TrimSpace(string(out))
			}
		}
	case "linux":
		i.OS, i.OSVersion = "Linux", ""
		if raw, err := readOS("/etc/os-release"); err == nil {
			i.OS, i.OSVersion = parseOSRelease(raw)
		}
		if raw, err := readOS("/proc/cpuinfo"); err == nil {
			i.Chip = parseCPUInfo(raw)
		}
		if raw, err := readOS("/sys/devices/virtual/dmi/id/product_name"); err == nil {
			i.Model = cleanValue(string(raw))
		}
	case "windows":
		i.OS = "Windows"
	}
	return i
}

// cleanValue keeps a printable one-line fact of at most 80 characters (a fact is shown as text, and nothing long or odd is kept).
func cleanValue(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if len([]rune(out)) > 80 {
		out = string([]rune(out)[:80])
	}
	return out
}

// parseProfiler reads only "Model Name", "Model Identifier" and "Chip" (or, on an Intel Mac, "Processor Name") from the text of
// `system_profiler SPHardwareDataType`. Every other line (serial number, hardware UUID, provisioning UDID) is never looked at.
func parseProfiler(raw []byte) (model, modelID, chip string) {
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		name, val, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(name) {
		case "Model Name":
			model = cleanValue(val)
		case "Model Identifier":
			modelID = cleanValue(val)
		case "Chip", "Processor Name":
			if chip == "" {
				chip = cleanValue(val)
			}
		}
	}
	return model, modelID, chip
}

// parseOSRelease returns the system name and version of /etc/os-release: NAME and VERSION_ID (PRETTY_NAME when there is no NAME).
func parseOSRelease(raw []byte) (name, version string) {
	var pretty string
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		switch k {
		case "NAME":
			name = cleanValue(v)
		case "VERSION_ID":
			version = cleanValue(v)
		case "PRETTY_NAME":
			pretty = cleanValue(v)
		}
	}
	if name == "" {
		name = pretty
	}
	if name == "" {
		name = "Linux"
	}
	return name, version
}

// parseCPUInfo returns the first "model name" of /proc/cpuinfo.
func parseCPUInfo(raw []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if ok && strings.TrimSpace(k) == "model name" {
			return cleanValue(v)
		}
	}
	return ""
}

var current atomic.Pointer[Info]

// Start reads the computer in the background and keeps the answer; Current is empty until it is done. Call it once at start-up.
func Start() {
	go func() {
		i := Read()
		current.Store(&i)
	}()
}

// Current is what Start found; empty (all facts unknown) while the reading is not done.
func Current() Info {
	if p := current.Load(); p != nil {
		return *p
	}
	return Info{}
}
