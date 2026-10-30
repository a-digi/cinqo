// Package catalog is the tool repository file format (version 1): the types,
// parsing, validation and the platform package picker — see
// plan/ai/tools/repository/tool-repositories.md and FORMAT.md next to this file.
// Deliberately pure: no network, no filesystem, no database. Everything that
// needs I/O (fetching the file, refusing unsafe addresses) lives elsewhere.
package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

const (
	FormatVersion = 1

	MaxFileBytes        = 2 << 20
	MaxTools            = 200
	MaxVersionsPerTool  = 200
	MaxPackagesPerVer   = 20
	MaxNameLen          = 100
	MaxDescriptionLen   = 2000
	MaxChangelogLen     = 5000
	MaxVersionIDLen     = 100
	MaxURLLen           = 2000
	MaxPackageSizeBytes = 200 << 20

	Any = "any"
)

var (
	toolIDPattern    = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	versionIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	semverPattern    = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	sha256Pattern    = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
	datePattern      = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(T[0-9:.]+(Z|[+-]\d{2}:\d{2}))?$`)

	validOS   = map[string]bool{"darwin": true, "linux": true, "windows": true, Any: true}
	validArch = map[string]bool{"arm64": true, "amd64": true, Any: true}
)

type Catalog struct {
	FormatVersion int    `json:"formatVersion"`
	Name          string `json:"name,omitempty"`
	Description   string `json:"description,omitempty"`
	Tools         []Tool `json:"tools"`
}

type Tool struct {
	ID               string    `json:"id"`
	CurrentVersionID string    `json:"currentVersionId"`
	Versions         []Version `json:"versions"`
}

type Version struct {
	VersionID     string    `json:"versionId"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	Version       string    `json:"version"`
	ReleasedAt    string    `json:"releasedAt,omitempty"`
	Changelog     string    `json:"changelog,omitempty"`
	MinAppVersion string    `json:"minAppVersion,omitempty"`
	Packages      []Package `json:"packages"`
}

type Package struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
}

// Problem is one thing wrong with a repository file. Path locates it, for
// example `tools[2].versions[0].packages[1].sha256`.
type Problem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (p Problem) String() string {
	if p.Path == "" {
		return p.Message
	}
	return p.Path + ": " + p.Message
}

// ValidationError carries every problem found, so a publisher can fix them
// all in one go; a file with any problem is rejected whole.
type ValidationError struct {
	Problems []Problem
}

func (e *ValidationError) Error() string {
	parts := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		parts[i] = p.String()
	}
	return fmt.Sprintf("invalid repository file (%d problems): %s", len(parts), strings.Join(parts, "; "))
}

// Tool returns the tool with the given id.
func (c *Catalog) Tool(id string) (*Tool, bool) {
	for i := range c.Tools {
		if c.Tools[i].ID == id {
			return &c.Tools[i], true
		}
	}
	return nil, false
}

// Version returns the version with the given versionId.
func (t *Tool) Version(versionID string) (*Version, bool) {
	for i := range t.Versions {
		if t.Versions[i].VersionID == versionID {
			return &t.Versions[i], true
		}
	}
	return nil, false
}

// Current returns the version named by currentVersionId.
func (t *Tool) Current() (*Version, bool) { return t.Version(t.CurrentVersionID) }

// PickPackage chooses the package for an OS and architecture, most specific
// first: exact, then any-arch, then any-OS, then any/any.
func (v *Version) PickPackage(goos, goarch string) (*Package, bool) {
	for _, want := range [][2]string{{goos, goarch}, {goos, Any}, {Any, goarch}, {Any, Any}} {
		for i := range v.Packages {
			if v.Packages[i].OS == want[0] && v.Packages[i].Arch == want[1] {
				return &v.Packages[i], true
			}
		}
	}
	return nil, false
}

// Parse decodes and validates a repository file. baseURL is the address the
// file was fetched from, used to resolve relative package URLs; resolved
// absolute URLs and lower-cased checksums are written into the result. Unknown
// fields are ignored. The error is a *ValidationError listing every problem.
func Parse(data []byte, baseURL string) (*Catalog, error) {
	if len(data) > MaxFileBytes {
		return nil, single("", fmt.Sprintf("file is larger than %d bytes", MaxFileBytes))
	}
	var c Catalog
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&c); err != nil {
		return nil, single("", "not a valid repository file: "+jsonProblem(err))
	}
	if dec.More() {
		return nil, single("", "not a valid repository file: unexpected data after the JSON document")
	}
	if probs := validate(&c, baseURL); len(probs) > 0 {
		return nil, &ValidationError{Problems: probs}
	}
	return &c, nil
}

