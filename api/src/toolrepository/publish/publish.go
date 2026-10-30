// Package publish builds a tool repository file from the packaged tools in
// tools/*/versions/ — see plan/ai/tools/repository/tool-repositories.md. It reads
// each package's own manifest.json, so the id and version in the repository can
// never disagree with the package; the SHA-256 and size come from the zip itself.
// The result is checked with the same validator the app uses, so a file this
// package writes is one the app accepts.
package publish

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/a-digi/cinqo/src/tool/manifest"
	"github.com/a-digi/cinqo/src/toolrepository/catalog"
)

// FilesDir is the folder, next to the repository file, that holds the packages.
const FilesDir = "files"

// FileName is the default name of the repository file.
const FileName = "repository.json"

var packageName = regexp.MustCompile(`^tool-([a-z][a-z0-9_]*)-(darwin|linux|windows)-(arm64|amd64)\.zip$`)

type Options struct {
	// ToolsDir holds one folder per tool, each with a versions/ folder of zips.
	ToolsDir string
	// OutDir receives the repository file and, in FilesDir, a copy of every package.
	OutDir string
	// FileName of the repository file inside OutDir; defaults to FileName.
	FileName    string
	Name        string
	Description string
	// BaseURL, when set, makes package URLs absolute: BaseURL/files/<zip>. Left
	// empty they are relative to the repository file, so the folder can be hosted
	// anywhere.
	BaseURL string
	// InPlace writes the repository file into ToolsDir itself and links each
	// package where it already is (<tool>/versions/<zip>), copying nothing. Serve
	// ToolsDir as a static folder and the file works as it stands. OutDir is
	// ignored.
	InPlace bool
	// AssetsURL, when set, makes every package link AssetsURL/<zip> — the
	// package files served flat from one address, such as a GitHub release's
	// download address (https://github.com/<owner>/<repo>/releases/download/<tag>).
	// It takes precedence over BaseURL. Package file names are unique per tool,
	// version-independent and platform-specific, so they work as release assets.
	AssetsURL string
}

// Result describes what was written.
type Result struct {
	File     string
	Catalog  *catalog.Catalog
	Packages int
}

type found struct {
	zipPath string
	// rel is zipPath relative to the tools folder, slash-separated.
	rel  string
	file string
	os   string
	arch string
	m    manifest.Manifest
	sum  string
	size int64
	date string
}

// Generate writes the repository file and copies the packages next to it.
func Generate(opts Options) (*Result, error) {
	if opts.FileName == "" {
		opts.FileName = FileName
	}
	if opts.InPlace {
		opts.OutDir = opts.ToolsDir
	}
	pkgs, err := scan(opts.ToolsDir)
	if err != nil {
		return nil, err
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found: expected tools/<tool>/versions/tool-<tool>-<os>-<arch>.zip under %s", opts.ToolsDir)
	}

	for i := range pkgs {
		rel, err := filepath.Rel(opts.ToolsDir, pkgs[i].zipPath)
		if err != nil {
			return nil, err
		}
		pkgs[i].rel = filepath.ToSlash(rel)
	}

	c := build(pkgs, opts)
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')

	// The file must be one the app accepts; check it with the app's own parser.
	validatedAs := "https://example.invalid/" + opts.FileName
	if _, err := catalog.Parse(data, validatedAs); err != nil {
		return nil, fmt.Errorf("generated file is not a valid repository: %w", err)
	}

	if !opts.InPlace {
		if err := os.MkdirAll(filepath.Join(opts.OutDir, FilesDir), 0o755); err != nil {
			return nil, err
		}
		for _, p := range pkgs {
			if err := copyFile(p.zipPath, filepath.Join(opts.OutDir, FilesDir, p.file)); err != nil {
				return nil, err
			}
		}
	}
	out := filepath.Join(opts.OutDir, opts.FileName)
	if err := os.WriteFile(out, data, 0o644); err != nil {
		return nil, err
	}
	return &Result{File: out, Catalog: c, Packages: len(pkgs)}, nil
}

func scan(toolsDir string) ([]found, error) {
	dirs, err := os.ReadDir(toolsDir)
	if err != nil {
		return nil, err
	}
	var out []found
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		versions := filepath.Join(toolsDir, d.Name(), "versions")
		entries, err := os.ReadDir(versions)
		if err != nil {
			continue // a tool without packaged versions is simply not published
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".zip") {
				continue
			}
			p, err := inspect(filepath.Join(versions, e.Name()))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", filepath.Join(d.Name(), "versions", e.Name()), err)
			}
			out = append(out, p)
		}
	}
	return out, nil
}

