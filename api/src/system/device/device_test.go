package device

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const profiler = `Hardware:

    Hardware Overview:

      Model Name: MacBook Air
      Model Identifier: MacBookAir10,1
      Model Number: Z1234LL/A
      Chip: Apple M1
      Total Number of Cores: 8 (4 performance and 4 efficiency)
      Memory: 8 GB
      System Firmware Version: 11881.1.1
      OS Loader Version: 11881.1.1
      Serial Number (system): C02XXXXXXXXX
      Hardware UUID: 11111111-2222-3333-4444-555555555555
      Provisioning UDID: 00008103-000000000000
      Activation Lock Status: Disabled
`

func withMachine(t *testing.T, goosV, arch string, cpus int, files map[string]string, progs map[string]string) {
	t.Helper()
	g, a, c, r, p := goos, goarch, numCPU, readOS, runProg
	t.Cleanup(func() { goos, goarch, numCPU, readOS, runProg = g, a, c, r, p })
	goos, goarch, numCPU = goosV, arch, func() int { return cpus }
	readOS = func(path string) ([]byte, error) {
		if v, ok := files[path]; ok {
			return []byte(v), nil
		}
		return nil, errors.New("no such file")
	}
	runProg = func(_ time.Duration, path string, args ...string) ([]byte, error) {
		if v, ok := progs[path+" "+strings.Join(args, " ")]; ok {
			return []byte(v), nil
		}
		return nil, errors.New("not available")
	}
}

func TestAMacIsReadFromTheAllowListOnly(t *testing.T) {
	withMachine(t, "darwin", "arm64", 8, nil, map[string]string{
		"/usr/bin/sw_vers -productVersion":             "15.1\n",
		"/usr/sbin/system_profiler SPHardwareDataType": profiler,
	})
	got := Read()
	want := Info{Model: "MacBook Air", ModelID: "MacBookAir10,1", Chip: "Apple M1", OS: "macOS", OSVersion: "15.1", Arch: "arm64", Cores: 8}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	for _, secret := range []string{"C02XXXXXXXXX", "11111111-2222", "00008103", "Z1234LL"} {
		if strings.Contains(strings.Join([]string{got.Model, got.ModelID, got.Chip, got.OS, got.OSVersion}, "|"), secret) {
			t.Errorf("%q must never be read", secret)
		}
	}
}

func TestAnIntelMacUsesTheProcessorName(t *testing.T) {
	m, id, chip := parseProfiler([]byte("Model Name: MacBook Pro\nModel Identifier: MacBookPro16,1\nProcessor Name: 8-Core Intel Core i9\nProcessor Speed: 2.3 GHz\n"))
	if m != "MacBook Pro" || id != "MacBookPro16,1" || chip != "8-Core Intel Core i9" {
		t.Errorf("%q %q %q", m, id, chip)
	}
	_, _, chip = parseProfiler([]byte("Chip: Apple M3\nProcessor Name: should not win\n"))
	if chip != "Apple M3" {
		t.Errorf("the first chip line wins, got %q", chip)
	}
}

func TestAMacWhoseProfilerFailsFallsBackToSysctl(t *testing.T) {
	withMachine(t, "darwin", "arm64", 8, nil, map[string]string{
		"/usr/sbin/sysctl -n hw.model":                 "Mac15,3\n",
		"/usr/sbin/sysctl -n machdep.cpu.brand_string": "Apple M3\n",
	})
	got := Read()
	if got.Model != "" || got.ModelID != "Mac15,3" || got.Chip != "Apple M3" || got.OSVersion != "" || got.OS != "macOS" {
		t.Errorf("%+v", got)
	}
}

