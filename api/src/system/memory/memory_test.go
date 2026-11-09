package memory

import (
	"errors"
	"os"
	"testing"
)

const vmStatSample = `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                                5886.
Pages active:                            286402.
Pages inactive:                          284379.
Pages speculative:                         1763.
Pages throttled:                              0.
Pages wired down:                        153252.
Pages purgeable:                          10159.
`

func withMachine(t *testing.T, os_ string, readFile func(string) ([]byte, error), sysctl func(string) ([]byte, error)) {
	t.Helper()
	oldOS, oldRead, oldSysctl := goos, readOS, sysctlN
	t.Cleanup(func() { goos, readOS, sysctlN = oldOS, oldRead, oldSysctl })
	goos = os_
	if readFile != nil {
		readOS = readFile
	}
	if sysctl != nil {
		sysctlN = sysctl
	}
}

func withRunCmd(t *testing.T, f func(string, ...string) ([]byte, error)) {
	t.Helper()
	old := runCmd
	t.Cleanup(func() { runCmd = old })
	runCmd = f
}

func TestParseVMStatSumsTheUsablePagesTimesThePageSize(t *testing.T) {
	want := uint64(5886+284379+1763+10159) * 16384
	if got := parseVMStat([]byte(vmStatSample)); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got := parseVMStat([]byte("Pages free: 10.\n")); got != 0 {
		t.Errorf("no page size must give 0, got %d", got)
	}
	if got := parseVMStat([]byte("garbage")); got != 0 {
		t.Errorf("garbage must give 0, got %d", got)
	}
}

func TestParseSwapUsed(t *testing.T) {
	cases := map[string]uint64{
		"total = 2048.00M  used = 1130.50M  free = 917.50M  (encrypted)": uint64(1130.5 * (1 << 20)),
		"total = 3.00G  used = 1.50G  free = 1.50G  (encrypted)":         uint64(1.5 * (1 << 30)),
		"total = 0.00M  used = 0.00M  free = 0.00M  (encrypted)":         0,
		"total = 8K  used = 4K  free = 4K":                               4096,
		"used = 12 free = 1":                                             0,
		"nothing here":                                                   0,
		"used = lotsM":                                                   0,
	}
	for in, want := range cases {
		if got := parseSwapUsed(in); got != want {
			t.Errorf("%q: got %d, want %d", in, got, want)
		}
	}
}

func TestParsePressureLevel(t *testing.T) {
	for in, want := range map[string]Pressure{"1\n": PressureNormal, "2": PressureWarn, "4": PressureCritical, "3": PressureUnknown, "": PressureUnknown, "x": PressureUnknown} {
		if got := parsePressureLevel(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestParseMeminfoAvailable(t *testing.T) {
	raw := []byte("MemTotal: 8000000 kB\nMemAvailable: 3000000 kB\nSwapTotal: 2000000 kB\nSwapFree: 500000 kB\n")
	tot, a, s := parseMeminfo(raw)
	if tot != 8000000*1024 || a != 3000000*1024 || s != 1500000*1024 {
		t.Errorf("got %d %d", a, s)
	}
	_, a, s = parseMeminfo([]byte("SwapTotal: 100 kB\nSwapFree: 200 kB\n"))
	if a != 0 || s != 0 {
		t.Errorf("free above total must not underflow: %d %d", a, s)
	}
}

func TestMemoryOnAMac(t *testing.T) {
	withMachine(t, "darwin", nil, func(name string) ([]byte, error) {
		switch name {
		case "hw.memsize":
			return []byte("8589934592"), nil
		case "vm.swapusage":
			return []byte("total = 2048.00M  used = 1024.00M  free = 1024.00M"), nil
		case "kern.memorystatus_vm_pressure_level":
			return []byte("2"), nil
		}
		t.Errorf("unexpected sysctl %q", name)
		return nil, errors.New("no")
	})
	withRunCmd(t, func(path string, args ...string) ([]byte, error) {
		if path != "/usr/bin/vm_stat" {
			t.Errorf("ran %q", path)
		}
		return []byte(vmStatSample), nil
	})
	m := Memory()
	if m.TotalBytes != 8589934592 || m.AvailableBytes != uint64(5886+284379+1763+10159)*16384 || m.SwapUsedBytes != 1<<30 || m.Pressure != PressureWarn {
		t.Errorf("got %+v", m)
	}
}

func TestMemoryOnLinuxAndWhenNothingCanBeRead(t *testing.T) {
	withMachine(t, "linux", func(string) ([]byte, error) {
		return []byte("MemTotal: 8000000 kB\nMemAvailable: 9000000 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n"), nil
	}, nil)
	m := Memory()
	if m.TotalBytes != 8000000*1024 || m.AvailableBytes != m.TotalBytes || m.Pressure != PressureUnknown {
		t.Errorf("available must be capped at total: %+v", m)
	}
	withMachine(t, "darwin", nil, func(string) ([]byte, error) { return nil, errors.New("no") })
	withRunCmd(t, func(string, ...string) ([]byte, error) { return nil, errors.New("no") })
	if m := Memory(); m != (MemoryInfo{Pressure: PressureUnknown}) {
		t.Errorf("unreadable must be all zero and unknown: %+v", m)
	}
}

func TestResident(t *testing.T) {
	withMachine(t, "darwin", nil, nil)
	withRunCmd(t, func(path string, args ...string) ([]byte, error) {
		if path != "/bin/ps" || len(args) != 4 || args[3] != "42" {
			t.Errorf("ran %q %v", path, args)
		}
		return []byte("  2048\n"), nil
	})
	if got := Resident(42); got != 2048*1024 {
		t.Errorf("darwin: %d", got)
	}
	if Resident(0) != 0 || Resident(-3) != 0 {
		t.Error("a non-positive pid must give 0")
	}
	withRunCmd(t, func(string, ...string) ([]byte, error) { return []byte("gone"), nil })
	if Resident(42) != 0 {
		t.Error("garbage must give 0")
	}
	withMachine(t, "linux", func(path string) ([]byte, error) {
		if path != "/proc/42/statm" {
			t.Errorf("read %q", path)
		}
		return []byte("1000 250 10 1 0 5 0\n"), nil
	}, nil)
	if got := Resident(42); got != 250*uint64(os.Getpagesize()) {
		t.Errorf("linux: %d", got)
	}
	withMachine(t, "linux", func(string) ([]byte, error) { return []byte("1"), nil }, nil)
	if Resident(42) != 0 {
		t.Error("short statm must give 0")
	}
}
