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

`make build-app` only builds `app/cinqo-app` (bumps `app/VERSION`; use
`NO_BUMP=1` to skip). Use `--data <dir>` before the action to change the data
directory: `./app/cinqo-app --data /path start`.

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
