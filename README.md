# Cinqo

Cinqo is a Go backend (`api/`) with a React + Vite frontend (`frontend/`), plus optional tools (`tools/`).

## Prerequisites

- Go 1.26+
- Node.js and npm
- `zig` (only for `make build-linux`, cross-compiling): `brew install zig`

## Project layout

| Path        | Description                                              |
| ----------- | -------------------------------------------------------- |
| `api/`      | Go module (backend, port `7026`); `api/config.json` holds the port and PID file |
| `frontend/` | React/Vite frontend (dev server on port `5173`)          |
| `tools/`    | Installable tools (`career`, `browser`, `pdf_tools`, …)  |
| `app/`      | Output of the single-executable build                    |
| `versions/` | Output of `make build` / `make package-tool`             |
| `Caddyfile` | Local "production-like" reverse proxy config (port `7030`) |

## Quick start (development)

1. Install frontend dependencies (first time only):

   ```sh
   cd frontend && npm install
   ```

2. Start the backend (runs in the background on `http://localhost:7026`):

   ```sh
   make run-dev
   ```

   If a `.env` file exists in the repo root it is loaded first. The current
   version from `api/VERSION` is embedded into the binary.

3. Start the frontend dev server (HMR) in another terminal:

   ```sh
   cd frontend && npm run dev
   ```

   Open <http://localhost:5173>. The dev server proxies `/api` and `/auth`
   to the backend. To point it at a different backend, set `API_URL`
   (default `http://localhost:7026`).

Stop the backend with:

```sh
make stop-dev        # graceful shutdown
make kill-backend    # force-kill whatever holds port 7026
```

## Run like production (Caddy)

Serves the built frontend and the backend on one origin, `http://localhost:7030`:

```sh
cd frontend && npm run build && cd ..
make run-dev         # backend must be running
make run-caddy       # builds Caddy from source, then runs it
```

## Single executable (desktop-style)

Builds backend + Caddy + embedded frontend into one binary and runs it in the
foreground (Ctrl+C stops everything):

```sh
make run-app
```

`make run-app` builds both apps (`make build-app-all`, without a version bump):
`app/cinqo-pro`, which it then starts, and `app/cinqo-air` for the MacBook Air.

`make build-app` (or `make build-app-pro`) only builds `app/cinqo-pro`, the
build for this machine (bumps `app/VERSION`; use `NO_BUMP=1` to skip).
`make build-app-air` builds `app/cinqo-air` for the MacBook Air (Apple Silicon,
macOS 12 or newer), and `make build-app-all` builds both with one version bump. Use `--data <dir>` before the action to change the data
directory: `./app/cinqo-pro --data /path start`.

The app opens its own Chrome as an **application window** (no address bar, no tabs). `--window=browser` (or the environment variable
`CINQO_WINDOW=browser`) opens a normal browser window instead; the flag wins over the variable, and any other value is refused at start.

### The two builds: `cinqo-pro` and `cinqo-air`

| File             | Built by             | For                                          | Needs macOS     |
| ---------------- | -------------------- | -------------------------------------------- | --------------- |
| `app/cinqo-pro`  | `make build-app-pro` | the MacBook Pro (the machine that builds it) | 26.0 or newer   |
| `app/cinqo-air`  | `make build-app-air` | the MacBook Air (Apple Silicon, arm64)       | 12.0 or newer   |

Both are single files with the frontend and the config inside. `cinqo-air` is
built for an older macOS on purpose (`AIR_MIN_MACOS=<version>` changes the
minimum): a build without it asks for the macOS of the machine that built it,
and an Air on an older macOS would refuse to start it.

To run `cinqo-air` on the Air, put the file in any folder there and start it:

```sh
./cinqo-air start
```

- It keeps its data next to itself (a `data/` folder in the same folder), or
  where `--data <dir>` says: `./cinqo-air --data ~/cinqo-data start`. The folder
  it runs from does not matter, only that it can write there.
- macOS blocks a file that arrived by **download, AirDrop, Mail or Messages**
  (it carries a "quarantine" mark, wherever you then put it). Remove the mark
  once: `xattr -d com.apple.quarantine cinqo-air`. A file that was **built on
  that machine**, or copied with `cp`, `scp`, `rsync` or `curl`, has no mark and
  needs nothing.
- Do not run `cinqo-pro` and `cinqo-air` from the same `app/` folder: they would
  share `app/data` and its PID file, and starting one stops the other.

## Common commands

| Command                       | Purpose                                          |
| ----------------------------- | ------------------------------------------------ |
| `make test`                   | Run backend tests (`test-verbose`, `test-cover` also available) |
| `make build`                  | Build backend to `versions/cinqo-<branch>` (bumps `api/VERSION`; `NO_BUMP=1` to skip) |
| `make build-linux`            | Cross-compile for Linux (`DEPLOY_GOARCH=arm64` or `amd64`) |
| `make lint-frontend`          | ESLint + Prettier check (`lint-frontend-fix` to auto-fix) |
| `make mod-tidy`               | `go mod tidy` in `api/`                          |
| `make package-tool TOOL=name` | Package `tools/<name>` into `versions/tool-<name>.zip` |

## License

See [LICENSE](LICENSE).
