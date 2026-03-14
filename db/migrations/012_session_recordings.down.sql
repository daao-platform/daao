-- 012_session_recordings.down.sql

DROP TABLE IF EXISTS session_recordings;
ALTER TABLE sessions DROP COLUMN IF EXISTS recording_enabled;
DROP TABLE IF EXISTS app_settings;
