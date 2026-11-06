package catalog

import (
	"errors"
	"strings"
	"testing"
)

func TestTheShippedCatalogIsValid(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatalf("the catalog shipped with the program must load: %v", err)
	}
	if len(c.Models) < 2 {
		t.Fatalf("%d models, want at least the two verified ones", len(c.Models))
	}
	m, ok := c.Find("phi-4-mini-instruct-q4_k_m")
	if !ok || m.License != "MIT" || m.ContextTokens != 131072 || m.SizeBytes != 2491874272 {
		t.Errorf("phi entry = %+v (found %v)", m, ok)
	}
	q, ok := c.Find("qwen2.5-1.5b-instruct-q4_k_m")
	if !ok || q.License != "Apache-2.0" || q.ContextTokens != 32768 || q.SizeBytes != 1117320736 {
		t.Errorf("qwen entry = %+v (found %v)", q, ok)
	}
	// Measured in step 8: Qwen called tools natively; Phi-4-mini made 0 of 40 calls natively and 50 of 50 through its own
	// dialect. A change here needs a new measurement.
	if !q.Tools || q.ToolDialect != "" || !m.Tools || m.ToolDialect != "phi-json" {
		t.Errorf("tools: qwen %v/%q phi %v/%q", q.Tools, q.ToolDialect, m.Tools, m.ToolDialect)
	}
	if _, ok := c.Find("nope"); ok {
		t.Error("an unknown id must not be found")
	}
	// Every shipped entry points at the one allowed host and at a pinned file.
	for _, m := range c.Models {
		if !strings.HasPrefix(m.URL, "https://huggingface.co/") || !strings.Contains(m.URL, "/resolve/") {
			t.Errorf("%s: url %q", m.ID, m.URL)
		}
	}
}

const validEntry = `{
  "id": "m1", "name": "Model One", "quantization": "Q4_K_M", "sizeBytes": 1000000000,
  "sha256": "88c00229914083cd112853aab84ed51b87bdf6b9ce42f532d8c85c7c63b1730a",
  "url": "https://huggingface.co/owner/repo/resolve/78eb92a46fc37e6b524df991ed9aca9bc6aa7b80/model.gguf",
  "license": "MIT", "contextTokens": 4096, "source": "owner/repo", "note": "n", "checkedOn": "2026-11-03"
}`

const validRuntime = `{
  "id": "rt1", "name": "llama.cpp b12064 for macOS", "version": "b12064", "os": "darwin", "arch": "arm64", "sizeBytes": 16548551,
  "sha256": "e566137a3c42aa91b3873f1728d8ba2922b2f70cf3cd5735839bdcd481cc1f2f",
  "url": "https://github.com/ggml-org/llama.cpp/releases/download/b12064/llama-b12064-bin-macos-arm64.tar.gz",
  "binary": "llama-b12064/llama-server", "license": "MIT", "source": "ggml-org/llama.cpp release b12064", "checkedOn": "2026-11-03"
}`

func file(entries ...string) string {
	return fileWith(validRuntime, entries...)
}

func fileWith(runtimes string, entries ...string) string {
	return `{"checkedOn": "2026-11-03", "models": [` + strings.Join(entries, ",") + `], "runtimes": [` + runtimes + `]}`
}

func mutateRuntime(old, new string) string { return strings.Replace(validRuntime, old, new, 1) }

func TestShippedRuntimesCoverTheSupportedSystems(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	mac, ok := c.RuntimeFor("darwin", "arm64")
	if !ok || mac.SizeBytes != 16548551 || mac.Version != "b12064" || mac.Binary != "llama-b12064/llama-server" {
		t.Errorf("macOS runtime = %+v (found %v)", mac, ok)
	}
	linux, ok := c.RuntimeFor("linux", "amd64")
	if !ok || linux.SizeBytes != 22934665 || linux.SHA256 != "9032dd278610d0f21fca50ba9e5037b6e4892d1854027572cb3a0aaba2104af3" {
		t.Errorf("linux runtime = %+v (found %v)", linux, ok)
	}
	for _, pair := range [][2]string{{"windows", "amd64"}, {"darwin", "amd64"}, {"linux", "arm64"}, {"", ""}} {
		if _, ok := c.RuntimeFor(pair[0], pair[1]); ok {
			t.Errorf("%v must not have a runtime yet", pair)
		}
	}
}

