-- 012_session_recordings.up.sql
-- Session Recording & Playback (#18)

-- 1. App-level settings key/value store (for global recording toggle etc.)
CREATE TABLE IF NOT EXISTS app_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO app_settings (key, value)
VALUES ('recording_enabled', 'true')
ON CONFLICT DO NOTHING;

-- 2. Per-session recording toggle (defaults to true = inherit global)
ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS recording_enabled BOOLEAN NOT NULL DEFAULT true;

-- 3. Recording metadata (actual .cast files live on disk, not in DB)
CREATE TABLE session_recordings (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id  UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    filename    TEXT NOT NULL,          -- relative path under recordings dir
    size_bytes  BIGINT NOT NULL DEFAULT 0,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    stopped_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_recordings_session
    ON session_recordings(session_id);
