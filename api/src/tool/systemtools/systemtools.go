// Package systemtools loads the optional system-tools.yaml config —
// the sole source of truth for which installed tools are protected
// from deletion. Never persisted to the database: a DB-cached copy of
// a config-file fact could drift from the file, so the flag is always
// computed fresh from this config on every read instead. Deliberately
// free of any DB dependency, same idiom as the manifest package. See
// plan/ai/tools/step-10-system-tools.md.
package systemtools

import "gopkg.in/yaml.v3"

// Entry is one protected tool. Reason is optional free text, surfaced
// in a blocked delete's error response.
type Entry struct {
	Slug   string `yaml:"slug"`
	Reason string `yaml:"reason,omitempty"`
}

// Config is the parsed system-tools.yaml. The zero value (no entries)
// means nothing is protected — the correct default when the file is
// absent or fails to parse.
type Config struct {
	SystemTools []Entry `yaml:"system_tools,omitempty"`
}

// Load parses raw system-tools.yaml bytes.
func Load(data []byte) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// IsSystemTool reports whether slug is protected, and its configured
// reason (empty if none given or not protected).
func (c Config) IsSystemTool(slug string) (protected bool, reason string) {
	for _, e := range c.SystemTools {
		if e.Slug == slug {
			return true, e.Reason
		}
	}
	return false, ""
}
