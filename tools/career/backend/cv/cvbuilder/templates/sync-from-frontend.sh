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

for id in modern-mono modern-sidebar classic \
  ats-clean ats-serif-classic ats-minimal-gray ats-executive ats-compact \
  ats-modern-sans ats-times ats-verdana ats-tahoma ats-calibri \
  ats-garamond ats-bold-headers ats-two-line-header ats-skills-first ats-academic \
  modern-banner modern-timeline creative-accent creative-bold \
  executive-elegant executive-serif-bold minimalist-lines minimalist-airy \
  modern-gradient modern-split-header modern-card modern-dark-header \
  modern-dots modern-photo-hero creative-pastel creative-monogram \
  creative-duotone creative-stripe creative-sunset executive-navy \
  executive-monogram-rule executive-charcoal minimalist-photo-left minimalist-whitespace \
  minimalist-mono-caps classic-portrait classic-ribbon classic-centered-photo \
  modern-slate-line modern-indigo-strip modern-ocean-banner modern-berry-tint \
  modern-emerald-timeline modern-copper-right modern-sky-hero modern-graphite-frame \
  creative-plum-duo creative-amber-overlap creative-rose-rule creative-teal-tint \
  creative-olive-banner creative-violet-center executive-crimson-center executive-navy-rule \
  executive-gold-dark executive-forest-frame executive-steel-strip executive-charcoal-right \
  minimalist-plain-left minimalist-plain-center minimalist-line-right minimalist-sage-rule \
  minimalist-steel-strip minimalist-sand-frame classic-navy-right classic-berry-center \
  classic-ink-left classic-forest-strip classic-gold-tint classic-crimson-rule \
  side-navy side-sage side-graphite-right side-coral \
  side-slate-wide side-sand-serif side-teal-banner side-night-mono \
  clean-blue-sidebar corporate-gold-sidebar contemporary-navy-sidebar maker-dark-sidebar \
  refined-red-sidebar leadership-brown-sidebar green-gradient-sidebar bold-blue-sidebar \
  midnight-gold-sidebar grey-pro-sidebar friendly-orange-sidebar photo-panel-steel \
  paint-splash square-teal orange-frame dotted-lime \
  night-mode gilded-frame green-colorful classic-red-columns \
  rich-gold-cream retro-green-double gradient-blue-diagonal skills-first-orange; do
  mkdir -p "$SCRIPT_DIR/$id"
  cp "$FRONTEND_TEMPLATES/$id/template.html" "$SCRIPT_DIR/$id/template.html"
  cp "$FRONTEND_TEMPLATES/$id/style.css" "$SCRIPT_DIR/$id/style.css"
done

echo "synced all CV templates from frontend-app"
