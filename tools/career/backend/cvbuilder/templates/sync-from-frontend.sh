#!/bin/sh
# sync-from-frontend.sh copies each CV template's own template.html/
# style.css from its real source of truth —
# tools/career/frontend-app/src/Components/CvBuilder/Template/<id>/ —
# into this directory's own per-id folder, which is what registry.go's
# own //go:embed directive actually reads from.
#
# This copy step exists, and can't be avoided, because Go's own
# //go:embed can only embed files inside the embedding package's own
# directory tree — it cannot reach across into frontend-app/, a
# completely separate directory (and, for the built/packaged tool, a
# completely separate artifact: frontend-app/ is SOURCE ONLY, compiled
# into tools/career/frontend/bundle.js for packaging, and never itself
# shipped). Editing HTML/CSS is meant to happen in frontend-app/'s own
# per-template folders (one folder per design, easy to find, easy to
# open in an editor, no Go noise) — this script is what makes that edit
# actually reach the backend's own build.
#
# Run this whenever a template.html/style.css changes, BEFORE
# `make package-tool TOOL=career` (which only runs `go build`, not this
# script — there's no tool-specific hook in that generic Makefile
# target to run it automatically). See
# plan/ai/career/cv-builder/step-02-templates-and-html-generator.md.
set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FRONTEND_TEMPLATES="$SCRIPT_DIR/../../../frontend-app/src/Components/CvBuilder/Template"

for id in modern-mono modern-sidebar classic; do
  cp "$FRONTEND_TEMPLATES/$id/template.html" "$SCRIPT_DIR/$id/template.html"
  cp "$FRONTEND_TEMPLATES/$id/style.css" "$SCRIPT_DIR/$id/style.css"
done

echo "synced modern-mono, modern-sidebar, classic from frontend-app"
