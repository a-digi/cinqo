# tools/ — Tool Source Workspace

Author a tool's own code here, then package it into a `.zip` and
install it through the running app's admin UI (`System → Tools`) or
directly against `POST /api/v1/tools/install`.

This is **source** — a plain, git-tracked working copy you edit. It is
not the same as `api/data/tools/{slug}/`, which is the install
pipeline's own runtime copy (overwritten on every install/update, never
hand-edited).

Full design, the manifest schema, the on-disk package layout, and
everything about how an installed tool actually runs:
see `plan/ai/tools/` (start at `plan/ai/tools/tools.md`).

## Layout

```
tools/
  <your-tool-name>/
    manifest.json          # required — see plan/ai/tools/step-02-manifest-and-safe-zip-extraction.md
    frontend/
      bundle.js             # optional — present only if your tool has a UI
    backend/
      go.mod                 # optional — present only if your tool has its own backend process;
      main.go                # its own, independent Go module — never part of api/go.mod
```

## Packaging

```
make package-tool TOOL=<your-tool-name>
```

Builds the backend (if present) into that tool's own
`tools/<your-tool-name>/build/generic/` (a gitignored build artifact,
never hand-edited) and produces `versions/tool-<name>.zip`, ready to
upload. A tool that needs to ship for more than the host's own OS
(e.g. `pdf_generator`, which cross-compiles for macOS/Linux/Windows)
can additionally have its own `Makefile` — see
`plan/ai/tools/pdf-generator/step-06-multi-os-packaging.md`.

## Examples

- `example-frontend-only/` — the smallest possible tool: a menu entry
  and a page, no backend process at all.
- `example-full/` — frontend + backend, showing the full round trip: the
  frontend calls its own backend through the reverse proxy (never the
  raw port directly), which is enforced per-request against the scope
  declared for that route in `manifest.json`.

Both are reference examples, not real features — copy one as your
starting point.
