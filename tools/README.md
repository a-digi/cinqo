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

## Publishing a repository

```
make repository                       # all tools with packaged versions/
make repository BASE_URL=https://example.com/cinqo REPO_NAME="My tools"
```

Reads every `tools/<tool>/versions/tool-<tool>-<os>-<arch>.zip`, takes the tool id,
name and version from each package's own `manifest.json`, and writes
`dist/repository/repository.json` (SHA-256 and size per package; the highest version
is the current one) plus a `files/` folder with the packages. Host that folder with
any static file server and add the link to `repository.json` on the Repositories page
(`Repositories` in the menu). The file is checked with the app's own validator before
it is written. See `plan/ai/tools/repository/tool-repositories.md`.

`make repository-in-place` instead writes `tools/repository.json` and links each
package where it already is (`<tool>/versions/<zip>`), copying nothing; serve the
`tools/` folder and add the link to `repository.json`.

`make repository-release TAG=tools-2026-10-30` writes `tools/repository.json` for a GitHub
release instead: every package link is
`https://github.com/<repo>/releases/download/<TAG>/<zip>` (`GITHUB_REPO`, default
`a-digi/cinqo`). It prints the `gh release create` and `git push` commands that publish
it and runs none of them. The app's built-in repository
(`tool_repository_url` in `config.json`) reads `tools/repository.json` from the ref in its
default link, so the file must be pushed there for the built-in repository to see it.
