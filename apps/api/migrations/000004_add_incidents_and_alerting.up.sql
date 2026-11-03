CREATE TABLE alert_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    monitor_id UUID NOT NULL UNIQUE
        REFERENCES monitors(id)
        ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    webhook_url TEXT NOT NULL DEFAULT '',
    failure_threshold INTEGER NOT NULL DEFAULT 3
        CHECK (failure_threshold BETWEEN 1 AND 20),
    failure_window_seconds INTEGER NOT NULL DEFAULT 300
        CHECK (failure_window_seconds BETWEEN 30 AND 86400),
    latency_threshold_ms INTEGER NOT NULL DEFAULT 0
        CHECK (latency_threshold_ms BETWEEN 0 AND 600000),
    latency_consecutive_checks INTEGER NOT NULL DEFAULT 3
        CHECK (latency_consecutive_checks BETWEEN 1 AND 20),
    cooldown_seconds INTEGER NOT NULL DEFAULT 300
        CHECK (cooldown_seconds BETWEEN 0 AND 86400),
    notify_recovery BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (NOT enabled OR webhook_url <> '')
);

CREATE TABLE maintenance_windows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    monitor_id UUID NOT NULL
        REFERENCES monitors(id)
        ON DELETE CASCADE,
    reason VARCHAR(240) NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    cancelled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (ends_at > starts_at)
);

CREATE INDEX maintenance_windows_monitor_period_idx
    ON maintenance_windows(monitor_id, starts_at, ends_at)
    WHERE cancelled_at IS NULL;

CREATE TABLE incidents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    monitor_id UUID NOT NULL
        REFERENCES monitors(id)
        ON DELETE CASCADE,
    kind VARCHAR(20) NOT NULL
        CHECK (kind IN ('AVAILABILITY', 'LATENCY')),
    status VARCHAR(20) NOT NULL DEFAULT 'OPEN'
        CHECK (status IN ('OPEN', 'ACKNOWLEDGED', 'RESOLVED')),
    summary VARCHAR(240) NOT NULL,
    trigger_check_result_id UUID
        REFERENCES check_results(id)
        ON DELETE SET NULL,
    started_at TIMESTAMPTZ NOT NULL,
    last_observation_at TIMESTAMPTZ NOT NULL,
    acknowledged_at TIMESTAMPTZ,
    acknowledged_by VARCHAR(120) NOT NULL DEFAULT '',
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX incidents_one_active_kind_per_monitor_idx
    ON incidents(monitor_id, kind)
    WHERE status <> 'RESOLVED';

CREATE INDEX incidents_monitor_started_at_idx
    ON incidents(monitor_id, started_at DESC);

CREATE INDEX incidents_status_started_at_idx
    ON incidents(status, started_at DESC);

CREATE TABLE incident_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id UUID NOT NULL
        REFERENCES incidents(id)
        ON DELETE CASCADE,
    event_type VARCHAR(30) NOT NULL
        CHECK (event_type IN ('OPENED', 'ACKNOWLEDGED', 'RESOLVED', 'NOTIFICATION_QUEUED', 'NOTIFICATION_DELIVERED', 'NOTIFICATION_FAILED')),
    message TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX incident_events_incident_created_at_idx
    ON incident_events(incident_id, created_at DESC);

CREATE TABLE notification_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id UUID NOT NULL
        REFERENCES incidents(id)
        ON DELETE CASCADE,
    event_type VARCHAR(30) NOT NULL
        CHECK (event_type IN ('INCIDENT_OPENED', 'INCIDENT_RESOLVED')),
    channel VARCHAR(20) NOT NULL DEFAULT 'WEBHOOK'
        CHECK (channel IN ('WEBHOOK')),
    webhook_url TEXT NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING', 'SENDING', 'DELIVERED', 'FAILED')),
    attempts INTEGER NOT NULL DEFAULT 0
        CHECK (attempts >= 0),
    max_attempts INTEGER NOT NULL DEFAULT 5
        CHECK (max_attempts BETWEEN 1 AND 20),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    response_status INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    delivered_at TIMESTAMPTZ,
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX notification_deliveries_pending_idx
    ON notification_deliveries(next_attempt_at, created_at)
    WHERE status IN ('PENDING', 'FAILED');

