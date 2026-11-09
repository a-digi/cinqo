package localai

import (
	"database/sql"
	"time"

	"github.com/a-digi/cinqo/src/localai/catalog"
	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/repository/persistent"
	"github.com/a-digi/cinqo/src/localai/repository/query"
)

// SystemSuggester is the name Cinqo gives itself when it suggests or takes over a model.
const SystemSuggester = "Cinqo"

// modelStater is what the reconciliation needs to know about the files on this computer.
type modelStater interface {
	ModelState(m catalog.Model) entity.FileInstall
}

// reconcileSuggestOnly runs at every start (plan 221). A model Cinqo only offers on suggestion (suggestOnly) is handled in one of two ways, never by
// downloading anything:
//
//   - Its file is already complete on this computer and nobody confirmed it yet (no row, or a waiting suggestion): it is taken over, that is confirmed on the
//     person's behalf, because what is on disk was downloaded to be used. Without this, a model downloaded before it became suggest-only would vanish from the
//     tools after an update. A model the person dismissed is left alone.
//   - It is not downloaded and Cinqo suggests it itself (suggestedByCinqo): Cinqo records itself as a suggester, with its own reason, so it waits on the
//     Local Agents page like a suggestion from a tool. Nothing is added twice, a dismissed one stays dismissed, and a model that is already added is left as is.
//
// It is idempotent and best effort: an error on one model leaves the others alone.
func reconcileSuggestOnly(db *sql.DB, files modelStater, known catalog.Catalog, now time.Time) {
	repo := query.NewAddedModelsQueryRepo(db)
	write := persistent.NewAddedModelsPersistentRepo(db)
	stamp := now.UTC().Format(time.RFC3339)
	for _, m := range known.Models {
		if !m.SuggestOnly {
			continue
		}
		row, err := repo.Find(m.ID)
		missing := err == sql.ErrNoRows
		if err != nil && !missing {
			continue
		}
		onDisk := files.ModelState(m).State == entity.InstallReady
		switch {
		case onDisk && (missing || row.Status == entity.AddedStatusSuggested):
			if missing {
				added := entity.FromCatalog(m, entity.AddedStatusAdded, stamp)
				added.SuggestedBy, added.Why = SystemSuggester, "It was already downloaded on this computer."
				if write.Insert(&added) != nil {
					continue
				}
			} else if write.SetStatus(m.ID, entity.AddedStatusAdded, stamp) != nil {
				continue
			}
			_ = write.AddSuggester(m.ID, SystemSuggester, "It was already downloaded on this computer.", stamp)
		case !onDisk && m.SuggestedByCinqo && missing:
			suggestion := entity.FromCatalog(m, entity.AddedStatusSuggested, stamp)
			suggestion.SuggestedBy, suggestion.Why = SystemSuggester, m.SuggestWhy
			if write.Insert(&suggestion) != nil {
				continue
			}
			_ = write.AddSuggester(m.ID, SystemSuggester, m.SuggestWhy, stamp)
		case !onDisk && m.SuggestedByCinqo && row.Status == entity.AddedStatusSuggested:
			// a tool suggested it first: Cinqo joins the tools that suggested it (once)
			_ = write.AddSuggester(m.ID, SystemSuggester, m.SuggestWhy, stamp)
		}
	}
}
