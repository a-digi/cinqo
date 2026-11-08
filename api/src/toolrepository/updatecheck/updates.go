package updatecheck

import (
	"sort"

	"github.com/a-digi/cinqo/src/tool/manifest"
	"github.com/a-digi/cinqo/src/toolrepository/catalog"
	repo_entity "github.com/a-digi/cinqo/src/toolrepository/entity"
	"github.com/a-digi/cinqo/src/toolrepository/status"
)

// Update is an installed tool that has a newer version it can be updated to.
type Update struct {
	Slug             string `json:"slug"`
	Name             string `json:"name"`
	InstalledVersion string `json:"installedVersion"`
	LatestVersion    string `json:"latestVersion"`
	VersionID        string `json:"versionId"`
	RepositoryID     string `json:"repositoryId"`
	RepositoryName   string `json:"repositoryName"`
}

// Updates lists, from the stored catalogs and without any network, the installed
// tools whose repository offers a newer version that can be installed here (the
// same rules as the repository page's "Update available"). A tool offered by
// several repositories appears once, with the highest version; on a tie the
// repository listed first wins (repos come built-in first, then oldest added).
// The result is sorted by slug and never nil.
func Updates(repos []*repo_entity.Repository, env status.Env) []Update {
	best := map[string]Update{}
	for _, repo := range repos {
		if repo.CatalogJSON == "" {
			continue
		}
		cat, err := catalog.Parse([]byte(repo.CatalogJSON), repo.URL)
		if err != nil {
			continue
		}
		for _, row := range status.ForCatalog(cat, env) {
			if row.Status != status.UpdateAvailable {
				continue
			}
			cand := Update{
				Slug: row.ID, Name: row.Name, InstalledVersion: row.InstalledVersion,
				LatestVersion: row.CurrentVersion, VersionID: row.CurrentVersionID,
				RepositoryID: repo.ID, RepositoryName: repo.Name,
			}
			if cur, ok := best[row.ID]; ok {
				if cmp, err := manifest.CompareVersions(cand.LatestVersion, cur.LatestVersion); err != nil || cmp <= 0 {
					continue
				}
			}
			best[row.ID] = cand
		}
	}
	out := make([]Update, 0, len(best))
	for _, u := range best {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// LastCheckedAt is the newest fetch time over all repositories ("" if none was
// ever fetched), as stored: UTC, "2006-01-02 15:04:05".
func LastCheckedAt(repos []*repo_entity.Repository) string {
	newest := ""
	for _, r := range repos {
		if r.LastFetchedAt > newest {
			newest = r.LastFetchedAt
		}
	}
	return newest
}