func inspect(zipPath string) (found, error) {
	name := filepath.Base(zipPath)
	match := packageName.FindStringSubmatch(name)
	if match == nil {
		return found{}, fmt.Errorf("the file name must look like tool-<tool>-<os>-<arch>.zip (os darwin, linux or windows; arch arm64 or amd64)")
	}
	slug, goos, goarch := match[1], match[2], match[3]

	info, err := os.Stat(zipPath)
	if err != nil {
		return found{}, err
	}
	raw, err := os.ReadFile(zipPath)
	if err != nil {
		return found{}, err
	}
	m, err := readManifest(raw)
	if err != nil {
		return found{}, err
	}
	if m.Slug != slug {
		return found{}, fmt.Errorf("the file name says tool %q but its manifest says %q", slug, m.Slug)
	}
	sum := sha256.Sum256(raw)
	return found{
		zipPath: zipPath, file: name, os: goos, arch: goarch, m: m,
		sum: hex.EncodeToString(sum[:]), size: int64(len(raw)), date: info.ModTime().UTC().Format("2006-01-02"),
	}, nil
}

func readManifest(raw []byte) (manifest.Manifest, error) {
	zr, err := zip.NewReader(strings.NewReader(string(raw)), int64(len(raw)))
	if err != nil {
		return manifest.Manifest{}, fmt.Errorf("not a valid zip file: %w", err)
	}
	for _, f := range zr.File {
		if f.Name != "manifest.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return manifest.Manifest{}, err
		}
		defer rc.Close()
		b, err := io.ReadAll(io.LimitReader(rc, 1<<20))
		if err != nil {
			return manifest.Manifest{}, err
		}
		return manifest.Parse(b)
	}
	return manifest.Manifest{}, fmt.Errorf("the package has no manifest.json")
}

func build(pkgs []found, opts Options) *catalog.Catalog {
	byTool := map[string]map[string][]found{} // slug -> version -> packages
	for _, p := range pkgs {
		if byTool[p.m.Slug] == nil {
			byTool[p.m.Slug] = map[string][]found{}
		}
		byTool[p.m.Slug][p.m.Version] = append(byTool[p.m.Slug][p.m.Version], p)
	}

	slugs := make([]string, 0, len(byTool))
	for s := range byTool {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)

	c := &catalog.Catalog{FormatVersion: catalog.FormatVersion, Name: opts.Name, Description: opts.Description}
	for _, slug := range slugs {
		versions := make([]string, 0, len(byTool[slug]))
		for v := range byTool[slug] {
			versions = append(versions, v)
		}
		sort.Slice(versions, func(i, j int) bool { return less(versions[i], versions[j]) })

		tool := catalog.Tool{ID: slug, CurrentVersionID: versions[len(versions)-1]}
		for _, v := range versions {
			group := byTool[slug][v]
			sort.Slice(group, func(i, j int) bool { return group[i].file < group[j].file })
			first := group[0]
			ver := catalog.Version{
				VersionID: v, Name: first.m.Name, Version: v,
				ReleasedAt: latestDate(group), MinAppVersion: first.m.MinAppVersion,
			}
			for _, p := range group {
				ver.Packages = append(ver.Packages, catalog.Package{
					OS: p.os, Arch: p.arch, URL: packageURL(opts, p), SHA256: p.sum, SizeBytes: p.size,
				})
			}
			tool.Versions = append(tool.Versions, ver)
		}
		c.Tools = append(c.Tools, tool)
	}
	return c
}

func packageURL(opts Options, p found) string {
	if opts.AssetsURL != "" {
		return strings.TrimRight(opts.AssetsURL, "/") + "/" + p.file
	}
	rel := path.Join(FilesDir, p.file)
	if opts.InPlace {
		rel = p.rel
	}
	if opts.BaseURL == "" {
		return rel
	}
	return strings.TrimRight(opts.BaseURL, "/") + "/" + rel
}

func latestDate(group []found) string {
	d := group[0].date
	for _, p := range group[1:] {
		if p.date > d {
			d = p.date
		}
	}
	return d
}

func less(a, b string) bool {
	cmp, err := manifest.CompareVersions(a, b)
	if err != nil {
		return a < b
	}
	return cmp < 0
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
