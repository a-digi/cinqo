package system

import (
	"errors"
	"testing"
)

func withLoad(t *testing.T, goos string, cpus int, readFile func(string) ([]byte, error), sysctl func() ([]byte, error)) {
	t.Helper()
	g, c, r, s := loadGoos, loadCPUs, loadReadOS, loadSysctl
	t.Cleanup(func() { loadGoos, loadCPUs, loadReadOS, loadSysctl = g, c, r, s })
	loadGoos, loadCPUs = goos, func() int { return cpus }
	if readFile != nil {
		loadReadOS = readFile
	}
	if sysctl != nil {
		loadSysctl = sysctl
	}
}

func TestParseLoad(t *testing.T) {
	for in, want := range map[string]float64{"1.52 1.80 1.90 2/345 6789": 1.52, "0.00 0.01 0.05": 0, "  3.25  ": 3.25, "": -1, "x": -1, "-1.5 1 1": -1} {
		if got := parseLoad(in); got != want {
			t.Errorf("%q: got %v, want %v", in, got, want)
		}
	}
}

func TestReadLoadOnAMacAndOnLinux(t *testing.T) {
	withLoad(t, "darwin", 8, nil, func() ([]byte, error) { return []byte("{ 2.50 1.80 1.90 }\n"), nil })
	if l, c := readLoad(); l != 2.5 || c != 8 {
		t.Errorf("darwin: %v %d", l, c)
	}
	withLoad(t, "linux", 4, func(p string) ([]byte, error) {
		if p != "/proc/loadavg" {
			t.Errorf("read %q", p)
		}
		return []byte("0.75 0.50 0.40 1/200 999\n"), nil
	}, nil)
	if l, c := readLoad(); l != 0.75 || c != 4 {
		t.Errorf("linux: %v %d", l, c)
	}
}

func TestAnUnreadableLoadIsMinusOneAndOtherSystemsToo(t *testing.T) {
	withLoad(t, "darwin", 8, nil, func() ([]byte, error) { return nil, errors.New("no") })
	if l, c := readLoad(); l != -1 || c != 8 {
		t.Errorf("failing sysctl: %v %d", l, c)
	}
	withLoad(t, "linux", 2, func(string) ([]byte, error) { return nil, errors.New("no") }, nil)
	if l, _ := readLoad(); l != -1 {
		t.Errorf("failing read: %v", l)
	}
	withLoad(t, "plan9", 1, nil, nil)
	if l, _ := readLoad(); l != -1 {
		t.Errorf("unknown system: %v", l)
	}
}

func TestDiskSpaceOfARealFolderAndOfNothing(t *testing.T) {
	if free, total := diskSpace(t.TempDir()); total == 0 || free > total {
		t.Errorf("free %d total %d", free, total)
	}
	if free, total := diskSpace(""); free != 0 || total != 0 {
		t.Error("no folder, no figures")
	}
	if free, total := diskSpace("/definitely/not/here"); free != 0 || total != 0 {
		t.Error("a missing folder, no figures")
	}
}
