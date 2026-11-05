# Go-Maso operations

These procedures cover the current local Docker Compose PostgreSQL instance. Test them against disposable data before using them in another environment.

## Backup

Create the destination directory and stream a custom-format PostgreSQL backup from the container:

```powershell
New-Item -ItemType Directory -Force -Path .\backups | Out-Null
docker exec go-maso-postgres sh -c 'export PGPASSWORD="$(cat /run/secrets/postgres_password)"; pg_dump --username=gomaso --dbname=go_maso --format=custom --no-owner --no-privileges --file=/tmp/go_maso.dump'
docker cp go-maso-postgres:/tmp/go_maso.dump .\backups\go_maso.dump
```

Store production backups outside the host running Go-Maso, encrypt them at rest, restrict access, and define an expiration policy.

## Restore test

The following operation replaces data and must only target an explicitly selected disposable database:

```powershell
docker exec go-maso-postgres sh -c 'export PGPASSWORD="$(cat /run/secrets/postgres_password)"; createdb --username=gomaso go_maso_restore_test'
docker cp .\backups\go_maso.dump go-maso-postgres:/tmp/go_maso.dump
docker exec go-maso-postgres sh -c 'export PGPASSWORD="$(cat /run/secrets/postgres_password)"; pg_restore --username=gomaso --dbname=go_maso_restore_test --clean --if-exists --no-owner --no-privileges /tmp/go_maso.dump'
```

Validate the restored schema and representative records, then remove the disposable database manually after confirming its exact name.

## Upgrade

1. Create and verify a backup.
2. Stop the API while leaving PostgreSQL available.
3. Build or install the new API version.
4. Run `go run ./cmd/migrate status`, then `go run ./cmd/migrate up` from `apps/api`.
5. Start the API and check `/healthz` and `/readyz`.
6. Validate application listing, a manual check, monitoring history, and dashboard aggregates.
7. Confirm pending webhook deliveries resume and review failed attempts from the incident history.

## Rollback

Application rollback and database rollback are separate decisions. Prefer restoring the previous application when its schema remains compatible. Run `go run ./cmd/migrate down 1` only after reviewing that migration's down SQL and its data-loss implications.

## Recovery checks

After an unexpected restart:

- `/healthz` returns HTTP 200;
- `/readyz` reports the database as `up`;
- enabled monitors resume checks;
- new results appear in history;
- previously stored history remains available.
- interrupted webhook deliveries return to the retry queue;
- open incidents and planned maintenance windows remain visible in the dashboard.

## Webhook delivery failures

Go-Maso retries failed webhook deliveries up to five times with increasing delays. Use `GET /incidents/{id}/deliveries` to inspect attempts, HTTP response codes, and the last error. Destination URLs and payloads are deliberately omitted from this response.

Before changing a failed destination, verify its DNS records and TLS configuration. Webhooks resolving to private, loopback, link-local, documentation, or otherwise prohibited networks are rejected. Saving a corrected alert rule affects future incident notifications; existing queued deliveries retain their original destination for auditability.

## Webhook signature verification

The dashboard reveals a monitor's signing key once when the alert rule is created or rotated. Store it in the receiving service's secret manager. Go-Maso signs the exact raw HTTP body with HMAC-SHA256:

```text
signed_value = X-Go-Maso-Timestamp + "." + raw_request_body
expected = "v1=" + lowercase_hex(HMAC-SHA256(signing_key, signed_value))
```

The signing key is the displayed Base64URL string itself; do not decode it before using it as the HMAC key. Compare signatures in constant time, reject timestamps older or newer than five minutes, and deduplicate accepted events with `Idempotency-Key`. Read the raw body only once and verify it before JSON parsing.

Rotation affects new deliveries. Deliveries already queued retain the previous key so retries remain reproducible; keep the previous key accepted during a short rotation grace period or rotate only after the delivery queue is empty.

## Production secret handling

- Run `scripts/init-secrets.ps1` once on the deployment host and restrict access to the generated `secrets` directory.
- Back up `encryption_key.txt` in a separate protected secret manager. Losing it makes encrypted webhook destinations and signing keys unreadable.
- Never rotate the encryption key by replacing the file alone; re-encryption tooling is required before such a rotation.
- Set `SESSION_SECURE_COOKIE=true` when the public endpoint uses HTTPS, and terminate TLS only at a controlled reverse proxy.
- Set `TRUST_PROXY_HEADERS=true` only when direct access to the API is blocked and the proxy overwrites forwarding headers.