func TestLinux(t *testing.T) {
	withMachine(t, "linux", "amd64", 4, map[string]string{
		"/etc/os-release": "PRETTY_NAME=\"Ubuntu 24.04 LTS\"\nNAME=\"Ubuntu\"\nVERSION_ID=\"24.04\"\n",
		"/proc/cpuinfo":   "processor\t: 0\nmodel name\t: Intel(R) Core(TM) i5-8250U\nmodel name\t: other\n",
		"/sys/devices/virtual/dmi/id/product_name": "ThinkPad T480\n",
	}, nil)
	got := Read()
	want := Info{Model: "ThinkPad T480", Chip: "Intel(R) Core(TM) i5-8250U", OS: "Ubuntu", OSVersion: "24.04", Arch: "amd64", Cores: 4}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	withMachine(t, "linux", "amd64", 2, nil, nil)
	if got := Read(); got.OS != "Linux" || got.OSVersion != "" || got.Chip != "" || got.Model != "" {
		t.Errorf("nothing readable: %+v", got)
	}
	n, v := parseOSRelease([]byte("PRETTY_NAME=Debian GNU/Linux 12\n"))
	if n != "Debian GNU/Linux 12" || v != "" {
		t.Errorf("pretty name only: %q %q", n, v)
	}
}

func TestOtherSystemsAndUnreadableMacsAreEmptyNotInvented(t *testing.T) {
	withMachine(t, "windows", "amd64", 6, nil, nil)
	if got := Read(); got != (Info{OS: "Windows", Arch: "amd64", Cores: 6}) {
		t.Errorf("%+v", got)
	}
	withMachine(t, "plan9", "386", 1, nil, nil)
	if got := Read(); got != (Info{Arch: "386", Cores: 1}) {
		t.Errorf("%+v", got)
	}
	withMachine(t, "darwin", "arm64", 8, nil, nil)
	if got := Read(); got != (Info{OS: "macOS", Arch: "arm64", Cores: 8}) {
		t.Errorf("%+v", got)
	}
}

func TestAFactIsOneShortPrintableLine(t *testing.T) {
	if got := cleanValue("  Mac\x00Book\x1b[31m  "); got != "MacBook[31m" {
		t.Errorf("control characters must go: %q", got)
	}
	if got := cleanValue(strings.Repeat("é", 200)); len([]rune(got)) != 80 {
		t.Errorf("at most 80 characters, got %d", len([]rune(got)))
	}
}

func TestStartKeepsTheAnswerAndCurrentIsEmptyBefore(t *testing.T) {
	current.Store(nil)
	if Current() != (Info{}) {
		t.Error("empty before the reading is done")
	}
	withMachine(t, "linux", "amd64", 2, map[string]string{"/etc/os-release": "NAME=Alpine\nVERSION_ID=3.20\n"}, nil)
	Start()
	deadline := time.Now().Add(2 * time.Second)
	for Current().OS == "" && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := Current(); got.OS != "Alpine" || got.OSVersion != "3.20" {
		t.Errorf("%+v", got)
	}
	current.Store(nil)
}

func TestTheEightyCharacterLimitIsExact(t *testing.T) {
	if got := cleanValue(strings.Repeat("a", 80)); len(got) != 80 {
		t.Errorf("80 characters are kept whole, got %d", len(got))
	}
	if got := cleanValue(strings.Repeat("a", 81)); len(got) != 80 {
		t.Errorf("81 characters are cut to 80, got %d", len(got))
	}
	if got := cleanValue("a\x7fb"); got != "ab" {
		t.Errorf("DEL must go: %q", got)
	}
}

func TestTheSysctlFallbackNeverOverridesWhatTheProfilerSaid(t *testing.T) {
	withMachine(t, "darwin", "arm64", 8, nil, map[string]string{
		"/usr/sbin/system_profiler SPHardwareDataType": profiler,
		"/usr/sbin/sysctl -n hw.model":                 "Other1,1\n",
		"/usr/sbin/sysctl -n machdep.cpu.brand_string": "Other chip\n",
	})
	if got := Read(); got.ModelID != "MacBookAir10,1" || got.Chip != "Apple M1" {
		t.Errorf("%+v", got)
	}
}

func TestALinuxWithoutANameIsJustLinux(t *testing.T) {
	if n, v := parseOSRelease([]byte("ID=foo\n")); n != "Linux" || v != "" {
		t.Errorf("%q %q", n, v)
	}
}
