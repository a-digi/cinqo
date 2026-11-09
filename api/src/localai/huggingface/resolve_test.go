package huggingface

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/toolrepository/fetch"
)

const (
	commit = "91cad51170dc346986eccefdc2dd33a9da36ead9"
	other  = "78eb92a46fc37e6b524df991ed9aca9bc6aa7b80"
	sum    = "6a1a2eb6d15622bf3c96857206351ba97e1af16c30d7a74ee38970e434e9407e"
	size   = 1117320736
)

// fake answers by address, counts the requests, and asks AllowURL about every address like the real fetcher does.
type fake struct {
	answers map[string]fakeAnswer
	asked   []string
	err     error
	opts    []fetch.StreamOptions
}

type fakeAnswer struct {
	status int
	body   string
}

func (f *fake) Stream(_ context.Context, rawURL string, opts fetch.StreamOptions) (*fetch.Stream, error) {
	f.asked = append(f.asked, rawURL)
	f.opts = append(f.opts, opts)
	if f.err != nil {
		return nil, f.err
	}
	if opts.AllowURL != nil {
		u, _ := url.Parse(rawURL)
		if err := opts.AllowURL(u); err != nil {
			return nil, err
		}
	}
	a, ok := f.answers[rawURL]
	if !ok {
		a = fakeAnswer{status: 404}
	}
	return &fetch.Stream{Status: a.status, ContentLength: int64(len(a.body)), Header: http.Header{}, Body: io.NopCloser(strings.NewReader(a.body))}, nil
}

// answerJSON is the shape the real API answers with blobs=true (read from the real one for Qwen/Qwen2.5-1.5B-Instruct-GGUF).
func answerJSON(mutate func(m map[string]string)) string {
	f := map[string]string{
		"sha": commit, "private": "false", "gated": "false", "disabled": "false",
		"card":  `"license":"apache-2.0"`,
		"tags":  `["gguf","license:apache-2.0"]`,
		"files": fmt.Sprintf(`{"rfilename":"README.md","size":4856},{"rfilename":"model-q4_k_m.gguf","size":%d,"lfs":{"sha256":"%s","size":%d,"pointerSize":135}}`, size, sum, size),
	}
	if mutate != nil {
		mutate(f)
	}
	return fmt.Sprintf(`{"id":"o/r","author":"o","sha":"%s","private":%s,"gated":%s,"disabled":%s,"tags":%s,"cardData":{%s},"siblings":[%s]}`,
		f["sha"], f["private"], f["gated"], f["disabled"], f["tags"], f["card"], f["files"])
}

const api = "https://huggingface.co/api/models/o/r/revision/main?blobs=true"

func one(body string) *fake {
	return &fake{answers: map[string]fakeAnswer{api: {200, body}}}
}

func req() Request { return Request{Repo: "o/r", File: "model-q4_k_m.gguf"} }

func TestAFileIsResolvedToAPinnedAddressWithItsSizeHashAndLicence(t *testing.T) {
	f := one(answerJSON(nil))
	got, err := Resolve(context.Background(), f, req())
	if err != nil {
		t.Fatal(err)
	}
	want := "https://huggingface.co/o/r/resolve/" + commit + "/model-q4_k_m.gguf"
	if got.URL != want || got.Commit != commit || got.SHA256 != sum || got.SizeBytes != size || got.License != "apache-2.0" {
		t.Fatalf("%+v", got)
	}
	if got.Publisher != "o" || got.Name != "model-q4_k_m" || got.Quantization != "Q4_K_M" || got.ID != entity.AddedID("o/r", "model-q4_k_m.gguf", commit) {
		t.Fatalf("%+v", got)
	}
	// exactly one request, to the API of Hugging Face, and nothing was downloaded
	if len(f.asked) != 1 || f.asked[0] != api {
		t.Fatalf("%v", f.asked)
	}
	// the result is a valid row for a suggestion
	row := got.AddedModel("Career", time.Now())
	if row.Status != entity.AddedStatusSuggested || row.Revision != commit || row.SuggestedBy != "Career" || row.ContextTokens != entity.DefaultAddedContextTokens {
		t.Fatalf("%+v", row)
	}
}

func TestAMovedBranchIsPinnedToTheCommitItPointsToNow(t *testing.T) {
	f := one(answerJSON(func(m map[string]string) { m["sha"] = other }))
	got, err := Resolve(context.Background(), f, req())
	if err != nil || got.Commit != other || strings.Contains(got.URL, "/main/") || !strings.Contains(got.URL, "/resolve/"+other+"/") {
		t.Fatalf("%+v %v", got, err)
	}
	// a revision that is itself a commit must be that commit
	r := req()
	r.Revision = commit
	f2 := &fake{answers: map[string]fakeAnswer{"https://huggingface.co/api/models/o/r/revision/" + commit + "?blobs=true": {200, answerJSON(func(m map[string]string) { m["sha"] = other })}}}
	if _, err := Resolve(context.Background(), f2, r); !errors.Is(err, ErrMismatch) {
		t.Fatalf("a commit that answers as another commit: %v", err)
	}
	// a branch with a slash is escaped in the address
	r.Revision = "refs/pr/1"
	f3 := &fake{answers: map[string]fakeAnswer{"https://huggingface.co/api/models/o/r/revision/refs%2Fpr%2F1?blobs=true": {200, answerJSON(nil)}}}
	if _, err := Resolve(context.Background(), f3, r); err != nil {
		t.Fatalf("%v %v", err, f3.asked)
	}
}

