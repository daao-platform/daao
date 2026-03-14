-- Create secret_scopes table for per-satellite secret configuration
-- satellite_id NULL = global scope
CREATE TABLE secret_scopes (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    satellite_id    UUID REFERENCES satellites(id),
    provider        TEXT NOT NULL,
    secret_ref      TEXT NOT NULL,
    backend         TEXT NOT NULL DEFAULT 'local',
    lease_ttl_min   INTEGER DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
