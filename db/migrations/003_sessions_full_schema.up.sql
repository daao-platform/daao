-- Add missing columns to sessions table to match full PRD schema
-- These columns support session naming, agent info, terminal dimensions, and lifecycle timestamps.

-- Session metadata
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT 'default';
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS agent_binary TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS agent_args TEXT[] NOT NULL DEFAULT '{}';

-- OS-level process info (set by satellite after fork)
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS os_pid INTEGER;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS pts_name TEXT;

-- Terminal dimensions
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS cols SMALLINT NOT NULL DEFAULT 80;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS rows SMALLINT NOT NULL DEFAULT 24;

-- Lifecycle timestamps
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS last_activity_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW();
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS started_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS detached_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS suspended_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS terminated_at TIMESTAMP WITH TIME ZONE;

-- Index for active sessions (non-terminated)
CREATE INDEX IF NOT EXISTS idx_sessions_active ON sessions (state) WHERE terminated_at IS NULL;
