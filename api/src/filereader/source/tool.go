package source

import (
	"regexp"
	"strings"
)

const (
	maxSegments = 6
	// ToolPrefix starts the id of every installed tool's source: "tool:<slug>:<id>".
	ToolPrefix = "tool:"
)

// segment is one part of a path inside a tool's data folder: it starts with a letter or digit (so never "." or
// ".." and never a hidden file) and holds only letters, digits, dots, dashes and underscores.
var segment = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// toolSource serves the files under one data folder of one installed tool, to people holding the one scope the
// tool declared for it. A file is named by a path relative to the folder, up to six segments deep.
type toolSource struct {
	id    string
	scope string
	root  string
}

// NewToolSource builds the source for a tool's declared file source. root is the tool's own data folder,
// computed by core (never taken from the manifest text).
func NewToolSource(slug, id, scope, root string) Source {
	return toolSource{id: ToolPrefix + slug + ":" + id, scope: scope, root: root}
}

func (t toolSource) ID() string       { return t.id }
func (t toolSource) Scopes() []string { return []string{t.scope} }

func (t toolSource) Resolve(ref string) (string, error) {
	parts := strings.Split(ref, "/")
	if len(parts) > maxSegments {
		return "", ErrNotFound
	}
	for _, p := range parts {
		if !segment.MatchString(p) {
			return "", ErrNotFound
		}
	}
	return ResolveUnder(t.root, parts...)
}
