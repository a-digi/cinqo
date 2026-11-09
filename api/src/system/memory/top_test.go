package memory

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// A slice of a real macOS table (ps -axww -o pid=,ppid=,rss=,comm=), with the nested helpers, a command line program, a login shell and a defunct entry.
const macTable = `    1     0  12000 /sbin/launchd
 4231   831 575136 claude
 4606     1 270016 node
 1276     1  98000 /Applications/Visual Studio Code.app/Contents/MacOS/Electron
 1278  1276 392096 /Applications/Visual Studio Code.app/Contents/Frameworks/Code Helper (Renderer).app/Contents/MacOS/Code Helper (Renderer)
 1279  1276 150000 /Applications/Visual Studio Code.app/Contents/Frameworks/Code Helper (GPU).app/Contents/MacOS/Code Helper (GPU)
60866     1 314720 /Applications/Google Chrome.app/Contents/MacOS/Google Chrome
60870 60866 100000 /Applications/Google Chrome.app/Contents/Frameworks/Google Chrome Framework.framework/Versions/157.0.8103.39/Helpers/Google Chrome Helper (Renderer).app/Contents/MacOS/Google Chrome Helper (Renderer)
60871 60866  90000 /Applications/Google Chrome.app/Contents/Frameworks/Google Chrome Framework.framework/Versions/157.0.8103.39/Helpers/Google Chrome Helper.app/Contents/MacOS/Google Chrome Helper
  587     1 268192 /opt/jc/bin/jumpcloud-agent
  700   699   2048 -zsh
  701   699      0 (kernel_task)
57512   831  60000 ./app/cinqo-pro
57600 57512  45000 /Users/andet/Desktop/Development/playground/go/cinqo/app/data/tools/browser/backend/tool
57700 57600  30000 /Users/andet/Library/Application Support/Cinqo/local-ai/runtime/llama-server
`

func TestParseTheRealTable(t *testing.T) {
	rows := ParseProcessTable([]byte(macTable))
	if len(rows) != 15 {
		t.Fatalf("%d rows, want 15", len(rows))
	}
	if r := rows[1]; r.PID != 4231 || r.PPID != 831 || r.RSSKB != 575136 || r.Command != "claude" {
		t.Errorf("%+v", r)
	}
	if r := rows[4]; r.PID != 1278 || !strings.HasSuffix(r.Command, "Code Helper (Renderer)") || !strings.Contains(r.Command, "Visual Studio Code.app") {
		t.Errorf("a command with spaces is kept whole: %+v", r)
	}
	if r := rows[14]; r.Command != "/Users/andet/Library/Application Support/Cinqo/local-ai/runtime/llama-server" {
		t.Errorf("%+v", r)
	}
}

func TestParseSkipsLinesThatDoNotFit(t *testing.T) {
	raw := "garbage\n\n  12   1   x /bin/a\n  13   1  100\n  -5   1  100 /bin/neg\n   0   1  100 /bin/zero\n  14   1  100 /bin/ok\n abc  1 100 /bin/c\n  15 x 100 /bin/d\n"
	rows := ParseProcessTable([]byte(raw))
	if len(rows) != 1 || rows[0].PID != 14 || rows[0].Command != "/bin/ok" {
		t.Errorf("%+v", rows)
	}
	if got := ParseProcessTable(nil); got != nil {
		t.Errorf("%v", got)
	}
}

