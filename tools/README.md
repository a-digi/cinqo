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
  declared for that route in `manifest.json`. It also carries three Home
  dashboard widgets (a stat, a list and a custom one) — see
  "Adding a widget to the Home dashboard" below.

Both are reference examples, not real features — copy one as your
starting point.

## Adding a widget to the Home dashboard

Cinqo's Home page is a grid of widgets, and a tool can add its own. You do not
write any dashboard UI for the common cases: you pick a **kind**, and your backend
returns data in that kind's shape; Cinqo validates it and draws it. Users can hide
and reorder widgets; a widget from a newly installed tool appears at the end.

A widget needs three things, all in your tool:

1. a `GET` route in `manifest.json`'s `routes` (this is where the data comes from),
2. a `widgets` entry that names that route,
3. a handler in your backend that answers it.

The working example is `example-full/` (`manifest.json`, `backend/main.go`,
`frontend/bundle.js`).

### 1. Declare it in `manifest.json`

```json
"routes": [
  { "method": "GET", "path_suffix": "dashboard/open-jobs", "required_scope": "tool:mytool:read" }
],
"widgets": [
  { "key": "open_jobs", "title": "Open jobs", "kind": "stat", "data_path": "dashboard/open-jobs" }
]
```

| Field | Required | Meaning |
|---|---|---|
| `key` | yes | `^[a-z][a-z0-9_]*$`, unique in your tool. The widget's id is `<your slug>.<key>`. |
| `title` | yes | Shown on the card. |
| `description` | no | Shown as a tooltip on the title and in the Customize list. |
| `kind` | yes | `stat`, `list`, `chart` or `custom` (below). |
| `data_path` | yes | The `path_suffix` of a `GET` route declared in the same `routes`. That route **must** have a `required_scope`. |
| `width` | no | Grid columns, 1–4 (default 1). The grid is 4 columns wide on a large screen, 2 on a tablet and 1 on a phone; a wider widget is capped to the screen. |
| `height` | no | Grid rows, 1–3 (default 1). |
| `refresh_seconds` | no | How often an open Home page asks again, 15–3600 (default 60). |
| `schema_version` | no | Only `1` exists; leave it out. |

Rules the install enforces: the tool must include a backend, keys are unique, and the
`data_path` route exists, is a `GET` and has a scope. A widget with a mistake rejects
the whole install, with a message naming the widget. A widget you remove in an update
disappears from Home.

### 2. Who sees it

A widget takes the **scope of its data route**. Only users who hold that scope (or
`cinqo:super:admin`) see it; for everyone else it does not exist. A disabled tool's
widgets are hidden. If the tool is enabled but not running, the card says "Not available
right now" without a request being made.

### 3. Answer the route

Cinqo calls `GET http://127.0.0.1:<your port>/<data_path>` with the signed-in user's
token as `Authorization: Bearer …` (call Cinqo back with it if you need to act as that
user; your tool does not otherwise need to authenticate, scope is already checked). Keep
it fast and read-only:

- **Timeout 3 seconds.** A slower answer shows "Not available right now".
- **No redirects** are followed. Answer `200` with JSON.
- **At most 256 KB.** More is an error.
- Answer `503` when your data source is down to show "Not available right now"; any other
  non-2xx is shown as "This widget couldn't load" (and logged by Cinqo). Failures are not
  retried for a few seconds; good answers are cached per user for `refresh_seconds`.

**Shorten text yourself.** Cinqo rejects a field over its limit instead of clipping it,
and the whole widget then shows as failed. Fields not listed below are ignored.

#### `stat` — one headline number

```json
{ "label": "Open jobs", "value": 12, "unit": "jobs",
  "delta": { "value": 3, "direction": "up", "good": true },
  "caption": "since Monday", "link": "/tools/mytool/jobs" }
```

`label` (≤ 80 characters) and `value` (a finite number, or a string ≤ 80) are required.
`unit` ≤ 40, `caption` ≤ 200. `delta.direction` is `up`, `down` or `flat`; `delta.good` says
whether that direction is desirable (it colours the arrow; leave it out for neutral).

#### `list` — up to 10 rows

```json
{ "items": [
  { "title": "Backend Engineer", "subtitle": "Acme · Berlin", "meta": "2d ago",
    "badge": { "text": "New", "variant": "green" }, "link": "/tools/mytool/job?id=7" } ] }
```

`items` is required (an empty array shows "Nothing to show yet"), at most 10. Per item:
`title` required (≤ 120), `subtitle` ≤ 200, `meta` ≤ 40, `badge.text` ≤ 30 with `variant` one of
`gray`, `green`, `amber`, `red`.

#### `chart` — bars or a line

```json
{ "type": "bar",
  "series": [ { "name": "New jobs", "points": [ { "x": "Mon", "y": 3 }, { "x": "Tue", "y": 5 } ] } ] }
```

`type` is `bar` or `line`; at most 3 series with at most 60 points each. `name` and each
`x` are required and ≤ 40 characters; `y` is a finite number. Categories come from the series
with the most points, so give every series the same points in the same order. A chart with no
points shows "Nothing to show yet".

#### Links

`link` (on a stat or a list item) must be an internal path starting with a single `/`, such
as `/tools/mytool/jobs` or `/admin/tools`. Anything else (`https://…`, `//host`, `javascript:`)
is silently dropped and the card is simply not clickable.

### 4. Draw it yourself (`custom`)

When the built-in kinds do not fit, declare `"kind": "custom"` and draw the card in your
frontend bundle. Your route returns any JSON **object** (at most 8 levels deep; `{}` shows
"Nothing to show yet"); Cinqo checks only that, and hands it to your renderer:

```js
window.__cinqoToolBridge.registerDashboardWidget({
  id: 'mytool.open_jobs',                 // "<slug>.<key>"
  mount: function (container, data) { /* draw into container */ },
  update: function (container, data) { /* optional: new data arrived */ },
  unmount: function (container) { /* optional: clean up timers/listeners */ },
})
```

- The first registration of an id wins; a malformed id or a missing `mount` is refused with a
  console error. If your bundle has not loaded yet the card says its display is not available
  and fills in when it does.
- Without `update`, Cinqo calls `unmount` then `mount` again on every refresh.
- A renderer that throws shows "This widget couldn't be displayed" in its own card; the rest
  of the page is unaffected.
- Your bundle is not part of Cinqo's build, so Tailwind classes do nothing: use inline styles
  and the `--cinqo-tool-*` CSS variables (listed in `frontend/src/config/tools/toolBridge.ts`),
  each with a literal fallback.
- **You are responsible for escaping.** Put data into the page with `textContent` or
  `createElement`, never `innerHTML`.

### 5. Check it

- `make package-tool TOOL=<name>` (or your tool's own `make zip`), install it, then open Home.
- The first request is made when the card appears; "This widget couldn't load" means your
  route answered non-2xx or invalid data — Cinqo's log says which, and why.
- Add a unit test next to your handler that decodes your payload and checks the required fields
  and the limits above; `example-full/backend/main_test.go` shows the shape.
- Bump your tool's `version`, rebuild the zips and regenerate `repository.json` so users get the
  widget on their next update.

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
