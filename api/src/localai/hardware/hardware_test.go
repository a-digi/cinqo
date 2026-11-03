package hardware

import (
	"errors"
	"os"
	"testing"
)

func withMachine(t *testing.T, os_, arch string, cpus int, readFile func(string) ([]byte, error), sysctl func(string) ([]byte, error)) {
	t.Helper()
	oldOS, oldArch, oldCPU, oldRead, oldSysctl := goos, goarch, numCPU, readOS, sysctlN
	t.Cleanup(func() { goos, goarch, numCPU, readOS, sysctlN = oldOS, oldArch, oldCPU, oldRead, oldSysctl })
	goos, goarch = os_, arch
	numCPU = func() int { return cpus }
	if readFile != nil {
		readOS = readFile
	}
	if sysctl != nil {
		sysctlN = sysctl
	}
}

func TestDetectOnAMac(t *testing.T) {
	withMachine(t, "darwin", "arm64", 8, nil, func(name string) ([]byte, error) {
		if name != "hw.memsize" {
			t.Errorf("asked for %q, want hw.memsize", name)
		}
		return []byte("17179869184\n"), nil
	})
	got := Detect()
	if got.OS != "darwin" || got.Arch != "arm64" || got.RAMBytes != 17179869184 || got.CPUs != 8 {
		t.Errorf("got %+v", got)
	}
	if !got.Supported() {
		t.Error("Apple Silicon is supported")
	}
}

func TestDetectOnLinux(t *testing.T) {
	withMachine(t, "linux", "amd64", 4, func(path string) ([]byte, error) {
		if path != "/proc/meminfo" {
			t.Errorf("read %q", path)
		}
		return []byte("MemTotal:        8000000 kB\nMemFree:         100 kB\n"), nil
	}, nil)
	got := Detect()
	if got.RAMBytes != 8000000*1024 || got.CPUs != 4 || !got.Supported() {
		t.Errorf("got %+v", got)
	}
}

func TestAMemoryThatCannotBeReadIsUnknownNotInvented(t *testing.T) {
	withMachine(t, "darwin", "arm64", 8, nil, func(string) ([]byte, error) { return nil, errors.New("no sysctl") })
	if got := Detect().RAMBytes; got != 0 {
		t.Errorf("darwin, sysctl failing: %d, want 0", got)
	}
	withMachine(t, "darwin", "arm64", 8, nil, func(string) ([]byte, error) { return []byte("lots"), nil })
	if got := Detect().RAMBytes; got != 0 {
		t.Errorf("darwin, garbage: %d, want 0", got)
	}
	withMachine(t, "linux", "amd64", 8, func(string) ([]byte, error) { return nil, os.ErrNotExist }, nil)
	if got := Detect().RAMBytes; got != 0 {
		t.Errorf("linux, no /proc/meminfo: %d, want 0", got)
	}
	withMachine(t, "windows", "amd64", 8, nil, nil)
	if got := Detect().RAMBytes; got != 0 {
		t.Errorf("an unsupported system: %d, want 0", got)
	}
}

func TestSupported(t *testing.T) {
	for _, c := range []struct {
		os, arch string
		want     bool
	}{
		{"darwin", "arm64", true}, {"linux", "amd64", true},
		{"darwin", "amd64", false}, {"linux", "arm64", false}, {"windows", "amd64", false}, {"freebsd", "amd64", false}, {"", "", false},
	} {
		if got := (Info{OS: c.os, Arch: c.arch}).Supported(); got != c.want {
			t.Errorf("%s/%s: %v, want %v", c.os, c.arch, got, c.want)
		}
	}
}

func TestParseMeminfo(t *testing.T) {
	ok := map[string]uint64{
		"MemTotal:       16384000 kB\n":          16384000 * 1024,
		"Foo: 1 kB\nMemTotal: 2 kB\nBar: 3 kB\n": 2048,
		"MemTotal:\t4 kB":                        4096,
	}
	for in, want := range ok {
		if got, found := parseMeminfo([]byte(in)); !found || got != want {
			t.Errorf("%q: %d, %v, want %d", in, got, found, want)
		}
	}
	for _, in := range []string{"", "MemFree: 5 kB\n", "MemTotal: 5 MB\n", "MemTotal: x kB\n", "MemTotal: 0 kB\n", "MemTotal: 99999999999999999999 kB\n", "MemTotal: 18014398509481984 kB\n", "MemTotal:5 kB\n", "MemTotal: 5\n"} {
		if got, found := parseMeminfo([]byte(in)); found || got != 0 {
			t.Errorf("%q: %d, %v, want not found", in, got, found)
		}
	}
}

func TestParseBytes(t *testing.T) {
	if v, ok := parseBytes(" 17179869184\n"); !ok || v != 17179869184 {
		t.Errorf("%d %v", v, ok)
	}
	for _, in := range []string{"", "0", "-1", "1.5", "abc", "18446744073709551616"} {
		if v, ok := parseBytes(in); ok || v != 0 {
			t.Errorf("%q: %d %v, want not ok", in, v, ok)
		}
	}
}

// The worked example: Phi-4-mini's file is 2,491,874,272 bytes, so the rule asks for 2,491,874,272 x 5/4 (integer
// division, 3,114,842,840) plus 3 GiB (3,221,225,472) = 6,336,068,312 bytes of memory.
func TestMinRAMBytesWorkedExample(t *testing.T) {
	if got := MinRAMBytes(2491874272); got != 6336068312 {
		t.Errorf("got %d, want 6336068312", got)
	}
	if got := MinRAMBytes(1117320736); got != 1117320736*5/4+3<<30 {
		t.Errorf("got %d", got)
	}
	if MinRAMBytes(0) != 0 || MinRAMBytes(-1) != 0 {
		t.Error("an unknown size needs nothing")
	}
}

func TestFitsAtTheExactBoundary(t *testing.T) {
	need := MinRAMBytes(2491874272)
	cases := []struct {
		ram  uint64
		want *bool
		name string
	}{
		{need, ptr(true), "exactly enough"},
		{need + 1, ptr(true), "one more"},
		{need - 1, ptr(false), "one byte short"},
		{0, nil, "unknown memory"},
	}
	for _, c := range cases {
		got := (Info{RAMBytes: c.ram}).Fits(2491874272)
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("%s: %v, want %v", c.name, deref(got), deref(c.want))
		}
	}
}

func ptr(b bool) *bool { return &b }
func deref(b *bool) any {
	if b == nil {
		return nil
	}
	return *b
}
