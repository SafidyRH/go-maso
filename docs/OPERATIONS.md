# Go-Maso operations

These procedures cover the current local Docker Compose PostgreSQL instance. Test them against disposable data before using them in another environment.

## Backup

Create the destination directory and stream a custom-format PostgreSQL backup from the container:

```powershell
New-Item -ItemType Directory -Force -Path .\backups | Out-Null
docker exec -e PGPASSWORD=gomaso go-maso-postgres pg_dump --username=gomaso --dbname=go_maso --format=custom --no-owner --no-privileges --file=/tmp/go_maso.dump
docker cp go-maso-postgres:/tmp/go_maso.dump .\backups\go_maso.dump
```

Store production backups outside the host running Go-Maso, encrypt them at rest, restrict access, and define an expiration policy.

## Restore test

The following operation replaces data and must only target an explicitly selected disposable database:

```powershell
docker exec -e PGPASSWORD=gomaso go-maso-postgres createdb --username=gomaso go_maso_restore_test
docker cp .\backups\go_maso.dump go-maso-postgres:/tmp/go_maso.dump
docker exec -e PGPASSWORD=gomaso go-maso-postgres pg_restore --username=gomaso --dbname=go_maso_restore_test --clean --if-exists --no-owner --no-privileges /tmp/go_maso.dump
```

Validate the restored schema and representative records, then remove the disposable database manually after confirming its exact name.

## Upgrade

1. Create and verify a backup.
2. Stop the API while leaving PostgreSQL available.
3. Build or install the new API version.
4. Run `go run ./cmd/migrate status`, then `go run ./cmd/migrate up` from `apps/api`.
5. Start the API and check `/healthz` and `/readyz`.
6. Validate application listing, a manual check, monitoring history, and dashboard aggregates.

## Rollback

Application rollback and database rollback are separate decisions. Prefer restoring the previous application when its schema remains compatible. Run `go run ./cmd/migrate down 1` only after reviewing that migration's down SQL and its data-loss implications.

## Recovery checks

After an unexpected restart:

- `/healthz` returns HTTP 200;
- `/readyz` reports the database as `up`;
- enabled monitors resume checks;
- new results appear in history;
- previously stored history remains available.
