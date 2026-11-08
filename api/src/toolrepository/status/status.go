// Package status works out, for each tool a repository lists, whether it is
// installed, up to date or can be updated — see
// plan/ai/tools/repository/tool-repositories.md. Pure: the installed tools and
// the running environment are passed in.
package status

import (
	"fmt"

	"github.com/a-digi/cinqo/src/tool/manifest"
	"github.com/a-digi/cinqo/src/toolmeta"
	"github.com/a-digi/cinqo/src/toolrepository/catalog"
)

const (
	NotInstalled        = "not_installed"
	UpToDate            = "up_to_date"
	UpdateAvailable     = "update_available"
	NewerThanRepository = "newer_than_repository"
	Unavailable         = "unavailable"
)

// Env is what the status depends on besides the repository itself.
type Env struct {
	OS   string
	Arch string
	// AppVersion is the running app's version; empty means unknown, in which
	// case minAppVersion is not checked.
	AppVersion string
	// Installed maps a tool id (slug) to its installed version.
	Installed map[string]string
}

// VersionInfo is one entry of a tool's version list, with whether it can be
// installed on this machine.
type VersionInfo struct {
	VersionID   string `json:"versionId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	ReleasedAt  string `json:"releasedAt"`
	Changelog   string `json:"changelog"`
	// Authors and License of this version (empty when the repository states none).
	Authors       []toolmeta.Author `json:"authors"`
	License       string            `json:"license"`
	MinAppVersion string            `json:"minAppVersion"`
	Available     bool              `json:"available"`
	// Reason says why it is not available.
	Reason string `json:"reason"`
}

// Tool is one row of the repository page.
type Tool struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Reason      string `json:"reason"`
	// Metadata is who made the current version, as the repository states it; null for a repository that states
	// nothing (the older format), which the page shows as "developer not stated".
	Metadata         *toolmeta.Info `json:"metadata"`
	InstalledVersion string         `json:"installedVersion"`
	CurrentVersion   string         `json:"currentVersion"`
	CurrentVersionID string         `json:"currentVersionId"`
	Versions         []VersionInfo  `json:"versions"`
}

// Counts summarises a repository for the list.
type Counts struct {
	Tools        int `json:"tools"`
	Installed    int `json:"installed"`
	UpToDate     int `json:"upToDate"`
	Updatable    int `json:"updatable"`
	NotInstalled int `json:"notInstalled"`
}

// Availability says whether a version can be installed on this machine.
func Availability(v *catalog.Version, env Env) (bool, string) {
	if _, ok := v.PickPackage(env.OS, env.Arch); !ok {
		return false, fmt.Sprintf("no package for %s/%s", env.OS, env.Arch)
	}
	if v.MinAppVersion != "" && env.AppVersion != "" {
		cmp, err := manifest.CompareVersions(env.AppVersion, v.MinAppVersion)
		if err != nil {
			return false, "the app version could not be compared"
		}
		if cmp < 0 {
			return false, fmt.Sprintf("requires app version %s or newer (running %s)", v.MinAppVersion, env.AppVersion)
		}
	}
	return true, ""
}

// ForTool computes one tool's row.
func ForTool(t *catalog.Tool, env Env) Tool {
	out := Tool{ID: t.ID, CurrentVersionID: t.CurrentVersionID, Versions: make([]VersionInfo, 0, len(t.Versions))}
	for i := range t.Versions {
		v := &t.Versions[i]
		ok, reason := Availability(v, env)
		out.Versions = append(out.Versions, VersionInfo{
			VersionID: v.VersionID, Name: v.Name, Description: v.Description, Version: v.Version,
			ReleasedAt: v.ReleasedAt, Changelog: v.Changelog, MinAppVersion: v.MinAppVersion,
			Available: ok, Reason: reason, Authors: authorsOrEmpty(v.Authors), License: v.License,
		})
	}

	cur, ok := t.Current()
	if !ok { // cannot happen for a parsed catalog
		out.Name, out.Status, out.Reason = t.ID, Unavailable, "the current version is missing"
		return out
	}
	out.Name, out.Description, out.CurrentVersion = cur.Name, cur.Description, cur.Version
	if !cur.Info.IsZero() {
		info := cur.Info
		out.Metadata = &info
	}

	installed, isInstalled := env.Installed[t.ID]
	out.InstalledVersion = installed
	curOK, curReason := Availability(cur, env)

	if !isInstalled {
		if !curOK {
			out.Status, out.Reason = Unavailable, curReason
		} else {
			out.Status = NotInstalled
		}
		return out
	}

	cmp, err := manifest.CompareVersions(installed, cur.Version)
	switch {
	case err != nil:
		out.Status, out.Reason = Unavailable, "the installed version could not be compared"
	case cmp == 0:
		out.Status = UpToDate
	case cmp > 0:
		out.Status = NewerThanRepository
	case !curOK:
		out.Status, out.Reason = Unavailable, curReason
	default:
		out.Status = UpdateAvailable
	}
	return out
}

// ForCatalog computes every tool's row, in the file's order.
func ForCatalog(c *catalog.Catalog, env Env) []Tool {
	out := make([]Tool, 0, len(c.Tools))
	for i := range c.Tools {
		out = append(out, ForTool(&c.Tools[i], env))
	}
	return out
}

// Count summarises rows. "Installed" counts every tool that is installed,
// whatever its status.
func Count(rows []Tool) Counts {
	c := Counts{Tools: len(rows)}
	for _, r := range rows {
		if r.InstalledVersion != "" {
			c.Installed++
		}
		switch r.Status {
		case UpToDate:
			c.UpToDate++
		case UpdateAvailable:
			c.Updatable++
		case NotInstalled:
			c.NotInstalled++
		}
	}
	return c
}

// authorsOrEmpty is the list as the page expects it: never null.
func authorsOrEmpty(a []toolmeta.Author) []toolmeta.Author {
	if a == nil {
		return []toolmeta.Author{}
	}
	return a
}
