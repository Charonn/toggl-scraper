# toggl-scraper

A small Go 1.25 service that scrapes Toggl Track time entries and syncs them into a MySQL table that your Metabase instance can read from.

This project uses a clean architecture layout: domain models, ports (interfaces), adapters (Toggl HTTP client, Metabase sink), and a use case orchestrating the sync.

## Structure

- `cmd/toggl-scraper`: CLI entrypoint
- `internal/domain`: Domain entities (e.g., `TimeEntry`)
- `internal/ports`: Interfaces for Toggl client and Sink
- `internal/adapter/toggl`: HTTP client for Toggl v9
- `internal/adapter/mysql`: MySQL sink adapter (upserts)
- `internal/usecase`: Sync use case
- `internal/app`: Wiring

## Configuration

Environment variables:

- `TOGGL_API_TOKEN` (required): Toggl API token
- `TOGGL_WORKSPACE_ID` (optional): Toggl workspace ID (used for metadata)
- `TOGGL_BASE_URL` (optional, default `https://api.track.toggl.com`)
- `MYSQL_DSN` (required): e.g. `user:pass@tcp(host:3306)/dbname?parseTime=true&multiStatements=true`

Flags:

- `--once`: Run a single sync and exit
- `--interval=15m`: Interval for periodic syncs (ignored with `--once`)
- `--from` / `--to`: RFC3339 time window (defaults to `[now-24h, now]`)
- `--http=:8085`: Start an HTTP trigger server (disabled by default)
- `-v`: Verbose logging

## Usage

Run a one-off sync for the last 24h:

```
TOGGL_API_TOKEN=... go run ./cmd/toggl-scraper --once
```

Run periodically every 15 minutes:

```
TOGGL_API_TOKEN=... go run ./cmd/toggl-scraper --interval=15m
```

Migrations run automatically at startup and create the table `toggl_time_entries` with columns:
`id BIGINT PRIMARY KEY, description TEXT, project_id BIGINT NULL, workspace_id BIGINT NULL, tags TEXT, start DATETIME(6) NOT NULL, stop DATETIME(6) NULL, duration_sec BIGINT NOT NULL`.
Tags are stored as a JSON-encoded string in `tags` (TEXT).

Date ranges:

- `--from` and `--to` accept RFC3339 or date-only `YYYY-MM-DD`.
- Date-only `--to` is treated as inclusive by converting to the next day at 00:00Z (exclusive upper bound).

Examples:

```
# All entries from Aug 1–15, 2025 (inclusive)
TOGGL_API_TOKEN=... MYSQL_DSN='user:pass@tcp(host:3306)/db?parseTime=true' \
  go run ./cmd/toggl-scraper --once --from 2025-08-01 --to 2025-08-15

# Explicit timestamps
go run ./cmd/toggl-scraper --once \
  --from 2025-08-01T00:00:00Z --to 2025-08-16T00:00:00Z
```

HTTP trigger (optional):

- Start the app with `--http=:8085` to enable a simple trigger server.
- Trigger a sync via curl with `from`/`to` in RFC3339 or `YYYY-MM-DD`:

```
curl "http://localhost:8085/sync?from=2025-08-01&to=2025-08-15"
# or explicit timestamps
curl "http://localhost:8085/sync?from=2025-08-01T00:00:00Z&to=2025-08-16T00:00:00Z"
```

Notes:
- Missing params default to `[now-24h, now]`.
- If a sync is already running, the endpoint returns HTTP 409.

## Docker

Build and run in Docker. The container defaults to daily-at-midnight mode (`--daily`).

Build:

```
make docker-build
```

Run (UTC midnight):

```
make docker-run E="-e TOGGL_API_TOKEN=YOUR_TOKEN -e MYSQL_DSN='user:pass@tcp(mysql:3306)/db?parseTime=true'"
```

Run with a timezone (e.g., Europe/Berlin):

```
make docker-run E="-e TOGGL_API_TOKEN=YOUR_TOKEN \
  -e MYSQL_DSN='user:pass@tcp(mysql:3306)/db?parseTime=true' \
  -e SYNC_TZ=Europe/Berlin"
```

Behavior:
- At the next local midnight (per `SYNC_TZ`), it syncs the previous 24 hours (i.e., the previous day in that timezone) into MySQL.
- It will then repeat at each subsequent midnight while the container runs.

## CI/CD Deployment (GitHub Actions + Tailscale)

This repo includes a workflow that builds and pushes a Docker image to GHCR and then deploys to your home server over Tailscale.

Prerequisites on your server (Debian):
- Install Docker and (optionally) Docker Compose
- Ensure the server is on your Tailscale tailnet
- Create an env file on the server: `/opt/toggl-scraper/.env`

Example `/opt/toggl-scraper/.env`:

```
TOGGL_API_TOKEN=... 
MYSQL_DSN=user:pass@tcp(mysql:3306)/db?parseTime=true&multiStatements=true
SYNC_TZ=UTC
```

GitHub Secrets required:
- `TAILSCALE_AUTHKEY`: Tailscale auth key to join the runner to your tailnet
- `DEPLOY_HOST`: Tailscale IP or MagicDNS name of your server
- `DEPLOY_USER`: SSH username on the server
- `SSH_PRIVATE_KEY`: Private key for the SSH user (corresponding pubkey must be in `~/.ssh/authorized_keys` on the server)
- Optional for private GHCR images: `GHCR_USERNAME`, `GHCR_TOKEN` (PAT with `read:packages`)

Flow:
1. On push to `main` (or tags), GitHub Actions builds and pushes `ghcr.io/<owner>/<repo>:latest` and related tags.
2. The job connects to Tailscale and SSHes into your server.
3. It pulls the latest image, restarts the container with `--env-file /opt/toggl-scraper/.env`.

Files:
- `.github/workflows/deploy.yml` — CI pipeline for build & deploy
- `deploy/remote-deploy.sh` — Optional helper script for manual deploys

## Notes

- Requires a MySQL driver for `database/sql` (we depend on `github.com/go-sql-driver/mysql`). Ensure your DSN includes `multiStatements=true` so migrations (which may include multiple statements) can run as a single batch.
- The Toggl client uses the v9 API (`/api/v9/me/time_entries`) with Basic auth (`token:api_token`).

## Tests (E2E with Testcontainers)

An end-to-end test spins up a real MySQL using Testcontainers, runs the sync with a fake Toggl client, and validates upserts.

Prerequisites:

- Docker running locally

Run:

```
go test -tags=e2e ./e2e -v
```

Note: The e2e tests require fetching modules and pulling a Docker image (`mysql:8.0`).

### HTTP Trigger in Docker

- The image exposes port `8085`. Start the container with the HTTP server enabled and publish the port:

```
make docker-run E="-e TOGGL_API_TOKEN=YOUR_TOKEN \
  -e MYSQL_DSN='user:pass@tcp(mysql:3306)/db?parseTime=true' \
  -p 8085:8085" \
  ARGS="--http=:8085"
```

Then trigger:

```
curl "http://localhost:8085/sync?from=2025-08-01&to=2025-08-15"
```
