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
- Version bumps are done with `make version <version>`, which updates `server/VERSION` and `package.json`/`package-lock.json` together. This fork uses `<upstream>-ludsoft.<fork>` versions, see "Versions and releases" below. Don't bump versions unless asked.
- Container images are built by `.github/workflows/container-release.yml`.
- If a change affects setup, config or user-facing behavior, consider updating the docs in `doc/docs/` and `FEATURES.md`.

## Version control: use jj, not git

This repo is a colocated [Jujutsu](https://jj-vcs.github.io/jj/) repository (`.jj/` next to `.git/`). **Always use `jj` for revisions** (creating, describing, moving, rebasing, pushing). Do not use `git commit`, `git checkout`/`switch`, `git branch`, `git stash`, `git rebase`, `git reset` or `git push`: they bypass jj's operation log and can leave jj with a divergent view. Never edit `.jj/` by hand.

- In jj the working copy is itself a revision (`@`), changes are snapshotted automatically (no staging area), and branches are called _bookmarks_.
- Common commands:

  | Task                                       | Command                                                                                         |
  | ------------------------------------------ | ----------------------------------------------------------------------------------------------- |
  | Status / history / diff                    | `jj st`, `jj log`, `jj diff` (`jj diff -r <rev>`)                                               |
  | Describe the current revision              | `jj describe -m "message"`                                                                      |
  | Finish it and start a new empty one on top | `jj commit -m "message"` (or `jj describe` + `jj new`)                                          |
  | Start new work on top of a revision        | `jj new <rev>` (e.g. `jj new dev`)                                                              |
  | Create / move a bookmark (branch)          | `jj bookmark create <name> -r <rev>`, `jj bookmark set <name> -r <rev>`                         |
  | Push a bookmark to the fork                | `jj git push --bookmark <name>` (also works for new bookmarks, which get tracked automatically) |
  | Fetch / rebase onto upstream               | `jj git fetch`, see "Syncing with upstream" below                                               |
  | Undo the last jj operation                 | `jj undo` (see `jj op log`)                                                                     |

- Keep one logical change per revision, with a descriptive message (same style as the existing history), and point the feature's bookmark at its last revision before pushing.
- Read-only git commands (`git log`, `git show`, `git diff`) are harmless, but prefer the jj equivalents.

## Fork model (ludsoft/Watcharr)

This repository is a **hard fork** of [sbondCo/Watcharr](https://github.com/sbondCo/Watcharr), maintained as a **patch stack on top of upstream** (like a distro's downstream patches):

| Branch / remote   | Role                                                                                                     |
| ----------------- | -------------------------------------------------------------------------------------------------------- |
| `upstream` remote | sbondCo/Watcharr. Fetch only, never push.                                                                |
| `dev`             | **Mirror of upstream `dev`. Never commit to it**; it only moves to `dev@upstream`.                       |
| `ludsoft-dev`     | **Default branch**: all fork revisions, on top of upstream `dev`. Images and releases are built from it. |
| feature bookmarks | Work in progress, on top of `ludsoft-dev`.                                                               |

`trunk()` is aliased to `dev@upstream` (repo jj config), so the whole fork stack (`trunk()..ludsoft-dev`) stays mutable and can be rebased.

**Keep fork changes easy to carry:** new code in new files/packages, small hooks into shared files, one logical change per revision, and fork-only plumbing (CI, AGENTS.md, versions) in their own revisions, separate from features (features can then be proposed upstream as they are).

**Stored values must never clash with upstream** (they end up in users' databases):

- Fork values of numeric enums that are persisted (e.g. `entity.UserType`, `entity.ActivityCreatedBy`) **count down from 99** (`OIDC_USER = 99`, next fork value 98, ...). Upstream counts up from 0.
- Fork data migrations go in `server/database/migrate/migrations_fork.go` (run after upstream's), with IDs `YYYYMMDDHHMM_ludsoft_NNNN`. Never add fork migrations to upstream's `migrations.go`.

### Merging a feature into `ludsoft-dev`: fast-forward, never the PR button

GitHub's merge button always rewrites history (merge commit, squash, or "Rebase and merge" which creates new SHAs). Fast-forward `ludsoft-dev` with jj instead; GitHub marks the PR as merged automatically.

```sh
jj git fetch
jj rebase -s <first revision of the feature> -d ludsoft-dev   # only if ludsoft-dev moved
jj git push --bookmark <feature>                             # PR head = exactly what gets merged
jj bookmark set ludsoft-dev -r <feature>                     # forward move only
jj git push --bookmark ludsoft-dev
```

Only merge when the feature passes the checks. Outside of an upstream sync, never rewrite `ludsoft-dev`.

### Syncing with upstream

Do it per upstream release (or whenever upstream `dev` has something needed):

```sh
jj git fetch --remote upstream
jj bookmark set dev -r dev@upstream --allow-backwards && jj git push --bookmark dev   # update the mirror
jj rebase -b ludsoft-dev -d dev@upstream     # replay the fork stack on top of upstream
jj log -r 'conflicts()'                      # resolve any conflicted revisions (jj resolve / edit)
# run the checks, then:
jj git push --bookmark ludsoft-dev           # this is the one allowed rewrite of ludsoft-dev
```

The version revision (see below) usually conflicts on `server/VERSION`/`package.json`: resolve it by bumping to the new upstream version (and the fork version as needed).

### Versions and releases

Fork versions are `<upstream version>-ludsoft.<fork version>`, both parts semver and independent, for example `4.4.0-ludsoft.1.0.0` (the upstream release the stack is based on, then the fork's own version). Prior art: LinuxServer.io (`<upstream>-ls<n>`), Debian (`<upstream>-<revision>`). Note: container tags only allow `[A-Za-z0-9_.-]` (no `/` or `+`), and strict semver tools see the suffix as a pre-release, which is fine for pinning.

Release steps (one revision on top of `ludsoft-dev`):

1. `make version <upstream>-ludsoft.<fork>` (updates `server/VERSION`, `package.json`, `package-lock.json`).
2. In `CHANGELOG.md`, turn `# [Unreleased]` into `# [<version>] - <UTC date>` and update the compare links at the bottom (to `ludsoft/Watcharr`).
3. `jj describe -m "v<version>"`, `jj tag set v<version> -r @`, move `ludsoft-dev` to it.
4. `jj git push --bookmark ludsoft-dev`, then `jj git push --tag v<version>`: CI publishes `ghcr.io/ludsoft/watcharr:v<version>` (and `latest`). The tag run doesn't rebuild: it waits for the `ludsoft-dev` build of the same commit and re-tags its `sha-<short sha>` image (it only builds if that image is missing). Images are amd64 only.

Bump the fork part like semver: major for breaking changes for users (config/data), minor for features, patch for fixes. When the upstream part changes, the fork part keeps counting (it is not reset).

## Contribution rules

- Large changes (more than a few lines) should be discussed in an issue first (see `CONTRIBUTING.md`).
- AI usage must be disclosed in pull requests, and submitted code must be fully understood by the submitter.
