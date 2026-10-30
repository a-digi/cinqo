package nativeconfig

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeyAbsentUsesTheDefault(t *testing.T) {
	for _, data := range []string{"", "other: 1\n", "# only a comment\n", "{}"} {
		r := Parse([]byte(data))
		if data == "" {
			r = Parse([]byte(data))
		}
		if r.URL != DefaultURL || r.Warning != "" || !r.Enabled() {
			t.Errorf("%q: %+v", data, r)
		}
	}
	if DefaultURL != "https://raw.githubusercontent.com/a-digi/cinqo/0.0.1/tools/repository.json" {
		t.Error("the default changed; update native-repository.md and config/tool-repository.yaml")
	}
}

func TestEmptyValueMeansNoNativeRepository(t *testing.T) {
	for _, v := range []string{`""`, `"   "`, `''`} {
		r := Parse([]byte("tool_repository_url: " + v + "\n"))
		if r.Enabled() || r.Warning != "" {
			t.Errorf("%s: %+v", v, r)
		}
	}
}

func TestValidValueIsUsedAndNormalised(t *testing.T) {
	cases := map[string]string{
		`"https://example.com/cinqo/tools.json"`:          "https://example.com/cinqo/tools.json",
		`"  https://example.com/cinqo/tools.json#frag  "`: "https://example.com/cinqo/tools.json",
		`https://example.com/plain/unquoted.json`:         "https://example.com/plain/unquoted.json",
		`"http://localhost:8099/repository.json"`:         "http://localhost:8099/repository.json",
		`"http://192.168.1.10/tools/repository.json"`:     "http://192.168.1.10/tools/repository.json",
		`"https://example.com/any-name.txt?token=1"`:      "https://example.com/any-name.txt?token=1",
	}
	for in, want := range cases {
		r := Parse([]byte("tool_repository_url: " + in + "\n"))
		if r.URL != want || r.Warning != "" {
			t.Errorf("%s: %+v", in, r)
		}
	}
}

func TestInvalidValueIsIgnoredWithAWarningAndNotReplacedByTheDefault(t *testing.T) {
	for _, v := range []string{
		`"example.com/tools.json"`, `"/tmp/tools.json"`, `"file:///tmp/tools.json"`, `"ftp://example.com/tools.json"`,
		`"http://example.com/tools.json"`, `"https://user:pw@example.com/tools.json"`,
		`"https://example.com/"`, `"https://example.com"`, `"https://example.com/folder/"`,
		`42`, `true`, `[x]`, `~`, `null`,
	} {
		r := Parse([]byte("tool_repository_url: " + v + "\n"))
		if r.Enabled() {
			t.Errorf("%s: must not be enabled (and must not fall back to the default): %+v", v, r)
		}
		if r.Warning == "" || !strings.Contains(r.Warning, Key) {
			t.Errorf("%s: want a warning naming the key, got %q", v, r.Warning)
		}
	}
}

func TestUnparseableFileFallsBackToTheDefaultWithAWarning(t *testing.T) {
	for _, data := range []string{"tool_repository_url: [", ":\n  - :", "\t\tbad: : :"} {
		r := Parse([]byte(data))
		if r.URL != DefaultURL || r.Warning == "" {
			t.Errorf("%q: %+v", data, r)
		}
	}
}

func TestLoadUsesTheReaderAndNamesTheFileInWarnings(t *testing.T) {
	read := func(content string, err error) func(string) ([]byte, error) {
		return func(name string) ([]byte, error) {
			if name != File {
				t.Errorf("read %q, want %q", name, File)
			}
			return []byte(content), err
		}
	}
	if r := Load(read("tool_repository_url: https://example.com/r.json\n", nil)); r.URL != "https://example.com/r.json" || r.Warning != "" {
		t.Errorf("%+v", r)
	}
	r := Load(read("tool_repository_url: nope\n", nil))
	if r.Enabled() || !strings.Contains(r.Warning, File) {
		t.Errorf("the warning should name the file: %+v", r)
	}
	if r := Load(read("", errors.New("no such file"))); r.URL != DefaultURL || r.Warning != "" {
		t.Errorf("a missing file uses the default: %+v", r)
	}
}

// The shipped file carries the default, and the user-editable root config files
// no longer carry any such key: the address is not something a user changes.
func TestShippedConfigCarriesTheDefaultAndRootConfigsDoNot(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "config", File))
	if err != nil {
		t.Fatal(err)
	}
	if r := Parse(raw); r.URL != DefaultURL || r.Warning != "" {
		t.Errorf("%+v", r)
	}
	if !strings.Contains(string(raw), Key+":") {
		t.Errorf("%s must spell out %s", File, Key)
	}
	for _, rel := range []string{
		filepath.Join("..", "..", "..", "config.json"),
		filepath.Join("..", "..", "..", "cmd", "app", "embedded-config.json"),
	} {
		b, err := os.ReadFile(rel)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), Key) {
			t.Errorf("%s must not carry %s: users edit it, the address is app-owned", rel, Key)
		}
	}
}
