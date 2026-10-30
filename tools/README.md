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
  dashboard widgets (a stat, a list and a custom one) and comments on an
  example item — see "Adding a widget to the Home dashboard" and
  "Comments on your tool's entities" below.

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

## Comments on your tool's entities

Cinqo has a comments engine: your tool can let people comment on its own things (a
Career job, an invoice, a ticket) without any storage, API or UI of its own. You say
which entities can be commented on, drop one line of code into the page that shows an
entity, and Cinqo draws the whole panel: list, add, reply, edit, delete and filters.

Where the comments live: in Cinqo, in a database that belongs to **your tool alone**
(`<data>/db/tools/<your slug>/comments.db`). No other tool's or the system's comments are
ever in that file, and your comments are not mixed with theirs. They are **kept when your
tool is uninstalled** (like the rest of its data) and come back if it is reinstalled. A
tool needs no backend for this; a frontend-only tool can use comments.

The working example is `example-full/` (`manifest.json`, `frontend/bundle.js`,
`backend/main.go`).

### 1. Declare the entity types in `manifest.json`

```json
"comments": [
  { "entity_type": "job", "read_scope": "tool:mytool:read", "write_scope": "tool:mytool:write" }
]
```

| Field | Meaning |
|---|---|
| `entity_type` | A name for the kind of thing: lower-case letters, digits and underscores, starting with a letter, at most 40 characters (`job`, `invoice`). Declared once per type. |
| `read_scope` | Who may see the comments. Must be a scope **your tool declares** in `scopes`. |
| `write_scope` | Who may add comments (and edit or delete their own). Also one of your tool's own scopes; it may be the same scope as `read_scope`. |

An install is refused if a name is invalid, a type is declared twice, or a scope is not one of
your tool's own. An update that no longer lists a type stops accepting comments for it (the
stored comments stay on disk).

### 2. Show the panel where the entity is shown

```js
var unmount = window.__cinqoToolBridge.mountComments(container, {
  owner: 'mytool',                 // your tool's own slug
  entityType: 'job',               // declared in manifest.json "comments"
  entityId: job.id,                // the entity's UUID
  title: 'Comments',               // optional
})
// later, e.g. from your route's unmount():
unmount()
```

- `entityId` must be a **UUID** (any case; Cinqo stores it lower-case). Comments belong to
  the combination of your tool, the entity type and that UUID.
- Call the function `mountComments` returns when your page goes away. Mounting into a container
  that already has a panel replaces it. A bad option is refused with a console error, and the
  returned function then does nothing.
- Hosts older than the comments feature do not have `mountComments`. Check
  `typeof window.__cinqoToolBridge.mountComments === 'function'` first (the example does) and
  show nothing otherwise.
- The panel draws itself with Cinqo's own styles; give it a container with some room (a
  full-width block below the entity works well).

### 3. Who can do what

