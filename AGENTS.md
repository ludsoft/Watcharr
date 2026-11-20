# AGENTS.md

Guidance for AI coding agents working in the Watcharr repository.

## What this is

Watcharr is a self-hosted watched list for movies, TV shows and anime (and video games via IGDB). Users track status, ratings, activity, tags, and can sync or import from Jellyfin/Emby, Plex, Trakt, MyAnimeList, IMDb, etc.

- **Backend:** Go (`server/`): Gin HTTP API, GORM with SQLite (`mattn/go-sqlite3`, needs CGO).
- **Frontend:** SvelteKit with **Svelte 5 runes** (`src/`), TypeScript, SCSS, `adapter-node`, PWA.
- **Docs site:** Docusaurus (`doc/`), published to https://watcharr.app.
- License: GPL-3.0.

## Repository layout

```
server/                 Go backend (module github.com/sbondCo/Watcharr)
  watcharr.go           Entry point: config, DB, service wiring, route registration, UI proxy
  feature/<name>/       One package per feature: Service (business logic) + Router (Gin handlers)
  domain/               Request/response DTOs and API-facing types
  database/entity/      GORM models (DB schema)
  database/migrate/     Custom data migrations (run after GORM AutoMigrate)
  database/dbmodel/     Shared GormModel base
  config/               Server config (persisted as <data>/watcharr.json)
  media/tmdb, media/igdb  External metadata providers
  router/               BaseRouter, middleware, pagination, ErrorResponse
  util/, image/, task/, job/, token/, logging/, cache/, activity/, arr/
  internal/testutil/    Test-only helpers (never used in prod code)
  VERSION               Embedded version string
src/                    SvelteKit frontend
  routes/(app)/         Authenticated app pages (tv, movie, game, person, discover, settings, ...)
  routes/(plain)/       Pages without the main app chrome (login, setup, etc.)
  lib/                  Shared components (Modal, Icon, poster/, settings/, ...)
  lib/util/api.ts       API base URL + `req` / `noAuthReq` clients (built on lib/util/fetch.ts)
  store.svelte.ts       Global app state (runes)
  types.ts              Shared TS types mirroring backend DTOs
  styles/               _vars.scss and _mixins.scss (auto-injected into every component), norm.scss (global)
static/                 Static assets (logos, icons)
doc/                    Docusaurus documentation site
changelog/<version>/    Images referenced from CHANGELOG.md
```

## Development

Prerequisites: Go (version in `server/go.mod`), a C toolchain for CGO, Node 24.

```sh
npm install                 # frontend deps
npm run dev                 # Vite dev server (or `make dev`; `make dev_host` to expose on LAN)
cd server && make run       # backend in DEV mode (MODE=DEV go run .), listens on :3080
```

- In dev, the frontend calls the API at `<host>:3080/api` (see `src/lib/util/api.ts`).
- In prod (`MODE` unset), the Go binary serves `/api` and reverse-proxies all other routes to the built SvelteKit node server via the Unix socket `/tmp/watcharrui`.
- Data (SQLite DB, `watcharr.json` config, logs, cached images) lives in `$WATCHARR_DATA` (defaults to `./data`, i.e. `server/data` when run from `server/`). A `.env` file is loaded if present.
- To test the production Docker image locally: `docker compose -f docker-compose.dev.yml up --build` (serves on :3081).

## Checks (run before finishing)

CI runs these on PRs; keep them green.

**Backend** (from `server/`):
```sh
go test ./...
gofmt -l .        # must print nothing; fix with `gofmt -w .`
```

**Frontend** (from repo root):
```sh
npm run lint      # prettier --check ./src/ && eslint ./src/
npm run check     # svelte-check type checking
npm run format    # auto-fix formatting
```

Formatting: Prettier with **tabs**, double quotes, semicolons, trailing commas, print width 80 (`.prettierrc`). Go uses standard gofmt.

## Backend conventions

- **Adding/extending a feature:** create or edit `server/feature/<name>/` with a `Service` (`NewService(db, deps...)`) and a `Router` (`NewRouter(br, service)` + `AddRoutes()`). Wire both up in `server/watcharr.go`. See `server/feature/tag/` for a small, representative example.
- Routes are registered under the `/api` group via `router.BaseRouter`. Protect user routes with `authmiddleware.AuthRequired(r.br.Cfg)`; the authed user ID comes from `c.MustGet("userId").(uint)`. Paginated endpoints use `router.PaginatedRequest(...)` and `util.PaginationParams` / `util.PaginationResponse`.
- Return errors as `router.ErrorResponse{Error: ...}` JSON with an appropriate status code.
- Use `log/slog` for logging with structured key/value pairs (`slog.Error("msg", "error", err)`).
- Request bodies go in `domain/` structs using Gin `binding` tags; custom validators are registered in `watcharr.go`.
- DB models live in `database/entity/`. Schema changes are applied via GORM AutoMigrate; **data** migrations go in `database/migrate/migrations.go` with a timestamped ID (`YYYYMMDDHHMM_NNNN`). **Never modify an existing migration** once released; add a new one.
- New config options go on `ServerConfig` in `server/config/config.go` (documented with comments; mind the JSON tags since this is persisted to `watcharr.json`). Consider whether the frontend needs it exposed via the features/server endpoints.
- Tests are standard `_test.go` files next to the code; shared helpers go in `server/internal/testutil`.

## Frontend conventions

- Svelte 5 runes only (`$state`, `$derived`, `$props`, `$effect`, snippets). Do not use legacy `export let` / `$:` syntax.
- Call the backend through `req` / `noAuthReq` from `src/lib/util/api.ts`, not raw `fetch`.
- Global state lives in `src/store.svelte.ts`; shared types in `src/types.ts` (keep them in sync with Go DTOs/entities).
- Styling is component-scoped SCSS (`<style lang="scss">`). Variables and mixins from `src/styles/_vars.scss` / `_mixins.scss` are available everywhere automatically; these partials must not emit CSS. Global styles go in `norm.scss`. Support both light and dark themes.
- `$lib` and `@` (→ `src`) aliases are available. The app version is exposed as `__WATCHARR_VERSION__`.

## Changelog, versioning and releases

- User-facing changes get an entry in `CHANGELOG.md` under `# [Unreleased]`, grouped as `## Added` / `## Changed` / `## Fixed`, prefixed by the area (e.g. `Person:`, `Activity:`). Credit external contributors.
- Version bumps are done with `make version X.Y.Z`, which updates `server/VERSION` and `package.json`/`package-lock.json` together. Don't bump versions unless asked.
- Container images are built by `.github/workflows/container-release.yml`.
- If a change affects setup, config or user-facing behavior, consider updating the docs in `doc/docs/` and `FEATURES.md`.

## Contribution rules

- Large changes (more than a few lines) should be discussed in an issue first (see `CONTRIBUTING.md`).
- AI usage must be disclosed in pull requests, and submitted code must be fully understood by the submitter.
- The repo is colocated with Jujutsu (`.jj/`) as well as Git; don't touch `.jj/`.
