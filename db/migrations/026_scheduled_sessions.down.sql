-- Revert trigger column from agent_definitions and trigger_source from agent_runs.

ALTER TABLE agent_runs
    DROP COLUMN IF EXISTS trigger_source;

ALTER TABLE agent_definitions
    DROP COLUMN IF EXISTS trigger;
