-- Add routing config to agent_definitions for opt-in auto-dispatch.
-- Schema: {"mode": "targeted"|"auto-dispatch", "require_tags": [...], "prefer_tags": [...]}
-- NULL means targeted (today's default behavior, no change).
ALTER TABLE agent_definitions ADD COLUMN routing JSONB DEFAULT NULL;
