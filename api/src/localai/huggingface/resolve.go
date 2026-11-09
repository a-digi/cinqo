// Package huggingface looks a model up on Hugging Face for the local AI (plan/ai/small-llm-local/step-220-tools-suggest-local-models.md, step 2). A tool
// says only where a model is (repository, file, revision); everything else is read here, from the Hugging Face API, never taken from the tool: the exact
// commit the revision points to (the address is pinned to it from then on), the file's size and SHA-256, and the licence. The result is checked with
// the same rules as an entry of the shipped catalog. Nothing is downloaded here, and the only address ever contacted is the Hugging Face API.
package huggingface

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/a-digi/cinqo/src/localai/catalog"
	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/toolrepository/fetch"
)

// Host is the only server the lookup talks to.
const Host = "huggingface.co"

// maxAnswerBytes bounds the answer of the API: a repository lists every file it has, so it can be long, but never this long.
const maxAnswerBytes = 8 << 20

// Why a lookup failed. Each is a whole sentence safe to show to the person.
var (
	ErrInvalidRequest = errors.New("the repository, file or revision is not a valid Hugging Face name")
	ErrNotFound       = errors.New("Hugging Face has no such repository, revision or file")
	ErrNotAvailable   = errors.New("Hugging Face says the repository does not exist, or it is private, gated or disabled, so Cinqo cannot download it")
	ErrNoChecksum     = errors.New("Hugging Face lists no SHA-256 for this file, so it cannot be verified")
	ErrNoLicense      = errors.New("the repository states no licence")
	ErrMismatch       = errors.New("what the tool stated does not match what Hugging Face says")
	ErrUnreachable    = errors.New("Hugging Face could not be reached or answered something unexpected")
	ErrInvalid        = errors.New("the model does not meet the rules of the model list")
)

