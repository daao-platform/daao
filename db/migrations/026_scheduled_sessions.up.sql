-- Add trigger column to agent_definitions for event-triggered agent sessions.
-- Add trigger_source column to agent_runs to track what initiated the run.
-- Part of Scheduled Sessions (#25).

ALTER TABLE agent_definitions
    ADD COLUMN IF NOT EXISTS trigger JSONB;

ALTER TABLE agent_runs
    ADD COLUMN IF NOT EXISTS trigger_source TEXT NOT NULL DEFAULT 'manual';