func jsonProblem(err error) string {
	var ute *json.UnmarshalTypeError
	if errors.As(err, &ute) {
		return fmt.Sprintf("field %q must be of type %s", ute.Field, ute.Type)
	}
	var se *json.SyntaxError
	if errors.As(err, &se) {
		return fmt.Sprintf("JSON syntax error at byte %d", se.Offset)
	}
	return "unreadable JSON"
}

func single(path, msg string) *ValidationError {
	return &ValidationError{Problems: []Problem{{Path: path, Message: msg}}}
}

type collector struct{ probs []Problem }

func (c *collector) add(path, format string, args ...any) {
	c.probs = append(c.probs, Problem{Path: path, Message: fmt.Sprintf(format, args...)})
}

func (c *collector) text(path, value string, max int, required bool) {
	n := len([]rune(value))
	switch {
	case required && strings.TrimSpace(value) == "":
		c.add(path, "is required")
	case n > max:
		c.add(path, "is longer than %d characters", max)
	}
}

func validate(c *Catalog, baseURL string) []Problem {
	col := &collector{}
	if c.FormatVersion != FormatVersion {
		col.add("formatVersion", "must be %d", FormatVersion)
	}
	col.text("name", c.Name, MaxNameLen, false)
	col.text("description", c.Description, MaxDescriptionLen, false)
	if len(c.Tools) > MaxTools {
		col.add("tools", "has %d entries, at most %d are allowed", len(c.Tools), MaxTools)
		return col.probs
	}
	seen := map[string]bool{}
	for i := range c.Tools {
		validateTool(col, &c.Tools[i], fmt.Sprintf("tools[%d]", i), baseURL, seen)
	}
	return col.probs
}

func validateTool(col *collector, t *Tool, path, baseURL string, seen map[string]bool) {
	switch {
	case !toolIDPattern.MatchString(t.ID):
		col.add(path+".id", "must match ^[a-z][a-z0-9_]*$")
	case seen[t.ID]:
		col.add(path+".id", "duplicate tool id %q", t.ID)
	default:
		seen[t.ID] = true
	}
	if len(t.Versions) == 0 {
		col.add(path+".versions", "needs at least one version")
	}
	if len(t.Versions) > MaxVersionsPerTool {
		col.add(path+".versions", "has %d entries, at most %d are allowed", len(t.Versions), MaxVersionsPerTool)
		return
	}
	ids, vers := map[string]bool{}, map[string]bool{}
	for i := range t.Versions {
		v := &t.Versions[i]
		vp := fmt.Sprintf("%s.versions[%d]", path, i)
		switch {
		case len(v.VersionID) > MaxVersionIDLen || !versionIDPattern.MatchString(v.VersionID):
			col.add(vp+".versionId", "must be 1-%d characters of letters, digits, '.', '_' or '-'", MaxVersionIDLen)
		case ids[v.VersionID]:
			col.add(vp+".versionId", "duplicate versionId %q", v.VersionID)
		default:
			ids[v.VersionID] = true
		}
		validateVersion(col, v, vp, baseURL, vers)
	}
	if t.CurrentVersionID == "" {
		col.add(path+".currentVersionId", "is required")
	} else if !ids[t.CurrentVersionID] {
		col.add(path+".currentVersionId", "%q is not one of the versionIds", t.CurrentVersionID)
	}
}

