// Package nativeconfig reads the address of Cinqo's own built-in tool repository
// from the app-owned config/tool-repository.yaml — see
// plan/ai/tools/repository/native-repository.md.
//
// The file is app-owned on purpose: the packaged app replaces the config folder
// from its embedded copy at every launch, so the address is fixed for the people
// who use the app and is changed at build time, never from the app or from
// config.json. The key is "tool_repository_url". Absent (or no file) means the
// built-in default; present and empty means no native repository; present and
// invalid is ignored with a warning, never an error: a bad value must not stop
// the app from starting, and must not silently turn into the default either,
// which would hide a typo.
package nativeconfig

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/a-digi/cinqo/src/toolrepository/service"
)

// File is the config file's name, inside the config folder.
const File = "tool-repository.yaml"

// Key is the YAML key that holds the address.
const Key = "tool_repository_url"

// DefaultURL is used when the file has no Key.
const DefaultURL = "https://raw.githubusercontent.com/a-digi/cinqo/0.0.1/tools/repository.json"

// Result is what the config says about the native repository.
type Result struct {
	// URL is the normalised link, empty when there is no native repository.
	URL string
	// Warning is set when something was wrong and ignored; the caller logs it.
	Warning string
}

// Enabled reports whether there is a native repository.
func (r Result) Enabled() bool { return r.URL != "" }

// Load reads File through read (config.ReadConfigFile in the app). A missing
// file means the default; an unreadable one falls back to it with a warning.
func Load(read func(name string) ([]byte, error)) Result {
	data, err := read(File)
	if err != nil {
		return Result{URL: DefaultURL}
	}
	res := Parse(data)
	if res.Warning != "" {
		res.Warning = File + ": " + res.Warning
	}
	return res
}

// Parse interprets the file's bytes; nil data gives the default, unparseable
// data gives the default with a warning.
func Parse(data []byte) Result {
	if data == nil {
		return Result{URL: DefaultURL}
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Result{URL: DefaultURL, Warning: "could not parse the config, using the default tool repository"}
	}
	raw, present := cfg[Key]
	if !present {
		return Result{URL: DefaultURL}
	}
	value, isString := raw.(string)
	if !isString {
		return Result{Warning: Key + " must be a string; no native tool repository"}
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return Result{}
	}
	link, err := service.NormalizeLink(value)
	if err != nil {
		return Result{Warning: fmt.Sprintf("%s %q is not usable (%v); no native tool repository", Key, value, err)}
	}
	return Result{URL: link}
}
