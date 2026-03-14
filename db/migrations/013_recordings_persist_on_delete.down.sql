-- 013_recordings_persist_on_delete.down.sql
-- Revert: Restore ON DELETE CASCADE (recording metadata will be deleted with sessions)

ALTER TABLE session_recordings DROP CONSTRAINT IF EXISTS session_recordings_session_id_fkey;
ALTER TABLE session_recordings ALTER COLUMN session_id SET NOT NULL;
ALTER TABLE session_recordings
    ADD CONSTRAINT session_recordings_session_id_fkey
        FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE;