func TestEverythingElseIsRefusedWithItsOwnReason(t *testing.T) {
	cases := []struct {
		name string
		f    *fake
		req  func(*Request)
		want error
	}{
		{"no such repository", &fake{answers: map[string]fakeAnswer{api: {404, ""}}}, nil, ErrNotFound},
		{"needs a login", &fake{answers: map[string]fakeAnswer{api: {401, ""}}}, nil, ErrNotAvailable},
		{"forbidden", &fake{answers: map[string]fakeAnswer{api: {403, ""}}}, nil, ErrNotAvailable},
		{"server error", &fake{answers: map[string]fakeAnswer{api: {500, ""}}}, nil, ErrUnreachable},
		{"not json", one("<html>"), nil, ErrUnreachable},
		{"network error", &fake{err: errors.New("dial")}, nil, ErrUnreachable},
		{"too long an answer", one(strings.Repeat("a", maxAnswerBytes+10)), nil, ErrUnreachable},
		// valid, but longer than any real answer: the limit must refuse it, not only a failure to parse
		{"too long a valid answer", one(answerJSON(func(m map[string]string) {
			m["files"] += strings.Repeat(`,{"rfilename":"x","size":1}`, 400000)
		})), nil, ErrUnreachable},
		{"private", one(answerJSON(func(m map[string]string) { m["private"] = "true" })), nil, ErrNotAvailable},
		{"disabled", one(answerJSON(func(m map[string]string) { m["disabled"] = "true" })), nil, ErrNotAvailable},
		{"gated auto", one(answerJSON(func(m map[string]string) { m["gated"] = `"auto"` })), nil, ErrNotAvailable},
		{"gated manual", one(answerJSON(func(m map[string]string) { m["gated"] = `"manual"` })), nil, ErrNotAvailable},
		{"no commit in the answer", one(answerJSON(func(m map[string]string) { m["sha"] = "main" })), nil, ErrUnreachable},
		{"the file is not there", one(answerJSON(nil)), func(r *Request) { r.File = "other.gguf" }, ErrNotFound},
		{"the file has no hash", one(answerJSON(func(m map[string]string) { m["files"] = `{"rfilename":"model-q4_k_m.gguf","size":5000000}` })), nil, ErrNoChecksum},
		{"a hash that is not one", one(answerJSON(func(m map[string]string) {
			m["files"] = `{"rfilename":"model-q4_k_m.gguf","size":5000000,"lfs":{"sha256":"abc","size":5000000}}`
		})), nil, ErrNoChecksum},
		{"two sizes", one(answerJSON(func(m map[string]string) {
			m["files"] = fmt.Sprintf(`{"rfilename":"model-q4_k_m.gguf","size":5000001,"lfs":{"sha256":"%s","size":5000000}}`, sum)
		})), nil, ErrUnreachable},
		{"no licence", one(answerJSON(func(m map[string]string) { m["card"] = ""; m["tags"] = `["gguf"]` })), nil, ErrNoLicense},
		{"empty licence", one(answerJSON(func(m map[string]string) { m["card"] = `"license":" "`; m["tags"] = `[]` })), nil, ErrNoLicense},
		{"too small", one(answerJSON(func(m map[string]string) {
			m["files"] = fmt.Sprintf(`{"rfilename":"model-q4_k_m.gguf","size":1000,"lfs":{"sha256":"%s","size":1000}}`, sum)
		})), nil, ErrInvalid},
		{"too big", one(answerJSON(func(m map[string]string) {
			m["files"] = fmt.Sprintf(`{"rfilename":"model-q4_k_m.gguf","size":30000000000,"lfs":{"sha256":"%s","size":30000000000}}`, sum)
		})), nil, ErrInvalid},
		{"the tool states another hash", one(answerJSON(nil)), func(r *Request) { r.ExpectSHA256 = strings.Repeat("0", 64) }, ErrMismatch},
		{"the tool states another size", one(answerJSON(nil)), func(r *Request) { r.ExpectSizeBytes = 5 }, ErrMismatch},
	}
	for _, c := range cases {
		r := req()
		if c.req != nil {
			c.req(&r)
		}
		if _, err := Resolve(context.Background(), c.f, r); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}

func TestWhatTheToolStatesIsOnlyComparedNeverUsed(t *testing.T) {
	r := req()
	r.ExpectSHA256, r.ExpectSizeBytes = sum, size
	got, err := Resolve(context.Background(), one(answerJSON(nil)), r)
	if err != nil || got.SHA256 != sum || got.SizeBytes != size {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestTheLicenceComesFromTheCardOrFromATag(t *testing.T) {
	for name, mutate := range map[string]func(map[string]string){
		"a list":     func(m map[string]string) { m["card"] = `"license":["mit","other"]`; m["tags"] = `[]` },
		"only a tag": func(m map[string]string) { m["card"] = ""; m["tags"] = `["gguf","license:mit"]` },
		"the card":   func(m map[string]string) { m["card"] = `"license":"mit"`; m["tags"] = `[]` },
	} {
		got, err := Resolve(context.Background(), one(answerJSON(mutate)), req())
		if err != nil || got.License != "mit" {
			t.Errorf("%s: %q %v", name, got.License, err)
		}
	}
}

func TestOnlyStrictNamesAreAccepted(t *testing.T) {
	bad := []Request{
		{Repo: "", File: "m.gguf"},
		{Repo: "justname", File: "m.gguf"},
		{Repo: "o/r/extra", File: "m.gguf"},
		{Repo: "../r", File: "m.gguf"},
		{Repo: "o/..", File: "m.gguf"},
		{Repo: "o/r with space", File: "m.gguf"},
		{Repo: "o/r?x=1", File: "m.gguf"},
		{Repo: "o/r", File: "sub/m.gguf"},
		{Repo: "o/r", File: "../m.gguf"},
		{Repo: "o/r", File: "m.bin"},
		{Repo: "o/r", File: "m.gguf.exe"},
		{Repo: "o/r", File: ".gguf"},
		{Repo: "o/r", File: "m.gguf", Revision: "../main"},
		{Repo: "o/r", File: "m.gguf", Revision: "a//b"},
		{Repo: "o/r", File: "m.gguf", Revision: "a/./b"},
		{Repo: "o/r", File: "m.gguf", Revision: "a b"},
		{Repo: "o/r", File: "m.gguf", Revision: "a?b"},
		{Repo: "o/r", File: "m.gguf", Revision: strings.Repeat("a", 101)},
		{Repo: "o/r", File: "m.gguf", ExpectSHA256: "xyz"},
		{Repo: "https://evil.example/o", File: "m.gguf"},
	}
	for _, r := range bad {
		f := &fake{}
		if _, err := Resolve(context.Background(), f, r); !errors.Is(err, ErrInvalidRequest) || len(f.asked) != 0 {
			t.Errorf("%+v: %v (asked %v)", r, err, f.asked)
		}
	}
	for _, r := range []Request{{Repo: "o/r", File: "m.gguf"}, {Repo: "Qwen/Qwen2.5-1.5B-Instruct-GGUF", File: "qwen2.5-1.5b-instruct-q4_k_m.gguf", Revision: "main"}, {Repo: "o/r", File: "m.gguf", Revision: "refs/pr/1"}, {Repo: "o/r", File: "m.gguf", Revision: commit}} {
		if err := r.Validate(); err != nil {
			t.Errorf("%+v: %v", r, err)
		}
	}
}

func TestOnlyHttpsOnHuggingFaceMayBeContacted(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://huggingface.co/api/models/o/r/revision/main":   true,
		"http://huggingface.co/api/models/o/r":                  false,
		"https://cdn.huggingface.co/x":                          false,
		"https://huggingface.co.evil.example/x":                 false,
		"https://evil.example/huggingface.co":                   false,
		"https://user:pw@huggingface.co/x":                      false,
		"https://hf.co/x":                                       false,
		"https://xhuggingface.co/x":                             false,
		"https://huggingface.co:8443/api/models/o/r/revision/m": true,
	} {
		u, _ := url.Parse(raw)
		if got := AllowURL(u) == nil; got != want {
			t.Errorf("%s: allowed=%v, want %v", raw, got, want)
		}
	}
	// a redirect to another host is refused by the rule the request carries
	f := &fake{answers: map[string]fakeAnswer{}}
	_, _ = Resolve(context.Background(), f, req())
	if len(f.asked) != 1 || f.opts[0].AllowURL == nil {
		t.Fatalf("the request must carry the rule for itself and for every redirect: %v", f.asked)
	}
	redirect, _ := url.Parse("https://evil.example/x")
	if f.opts[0].AllowURL(redirect) == nil {
		t.Error("a redirect to another host must be refused")
	}
}

func TestTheIdIsStableAndDependsOnTheCommit(t *testing.T) {
	a := entity.AddedID("o/r", "m.gguf", commit)
	if a != entity.AddedID("o/r", "m.gguf", commit) || a == entity.AddedID("o/r", "m.gguf", other) || a == entity.AddedID("o/r", "n.gguf", commit) || a == entity.AddedID("p/r", "m.gguf", commit) {
		t.Fatal(a)
	}
	if !strings.HasPrefix(a, entity.AddedIDPrefix) || len(a) != len(entity.AddedIDPrefix)+12 {
		t.Fatal(a)
	}
}
