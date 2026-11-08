// Package catalog is the list of local models Cinqo knows how to download: shipped inside the program (embedded, so
// editing it means a new build), validated strictly when it is loaded, and never edited at run time. Every entry names one
// file by an immutable address (a Hugging Face commit), its exact size and its SHA-256, so a download can be checked
// before anything is used. See plan/ai/small-llm-local/step-181-local-ai-small-models.md.
package catalog

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

//go:embed catalog.json
var embedded []byte

// Limits of what an entry may claim.
const (
	MinModelBytes    = 1 << 20  // 1 MiB: anything smaller is not a model
	MaxModelBytes    = 20 << 30 // 20 GiB: a size cap for the download manager and the disk check
	MinContext       = 512
	MaxContext       = 4 << 20
	modelHost        = "huggingface.co"
	maxNameLength    = 100
	maxNoteLength    = 300
	maxDialectLength = 32
	// MaxKVBytesPerToken is far beyond the cache of any model this catalog could hold (4 MiB per token).
	MaxKVBytesPerToken = 4 << 20
	maxSourceLength    = 200
)

// toolDialectName is the shape of a toolDialect: lower-case words joined by hyphens (the same shape the dialect registry accepts).
var toolDialectName = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

var (
	idPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	runtimeHost   = "github.com"
	// A release file of llama.cpp: /ggml-org/llama.cpp/releases/download/<tag>/llama-<tag>-bin-<system>.tar.gz
	runtimePathPattern   = regexp.MustCompile(`^/ggml-org/llama\.cpp/releases/download/(b[0-9]{1,6})/llama-(b[0-9]{1,6})-bin-[a-z0-9]+(-[a-z0-9]+)*\.tar\.gz$`)
	versionPattern       = regexp.MustCompile(`^b[0-9]{1,6}$`)
	runtimeBinaryPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/llama-server$`)

	// A pinned file: /<owner>/<repo>/resolve/<40 hex commit>/<file>.gguf, so the bytes behind the address never change. Every
	// name starts with a letter or digit, so "." and ".." can never be a path segment.
	pathPattern = regexp.MustCompile(`^/[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*/resolve/[0-9a-f]{40}/[A-Za-z0-9][A-Za-z0-9._-]*\.gguf$`)
)

// Model is one downloadable model file.
type Model struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Quantization  string `json:"quantization"`
	SizeBytes     int64  `json:"sizeBytes"`
	SHA256        string `json:"sha256"`
	URL           string `json:"url"`
	License       string `json:"license"`
	ContextTokens int    `json:"contextTokens"`
	// Tools says the model was measured to call tools reliably (step 8): only then a conversation may be given an allowlist.
	Tools bool `json:"tools"`
	// KVBytesPerToken is what the model's cache holds per token of context (layers x KV heads x head size x 2 x 2 bytes, f16),
	// read from the model's own configuration. 0 means not stated: the memory check then does not count the cache.
	KVBytesPerToken int64 `json:"kvBytesPerToken"`
	// ToolDialect names how a model whose chat template cannot take tools is taught them; empty for a model that takes them
	// natively. The catalog only checks the shape of the name: which names exist is decided where the module is put together,
	// and a model whose dialect is missing there has no tools.
	ToolDialect string `json:"toolDialect"`
	// UnsuitableFor lists the tasks this model was measured NOT to manage (plan 207): a tool that does such a task can warn before it
	// starts. Only the tasks in Tasks are known; empty (the default) says nothing, which is not the same as "suitable".
	UnsuitableFor []string `json:"unsuitableFor"`
	// Driver says how a caller should run a long task for this model (plan 209). Empty (the default) leaves it to the platform. DriverCode: the
	// caller does the steps itself, in code, and asks the model only small questions with no tools (a model that cannot take a whole CV).
	Driver    string `json:"driver"`
	Source    string `json:"source"`
	Note      string `json:"note"`
	CheckedOn string `json:"checkedOn"`
}

// Runtime is one build of the program that runs the models (llama.cpp's llama-server) for one system: an archive that is
// downloaded, checked and unpacked. Binary is the path of the server program inside the archive.
type Runtime struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	SizeBytes int64  `json:"sizeBytes"`
	SHA256    string `json:"sha256"`
	URL       string `json:"url"`
	Binary    string `json:"binary"`
	License   string `json:"license"`
	Source    string `json:"source"`
	CheckedOn string `json:"checkedOn"`
}

// Catalog is the whole list and the day it was last checked against the real pages.
type Catalog struct {
	CheckedOn string    `json:"checkedOn"`
	Models    []Model   `json:"models"`
	Runtimes  []Runtime `json:"runtimes"`
}

// ErrInvalid wraps every reason a catalog file is refused.
var ErrInvalid = errors.New("invalid local AI catalog")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Load parses and validates a catalog file. Unknown fields, a missing field, a duplicate id, an address that is not a
// pinned https Hugging Face file, a size or hash that cannot be right: all refuse the whole file, never one entry.
func Load(raw []byte) (Catalog, error) {
	var c Catalog
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Catalog{}, invalid("%v", err)
	}
	if dec.More() {
		return Catalog{}, invalid("trailing data after the catalog")
	}
	if !validDate(c.CheckedOn) {
		return Catalog{}, invalid("checkedOn must be a date like 2026-11-03")
	}
	if len(c.Models) == 0 {
		return Catalog{}, invalid("the catalog has no model")
	}
	if len(c.Runtimes) == 0 {
		return Catalog{}, invalid("the catalog has no runtime")
	}
	seen := make(map[string]bool, len(c.Models))
	for i, m := range c.Models {
		if err := validateModel(m); err != nil {
			return Catalog{}, invalid("model %d (%q): %v", i+1, m.ID, err)
		}
		if seen[m.ID] {
			return Catalog{}, invalid("model id %q appears twice", m.ID)
		}
		seen[m.ID] = true
	}
	seenRuntime := make(map[string]bool, len(c.Runtimes))
	seenSystem := make(map[string]bool, len(c.Runtimes))
	for i, r := range c.Runtimes {
		if err := validateRuntime(r); err != nil {
			return Catalog{}, invalid("runtime %d (%q): %v", i+1, r.ID, err)
		}
		if seenRuntime[r.ID] || seen[r.ID] {
			return Catalog{}, invalid("id %q appears twice", r.ID)
		}
		system := r.OS + "/" + r.Arch
		if seenSystem[system] {
			return Catalog{}, invalid("there are two runtimes for %s", system)
		}
		seenRuntime[r.ID], seenSystem[system] = true, true
	}
	return c, nil
}

// MaxRuntimeBytes is the largest archive a runtime entry may announce.
const MaxRuntimeBytes = 1 << 30

func validateRuntime(r Runtime) error {
	switch {
	case !idPattern.MatchString(r.ID):
		return errors.New("id must be lower-case letters, digits, dot, dash or underscore (at most 64)")
	case strings.TrimSpace(r.Name) == "" || len(r.Name) > maxNameLength:
		return errors.New("name is required (at most 100 characters)")
	case !versionPattern.MatchString(r.Version):
		return errors.New("version must look like b12064")
	case r.OS != "darwin" && r.OS != "linux":
		return errors.New("os must be darwin or linux")
	case r.Arch != "arm64" && r.Arch != "amd64":
		return errors.New("arch must be arm64 or amd64")
	case r.SizeBytes < MinModelBytes || r.SizeBytes > MaxRuntimeBytes:
		return fmt.Errorf("sizeBytes must be between %d and %d", MinModelBytes, int64(MaxRuntimeBytes))
	case !sha256Pattern.MatchString(r.SHA256):
		return errors.New("sha256 must be 64 lower-case hex digits")
	case !runtimeBinaryPattern.MatchString(r.Binary):
		return errors.New("binary must be <folder>/llama-server inside the archive")
	case strings.TrimSpace(r.License) == "":
		return errors.New("license is required")
	case strings.TrimSpace(r.Source) == "" || len(r.Source) > maxSourceLength:
		return errors.New("source is required (at most 200 characters)")
	case !validDate(r.CheckedOn):
		return errors.New("checkedOn must be a date like 2026-11-03")
	}
	u, err := url.Parse(r.URL)
	if err != nil || u.Scheme != "https" || u.Host != runtimeHost || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("url must be an https://%s address", runtimeHost)
	}
	m := runtimePathPattern.FindStringSubmatch(u.Path)
	if m == nil {
		return errors.New("url must point at a llama.cpp release archive (/ggml-org/llama.cpp/releases/download/<tag>/llama-<tag>-bin-<system>.tar.gz)")
	}
	if m[1] != r.Version || m[2] != r.Version {
		return errors.New("the version, the release tag and the file name must be the same")
	}
	return nil
}

func validateModel(m Model) error {
	switch {
	case !idPattern.MatchString(m.ID):
		return errors.New("id must be lower-case letters, digits, dot, dash or underscore (at most 64)")
	case strings.TrimSpace(m.Name) == "" || len(m.Name) > maxNameLength:
		return errors.New("name is required (at most 100 characters)")
	case strings.TrimSpace(m.Quantization) == "":
		return errors.New("quantization is required")
	case m.SizeBytes < MinModelBytes || m.SizeBytes > MaxModelBytes:
		return fmt.Errorf("sizeBytes must be between %d and %d", MinModelBytes, int64(MaxModelBytes))
	case !sha256Pattern.MatchString(m.SHA256):
		return errors.New("sha256 must be 64 lower-case hex digits")
	case strings.TrimSpace(m.License) == "":
		return errors.New("license is required")
	case m.ContextTokens < MinContext || m.ContextTokens > MaxContext:
		return fmt.Errorf("contextTokens must be between %d and %d", MinContext, MaxContext)
	case strings.TrimSpace(m.Source) == "" || len(m.Source) > maxSourceLength:
		return errors.New("source is required (at most 200 characters)")
	case len(m.Note) > maxNoteLength:
		return errors.New("note is too long (at most 300 characters)")
	case !validDate(m.CheckedOn):
		return errors.New("checkedOn must be a date like 2026-11-03")
	case m.ToolDialect != "" && (len(m.ToolDialect) > maxDialectLength || !toolDialectName.MatchString(m.ToolDialect)):
		return fmt.Errorf("toolDialect %q is not a name like phi-json", m.ToolDialect)
	case m.KVBytesPerToken < 0 || m.KVBytesPerToken > MaxKVBytesPerToken:
		return fmt.Errorf("kvBytesPerToken must be between 0 and %d", MaxKVBytesPerToken)
	case m.ToolDialect != "" && !m.Tools:
		return errors.New("a toolDialect only makes sense for a model that can use tools")
	}
	if err := validateUnsuitableFor(m.UnsuitableFor); err != nil {
		return err
	}
	if m.Driver != "" && m.Driver != DriverCode {
		return fmt.Errorf("driver %q is not known (empty or %q)", m.Driver, DriverCode)
	}
	return validateURL(m.URL)
}

func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != modelHost {
		return fmt.Errorf("url must be an https://%s address", modelHost)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("url must have no credentials, query or fragment")
	}
	if !pathPattern.MatchString(u.Path) {
		return errors.New("url must point at one .gguf file of a pinned commit (/<owner>/<repo>/resolve/<40-hex commit>/<file>.gguf)")
	}
	return nil
}

func validDate(s string) bool {
	t, err := time.Parse("2006-01-02", s)
	return err == nil && t.Format("2006-01-02") == s
}

// Find returns the model with this id.
func (c Catalog) Find(id string) (Model, bool) {
	for _, m := range c.Models {
		if m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}

// RuntimeFor returns the runtime build for a system (os as GOOS names it, arch as GOARCH names it).
func (c Catalog) RuntimeFor(goos, goarch string) (Runtime, bool) {
	for _, r := range c.Runtimes {
		if r.OS == goos && r.Arch == goarch {
			return r, true
		}
	}
	return Runtime{}, false
}

var (
	defaultOnce    sync.Once
	defaultCatalog Catalog
	defaultErr     error
)

// Default is the catalog shipped with the program, loaded and validated once.
func Default() (Catalog, error) {
	defaultOnce.Do(func() { defaultCatalog, defaultErr = Load(embedded) })
	return defaultCatalog, defaultErr
}

// TaskCVImport is the task of reading a CV into a profile (Career's Import from CV): about a dozen dependent tool calls with large arguments.
const TaskCVImport = "cv-import"

// DriverCode is the driver of a model whose long tasks are run in code, one small question at a time (see Model.Driver).
const DriverCode = "code"

// Tasks are the tasks a model can be marked unsuitable for.
var Tasks = []string{TaskCVImport}

func validateUnsuitableFor(tasks []string) error {
	seen := map[string]bool{}
	for _, task := range tasks {
		known := false
		for _, t := range Tasks {
			known = known || t == task
		}
		switch {
		case !known:
			return fmt.Errorf("unsuitableFor %q is not a known task (%s)", task, strings.Join(Tasks, ", "))
		case seen[task]:
			return fmt.Errorf("unsuitableFor lists %q twice", task)
		}
		seen[task] = true
	}
	return nil
}
