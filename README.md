# Go-Maso

Go-Maso is a lightweight monitoring platform. The current implementation contains the Go API, PostgreSQL persistence, HTTP checks, scheduling, monitoring history, and dashboard aggregates.

## Requirements

- Go 1.26.2 or newer compatible toolchain
- Docker Desktop with Docker Compose
- PowerShell for the Windows commands below

## Start the development environment on Windows

From the repository root:

```powershell
docker compose up -d postgres

$env:DATABASE_URL = 'postgres://gomaso:gomaso@localhost:5434/go_maso?sslmode=disable'
$env:HTTP_ADDRESS = ':8080'

Set-Location apps/api
go run ./cmd/migrate up
go run ./cmd/api
```

If port `5434` is already in use, choose another host port before starting Compose and use the same port in `DATABASE_URL`:

```powershell
$env:POSTGRES_PORT = '5435'
$env:DATABASE_URL = 'postgres://gomaso:gomaso@localhost:5435/go_maso?sslmode=disable'
docker compose up -d postgres
```

In another terminal, verify liveness and database readiness:

```powershell
Invoke-RestMethod http://localhost:8080/healthz
Invoke-RestMethod http://localhost:8080/readyz
```

`GET /health` remains available as a compatibility alias for `GET /readyz`.

## Database migrations

Run commands from `apps/api` with `DATABASE_URL` set:

```powershell
go run ./cmd/migrate up
go run ./cmd/migrate status
go run ./cmd/migrate down 1
```

Every migration must have matching `*.up.sql` and `*.down.sql` files. Migrations are embedded in the binary and protected by a PostgreSQL advisory lock so that only one runner applies them at a time.

## Quality checks

```powershell
Set-Location apps/api
gofmt -w .
go vet ./...
go test ./...
```

The CI pipeline performs formatting, vet, PostgreSQL migration integration tests, and race-enabled tests on Linux for each push and pull request. The race detector requires a CGO-enabled toolchain and is therefore intentionally run in CI rather than assumed on every Windows workstation.

## Configuration

| Variable | Required | Default | Description |
|---|---:|---|---|
| `DATABASE_URL` | yes | — | PostgreSQL connection URL |
| `HTTP_ADDRESS` | no | `:8080` | API listen address |
| `SHUTDOWN_TIMEOUT` | no | `10s` | Graceful HTTP shutdown deadline |
| `MONITOR_WORKERS` | no | `10` | Maximum number of checks running concurrently |
| `RESULT_RETENTION_DAYS` | no | `30` | Number of days to retain raw check results |

See `.env.example` for local values. The application does not automatically load `.env`; export the variables in the shell or inject them through the process manager.

## API currently available

- `GET /healthz`
- `GET /readyz`
- `POST /applications`
- `GET /applications`
- `GET|PATCH|DELETE /applications/{id}`
- `POST|GET /applications/{applicationID}/environments`
- `GET|PATCH|DELETE /environments/{id}`
- `POST|GET /environments/{environmentID}/monitors`
- `GET|PATCH|DELETE /monitors/{id}`
- `POST /monitors/{id}/check`
- `GET /monitors/{id}/checks`
- `GET /monitors/{id}/monitoring-summary`
- `GET /dashboard`

The complete request and response contract is in `docs/openapi.yaml`.

## Operations

Backup, restore, update, and recovery procedures are documented in `docs/OPERATIONS.md`.
