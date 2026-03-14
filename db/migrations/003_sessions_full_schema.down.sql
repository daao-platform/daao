-- Reverse migration: remove columns added in 003
ALTER TABLE sessions DROP COLUMN IF EXISTS name;
ALTER TABLE sessions DROP COLUMN IF EXISTS agent_binary;
ALTER TABLE sessions DROP COLUMN IF EXISTS agent_args;
ALTER TABLE sessions DROP COLUMN IF EXISTS os_pid;
ALTER TABLE sessions DROP COLUMN IF EXISTS pts_name;
ALTER TABLE sessions DROP COLUMN IF EXISTS cols;
ALTER TABLE sessions DROP COLUMN IF EXISTS rows;
ALTER TABLE sessions DROP COLUMN IF EXISTS last_activity_at;
ALTER TABLE sessions DROP COLUMN IF EXISTS started_at;
ALTER TABLE sessions DROP COLUMN IF EXISTS detached_at;
ALTER TABLE sessions DROP COLUMN IF EXISTS suspended_at;
ALTER TABLE sessions DROP COLUMN IF EXISTS terminated_at;
DROP INDEX IF EXISTS idx_sessions_active;
