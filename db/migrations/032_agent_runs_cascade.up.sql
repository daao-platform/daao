-- Fix agent_runs.session_id FK to allow session deletion
-- Changes from RESTRICT (default) to SET NULL so deleting a session
-- doesn't fail when agent_runs reference it.
ALTER TABLE agent_runs DROP CONSTRAINT IF EXISTS agent_runs_session_id_fkey;
ALTER TABLE agent_runs
    ADD CONSTRAINT agent_runs_session_id_fkey
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE SET NULL;
