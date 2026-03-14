-- 018_context_files.up.sql
-- Context Files DB Migration - §10.3

-- Satellite context files table
CREATE TABLE IF NOT EXISTS satellite_context_files (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    satellite_id    UUID NOT NULL REFERENCES satellites(id),
    file_path       TEXT NOT NULL,
    content         TEXT NOT NULL,
    version         INTEGER NOT NULL DEFAULT 1,
    last_modified_by TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (satellite_id, file_path)
);

-- Context file history table (FK to satellite_context_files)
CREATE TABLE IF NOT EXISTS context_file_history (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    context_file_id UUID NOT NULL REFERENCES satellite_context_files(id),
    version         INTEGER NOT NULL,
    content         TEXT NOT NULL,
    modified_by     TEXT NOT NULL,
    modified_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_context_file_history_context_file_id
    ON context_file_history(context_file_id);