var (
	repoPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,95}/[A-Za-z0-9][A-Za-z0-9._-]{0,95}$`)
	filePattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,150}\.gguf$`)
	revisionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,99}$`)
	commitPattern   = regexp.MustCompile(`^[0-9a-f]{40}$`)
	sha256Pattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	quantPattern    = regexp.MustCompile(`(?i)(IQ[0-9]_[A-Z0-9_]+|Q[0-9]_[A-Z0-9_]+|Q[0-9]+|BF16|F16|F32)`)
)

// Streamer is what the lookup needs to reach the API: the fetcher's streaming request (the tests replace it).
type Streamer interface {
	Stream(ctx context.Context, rawURL string, opts fetch.StreamOptions) (*fetch.Stream, error)
}

// Request says where a model is. Revision is a branch, a tag or a commit (empty means main). ExpectSHA256 and ExpectSizeBytes are what a tool may
// state about the file: they are never used, only compared with what Hugging Face says.
type Request struct {
	Repo            string
	File            string
	Revision        string
	ExpectSHA256    string
	ExpectSizeBytes int64
}

// Resolved is everything the lookup found out. URL is pinned to Commit.
type Resolved struct {
	ID           string
	Repo         string
	File         string
	Commit       string
	URL          string
	SHA256       string
	SizeBytes    int64
	License      string
	Publisher    string
	Name         string
	Quantization string
}

// AllowURL is the rule for every address the lookup uses, the first one and every redirect: https, on Hugging Face, no credentials.
func AllowURL(u *url.URL) error {
	if u.Scheme != "https" || u.Hostname() != Host || u.User != nil {
		return fmt.Errorf("only https://%s may be used", Host)
	}
	return nil
}

// Validate checks the three names of a request with strict patterns. A revision with a ".." part is refused.
func (r Request) Validate() error {
	rev := r.Revision
	if rev == "" {
		rev = "main"
	}
	if !repoPattern.MatchString(r.Repo) || !filePattern.MatchString(r.File) || !revisionPattern.MatchString(rev) {
		return ErrInvalidRequest
	}
	for _, part := range strings.Split(rev, "/") {
		if part == "" || part == "." || part == ".." {
			return ErrInvalidRequest
		}
	}
	if r.ExpectSHA256 != "" && !sha256Pattern.MatchString(r.ExpectSHA256) {
		return ErrInvalidRequest
	}
	return nil
}

type apiAnswer struct {
	ID       string `json:"id"`
	Author   string `json:"author"`
	SHA      string `json:"sha"`
	Private  bool   `json:"private"`
	Disabled bool   `json:"disabled"`
	// Gated is false, or the string "auto" or "manual": decoded loosely.
	Gated    json.RawMessage `json:"gated"`
	Tags     []string        `json:"tags"`
	CardData struct {
		License json.RawMessage `json:"license"`
	} `json:"cardData"`
	Siblings []struct {
		Name string `json:"rfilename"`
		Size int64  `json:"size"`
		LFS  *struct {
			SHA256 string `json:"sha256"`
			Size   int64  `json:"size"`
		} `json:"lfs"`
	} `json:"siblings"`
}

// Resolve asks Hugging Face about the file and returns what it found, checked against the rules of the model list. A tool-stated size or SHA-256
// that differs from what Hugging Face says is refused.
func Resolve(ctx context.Context, s Streamer, req Request) (Resolved, error) {
	if err := req.Validate(); err != nil {
		return Resolved{}, err
	}
	rev := req.Revision
	if rev == "" {
		rev = "main"
	}
	api := "https://" + Host + "/api/models/" + req.Repo + "/revision/" + url.PathEscape(rev) + "?blobs=true"
	body, err := get(ctx, s, api)
	if err != nil {
		return Resolved{}, err
	}
	var a apiAnswer
	if err := json.Unmarshal(body, &a); err != nil {
		return Resolved{}, ErrUnreachable
	}
	if a.Private || a.Disabled || gated(a.Gated) {
		return Resolved{}, ErrNotAvailable
	}
	if !commitPattern.MatchString(a.SHA) {
		return Resolved{}, ErrUnreachable
	}
	if commitPattern.MatchString(rev) && rev != a.SHA {
		return Resolved{}, ErrMismatch
	}

	var size int64
	var sum string
	found := false
	for _, f := range a.Siblings {
		if f.Name != req.File {
			continue
		}
		found = true
		if f.LFS == nil || !sha256Pattern.MatchString(f.LFS.SHA256) {
			return Resolved{}, ErrNoChecksum
		}
		size, sum = f.LFS.Size, f.LFS.SHA256
		if size == 0 {
			size = f.Size
		}
		if f.Size != 0 && f.LFS.Size != 0 && f.Size != f.LFS.Size {
			return Resolved{}, ErrUnreachable
		}
		break
	}
	if !found {
		return Resolved{}, ErrNotFound
	}
	if (req.ExpectSHA256 != "" && req.ExpectSHA256 != sum) || (req.ExpectSizeBytes != 0 && req.ExpectSizeBytes != size) {
		return Resolved{}, ErrMismatch
	}
	license := licenseOf(a)
	if license == "" {
		return Resolved{}, ErrNoLicense
	}

	name := strings.TrimSuffix(req.File, ".gguf")
	quant := strings.ToUpper(quantPattern.FindString(name))
	if quant == "" {
		quant = "unknown"
	}
	out := Resolved{
		ID: entity.AddedID(req.Repo, req.File, a.SHA), Repo: req.Repo, File: req.File, Commit: a.SHA,
		URL:    "https://" + Host + "/" + req.Repo + "/resolve/" + a.SHA + "/" + req.File,
		SHA256: sum, SizeBytes: size, License: license, Publisher: strings.SplitN(req.Repo, "/", 2)[0], Name: name, Quantization: quant,
	}
	// the same rules as a shipped entry (size between 1 MiB and 20 GiB, a pinned Hugging Face file, a licence, ...)
	if err := catalog.ValidateModel(out.AddedModel("", time.Now().UTC()).CatalogModel()); err != nil {
		return Resolved{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return out, nil
}

// AddedModel is the row a suggestion of this result would store (the tool's own words are added by the caller).
func (r Resolved) AddedModel(suggestedBy string, now time.Time) entity.AddedModel {
	return entity.AddedModel{
		ID: r.ID, Repo: r.Repo, File: r.File, Revision: r.Commit, URL: r.URL, SHA256: r.SHA256, SizeBytes: r.SizeBytes, Quantization: r.Quantization,
		License: r.License, Name: r.Name, Publisher: r.Publisher, ContextTokens: entity.DefaultAddedContextTokens, SuggestedBy: suggestedBy,
		Status: entity.AddedStatusSuggested, CreatedAt: now.Format(time.RFC3339),
	}
}

func get(ctx context.Context, s Streamer, rawURL string) ([]byte, error) {
	st, err := s.Stream(ctx, rawURL, fetch.StreamOptions{AllowURL: AllowURL})
	if err != nil || st == nil {
		return nil, ErrUnreachable
	}
	defer st.Body.Close()
	switch {
	case st.Status == 404:
		return nil, ErrNotFound
	case st.Status == 401 || st.Status == 403:
		return nil, ErrNotAvailable
	case st.Status != 200:
		return nil, ErrUnreachable
	}
	b, err := io.ReadAll(io.LimitReader(st.Body, maxAnswerBytes+1))
	if err != nil || len(b) > maxAnswerBytes {
		return nil, ErrUnreachable
	}
	return b, nil
}

// gated reports whether the repository asks for access to be requested: "gated" is false, or "auto" or "manual".
func gated(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s != "" && s != "false" && s != "null"
}

// licenseOf is the licence the repository states: its card's license (a text, or the first of a list), else a "license:" tag.
func licenseOf(a apiAnswer) string {
	var one string
	if json.Unmarshal(a.CardData.License, &one) == nil && strings.TrimSpace(one) != "" {
		return clean(one)
	}
	var many []string
	if json.Unmarshal(a.CardData.License, &many) == nil && len(many) > 0 && strings.TrimSpace(many[0]) != "" {
		return clean(many[0])
	}
	for _, t := range a.Tags {
		if v, ok := strings.CutPrefix(t, "license:"); ok && strings.TrimSpace(v) != "" {
			return clean(v)
		}
	}
	return ""
}

func clean(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 100 {
		s = s[:100]
	}
	return s
}