func TestApplicationNames(t *testing.T) {
	cases := []struct {
		cmd  string
		pid  int
		want string
	}{
		{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", 1, "Google Chrome"},
		{"/Applications/Google Chrome.app/Contents/Frameworks/Google Chrome Framework.framework/Versions/157/Helpers/Google Chrome Helper (Renderer).app/Contents/MacOS/Google Chrome Helper (Renderer)", 1, "Google Chrome"},
		{"/Applications/Visual Studio Code.app/Contents/Frameworks/Code Helper (GPU).app/Contents/MacOS/Code Helper (GPU)", 1, "Visual Studio Code"},
		{"/System/Applications/Utilities/Activity Monitor.app", 1, "Activity Monitor"},
		{"claude", 1, "claude"},
		{"node", 1, "node"},
		{"/opt/jc/bin/jumpcloud-agent", 1, "jumpcloud-agent"},
		{"/usr/libexec/trustd", 1, "trustd"},
		{"-zsh", 1, "zsh"},
		{"(kernel_task)", 1, "kernel_task"},
		{"( spaced )", 1, "spaced"},
		{"Code Helper (Renderer)", 1, "Code Helper (Renderer)"}, // parentheses inside a name are kept
		{"/usr/bin/my.application/run", 1, "run"},               // ".app" inside a longer folder name is not a bundle
		{"/tmp/x.app-data/y", 1, "y"},
		{"/tmp/x.app", 1, "x"},
		{"/Users/me/Downloads/Cool Tool.app/Contents/MacOS/cool", 1, "Cool Tool"},
		{"/a/Outer.app/Contents/Inner.app/Contents/MacOS/inner", 1, "Outer"},
		{"", 42, "process 42"},
		{"   ", 43, "process 43"},
		{"1234", 44, "process 44"},
		{"/", 45, "process 45"},
		{"/bin/\x00bad\x07name\n", 1, "badname"},
		{"/bin/Café Ärger ✓", 1, "Café Ärger ✓"},
		{"/bin/" + strings.Repeat("x", 200), 1, strings.Repeat("x", 80)},
		{"/bin/" + strings.Repeat("é", 100), 1, strings.Repeat("é", 80)},
	}
	for _, c := range cases {
		if got := AppName(c.cmd, c.pid); got != c.want {
			t.Errorf("%q: got %q, want %q", c.cmd, got, c.want)
		}
	}
}

func byName(apps []App) map[string]App {
	m := map[string]App{}
	for _, a := range apps {
		m[a.Name] = a
	}
	return m
}

func TestTopAppsGroupsSumsAndPicksTheBiggestProcessAsThePID(t *testing.T) {
	apps := TopApps(ParseProcessTable([]byte(macTable)), 57512, 10)
	m := byName(apps)
	chrome := m["Google Chrome"]
	if chrome.Processes != 3 || chrome.RSSBytes != (314720+100000+90000)*1024 || chrome.PID != 60866 || chrome.Cinqo {
		t.Errorf("chrome %+v", chrome)
	}
	code := m["Visual Studio Code"]
	if code.Processes != 3 || code.RSSBytes != (98000+392096+150000)*1024 || code.PID != 1278 {
		t.Errorf("code %+v", code)
	}
	if c := m["claude"]; c.Processes != 1 || c.PID != 4231 || c.RSSBytes != 575136*1024 {
		t.Errorf("claude %+v", c)
	}
	if _, ok := m["kernel_task"]; ok {
		t.Error("a process with no resident memory is left out")
	}
}

func TestTopAppsMergesCinqosFamilyIntoOneRow(t *testing.T) {
	apps := TopApps(ParseProcessTable([]byte(macTable)), 57512, 10)
	m := byName(apps)
	c := m[CinqoName]
	if !c.Cinqo || c.Processes != 3 || c.RSSBytes != (60000+45000+30000)*1024 || c.PID != 57512 {
		t.Errorf("%+v", c)
	}
	if _, ok := m["tool"]; ok {
		t.Error("the tool must be inside the Cinqo row")
	}
	if _, ok := m["llama-server"]; ok {
		t.Error("the model program must be inside the Cinqo row")
	}
	// a program that merely is called Cinqo but is not below this one stays a separate row
	rows := append(ParseProcessTable([]byte(macTable)), ProcRow{PID: 9000, PPID: 1, RSSKB: 500, Command: "/Applications/Cinqo.app/Contents/MacOS/Cinqo"})
	got := TopApps(rows, 57512, 20)
	n := 0
	for _, a := range got {
		if a.Name == CinqoName {
			n++
		}
	}
	if n != 2 {
		t.Errorf("an outside program named Cinqo is its own row: %+v", got)
	}
}

func TestTopAppsOrderTiesAndLimit(t *testing.T) {
	rows := []ProcRow{
		{PID: 10, PPID: 1, RSSKB: 100, Command: "/bin/bravo"},
		{PID: 11, PPID: 1, RSSKB: 100, Command: "/bin/alpha"},
		{PID: 12, PPID: 1, RSSKB: 300, Command: "/bin/big"},
		{PID: 13, PPID: 1, RSSKB: 50, Command: "/bin/small"},
	}
	got := TopApps(rows, 0, 3)
	names := []string{}
	for _, a := range got {
		names = append(names, a.Name)
	}
	if strings.Join(names, ",") != "big,alpha,bravo" {
		t.Errorf("%v: biggest first, ties by name, cut to n", names)
	}
	if len(TopApps(rows, 0, 10)) != 4 || len(TopApps(rows, 0, 1)) != 1 {
		t.Error("n bounds the list")
	}
	for _, n := range []int{0, -1, -100} {
		if got := TopApps(rows, 0, n); got == nil || len(got) != 0 {
			t.Errorf("n=%d: %v (an empty list, never nil)", n, got)
		}
	}
	if got := TopApps(nil, 5, 10); got == nil || len(got) != 0 {
		t.Errorf("nothing: %v", got)
	}
}

func TestTheBiggestProcessWinsTheAppsPIDAndTiesGoToTheLowerPID(t *testing.T) {
	rows := []ProcRow{
		{PID: 30, PPID: 1, RSSKB: 100, Command: "/Applications/X.app/Contents/MacOS/x"},
		{PID: 20, PPID: 1, RSSKB: 100, Command: "/Applications/X.app/Contents/Frameworks/H.app/Contents/MacOS/h"},
		{PID: 25, PPID: 1, RSSKB: 100, Command: "/Applications/X.app/Contents/Frameworks/H2.app/Contents/MacOS/h2"},
	}
	if a := TopApps(rows, 0, 5)[0]; a.PID != 20 || a.Processes != 3 {
		t.Errorf("%+v", a)
	}
	rows = append(rows, ProcRow{PID: 99, PPID: 1, RSSKB: 101, Command: "/Applications/X.app/Contents/MacOS/x2"})
	if a := TopApps(rows, 0, 5)[0]; a.PID != 99 {
		t.Errorf("a bigger process takes the PID: %+v", a)
	}
}

func TestAFamilyWithALoopOrNoOwnProcessIsHandled(t *testing.T) {
	loop := []ProcRow{{PID: 5, PPID: 6, RSSKB: 10, Command: "a"}, {PID: 6, PPID: 5, RSSKB: 10, Command: "b"}, {PID: 7, PPID: 6, RSSKB: 10, Command: "c"}}
	got := TopApps(loop, 5, 10)
	if len(got) != 1 || got[0].Name != CinqoName || got[0].Processes != 3 {
		t.Errorf("a loop in the table must not run forever and the family is merged: %+v", got)
	}
	if got := TopApps(loop, 0, 10); len(got) != 3 {
		t.Errorf("no own process: nothing is merged: %+v", got)
	}
	if got := TopApps(loop, -3, 10); len(got) != 3 {
		t.Errorf("%+v", got)
	}
	if got := TopApps(loop, 12345, 10); len(got) != 3 {
		t.Errorf("an own process that is not in the table merges nothing: %+v", got)
	}
}

func TestLinuxNamesGroupByCommand(t *testing.T) {
	raw := "  1 0 1000 systemd\n 50 1 20000 chrome\n 51 50 30000 chrome\n 52 50 10000 chrome\n 60 1 5000 Xorg\n"
	got := TopApps(ParseProcessTable([]byte(raw)), 0, 10)
	if got[0].Name != "chrome" || got[0].Processes != 3 || got[0].RSSBytes != 60000*1024 || got[0].PID != 51 {
		t.Errorf("%+v", got[0])
	}
}

func TestReadProcessesUsesPsByAbsolutePathWithFixedArguments(t *testing.T) {
	withMachine(t, "darwin", nil, nil)
	withRunCmd(t, func(path string, args ...string) ([]byte, error) {
		if (path != "/bin/ps" && path != "/usr/bin/ps") || strings.Join(args, " ") != "-axww -o pid=,ppid=,rss=,comm=" {
			t.Errorf("ran %q %v", path, args)
		}
		return []byte(macTable), nil
	})
	if rows := ReadProcesses(); len(rows) != 15 {
		t.Errorf("%d rows", len(rows))
	}
}

func TestReadProcessesFallsBackToTheSecondPsAndGivesUpQuietly(t *testing.T) {
	withMachine(t, "linux", nil, nil)
	var tried []string
	withRunCmd(t, func(path string, args ...string) ([]byte, error) {
		tried = append(tried, path)
		if path == "/bin/ps" {
			return nil, errors.New("no such file")
		}
		return []byte(" 1 0 10 a\n"), nil
	})
	if rows := ReadProcesses(); len(rows) != 1 || len(tried) != 2 {
		t.Errorf("%v %v", rows, tried)
	}
	withRunCmd(t, func(string, ...string) ([]byte, error) { return nil, errors.New("no") })
	if rows := ReadProcesses(); rows != nil {
		t.Errorf("a failing ps is nil, not a crash: %v", rows)
	}
}

func TestReadProcessesIsNilOnASystemWithoutPs(t *testing.T) {
	withMachine(t, "windows", nil, nil)
	withRunCmd(t, func(string, ...string) ([]byte, error) { t.Error("nothing may be run"); return nil, nil })
	if rows := ReadProcesses(); rows != nil {
		t.Errorf("%v", rows)
	}
}

func TestTheRealTableHasThisProgramWithItsOwnNameAndASaneSize(t *testing.T) {
	if goos != "darwin" && goos != "linux" {
		t.Skip("no ps here")
	}
	rows := ReadProcesses()
	if len(rows) == 0 {
		t.Skip("ps cannot be read here")
	}
	var me *ProcRow
	for i := range rows {
		if rows[i].PID == os.Getpid() {
			me = &rows[i]
		}
	}
	if me == nil || me.RSSKB == 0 || me.PPID != os.Getppid() {
		t.Fatalf("this process in the table: %+v", me)
	}
	apps := TopApps(rows, os.Getpid(), 10)
	if len(apps) == 0 || len(apps) > 10 {
		t.Fatalf("%d apps", len(apps))
	}
	for i, a := range apps {
		if a.Name == "" || a.PID <= 0 || a.Processes < 1 || a.RSSBytes == 0 || (i > 0 && a.RSSBytes > apps[i-1].RSSBytes) {
			t.Errorf("row %d: %+v", i, a)
		}
	}
}

func TestTheEightyCharacterLimitOfANameIsExact(t *testing.T) {
	if got := AppName("/bin/"+strings.Repeat("a", 80), 1); len([]rune(got)) != 80 {
		t.Errorf("80 characters are kept whole: %d", len([]rune(got)))
	}
	if got := AppName("/bin/"+strings.Repeat("a", 81), 1); len([]rune(got)) != 80 {
		t.Errorf("81 characters are cut to 80: %d", len([]rune(got)))
	}
}
