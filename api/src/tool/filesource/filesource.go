// Package filesource connects installed tools to the file reader: it finds the file sources a tool declared in
// its manifest.json ("file_sources") and turns each into a reader source, but only while the tool is enabled.
// See plan/ai/file-reader/file-reader-engine.md, step 7.
//
// The declaration is read from the tool's manifest.json on disk, so an update takes effect at once and nothing
// is cached in the database that could go stale. The manifest was validated at install; what is read here is
// re-checked anyway (id shape, folder name, scope declared by this tool), and the folder itself is never taken
// from the manifest: core builds it from the data directory and the tool's slug.
package filesource

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/a-digi/cinqo/src/filereader/source"
	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
	"github.com/a-digi/cinqo/src/tool/manifest"
)

// Finder looks an installed tool up by slug (the tool query repository satisfies it).
type Finder interface {
	FindBySlug(slug string) (*tool_entity.Tool, error)
}

var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// maxManifestBytes bounds how much of a manifest is read.
const maxManifestBytes = 1 << 20

type cached struct {
	modTime time.Time
	size    int64
	m       manifest.Manifest
}

type resolver struct {
	finder  Finder
	dataDir string

	mu    sync.Mutex
	cache map[string]cached
}

// NewResolver returns the resolver for ids of the form "tool:<slug>:<id>"; dataDir is the app's data directory.
func NewResolver(finder Finder, dataDir string) source.Resolver {
	r := &resolver{finder: finder, dataDir: dataDir, cache: map[string]cached{}}
	return r.resolve
}

func (r *resolver) resolve(id string) (source.Source, bool) {
	parts := strings.Split(id, ":")
	if len(parts) != 3 || parts[0]+":" != source.ToolPrefix || !slugPattern.MatchString(parts[1]) || !manifest.ValidFileSourceID(parts[2]) {
		return nil, false
	}
	slug, sourceID := parts[1], parts[2]

	t, err := r.finder.FindBySlug(slug)
	if err != nil || t == nil || !t.Enabled || t.Slug != slug {
		return nil, false
	}
	m, ok := r.manifestOf(t)
	if !ok {
		return nil, false
	}
	for _, d := range m.FileSources {
		if d.ID != sourceID {
			continue
		}
		// re-check what was validated at install, since this file is read at request time
		if err := manifest.ValidateFileSources(manifest.Manifest{Scopes: m.Scopes, FileSources: []manifest.FileSourceDecl{d}}); err != nil {
			return nil, false
		}
		root := filepath.Join(r.dataDir, d.Dir, "tools", slug)
		return source.NewToolSource(slug, d.ID, d.Scope, root), true
	}
	return nil, false
}

// manifestOf reads the tool's manifest.json, caching it until the file's size or modification time changes.
func (r *resolver) manifestOf(t *tool_entity.Tool) (manifest.Manifest, bool) {
	path := filepath.Join(t.InstallPath, "manifest.json")
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() || st.Size() > maxManifestBytes {
		return manifest.Manifest{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.cache[path]; ok && c.size == st.Size() && c.modTime.Equal(st.ModTime()) {
		return c.m, true
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return manifest.Manifest{}, false
	}
	m, err := manifest.Parse(raw)
	if err != nil || m.Slug != t.Slug {
		return manifest.Manifest{}, false
	}
	r.cache[path] = cached{modTime: st.ModTime(), size: st.Size(), m: m}
	return m, true
}
