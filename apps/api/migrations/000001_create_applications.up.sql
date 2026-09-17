CREATE TABLE applications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    name VARCHAR(120) NOT NULL,
    url TEXT NOT NULL,
    health_check_url TEXT NOT NULL,

    check_interval_seconds INTEGER NOT NULL DEFAULT 30
        CHECK (check_interval_seconds >= 5),

    enabled BOOLEAN NOT NULL DEFAULT TRUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);