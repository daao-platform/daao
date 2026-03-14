-- 013_recordings_persist_on_delete.up.sql
-- Fix: Recording metadata should persist when sessions are deleted.
-- Changes ON DELETE CASCADE to ON DELETE SET NULL so recording history is preserved.
-- The .cast files on disk are unaffected by session deletion.

-- Step 1: Make session_id nullable (recordings persist even after session is deleted)
ALTER TABLE session_recordings ALTER COLUMN session_id DROP NOT NULL;

-- Step 2: Drop the old CASCADE constraint and add SET NULL
ALTER TABLE session_recordings DROP CONSTRAINT IF EXISTS session_recordings_session_id_fkey;
ALTER TABLE session_recordings
    ADD CONSTRAINT session_recordings_session_id_fkey
        FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE SET NULL;
