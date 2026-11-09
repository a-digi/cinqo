package localai

import (
	"github.com/a-digi/cinqo/src/localai/catalog"
	"github.com/a-digi/cinqo/src/localai/models"
)

// Most tests are about what a listed model can do (its dialect, its tools, its size), so by default every model Cinqo knows is listed, the ones it only
// offers on suggestion included. A test of the hiding itself sets its own source (models.FromDB) and puts this one back afterwards (resetModels).
func allModels() (catalog.Catalog, error) { return catalog.Default() }

func resetModels() { models.SetSource(allModels) }

func init() { resetModels() }
