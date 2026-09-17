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

CREATE INDEX idx_health_checks_application_checked_at
    ON health_checks(application_id, checked_at DESC);