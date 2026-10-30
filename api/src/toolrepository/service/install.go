package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"runtime"

	"github.com/a-digi/cinqo/src/tool/manifest"
	"github.com/a-digi/cinqo/src/toolrepository/catalog"
	"github.com/a-digi/cinqo/src/toolrepository/fetch"
	"github.com/a-digi/cinqo/src/toolrepository/status"
)

var (
	ErrToolNotFound   = errorKind("tool not found in this repository")
	ErrVersionMissing = errorKind("version not found")
	ErrNotInstallable = errorKind("this version cannot be installed")
	ErrDownload       = errorKind("the package could not be downloaded")
)

type errorKind string

func (e errorKind) Error() string { return string(e) }

// WithDownloader sets the client used for package downloads, which need a much
// longer timeout than the repository file.
func (s *Service) WithDownloader(d Fetcher) *Service {
	s.downloader = d
	return s
}

// InstallPlan is a checked decision to install one version: what to download,
// what it must hash to, and what its manifest must say.
type InstallPlan struct {
	URL       string
	SHA256    string
	SizeBytes int64
	ToolID    string
	Version   string
	VersionID string
	// Local says whether the repository was on a local address.
	Local bool
}

// PlanInstall checks that versionID (default: the current version) of toolID
// can be installed here and is not already installed: a fresh install of any
// version is allowed, an update only to a strictly newer one.
func (s *Service) PlanInstall(id, toolID, versionID string, env status.Env) (*InstallPlan, error) {
	repo, err := s.find(id)
	if err != nil {
		return nil, err
	}
	if repo.CatalogJSON == "" {
		return nil, &Error{ErrToolNotFound, "this repository has not been loaded yet"}
	}
	cat, err := catalog.Parse([]byte(repo.CatalogJSON), repo.URL)
	if err != nil {
		return nil, &Error{ErrInvalidFile, "the stored repository file is no longer valid; refresh the repository"}
	}
	tool, ok := cat.Tool(toolID)
	if !ok {
		return nil, &Error{ErrToolNotFound, "tool not found in this repository"}
	}
	if versionID == "" {
		versionID = tool.CurrentVersionID
	}
	v, ok := tool.Version(versionID)
	if !ok {
		return nil, &Error{ErrVersionMissing, "version not found"}
	}
	if okAvail, reason := status.Availability(v, env); !okAvail {
		return nil, &Error{ErrNotInstallable, "this version cannot be installed here: " + reason}
	}
	if installed, isInstalled := env.Installed[toolID]; isInstalled {
		cmp, err := manifest.CompareVersions(v.Version, installed)
		if err != nil {
			return nil, &Error{ErrNotInstallable, "the installed version could not be compared"}
		}
		if cmp == 0 {
			return nil, &Error{ErrNotInstallable, fmt.Sprintf("version %s is already installed", installed)}
		}
		if cmp < 0 {
			return nil, &Error{ErrNotInstallable, fmt.Sprintf("version %s is older than the installed version %s; an update must be newer", v.Version, installed)}
		}
	}
	p, _ := v.PickPackage(env.OS, env.Arch)
	return &InstallPlan{
		URL: p.URL, SHA256: p.SHA256, SizeBytes: p.SizeBytes,
		ToolID: toolID, Version: v.Version, VersionID: v.VersionID, Local: repo.IsLocal,
	}, nil
}

// Download fetches the package and checks it against the checksum the
// repository lists. The package's own manifest is checked later, by the
// install pipeline, against ToolID and Version.
func (s *Service) Download(ctx context.Context, plan *InstallPlan) ([]byte, error) {
	policy := fetch.LocalForbidden
	if plan.Local {
		policy = fetch.LocalAllowed
	}
	res, err := s.downloader.Get(ctx, plan.URL, catalog.MaxPackageSizeBytes, policy)
	if err != nil {
		return nil, &Error{ErrDownload, "the package could not be downloaded: " + err.Error()}
	}
	if plan.SizeBytes > 0 && int64(len(res.Body)) != plan.SizeBytes {
		return nil, &Error{ErrDownload, "the downloaded package does not have the size the repository lists"}
	}
	sum := sha256.Sum256(res.Body)
	if hex.EncodeToString(sum[:]) != plan.SHA256 {
		return nil, &Error{ErrDownload, "the downloaded package does not match the checksum the repository lists"}
	}
	return res.Body, nil
}

// CurrentEnv is this machine's OS and architecture with no installed tools; the
// handler fills the rest.
func CurrentEnv() status.Env { return status.Env{OS: runtime.GOOS, Arch: runtime.GOARCH} }