func TestABrokenRuntimeRefusesTheWholeFile(t *testing.T) {
	cases := map[string]string{
		"an upper-case id":          mutateRuntime(`"id": "rt1"`, `"id": "RT1"`),
		"no name":                   mutateRuntime(`"name": "llama.cpp b12064 for macOS"`, `"name": ""`),
		"a bad version":             mutateRuntime(`"version": "b12064"`, `"version": "12064"`),
		"a windows system":          mutateRuntime(`"os": "darwin"`, `"os": "windows"`),
		"an unknown architecture":   mutateRuntime(`"arch": "arm64"`, `"arch": "riscv"`),
		"a size of zero":            mutateRuntime(`"sizeBytes": 16548551`, `"sizeBytes": 0`),
		"a size over the cap":       mutateRuntime(`"sizeBytes": 16548551`, `"sizeBytes": 1073741825`),
		"a bad hash":                mutateRuntime(`e566137a`, `E566137A`),
		"a foreign host":            mutateRuntime(`https://github.com`, `https://example.com`),
		"plain http":                mutateRuntime(`https://github.com`, `http://github.com`),
		"a look-alike host":         mutateRuntime(`https://github.com`, `https://github.com.evil.example`),
		"credentials":               mutateRuntime(`https://github.com`, `https://u:p@github.com`),
		"another repository":        mutateRuntime(`/ggml-org/llama.cpp/`, `/someone/llama.cpp/`),
		"not an archive":            mutateRuntime(`.tar.gz"`, `.exe"`),
		"a query":                   mutateRuntime(`.tar.gz"`, `.tar.gz?x=1"`),
		"a tag that differs":        mutateRuntime(`download/b12064/`, `download/b12063/`),
		"a file name that differs":  mutateRuntime(`llama-b12064-bin`, `llama-b12063-bin`),
		"a traversal in the path":   mutateRuntime(`/download/b12064/`, `/download/../b12064/`),
		"a binary outside a folder": mutateRuntime(`"binary": "llama-b12064/llama-server"`, `"binary": "llama-server"`),
		"a binary with a traversal": mutateRuntime(`"binary": "llama-b12064/llama-server"`, `"binary": "../llama-server"`),
		"another program":           mutateRuntime(`llama-b12064/llama-server`, `llama-b12064/llama-cli`),
		"no license":                mutateRuntime(`"license": "MIT"`, `"license": ""`),
		"no source":                 mutateRuntime(`"source": "ggml-org/llama.cpp release b12064"`, `"source": ""`),
		"a bad date":                mutateRuntime(`"checkedOn": "2026-11-03"`, `"checkedOn": "yesterday"`),
		"an unknown field":          mutateRuntime(`"license": "MIT"`, `"license": "MIT", "extra": 1`),
	}
	for name, rt := range cases {
		if _, err := Load([]byte(fileWith(rt, validEntry))); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	if _, err := Load([]byte(fileWith("", validEntry))); !errors.Is(err, ErrInvalid) {
		t.Errorf("no runtime at all: err = %v, want ErrInvalid", err)
	}
	two := validRuntime + "," + strings.Replace(validRuntime, `"id": "rt1"`, `"id": "rt2"`, 1)
	if _, err := Load([]byte(fileWith(two, validEntry))); !errors.Is(err, ErrInvalid) {
		t.Errorf("two runtimes for one system: err = %v, want ErrInvalid", err)
	}
	same := validRuntime + "," + strings.Replace(strings.Replace(validRuntime, `"os": "darwin"`, `"os": "linux"`, 1), `"arch": "arm64"`, `"arch": "amd64"`, 1)
	if _, err := Load([]byte(fileWith(same, validEntry))); !errors.Is(err, ErrInvalid) {
		t.Errorf("the same id for two runtimes: err = %v, want ErrInvalid", err)
	}
	clash := strings.Replace(validRuntime, `"id": "rt1"`, `"id": "m1"`, 1)
	if _, err := Load([]byte(fileWith(clash, validEntry))); !errors.Is(err, ErrInvalid) {
		t.Errorf("a runtime with a model's id: err = %v, want ErrInvalid", err)
	}
}

func TestRuntimeSizeBoundsAreInclusive(t *testing.T) {
	for _, size := range []string{"1048576", "1073741824"} {
		if _, err := Load([]byte(fileWith(mutateRuntime(`16548551`, size), validEntry))); err != nil {
			t.Errorf("runtime size %s must be accepted: %v", size, err)
		}
	}
}

func TestAWellFormedFileLoads(t *testing.T) {
	if _, err := Load([]byte(file(validEntry))); err != nil {
		t.Fatal(err)
	}
}

func mutate(old, new string) string { return strings.Replace(validEntry, old, new, 1) }

func TestABrokenFileIsRefusedWholeWithAReason(t *testing.T) {
	cases := map[string]string{
		"not json":                    `{`,
		"trailing data":               file(validEntry) + ` {}`,
		"an unknown field":            file(mutate(`"note": "n"`, `"note": "n", "extra": 1`)),
		"no models":                   file(),
		"a bad file date":             strings.Replace(file(validEntry), `"checkedOn": "2026-11-03", "models"`, `"checkedOn": "2026-13-03", "models"`, 1),
		"a missing file date":         strings.Replace(file(validEntry), `"checkedOn": "2026-11-03", "models"`, `"models"`, 1),
		"an upper-case id":            file(mutate(`"id": "m1"`, `"id": "M1"`)),
		"an empty id":                 file(mutate(`"id": "m1"`, `"id": ""`)),
		"an id with a slash":          file(mutate(`"id": "m1"`, `"id": "a/b"`)),
		"an id that is too long":      file(mutate(`"id": "m1"`, `"id": "`+strings.Repeat("a", 65)+`"`)),
		"no name":                     file(mutate(`"name": "Model One"`, `"name": " "`)),
		"no quantization":             file(mutate(`"quantization": "Q4_K_M"`, `"quantization": ""`)),
		"a name that is too long":     file(mutate(`"name": "Model One"`, `"name": "`+strings.Repeat("n", 101)+`"`)),
		"a size of zero":              file(mutate(`"sizeBytes": 1000000000`, `"sizeBytes": 0`)),
		"a size below the minimum":    file(mutate(`"sizeBytes": 1000000000`, `"sizeBytes": 1000`)),
		"a size above the cap":        file(mutate(`"sizeBytes": 1000000000`, `"sizeBytes": 21474836481`)),
		"a negative size":             file(mutate(`"sizeBytes": 1000000000`, `"sizeBytes": -5`)),
		"an upper-case hash":          file(mutate(`88c00229`, `88C00229`)),
		"a short hash":                file(mutate(`"sha256": "88c00229914083cd112853aab84ed51b87bdf6b9ce42f532d8c85c7c63b1730a"`, `"sha256": "88c0"`)),
		"a non-hex hash":              file(mutate(`88c00229`, `88c0022z`)),
		"no license":                  file(mutate(`"license": "MIT"`, `"license": ""`)),
		"a context below the minimum": file(mutate(`"contextTokens": 4096`, `"contextTokens": 100`)),
		"a context above the maximum": file(mutate(`"contextTokens": 4096`, `"contextTokens": 99999999`)),
		"no source":                   file(mutate(`"source": "owner/repo"`, `"source": ""`)),
		"a too long note":             file(mutate(`"note": "n"`, `"note": "`+strings.Repeat("n", 301)+`"`)),
		"a bad entry date":            file(mutate(`"checkedOn": "2026-11-03"`, `"checkedOn": "03.11.2026"`)),
		"an http address":             file(mutate(`https://huggingface.co`, `http://huggingface.co`)),
		"a foreign host":              file(mutate(`https://huggingface.co`, `https://example.com`)),
		"a look-alike host":           file(mutate(`https://huggingface.co`, `https://huggingface.co.evil.example`)),
		"credentials in the address":  file(mutate(`https://huggingface.co`, `https://u:p@huggingface.co`)),
		"a query in the address":      file(mutate(`model.gguf"`, `model.gguf?download=1"`)),
		"a fragment in the address":   file(mutate(`model.gguf"`, `model.gguf#x"`)),
		"an unpinned branch":          file(mutate(`78eb92a46fc37e6b524df991ed9aca9bc6aa7b80`, `main`)),
		"a short commit":              file(mutate(`78eb92a46fc37e6b524df991ed9aca9bc6aa7b80`, `78eb92a`)),
		"not a gguf file":             file(mutate(`model.gguf`, `model.bin`)),
		"an encoded traversal":        file(mutate(`/owner/repo/`, `/owner/%2e%2e/`)),
		"a hidden file name":          file(mutate(`/model.gguf`, `/.gguf`)),
		"a path with a traversal":     file(mutate(`/owner/repo/`, `/owner/../`)),
		"a duplicate id":              file(validEntry, validEntry),
	}
	for name, raw := range cases {
		_, err := Load([]byte(raw))
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestTheNumericBoundsAreInclusive(t *testing.T) {
	if _, err := Load([]byte(file(mutate(`"name": "Model One"`, `"name": "`+strings.Repeat("n", 100)+`"`)))); err != nil {
		t.Errorf("a name of exactly 100 characters must be accepted: %v", err)
	}
	for _, size := range []string{"1048576", "21474836480"} {
		if _, err := Load([]byte(file(mutate(`1000000000`, size)))); err != nil {
			t.Errorf("size %s must be accepted: %v", size, err)
		}
	}
	for _, ctx := range []string{"512", "4194304"} {
		if _, err := Load([]byte(file(mutate(`4096`, ctx)))); err != nil {
			t.Errorf("context %s must be accepted: %v", ctx, err)
		}
	}
}

func TestTheToolsFlagIsReadAndDefaultsToOff(t *testing.T) {
	on, err := Load([]byte(file(mutate(`"contextTokens": 4096`, `"contextTokens": 4096, "tools": true`))))
	if err != nil || !on.Models[0].Tools {
		t.Fatalf("tools true: %v %+v", err, on.Models)
	}
	off, err := Load([]byte(file(validEntry)))
	if err != nil || off.Models[0].Tools {
		t.Fatalf("no flag must mean no tools: %v %+v", err, off.Models)
	}
}

func TestTheLargerQwenIsPinnedAndNotYetMarkedForTools(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	m, ok := c.Find("qwen2.5-7b-instruct-q4_k_m")
	if !ok || m.License != "Apache-2.0" || m.SizeBytes != 4683074240 || m.ContextTokens != 32768 ||
		m.SHA256 != "65b8fcd92af6b4fefa935c625d1ac27ea29dcb6ee14589c55a8f115ceaaa1423" ||
		!strings.Contains(m.URL, "/bartowski/Qwen2.5-7B-Instruct-GGUF/resolve/8911e8a47f92bac19d6f5c64a2e2095bd2f7d031/") {
		t.Errorf("entry = %+v (found %v)", m, ok)
	}
	if m.Tools {
		t.Error("tools must stay false until the model is measured on the CV import prompt")
	}
}

func TestTheQwenCoder3BIsPinnedAndMeasuredForToolsThroughItsOwnDialect(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	m, ok := c.Find("qwen2.5-coder-3b-instruct-q4_k_m")
	if !ok || m.License != "Qwen-Research" || m.SizeBytes != 2104932800 || m.ContextTokens != 32768 ||
		m.SHA256 != "724fb256bec1ff062b2f65e4569e871ad2e95ab2a3989723d1769c54294730b7" ||
		!strings.Contains(m.URL, "/Qwen/Qwen2.5-Coder-3B-Instruct-GGUF/resolve/f74adce6aa16316c625447af059dbebe4983757c/") {
		t.Errorf("entry = %+v (found %v)", m, ok)
	}
	// Measured 2026-11-06 on the real Katana recipes (plan 196): it writes calls as text, so it needs a dialect that reads them.
	if !m.Tools || m.ToolDialect != "qwen-json" {
		t.Errorf("tools %v dialect %q: the model was measured to call tools only through its text-reading dialect", m.Tools, m.ToolDialect)
	}
}

func TestAToolDialectIsAWellFormedNameAndBelongsToAModelWithTools(t *testing.T) {
	good := file(mutate(`"contextTokens": 4096`, `"contextTokens": 4096, "tools": true, "toolDialect": "phi-json"`))
	if c, err := Load([]byte(good)); err != nil || c.Models[0].ToolDialect != "phi-json" {
		t.Fatalf("a known dialect: %v", err)
	}
	withTools := func(dialect string) string {
		return file(mutate(`"contextTokens": 4096`, `"contextTokens": 4096, "tools": true, "toolDialect": "`+dialect+`"`))
	}
	// Which dialects exist is the module's business (a missing one means no tools), the catalog only checks the shape.
	if _, err := Load([]byte(withTools("made-up"))); err != nil {
		t.Errorf("a well-formed name the catalog does not know is not its business: %v", err)
	}
	for name, raw := range map[string]string{
		"a dialect without tools": file(mutate(`"contextTokens": 4096`, `"contextTokens": 4096, "toolDialect": "phi-json"`)),
		"upper case":              withTools("Phi-JSON"),
		"a space":                 withTools("phi json"),
		"a leading hyphen":        withTools("-phi"),
		"a double hyphen":         withTools("phi--json"),
		"too long":                withTools("a" + strings.Repeat("b", 32)),
	} {
		if _, err := Load([]byte(raw)); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
}

func TestTheCacheSizePerTokenIsStatedForEveryShippedModelAndBounded(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	// layers x KV heads x head size x 2 x 2 bytes, from each model's own config.json (read 2026-11-03)
	want := map[string]int64{"phi-4-mini-instruct-q4_k_m": 32 * 8 * 128 * 4, "qwen2.5-1.5b-instruct-q4_k_m": 28 * 2 * 128 * 4, "qwen2.5-7b-instruct-q4_k_m": 28 * 4 * 128 * 4, "qwen2.5-coder-3b-instruct-q4_k_m": 36 * 2 * 128 * 4}
	for _, m := range c.Models {
		if m.KVBytesPerToken != want[m.ID] {
			t.Errorf("%s: kvBytesPerToken %d, want %d", m.ID, m.KVBytesPerToken, want[m.ID])
		}
	}
	with := func(v string) string {
		return file(mutate(`"contextTokens": 4096`, `"contextTokens": 4096, "kvBytesPerToken": `+v))
	}
	for _, v := range []string{"0", "1", "131072", "4194304"} {
		if _, err := Load([]byte(with(v))); err != nil {
			t.Errorf("kvBytesPerToken %s must load: %v", v, err)
		}
	}
	for _, v := range []string{"-1", "4194305"} {
		if _, err := Load([]byte(with(v))); err == nil {
			t.Errorf("kvBytesPerToken %s must be refused", v)
		}
	}
	if m, err := Load([]byte(file(validEntry))); err != nil || m.Models[0].KVBytesPerToken != 0 {
		t.Errorf("an entry without the field means not stated (0): %v", err)
	}
}
