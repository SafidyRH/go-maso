ALTER TABLE applications
    ADD COLUMN owner VARCHAR(120) NOT NULL DEFAULT '',
    ADD COLUMN archived_at TIMESTAMPTZ;

CREATE TABLE environments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL
        REFERENCES applications(id)
        ON DELETE CASCADE,
    name VARCHAR(120) NOT NULL,
    base_url TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX environments_application_name_unique
    ON environments(application_id, LOWER(name))
    WHERE archived_at IS NULL;

CREATE INDEX environments_application_id_idx
    ON environments(application_id);

CREATE TABLE monitors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    environment_id UUID NOT NULL
        REFERENCES environments(id)
        ON DELETE CASCADE,
    name VARCHAR(120) NOT NULL,
    url TEXT NOT NULL,
    method VARCHAR(10) NOT NULL DEFAULT 'GET'
        CHECK (method IN ('GET', 'HEAD')),
    check_interval_seconds INTEGER NOT NULL DEFAULT 30
        CHECK (check_interval_seconds >= 5),
    timeout_ms INTEGER NOT NULL DEFAULT 5000
        CHECK (timeout_ms BETWEEN 100 AND 60000),
    expected_status_min INTEGER NOT NULL DEFAULT 200
        CHECK (expected_status_min BETWEEN 100 AND 599),
    expected_status_max INTEGER NOT NULL DEFAULT 299
        CHECK (expected_status_max BETWEEN 100 AND 599),
    failure_threshold INTEGER NOT NULL DEFAULT 3
        CHECK (failure_threshold BETWEEN 1 AND 20),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    current_status VARCHAR(10) NOT NULL DEFAULT 'UNKNOWN'
        CHECK (current_status IN ('UP', 'DOWN', 'DEGRADED', 'UNKNOWN')),
    consecutive_failures INTEGER NOT NULL DEFAULT 0
        CHECK (consecutive_failures >= 0),
    last_checked_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (expected_status_min <= expected_status_max)
);

CREATE UNIQUE INDEX monitors_environment_name_unique
    ON monitors(environment_id, LOWER(name))
    WHERE archived_at IS NULL;

CREATE INDEX monitors_environment_id_idx
    ON monitors(environment_id);

CREATE TABLE check_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    monitor_id UUID NOT NULL
        REFERENCES monitors(id)
        ON DELETE CASCADE,
    outcome VARCHAR(10) NOT NULL
        CHECK (outcome IN ('UP', 'DOWN')),
    computed_status VARCHAR(10) NOT NULL
        CHECK (computed_status IN ('UP', 'DOWN', 'DEGRADED', 'UNKNOWN')),
    consecutive_failures INTEGER NOT NULL DEFAULT 0
        CHECK (consecutive_failures >= 0),
    status_code INTEGER NOT NULL DEFAULT 0,
    response_time_ms BIGINT NOT NULL
        CHECK (response_time_ms >= 0),
    error_message TEXT NOT NULL DEFAULT '',
    checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX check_results_monitor_checked_at_idx
    ON check_results(monitor_id, checked_at DESC);

INSERT INTO environments (application_id, name, base_url, enabled, created_at, updated_at)
SELECT id, 'production', url, enabled, created_at, updated_at
FROM applications;

INSERT INTO monitors (
    environment_id,
    name,
    url,
    method,
    check_interval_seconds,
    timeout_ms,
    expected_status_min,
    expected_status_max,
    failure_threshold,
    enabled,
    current_status,
    last_checked_at,
    created_at,
    updated_at
)
SELECT
    e.id,
    'Health check',
    a.health_check_url,
    'GET',
    a.check_interval_seconds,
    5000,
    200,
    299,
    3,
    a.enabled,
    COALESCE(latest.status, 'UNKNOWN'),
    latest.checked_at,
    a.created_at,
    a.updated_at
FROM applications a
JOIN environments e ON e.application_id = a.id
LEFT JOIN LATERAL (
    SELECT status, checked_at
    FROM health_checks
    WHERE application_id = a.id
    ORDER BY checked_at DESC
    LIMIT 1
) latest ON TRUE;

INSERT INTO check_results (
    id,
    monitor_id,
    outcome,
    computed_status,
    consecutive_failures,
    status_code,
    response_time_ms,
    error_message,
    checked_at
)
SELECT
    hc.id,
    m.id,
    hc.status,
    hc.status,
    0,
    hc.status_code,
    hc.response_time_ms,
    hc.error_message,
    hc.checked_at
FROM health_checks hc
JOIN environments e ON e.application_id = hc.application_id
JOIN monitors m ON m.environment_id = e.id;

DROP TABLE health_checks;

ALTER TABLE applications
    DROP COLUMN url,
    DROP COLUMN health_check_url,
    DROP COLUMN check_interval_seconds,
    DROP COLUMN enabled;
