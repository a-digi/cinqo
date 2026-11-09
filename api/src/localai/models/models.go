// Package models is the list of local models Cinqo knows: the ones shipped in the program (package catalog, checked and never changed at run time) and
// the ones added later after a tool suggested them and the person confirmed (plan/ai/small-llm-local/step-220-tools-suggest-local-models.md). Every
// place that needs "the models" asks Current, so an added model is found wherever a shipped one is.
package models

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"

	"github.com/a-digi/cinqo/src/localai/catalog"
	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/repository/query"
)

// Merge returns the list of models: the shipped catalog with the added models appended. The shipped part is never changed, except that a model marked
// suggestOnly (one Cinqo knows and measured but does not offer on its own) is left out unless a person confirmed it: a row of the added models with the
// model's own id and status added reveals it, and it then has everything the file says about it (its tools, its dialect, its memory profile). An added
// model that breaks a rule of the catalog (the same rules as a shipped entry), takes a shipped id, or repeats another added id is left out and reported
// in the second result: a broken row must never take the shipped models down.
func Merge(shipped catalog.Catalog, added []entity.AddedModel) (catalog.Catalog, []error) {
	out := shipped
	out.Models = make([]catalog.Model, 0, len(shipped.Models)+len(added))
	used := map[string]bool{}
	hidden := map[string]bool{}
	for _, m := range shipped.Models {
		used[m.ID] = true
		if m.SuggestOnly {
			hidden[m.ID] = true
		}
	}
	for _, r := range shipped.Runtimes {
		used[r.ID] = true
	}
	revealed := map[string]bool{}
	for _, a := range added {
		if hidden[a.ID] {
			revealed[a.ID] = true
		}
	}
	for _, m := range shipped.Models {
		if !m.SuggestOnly || revealed[m.ID] {
			out.Models = append(out.Models, m)
		}
	}
	var problems []error
	for _, a := range added {
		m := a.CatalogModel()
		switch {
		case hidden[a.ID]:
			// a confirmed model that ships hidden: it is the shipped entry that was added to the list above
		case !strings.HasPrefix(a.ID, entity.AddedIDPrefix):
			problems = append(problems, fmt.Errorf("added model %q: its id must start with %q", a.ID, entity.AddedIDPrefix))
		case used[a.ID]:
			problems = append(problems, fmt.Errorf("added model %q: the id is already taken", a.ID))
		default:
			if err := catalog.ValidateModel(m); err != nil {
				problems = append(problems, fmt.Errorf("added model %q: %w", a.ID, err))
				continue
			}
			used[a.ID] = true
			out.Models = append(out.Models, m)
		}
	}
	return out, problems
}

var (
	mu     sync.RWMutex
	source func() (catalog.Catalog, error)
)

// SetSource tells the package where the current list comes from (module start-up). Without a source, Current is the shipped models Cinqo offers on its own.
func SetSource(f func() (catalog.Catalog, error)) {
	mu.Lock()
	source = f
	mu.Unlock()
}

// Current is the list of models now: shipped and added.
func Current() (catalog.Catalog, error) {
	mu.RLock()
	f := source
	mu.RUnlock()
	if f == nil {
		return defaultList()
	}
	return f()
}

// FromDB is a source that reads the added models from the main database on every call (never cached: a confirmation changes the list at once). When
// the table cannot be read the shipped models are still answered, never an error: the shipped list must not depend on the added one.
func FromDB(db *sql.DB, onProblem func(error)) func() (catalog.Catalog, error) {
	return func() (catalog.Catalog, error) {
		shipped, err := catalog.Default()
		if err != nil {
			return catalog.Catalog{}, err
		}
		added, err := query.NewAddedModelsQueryRepo(db).ListAdded()
		if err != nil {
			if onProblem != nil {
				onProblem(err)
			}
			list, _ := Merge(shipped, nil)
			return list, nil
		}
		merged, problems := Merge(shipped, added)
		if onProblem != nil {
			for _, p := range problems {
				onProblem(p)
			}
		}
		return merged, nil
	}
}

// defaultList is the list of models with nobody having added anything: the shipped models Cinqo offers on its own.
func defaultList() (catalog.Catalog, error) {
	shipped, err := catalog.Default()
	if err != nil {
		return catalog.Catalog{}, err
	}
	list, _ := Merge(shipped, nil)
	return list, nil
}