| Action | Needs |
|---|---|
| See the comments | `read_scope` |
| Add a comment, or reply to one | `write_scope` |
| Edit a comment | being its author, and still holding `write_scope` (nobody rewrites someone else's words, not even an administrator) |
| Delete a comment | being its author (holding `write_scope`), or `cinqo:super:admin`; this also deletes every reply under it |

A user without `read_scope` gets "not found" for everything, the same answer as for a type that
does not exist, so nothing about the comments is revealed (the panel shows an error instead of
comments). A user with `read_scope` but not `write_scope` sees the comments, and a note in place of
the box to add one. If your tool is **disabled**, its
comments are unreachable (even for an administrator) until it is enabled again. The panel only
shows Edit and Delete where the server says the user may, so it never offers something that
would be refused.

### 4. Clean up when an entity is deleted

Cinqo cannot know when your entity disappears, so tell it. From your backend, when you delete an
entity:

```go
body, _ := json.Marshal(map[string]string{"entityType": "job", "entityId": id})
req, _ := http.NewRequestWithContext(ctx, http.MethodPost, os.Getenv("CORE_API_URL")+"/api/v1/comments/purge", bytes.NewReader(body))
req.Header.Set("Authorization", "ToolService "+os.Getenv("TOOL_SERVICE_TOKEN")) // not "Bearer"
req.Header.Set("Content-Type", "application/json")
// then http.DefaultClient.Do(req), and log (do not fail the delete) if it errors
```

- It answers `200 {"deleted": n}`. Purging an entity with no comments is a `200` with `0`, so you
  can call it on every delete without checking.
- The comments removed are **your tool's own**: Cinqo takes the owner from the service token, so
  nothing in the request names one (and a name in the body is ignored). It is refused (`403`) for
  an entity type you did not declare or while your tool is disabled; `401` for a missing or
  unknown token.
- Keep it best effort with a short timeout, as the example does. If it fails the comments stay on
  disk but nothing shows them any more.

### 5. Behaviour to know about

- **Plain text only**, 1 to 4000 characters, shown exactly as written (never interpreted as HTML
  or markdown). No mentions, reactions or attachments.
- **Replies:** a comment can be replied to, and so can a reply, up to 5 levels deep. A reply is always
  shown directly below its parent, stepped in by its depth. A thread (a top-level comment and
  everything under it) holds at most 200 comments. A reply cannot be moved to another parent, and
  deleting a comment deletes its replies too; the panel says how many before it does.
- **Newest thread first**, 20 threads at a time with a "Show older comments" button. Inside a thread
  the replies run oldest first, like a conversation.
- **Author:** the id of the signed-in user, taken from their login (never from anything the page
  sends). Shown name: Cinqo's login tokens carry only an id, so the name comes from the profile the
  identity provider returned at sign-in; when there is none the panel shows "User". That name is a
  label only: it is not verified, and no permission ever depends on it.
- **Filters** in the panel: Everyone / Only mine, and a From / To date (the viewer's own days; the
  To day is included in full). A reply that matches is shown with its parents so it is never
  orphaned; a parent that does not match itself is dimmed and marked "shown for context".
- Comments are visible to everyone who may read the entity; there are no private comments.
- Cinqo does not check that the UUID belongs to a real entity of yours; it trusts your page to
  mount the panel only for entities that exist, and your backend to purge when they are deleted.

### 6. The routes, if you need them

The panel uses these; you do not normally call them yourself. All need a signed-in user, except
purge.

| Route | |
|---|---|
| `GET /api/v1/comments/{owner}/{entityType}/{entityId}` | List. Optional `limit` (max 100), `before` (a comment id, for the next older page of threads; `limit` counts threads), `authorId` (a user id, exact), `createdFrom` (inclusive) and `createdTo` (exclusive) as RFC 3339 timestamps or `YYYY-MM-DD`. Answers `{comments, total, nextBefore, canCreate}` with whole threads in display order (replies directly below their parent); `total` counts the comments that match the filters. |
| `POST` the same path, body `{"body": "…"}` or `{"body": "…", "parentId": "<comment id>"}` | Add a comment, or a reply to that comment of the same entity. |
| `PATCH …/{entityId}/{commentId}`, body `{"body": "…"}` | Edit one (the author). |
| `DELETE …/{entityId}/{commentId}` | Delete one, with all its replies. |
| `POST /api/v1/comments/purge` | Tool only (`ToolService` token): remove all comments of an entity. |

Each comment comes back with `id, entityType, entityId, body, authorId, authorName, createdAt,
updatedAt, parentId` (empty for a top-level comment), `depth` (0 for one) and `context` (true for a comment shown only
because a reply below it matched a filter), and `canEdit` / `canDelete` for the caller.

### 7. Check it

- Install the tool, open the page that shows an entity, and add a comment as a user with the write
  scope; then as one with only the read scope (the comments and a note instead of the box), and as one with
  neither (an error where the comments would be).
- Delete the entity and confirm your backend calls purge (a unit test with a fake core, like
  `example-full/backend/main_test.go`, is enough).
- Bump the tool's `version`, rebuild the zips and regenerate `repository.json` so users get it.

## Showing a big text file (the file reader)

Cinqo has a file reader: a terminal-style viewer that reads a big text file a window of up to
500 lines at a time, with search, and never loads the file whole. Your tool can use it to show
its own logs or exports. It needs no backend route: core reads the file.

### 1. Declare the source in `manifest.json`

```json
"file_sources": [
  { "id": "crawl-logs", "dir": "logs", "scope": "tool:browser:read" }
]
```

| Field | Meaning |
|---|---|
| `id` | A name for the place (lower-case letters, digits, `-` and `_`, at most 32 characters, starting with a letter). |
| `dir` | Which of **your tool's own data folders** it lives in: `logs`, `uploads`, `cache` or `tmp`. The database folder is never allowed. Cinqo works out the real folder from its data directory and your slug; the manifest never supplies a path. |
| `scope` | Who may read it. Must be a scope **your tool declares** in `scopes`. `cinqo:super:admin` may always read. |

At most 8 sources. A tool must be **enabled** for its sources to work; an update that removes
a source stops it at once (the manifest on disk is what counts).

### 2. Show a file

```js
var unmount = window.__cinqoToolBridge.mountFileReader(container, {
  tool: 'browser',                       // your slug
  source: 'crawl-logs',                  // declared in manifest.json "file_sources"
  ref: 'crawl/2026-10-31/run-1.log',     // the file, relative to that folder
  title: 'Crawl run 1',                  // optional, shown in the title bar
  initialWindow: 200                     // optional: 50, 100, 200 or 500 lines
})
```

- `ref` is a **relative path** of up to 6 parts; each part is letters, digits, dots, dashes and
  underscores and starts with a letter or digit. No `..`, no leading dot, no absolute path, no
  symlink that leaves the folder: Cinqo answers "not found" for all of them.
- Call the function `mountFileReader` returns when your page goes away. Mounting into a
  container that already has a reader replaces it. A bad option is refused with a console error
  and returns a function that does nothing.
- Hosts older than this feature do not have `mountFileReader`; check
  `typeof window.__cinqoToolBridge.mountFileReader === 'function'` first.

### 3. Limits and behaviour to know about

- Text only: a file with NUL bytes is refused ("not a text file"); files over 1 GiB are refused.
  A line longer than 16 KiB is cut in the view (and in search), and says how much is hidden.
- Search is plain text (never a pattern), case-insensitive unless the person turns "Aa" on.
- Each time someone opens a file, Cinqo writes one line to its server log: who, which source and
  file, never the content.
- Who can read is decided by the server from `scope` on every request.

## Previewing a PDF from Media

A tool bundle is one file, so it cannot ship pdf.js and its worker. To show the first page of a PDF
that is stored in Media (a generated CV, an invoice), ask Cinqo to render it:

```js
var bridge = window.__cinqoToolBridge
if (typeof bridge.pdfThumbnail === 'function') {
  bridge.pdfThumbnail(mediaFileId, doc.updatedAt).then(function (dataUrl) {
    if (dataUrl) img.src = dataUrl // a JPEG data URL; null = cannot be previewed, keep your own placeholder
  })
}
```

- Pass the **Media file id** (a UUID) and nothing else: an id that is not a UUID is answered with `null`
  without any request, and the bridge never takes a URL.
- The file is fetched with the signed-in user's own session, exactly like the Media page and a download link,
  so a user who cannot open the file gets `null`.
- The second argument is any string that changes when the file does (a document's `updatedAt`); it only keeps
  a file replaced under the same id from being shown stale. Renders run one at a time and are cached per id and
  version, so call it when the preview scrolls into view, not for a whole list at once.
- `null` also means "not a PDF", "deleted", "too large to preview" or "unreadable": show a placeholder.
- Hosts older than this feature have no `pdfThumbnail`; check with `typeof` first and fall back to your own image.

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
