// Package webapp embeds the built frontend into the api binary. The
// dist/ directory is not source — it's copied in by `make
// embed-frontend` from frontend/dist/ (go:embed cannot reach outside
// the tree of the file containing the directive, so the frontend's own
// build output has to be copied under api/ first). See
// plan/ai/build/app/step-02-embed-frontend-build.md.
package webapp

import "embed"

//go:embed dist
var FS embed.FS
