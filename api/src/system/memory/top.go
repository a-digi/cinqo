package memory

import (
	"bufio"
	"bytes"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// The applications that use the most memory (plan 226). One read of the whole process table gives, for every process, its id, its parent, its
// resident size and the name of the program it runs; the processes are then grouped into applications (Chrome and VS Code run dozens), and the
// biggest ones are returned. Only the command NAME is read, never the command line (it can hold tokens, passwords and URLs), and the
// executable path is used to find the application's name and then dropped (it holds user names).

// ProcRow is one row of the process table.
type ProcRow struct {
	PID     int
	PPID    int
	RSSKB   uint64 // resident size in kilobytes, as ps prints it
	Command string // the executable as ps shows it (a path on macOS, a command name on Linux); never sent anywhere
}

// App is one application: all the processes with the same name, summed.
type App struct {
	Name      string `json:"name"`
	PID       int    `json:"pid"`       // the application's biggest process
	Processes int    `json:"processes"` // how many processes it has
	RSSBytes  uint64 `json:"rssBytes"`
	Cinqo     bool   `json:"cinqo"` // Cinqo's own program, its tools and the model program, merged into one application
}

// CinqoName is the name of the merged row of Cinqo's own processes.
const CinqoName = "Cinqo"

const maxNameRunes = 80

// ParseProcessTable reads the output of `ps -axww -o pid=,ppid=,rss=,comm=`: three numbers, then the command (which may have spaces). A line
// that does not fit is skipped.
func ParseProcessTable(raw []byte) []ProcRow {
	var out []ProcRow
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		pid, e1 := strconv.Atoi(f[0])
		ppid, e2 := strconv.Atoi(f[1])
		rss, e3 := strconv.ParseUint(f[2], 10, 64)
		if e1 != nil || e2 != nil || e3 != nil || pid <= 0 {
			continue
		}
		cmd := line
		for i := 0; i < 3; i++ { // the command is the rest of the line after the three numbers, its own spacing kept
			cmd = strings.TrimSpace(strings.TrimPrefix(cmd, f[i]))
		}
		out = append(out, ProcRow{PID: pid, PPID: ppid, RSSKB: rss, Command: cmd})
	}
	return out
}

// AppName is the name of the application behind an executable. A program inside an application bundle takes the name of the OUTERMOST bundle
// ("/Applications/Google Chrome.app/Contents/Frameworks/Google Chrome Helper.app/..." is "Google Chrome"); any other program is named after its
// file. A leading "-" (a login shell) and the parentheses of a defunct process are dropped. The result is one line of at most 80 printable
// characters; when nothing readable is left it is "process <pid>".
func AppName(command string, pid int) string {
	name := nameFromPath(strings.TrimSpace(command))
	if strings.HasPrefix(name, "(") && strings.HasSuffix(name, ")") { // a defunct process is shown as "(name)"
		name = strings.TrimSpace(name[1 : len(name)-1])
	}
	name = strings.TrimLeft(name, "-")
	name = cleanName(name)
	if name == "" || allDigits(name) {
		return "process " + strconv.Itoa(pid)
	}
	return name
}

func nameFromPath(p string) string {
	if i := bundleEnd(p); i >= 0 {
		dir := p[:i]
		if j := strings.LastIndex(dir, "/"); j >= 0 {
			dir = dir[j+1:]
		}
		return strings.TrimSuffix(dir, ".app")
	}
	if j := strings.LastIndex(p, "/"); j >= 0 {
		return p[j+1:]
	}
	return p
}

// bundleEnd is the index just after the first ".app" that ends a path element, or -1.
func bundleEnd(p string) int {
	for from := 0; ; {
		i := strings.Index(p[from:], ".app")
		if i < 0 {
			return -1
		}
		end := from + i + len(".app")
		if end == len(p) || p[end] == '/' {
			return end
		}
		from = end
	}
}

func cleanName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) || r == unicode.ReplacementChar {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if r := []rune(out); len(r) > maxNameRunes {
		out = string(r[:maxNameRunes])
	}
	return out
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// TopApps groups the processes into applications and returns the n that use the most memory, biggest first (ties by name, then by process id).
// Every process below ownPID (this program, its tools and the model program: whatever its ancestors lead back to ownPID) is merged into one row
// named Cinqo. Processes with no resident memory are left out. n of 0 or less returns nothing.
func TopApps(rows []ProcRow, ownPID, n int) []App {
	if n <= 0 {
		return []App{}
	}
	family := descendantsOf(rows, ownPID)
	type group struct {
		app       App
		biggestKB uint64
	}
	groups := map[string]*group{}
	var order []string
	for _, r := range rows {
		if r.RSSKB == 0 {
			continue
		}
		cinqo := family[r.PID]
		name := CinqoName
		key := "\x00cinqo"
		if !cinqo {
			name = AppName(r.Command, r.PID)
			key = "n:" + name
		}
		g, ok := groups[key]
		if !ok {
			g = &group{app: App{Name: name, Cinqo: cinqo}}
			groups[key] = g
			order = append(order, key)
		}
		g.app.Processes++
		g.app.RSSBytes += r.RSSKB * 1024
		if r.RSSKB > g.biggestKB || (r.RSSKB == g.biggestKB && (g.app.PID == 0 || r.PID < g.app.PID)) {
			g.biggestKB = r.RSSKB
			g.app.PID = r.PID
		}
	}
	apps := make([]App, 0, len(order))
	for _, k := range order {
		apps = append(apps, groups[k].app)
	}
	sort.Slice(apps, func(i, j int) bool {
		if apps[i].RSSBytes != apps[j].RSSBytes {
			return apps[i].RSSBytes > apps[j].RSSBytes
		}
		if apps[i].Name != apps[j].Name {
			return apps[i].Name < apps[j].Name
		}
		return apps[i].PID < apps[j].PID
	})
	if len(apps) > n {
		apps = apps[:n]
	}
	return apps
}

// descendantsOf is the set of process ids that are root or have root among their ancestors (root included when it is in the table). A loop
// in the table cannot run it forever.
func descendantsOf(rows []ProcRow, root int) map[int]bool {
	kids := map[int][]int{}
	for _, r := range rows {
		kids[r.PPID] = append(kids[r.PPID], r.PID)
	}
	out := map[int]bool{}
	if root <= 0 {
		return out
	}
	out[root] = true
	queue := []int{root}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, c := range kids[p] {
			if !out[c] {
				out[c] = true
				queue = append(queue, c)
			}
		}
	}
	return out
}

// psPaths are the places `ps` lives; an absolute path is always used, never a search of the path.
var psPaths = []string{"/bin/ps", "/usr/bin/ps"}

// ReadProcesses reads the whole process table now. nil when it cannot be read (an unsupported system, or ps failing).
func ReadProcesses() []ProcRow {
	if goos != "darwin" && goos != "linux" {
		return nil
	}
	for _, ps := range psPaths {
		if out, err := runCmd(ps, "-axww", "-o", "pid=,ppid=,rss=,comm="); err == nil {
			return ParseProcessTable(out)
		}
	}
	return nil
}
