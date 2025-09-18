# AGENTS: Working Guidelines for toggl-scraper

This document is for future agents and contributors working on this repository. It explains the project intent, architecture, operational model, and contribution expectations so that changes remain consistent, safe, and maintainable.

## Mission
- Scrape Toggl Track time entries and related project metadata, syncing both into MySQL tables that Metabase reads from.
- Favor the Go standard library and a clean architecture (domain → ports → adapters → use cases → wiring).
- Provide an idempotent, reliable daily sync that runs at local midnight (configurable timezone).

## Architecture
- Domain: `internal/domain`
  - Core entities (e.g., `TimeEntry`, `Project`).
- Ports: `internal/ports`
  - Interfaces for external integrations (`TogglClient`, `Sink`).
- Adapters: `internal/adapter`
  - `toggl/`: HTTP client for Toggl v9 (`/api/v9/me/time_entries`, `/api/v9/me/projects`).
  - `mysql/`: `database/sql` sink that upserts into `toggl_time_entries` and `toggl_projects`.
- Use Cases: `internal/usecase`
  - `SyncUseCase` orchestrates fetching from Toggl and writing to the sink.
- App wiring: `internal/app`
  - Wires config, logger, ports/adapters, and exposes `RunOnce`.
- CLI: `cmd/toggl-scraper`
  - Flags (`--once`, `--daily`, `--interval`, `--from`, `--to`, `-v`), signal handling, scheduling.

## Data Model (MySQL)
Tables:
- `toggl_time_entries`
  - `id BIGINT PRIMARY KEY`
  - `description TEXT`
  - `project_id BIGINT NULL`
  - `workspace_id BIGINT NULL`
  - `tags TEXT` (JSON-encoded array)
  - `start DATETIME(6) NOT NULL` (UTC)
  - `stop DATETIME(6) NULL` (UTC)
  - `duration_sec BIGINT NOT NULL`
  - Upsert: `INSERT ... ON DUPLICATE KEY UPDATE` for idempotency.
- `toggl_projects`
  - `id BIGINT PRIMARY KEY`
  - `workspace_id BIGINT NOT NULL`
  - `name TEXT NOT NULL`
  - `active TINYINT(1) NOT NULL`
  - `is_private TINYINT(1) NOT NULL`
  - `color VARCHAR(32) NOT NULL`
  - `client_id BIGINT NULL`
  - `at DATETIME(6) NOT NULL`
  - Upsert for idempotency mirrors the time entry sink strategy.

## Build & Run (Local)
Use the Makefile targets. Defaults keep caches in-repo and avoid toolchain downloads.
- `make deps` — download modules
- `make build` — build to `./bin/toggl-scraper`
- `make run ARGS="--once ..."` — run with flags
- `make test` — unit tests (currently minimal)
- `make test-e2e` — E2E tests with Testcontainers (requires Docker)

Environment overrides (defaults shown):
- `GOTOOLCHAIN=local` — do not auto-download Go toolchains
- `GOMODCACHE=.gocache/mod`, `GOPATH=.gocache`, `GOCACHE=.gocache/build`

Go version: code targets Go 1.25. For constrained environments, the Makefile runs with the local toolchain; prefer upgrading Go to 1.25 for production builds.

## Configuration (Env Vars)
- `TOGGL_API_TOKEN` (required): Toggl API token
- `TOGGL_WORKSPACE_ID` (optional): workspace ID
- `TOGGL_BASE_URL` (default: `https://api.track.toggl.com`)
- `MYSQL_DSN` (required): e.g., `user:pass@tcp(host:3306)/db?parseTime=true&multiStatements=true`
- `SYNC_TZ` (default: `UTC`): timezone name (IANA) used for midnight scheduling

## Scheduling Modes
- `--daily`: runs at local midnight (per `SYNC_TZ`), syncing the previous 24 hours (previous local day) as `[midnight-24h, midnight)`.
- `--once`: runs a single sync for the provided window (`--from`, `--to`), defaulting to last 24 hours.
- `--interval`: periodic sync every N; kept for manual/testing scenarios.
- `--http=:8080`: starts a simple HTTP trigger server with `/sync`.

Date parsing:
- `--from`/`--to` accept RFC3339 or `YYYY-MM-DD`.
- Date-only `--to` is inclusive by converting to the next-day 00:00Z (exclusive bound).

## Docker
- Image: built via `make docker-build` (multi-stage, distroless runtime).
- Runtime: `make docker-run E="-e TOGGL_API_TOKEN=... -e MYSQL_DSN=... [-e SYNC_TZ=...]"`
- Default entrypoint: `--daily` with `SYNC_TZ=UTC`.

## Testing
- E2E: `e2e/sync_e2e_test.go` (tag `e2e`) spins up MySQL via Testcontainers, runs sync with a fake Toggl client, and asserts upsert and idempotency.
  - Run with Docker available: `make test-e2e` or `go test -tags=e2e ./e2e -v`.
- Unit tests: add focused tests when you change mapping logic, time-window calculations, or error handling. Prefer `httptest.Server` for the Toggl client mapping.

## Dependencies
- Runtime:
  - Standard library
  - `github.com/go-sql-driver/mysql` (database/sql driver)
- Test-only:
  - `github.com/testcontainers/testcontainers-go`
Keep dependencies minimal; prefer stdlib first.

## Coding Standards
- Clean architecture boundaries: domain/ports isolated from concrete adapters.
- Go style: small, focused functions; zero or minimal external deps; handle contexts/timeouts; structured logs via `log/slog`.
- Errors: return annotated errors; log at edges (adapters/use cases), not deep in domain models.
- Idempotency: preserve upsert semantics and primary keys.
- Time: store and compare in UTC; convert only at input/parsing and scheduling boundaries.
- Config: read via env; validate early; provide sensible defaults.

## API Notes (Toggl)
- Time entries: `GET /api/v9/me/time_entries?start_date=RFC3339&end_date=RFC3339`
- Projects: `GET /api/v9/me/projects` (or workspace-scoped `GET /api/v9/workspaces/{workspace_id}/projects` when a workspace is configured)
- Auth: HTTP Basic with `token:api_token`.
- Running entries may have negative duration; we store the API-provided value as-is.

## Operational Guidance
- On startup in `--daily` mode, the process waits until the next local midnight.
- Consider adding an optional “initial catch-up” if users need immediate sync on container start.
- Monitor via container logs; no metrics are included by default to keep dependencies minimal.
- Optional HTTP trigger: If started with `--http`, an in-process HTTP server exposes:
  - `GET /healthz` → 200 `ok`
  - `GET/POST /sync?from=...&to=...` → triggers a sync for the provided window. Parameters accept RFC3339 or `YYYY-MM-DD`. Missing params default to `[now-24h, now]`.
  - Concurrency: the app prevents overlapping runs; if a run is in progress, `/sync` returns HTTP 409.

## Common Pitfalls
- Toolchain downloads blocked: use the Makefile defaults; for production, install Go 1.25.
- Missing MySQL index: ensure `id` is the primary key for idempotency (auto-created schema sets it).
- Timezones: `SYNC_TZ` must be an IANA TZ string (e.g., `Europe/Berlin`).

## Change Management
- Keep changes scoped and aligned to the architecture.
- If adding a new sink or source, define/extend ports first; then implement adapters.
- Update README and this AGENTS.md when behavior or interfaces change.
- Avoid adding non-essential libraries; justify any new dependency.
