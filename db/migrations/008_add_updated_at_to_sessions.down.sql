-- Revert: drop the updated_at column and trigger
DROP TRIGGER IF EXISTS trigger_sessions_updated_at ON sessions;
DROP FUNCTION IF EXISTS update_sessions_updated_at();
ALTER TABLE sessions DROP COLUMN IF EXISTS updated_at;
