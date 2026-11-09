// Package memory reads the memory of the computer right now (total, available, swap, the system's pressure word) and the resident
// memory of one process. Every number comes from the operating system; a value that cannot be read is 0 / unknown, never guessed.
// It stands outside the local AI module on purpose: the resource bar of the shell uses it, and the module may use it too.
package memory

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Pressure is the operating system's own word for how tight the memory is. Unknown when the system does not say (Linux, or a
// reader that failed): the callers then judge by the numbers and never raise an alarm from "unknown".
type Pressure string

const (
	PressureUnknown  Pressure = "unknown"
	PressureNormal   Pressure = "normal"
	PressureWarn     Pressure = "warn"
	PressureCritical Pressure = "critical"
)

// MemoryInfo is the memory of the machine right now. A value that cannot be read is 0 (never guessed).
type MemoryInfo struct {
	TotalBytes     uint64
	AvailableBytes uint64 // what a program can get without pushing others out: free + cached + purgeable
	SwapUsedBytes  uint64
	Pressure       Pressure
}

// The readers are variables so the tests can replace them.
var (
	goos   = runtime.GOOS
	readOS = os.ReadFile
	// An absolute path: the program never looks this tool up on a search path.
	sysctlN = func(name string) ([]byte, error) { return runCmd("/usr/sbin/sysctl", "-n", name) }
)

// runCmd runs a program by absolute path with fixed arguments; a variable so the tests can replace it.
var runCmd = func(path string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, path, args...).Output()
}

// Memory reads the memory now. It never fails: what cannot be read stays 0 / unknown.
func Memory() MemoryInfo {
	m := MemoryInfo{Pressure: PressureUnknown}
	switch goos {
	case "darwin":
		if out, err := sysctlN("hw.memsize"); err == nil {
			m.TotalBytes, _ = strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
		}
		if out, err := runCmd("/usr/bin/vm_stat"); err == nil {
			m.AvailableBytes = parseVMStat(out)
		}
		if out, err := sysctlN("vm.swapusage"); err == nil {
			m.SwapUsedBytes = parseSwapUsed(string(out))
		}
		if out, err := sysctlN("kern.memorystatus_vm_pressure_level"); err == nil {
			m.Pressure = parsePressureLevel(string(out))
		}
	case "linux":
		if raw, err := readOS("/proc/meminfo"); err == nil {
			m.TotalBytes, m.AvailableBytes, m.SwapUsedBytes = parseMeminfo(raw)
		}
	}
	if m.TotalBytes > 0 && m.AvailableBytes > m.TotalBytes {
		m.AvailableBytes = m.TotalBytes
	}
	return m
}

// parseVMStat sums the pages a program can still use: free, inactive, purgeable and speculative, times the page size the
// header names ("page size of 16384 bytes"). 0 when the page size or every counter is missing.
func parseVMStat(raw []byte) uint64 {
	var pageSize, pages uint64
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		line := sc.Text()
		if strings.Contains(line, "page size of") {
			f := strings.Fields(line)
			for i, w := range f {
				if w == "of" && i+1 < len(f) {
					pageSize, _ = strconv.ParseUint(f[i+1], 10, 64)
				}
			}
			continue
		}
		name, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(name) {
		case "Pages free", "Pages inactive", "Pages purgeable", "Pages speculative":
			n, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(val), "."), 10, 64)
			if err == nil {
				pages += n
			}
		}
	}
	if pageSize == 0 {
		return 0
	}
	return pages * pageSize
}

// parseSwapUsed reads "total = 2048.00M  used = 1130.50M  free = 917.50M  (encrypted)" and returns used in bytes.
func parseSwapUsed(s string) uint64 {
	f := strings.Fields(s)
	for i, w := range f {
		if w != "used" || i+2 >= len(f) || f[i+1] != "=" {
			continue
		}
		v := f[i+2]
		mult := float64(1)
		switch {
		case strings.HasSuffix(v, "K"):
			mult = 1 << 10
		case strings.HasSuffix(v, "M"):
			mult = 1 << 20
		case strings.HasSuffix(v, "G"):
			mult = 1 << 30
		default:
			return 0
		}
		n, err := strconv.ParseFloat(strings.TrimSuffix(v, v[len(v)-1:]), 64)
		if err != nil || n < 0 {
			return 0
		}
		return uint64(n * mult)
	}
	return 0
}

// parsePressureLevel maps macOS kern.memorystatus_vm_pressure_level (1 normal, 2 warning, 4 critical).
func parsePressureLevel(s string) Pressure {
	switch strings.TrimSpace(s) {
	case "1":
		return PressureNormal
	case "2":
		return PressureWarn
	case "4":
		return PressureCritical
	}
	return PressureUnknown
}

// parseMeminfo returns MemTotal, MemAvailable and the swap in use (SwapTotal - SwapFree), in bytes.
func parseMeminfo(raw []byte) (total, available, swapUsed uint64) {
	var swapTotal, swapFree uint64
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 3 || f[2] != "kB" {
			continue
		}
		kb, err := strconv.ParseUint(f[1], 10, 64)
		if err != nil || kb > (1<<64-1)/1024 {
			continue
		}
		switch f[0] {
		case "MemTotal:":
			total = kb * 1024
		case "MemAvailable:":
			available = kb * 1024
		case "SwapTotal:":
			swapTotal = kb * 1024
		case "SwapFree:":
			swapFree = kb * 1024
		}
	}
	if swapTotal > swapFree {
		swapUsed = swapTotal - swapFree
	}
	return total, available, swapUsed
}

// Resident is the resident memory of a process of this computer, in bytes; 0 when it cannot be read (the process is gone, or the
// system does not say). pid must be one of Cinqo's own processes: the callers never take it from a request.
func Resident(pid int) uint64 {
	if pid <= 0 {
		return 0
	}
	switch goos {
	case "darwin":
		out, err := runCmd("/bin/ps", "-o", "rss=", "-p", strconv.Itoa(pid))
		if err != nil {
			return 0
		}
		kb, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
		if err != nil {
			return 0
		}
		return kb * 1024
	case "linux":
		raw, err := readOS(fmt.Sprintf("/proc/%d/statm", pid))
		if err != nil {
			return 0
		}
		f := strings.Fields(string(raw))
		if len(f) < 2 {
			return 0
		}
		pages, err := strconv.ParseUint(f[1], 10, 64)
		if err != nil {
			return 0
		}
		return pages * uint64(os.Getpagesize())
	}
	return 0
}
