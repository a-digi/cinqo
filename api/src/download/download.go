// Package download is a reusable download manager with visible progress: a feature starts a download of one file and gets
// a job id at once; the transfer runs in the background and reports its progress (bytes, speed, time left, and the phases
// downloading, verifying and installing) for anyone who asks. It resumes an interrupted transfer, refuses a file that is
// not exactly the announced size or does not match its SHA-256, and puts the file in its place only after it was checked.
// It lives in the core, not in any one feature, so every feature that downloads something can use it.
package download

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// State is where a job is.
type State string

const (
	StateQueued      State = "queued"      // waiting for a free slot
	StateDownloading State = "downloading" // bytes are arriving
	StateVerifying   State = "verifying"   // the file is complete and its checksum is being checked
	StateInstalling  State = "installing"  // the checked file is being put to use (the Install hook of the spec)
	StateDone        State = "done"
	StateFailed      State = "failed"
	StateCancelled   State = "cancelled"
)

// Active reports whether the job still has work to do.
func (s State) Active() bool {
	switch s {
	case StateQueued, StateDownloading, StateVerifying, StateInstalling:
		return true
	}
	return false
}

// MaxBytes is the largest file a job may announce: a cap for the size check and for the free-space check.
const MaxBytes = 20 << 30

// Progress is what anyone watching a job sees. BytesDone and BytesTotal count the bytes received while downloading and the
// bytes checked while verifying. SecondsLeft is null while it cannot be known (no speed yet, or not downloading). Stalled is
// true while no byte has arrived for a while.
type Progress struct {
	ID             string  `json:"id"`
	Label          string  `json:"label"`
	State          State   `json:"state"`
	BytesDone      int64   `json:"bytesDone"`
	BytesTotal     int64   `json:"bytesTotal"`
	BytesPerSecond float64 `json:"bytesPerSecond"`
	SecondsLeft    *int64  `json:"secondsLeft"`
	Stalled        bool    `json:"stalled"`
	Error          string  `json:"error"`
	// Notice says what is going on besides downloading: while it is not empty the connection was lost and the manager is
	// waiting to reconnect at the byte where it stopped.
	Notice     string `json:"notice"`
	StartedAt  string `json:"startedAt"`
	FinishedAt string `json:"finishedAt"`
}

// Spec describes one file to download.
type Spec struct {
	// Key identifies the destination: a second Start with the same key joins the running job instead of starting another.
	// Empty means the destination path.
	Key string
	// Label is the name people see ("Phi-4-mini-instruct").
	Label string
	// URL is the pinned https address of the file.
	URL string
	// Dest is the absolute path the finished file is put at.
	Dest string
	// SizeBytes and SHA256 are the exact size and the lower-case hex SHA-256 the file must have.
	SizeBytes int64
	SHA256    string
	// AllowHost says whether a host may be contacted. It is asked about the first address and about every redirect target.
	AllowHost func(host string) bool
	// Scope is the scope a caller needs to see and cancel the job (super admin always may).
	Scope string
	// Install, when set, runs after the checksum matched and the file is in place (for example unpacking an archive). An
	// error fails the job; the downloaded file stays.
	Install func(path string) error
}

var (
	sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

	// ErrInvalidSpec wraps every reason a spec is refused.
	ErrInvalidSpec = errors.New("invalid download")
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidSpec, fmt.Sprintf(format, args...))
}

func (s Spec) validate() error {
	switch {
	case strings.TrimSpace(s.Label) == "" || len(s.Label) > 100:
		return invalid("a label of at most 100 characters is required")
	case !filepath.IsAbs(s.Dest) || filepath.Clean(s.Dest) != s.Dest || strings.HasSuffix(s.Dest, string(filepath.Separator)):
		return invalid("the destination must be a clean absolute file path")
	case s.SizeBytes <= 0 || s.SizeBytes > MaxBytes:
		return invalid("the size must be between 1 byte and %d bytes", int64(MaxBytes))
	case !sha256Pattern.MatchString(s.SHA256):
		return invalid("the checksum must be 64 lower-case hex digits")
	case s.AllowHost == nil:
		return invalid("a host rule is required")
	case strings.TrimSpace(s.Scope) == "":
		return invalid("a scope is required")
	}
	u, err := url.Parse(s.URL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return invalid("the address must be an https address without credentials")
	}
	if !s.AllowHost(u.Hostname()) {
		return invalid("the host of the address is not allowed")
	}
	return nil
}

func (s Spec) key() string {
	if s.Key != "" {
		return s.Key
	}
	return s.Dest
}

// formatRFC3339 renders a time for Progress; the zero time is an empty string.
func formatRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
