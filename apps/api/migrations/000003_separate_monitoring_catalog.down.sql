ALTER TABLE applications
    ADD COLUMN url TEXT NOT NULL DEFAULT '',
    ADD COLUMN health_check_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN check_interval_seconds INTEGER NOT NULL DEFAULT 30
        CHECK (check_interval_seconds >= 5),
    ADD COLUMN enabled BOOLEAN NOT NULL DEFAULT TRUE;

UPDATE applications a
SET
    url = selected.base_url,
    health_check_url = selected.monitor_url,
    check_interval_seconds = selected.check_interval_seconds,
    enabled = selected.environment_enabled AND selected.monitor_enabled
FROM (
    SELECT DISTINCT ON (e.application_id)
        e.application_id,
        e.base_url,
        e.enabled AS environment_enabled,
        m.url AS monitor_url,
        m.check_interval_seconds,
        m.enabled AS monitor_enabled
    FROM environments e
    JOIN monitors m ON m.environment_id = e.id
    WHERE e.archived_at IS NULL AND m.archived_at IS NULL
    ORDER BY e.application_id, e.created_at, m.created_at
) selected
WHERE selected.application_id = a.id;

CREATE TABLE health_checks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL
        REFERENCES applications(id)
        ON DELETE CASCADE,
    status VARCHAR(10) NOT NULL
        CHECK (status IN ('UP', 'DOWN')),
    status_code INTEGER NOT NULL DEFAULT 0,
    response_time_ms BIGINT NOT NULL
        CHECK (response_time_ms >= 0),
    error_message TEXT NOT NULL DEFAULT '',
    checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO health_checks (
    id,
    application_id,
    status,
    status_code,
    response_time_ms,
    error_message,
    checked_at
)
SELECT
    cr.id,
    e.application_id,
    cr.outcome,
    cr.status_code,
    cr.response_time_ms,
    cr.error_message,
    cr.checked_at
FROM check_results cr
JOIN monitors m ON m.id = cr.monitor_id
JOIN environments e ON e.id = m.environment_id;

CREATE INDEX idx_health_checks_application_checked_at
    ON health_checks(application_id, checked_at DESC);

DROP TABLE check_results;
DROP TABLE monitors;
DROP TABLE environments;

ALTER TABLE applications
    DROP COLUMN owner,
    DROP COLUMN archived_at;
