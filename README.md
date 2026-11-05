# Go-Maso

Go-Maso is a lightweight monitoring platform. The current implementation contains the Go API, PostgreSQL persistence, HTTP checks, scheduling, monitoring history, and an embedded operational dashboard.

## Requirements

- Go 1.26.2 or newer compatible toolchain
- Docker Desktop with Docker Compose
- PowerShell for the Windows commands below

## Start the complete MVP with Docker

From the repository root:

```powershell
./scripts/init-secrets.ps1
docker compose up -d --build
```

The script creates four ignored secret files without overwriting existing values. Read `secrets/admin_password.txt` for the initial login, then open `http://localhost:8080`. The container applies database migrations before starting the API.

If a host port is already in use, select another one before starting Compose:

```powershell
$env:POSTGRES_PORT = '5435'
$env:API_PORT = '8081'
docker compose up -d --build
```

In another terminal, verify liveness and database readiness:

```powershell
Invoke-RestMethod http://localhost:8080/healthz
Invoke-RestMethod http://localhost:8080/readyz
```

`GET /health` remains available as a compatibility alias for `GET /readyz`.

Open `http://localhost:8080` to use the dashboard. The interface is embedded in the API binary, requires no separate frontend process, refreshes every 30 seconds, and displays UTC timestamps in the browser's local time zone.

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
| `DATABASE_URL` or `DATABASE_URL_FILE` | yes | — | PostgreSQL connection URL or path to its secret file |
| `HTTP_ADDRESS` | no | `:8080` | API listen address |
| `SHUTDOWN_TIMEOUT` | no | `10s` | Graceful HTTP shutdown deadline |
| `MONITOR_WORKERS` | no | `10` | Maximum number of checks running concurrently |
| `RESULT_RETENTION_DAYS` | no | `30` | Number of days to retain raw check results |
| `ADMIN_EMAIL` | no | `admin@gomaso.local` | Bootstrap administrator email |
| `ADMIN_PASSWORD` or `ADMIN_PASSWORD_FILE` | first start | — | Bootstrap administrator password; minimum 12 characters |
| `SECRETS_ENCRYPTION_KEY` or `SECRETS_ENCRYPTION_KEY_FILE` | yes | — | Standard Base64 encoding of exactly 32 random bytes |
| `SESSION_SECURE_COOKIE` | production | `false` | Must be `true` behind HTTPS to use `Secure` `__Host-` cookies |
| `SESSION_IDLE_TIMEOUT` | no | `30m` | Session idle expiration |
| `SESSION_ABSOLUTE_TIMEOUT` | no | `12h` | Session absolute expiration |
| `TRUST_PROXY_HEADERS` | no | `false` | Trust the first `X-Forwarded-For` address only behind a controlled proxy |

See `.env.example` for local values. The application does not automatically load `.env`; export the variables in the shell or inject them through the process manager.

## API currently available

- `GET /healthz`
- `GET /readyz`
- `POST /auth/login`
- `GET /auth/me`
- `POST /auth/logout`
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
- `GET /dashboard/applications/{id}?period_hours=24`
- `GET|PUT /monitors/{id}/alert-rule`
- `POST /monitors/{id}/alert-rule/signing-secret/rotate`
- `GET /incidents`
- `GET /incidents/{id}`
- `POST /incidents/{id}/acknowledge`
- `GET /incidents/{id}/events`
- `GET /incidents/{id}/deliveries`
- `POST|GET /monitors/{id}/maintenance-windows`
- `DELETE /maintenance-windows/{id}`

The complete request and response contract is in `docs/openapi.yaml`.

## Alerting

Alert rules are configured per monitor from the dashboard. The MVP notification channel is a generic public HTTP(S) webhook. Go-Maso validates and re-resolves webhook destinations using the same SSRF protections as monitor targets, sends an idempotency key, retries failed deliveries with bounded backoff, and can send a recovery notification when the incident closes.

Webhook destinations and HMAC keys are encrypted with AES-256-GCM before storage. The key is revealed only when a rule is created or explicitly rotated. Each request carries `X-Go-Maso-Timestamp` and `X-Go-Maso-Signature`; verification is documented in `docs/OPERATIONS.md`.

Failure thresholds, observation windows, excessive-latency thresholds, cooldown, acknowledgement, delivery history, and planned maintenance windows are persisted in PostgreSQL. Active maintenance suppresses new incidents and notifications for the selected monitor.

## Operations

Backup, restore, update, and recovery procedures are documented in `docs/OPERATIONS.md`.
