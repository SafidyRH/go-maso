# ADR 0001: Separate applications, environments, and monitors

- Status: accepted
- Date: 2026-10-16

## Context

The prototype stores an application and its single HTTP health check in one row. This prevents an application from having production and staging environments, multiple endpoints, or independently configurable checks.

## Decision

The monitoring catalog uses three separate resources:

- `Application` is the product or service owned by a user or team.
- `Environment` belongs to one application and represents a deployment context such as production or staging.
- `Monitor` belongs to one environment and defines one HTTP(S) check.

Check results belong to a monitor. Application status and statistics are derived from its active monitors. Deletion through the API archives catalog resources; check history remains available until the retention policy removes it.

The initial API is not versioned and has not been released, so the prototype application-level monitoring endpoints are replaced by monitor-level endpoints before the first stable API.

## Consequences

- One application can have several environments and several monitors per environment.
- Scheduling and state calculations operate on monitor IDs.
- The phase 2 migration converts every existing application to a `production` environment with one `Health check` monitor and transfers its existing history.
- Future authentication can replace the temporary `owner` label with a workspace or user foreign key.