func validateVersion(col *collector, v *Version, path, baseURL string, vers map[string]bool) {
	col.text(path+".name", v.Name, MaxNameLen, true)
	col.text(path+".description", v.Description, MaxDescriptionLen, false)
	col.text(path+".changelog", v.Changelog, MaxChangelogLen, false)
	switch {
	case !semverPattern.MatchString(v.Version):
		col.add(path+".version", "must look like MAJOR.MINOR.PATCH")
	case vers[v.Version]:
		col.add(path+".version", "duplicate version %q", v.Version)
	default:
		vers[v.Version] = true
	}
	if v.MinAppVersion != "" && !semverPattern.MatchString(v.MinAppVersion) {
		col.add(path+".minAppVersion", "must look like MAJOR.MINOR.PATCH")
	}
	if v.ReleasedAt != "" && !datePattern.MatchString(v.ReleasedAt) {
		col.add(path+".releasedAt", "must be a date (2026-10-30) or an RFC 3339 timestamp")
	}
	if len(v.Packages) == 0 {
		col.add(path+".packages", "needs at least one package")
	}
	if len(v.Packages) > MaxPackagesPerVer {
		col.add(path+".packages", "has %d entries, at most %d are allowed", len(v.Packages), MaxPackagesPerVer)
		return
	}
	platforms := map[string]bool{}
	for i := range v.Packages {
		p := &v.Packages[i]
		pp := fmt.Sprintf("%s.packages[%d]", path, i)
		if !validOS[p.OS] {
			col.add(pp+".os", "must be darwin, linux, windows or any")
		}
		if !validArch[p.Arch] {
			col.add(pp+".arch", "must be arm64, amd64 or any")
		}
		if validOS[p.OS] && validArch[p.Arch] {
			key := p.OS + "/" + p.Arch
			if platforms[key] {
				col.add(pp, "duplicate package for %s", key)
			}
			platforms[key] = true
		}
		if !sha256Pattern.MatchString(p.SHA256) {
			col.add(pp+".sha256", "must be 64 hexadecimal characters")
		} else {
			p.SHA256 = strings.ToLower(p.SHA256)
		}
		if p.SizeBytes < 0 || p.SizeBytes > MaxPackageSizeBytes {
			col.add(pp+".sizeBytes", "must be between 0 and %d", MaxPackageSizeBytes)
		}
		resolved, err := ResolvePackageURL(baseURL, p.URL)
		if err != nil {
			col.add(pp+".url", "%s", err.Error())
		} else {
			p.URL = resolved
		}
	}
}

// ResolvePackageURL turns a package URL from the file into an absolute one,
// resolving a relative URL against the repository file's own address. The
// result must be https, or http only for a local address (localhost, a
// loopback or private IP, or a .local / single-label host). Whether a host
// really resolves to a local address is the fetcher's job, at connect time.
func ResolvePackageURL(baseURL, raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", errors.New("is required")
	}
	if len(raw) > MaxURLLen {
		return "", fmt.Errorf("is longer than %d characters", MaxURLLen)
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("is not a valid URL")
	}
	u := ref
	if !ref.IsAbs() {
		if ref.Path == "" {
			return "", errors.New("must name a file")
		}
		if baseURL == "" {
			return "", errors.New("is relative but the repository address is unknown")
		}
		base, err := url.Parse(baseURL)
		if err != nil || !base.IsAbs() {
			return "", errors.New("is relative but the repository address is not absolute")
		}
		u = base.ResolveReference(ref)
	}
	if u.Host == "" || u.Hostname() == "" {
		return "", errors.New("must have a host")
	}
	if u.User != nil {
		return "", errors.New("must not contain credentials")
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !IsLocalHost(u.Hostname()) {
			return "", errors.New("must use https (plain http is only allowed for local addresses)")
		}
	default:
		return "", errors.New("must use https")
	}
	u.Fragment = ""
	return u.String(), nil
}

// IsLocalHost reports whether a host name or IP literal is on this machine or
// a private network: localhost, loopback, private, link-local or unspecified
// IPs, and names ending in .local or without a dot.
func IsLocalHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
	}
	return host == "localhost" || strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".local") || !strings.Contains(host, ".")
}

// LocalPackageProblems lists every package URL that points at a local address.
// A repository served from a public address must not send the app to the
// administrator's own network; a local repository may, so the caller applies
// this only to repositories it fetched from a public address.
func LocalPackageProblems(c *Catalog) []Problem {
	var out []Problem
	for ti, t := range c.Tools {
		for vi, v := range t.Versions {
			for pi, p := range v.Packages {
				u, err := url.Parse(p.URL)
				if err != nil || !IsLocalHost(u.Hostname()) {
					continue
				}
				out = append(out, Problem{
					Path:    fmt.Sprintf("tools[%d].versions[%d].packages[%d].url", ti, vi, pi),
					Message: "a repository on a public address must not point at a local address",
				})
			}
		}
	}
	return out
}
