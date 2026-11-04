package providers

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"github.com/a-digi/cinqo/src/filereader/source"
	"github.com/a-digi/cinqo/src/livelog"
	"github.com/a-digi/cinqo/src/livelog/filetail"
)

// SystemLogID is the id of the application log provider.
const SystemLogID = "system-log"

// CurrentRef is the reference that means "the newest log file": the one the running application is writing.
const CurrentRef = "current"

// The application's own log (coco-logger) is one file per start, <logs>/YYYY/MM/DD/cinqo_YYYYMMDD_HHMMSS.log, in the folder of the day it
// started. Only files of exactly that shape are ever opened (the tool logs that live beside them, and anything else, are not).
var (
	logFileName = regexp.MustCompile(`^cinqo_(\d{4})(\d{2})(\d{2})_\d{6}\.log$`)
	yearDir     = regexp.MustCompile(`^\d{4}$`)
	twoDigitDir = regexp.MustCompile(`^\d{2}$`)
)

// maxDirReads bounds how many folders "current" may read while looking for the newest file, so a folder tree full of empty days cannot make
// one request walk for long.
const maxDirReads = 400

type systemLog struct {
	root string
	opts filetail.Options
}

// NewSystemLog follows the application's own log files under logsDir, for administrators only. The reference is a file name
// ("cinqo_20261104_093015.log": its folder is the date in its name) or "current" for the newest file. It is the module's proof that a kind of
// log is one file here and one line in the composition root: it shares nothing with the AI trace provider but the interface.
func NewSystemLog(logsDir string) livelog.Provider { return systemLog{root: logsDir} }

func (systemLog) ID() string       { return SystemLogID }
func (systemLog) Title() string    { return "System log" }
func (systemLog) Scopes() []string { return []string{livelog.AdminScope} }

func (s systemLog) Open(ref string) (livelog.Follower, error) {
	var segments []string
	if ref == CurrentRef {
		segments = s.newest()
	} else {
		segments = segmentsOf(ref)
	}
	if segments == nil {
		return nil, livelog.ErrNotFound
	}
	path, err := source.ResolveUnder(s.root, segments...)
	if err != nil {
		return nil, livelog.ErrNotFound // every way a reference can fail is the same answer
	}
	return filetail.New(path, s.opts), nil
}

// segmentsOf turns a log file name into its path under the root (year, month, day, name), or nil when it is not exactly such a name with a
// possible date.
func segmentsOf(name string) []string {
	m := logFileName.FindStringSubmatch(name)
	if m == nil {
		return nil
	}
	month, _ := strconv.Atoi(m[2])
	day, _ := strconv.Atoi(m[3])
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return nil
	}
	return []string{m[1], m[2], m[3], name}
}

// newest finds the path (as segments) of the newest log file: the highest year, then month, then day that holds one, and in it the highest
// name (the names sort by time). nil when there is none.
func (s systemLog) newest() []string {
	reads := 0
	var walk func(prefix []string, depth int) []string
	walk = func(prefix []string, depth int) []string {
		if reads >= maxDirReads {
			return nil
		}
		reads++
		entries, err := os.ReadDir(filepath.Join(append([]string{s.root}, prefix...)...))
		if err != nil {
			return nil
		}
		if depth == 3 { // the day folder: its files
			var names []string
			for _, e := range entries {
				if logFileName.MatchString(e.Name()) && !e.IsDir() {
					names = append(names, e.Name())
				}
			}
			if len(names) == 0 {
				return nil
			}
			sort.Strings(names)
			return append(append([]string(nil), prefix...), names[len(names)-1])
		}
		pattern := twoDigitDir
		if depth == 0 {
			pattern = yearDir
		}
		var dirs []string
		for _, e := range entries {
			if e.IsDir() && pattern.MatchString(e.Name()) {
				dirs = append(dirs, e.Name())
			}
		}
		sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
		for _, d := range dirs {
			if found := walk(append(append([]string(nil), prefix...), d), depth+1); found != nil {
				return found
			}
		}
		return nil
	}
	return walk(nil, 0)
}
