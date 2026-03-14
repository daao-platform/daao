-- Migration 025: Agent Version History (down)
-- Reverts agent versioning changes

-- Drop author column from agent_definitions
ALTER TABLE agent_definitions DROP COLUMN IF EXISTS author;

-- Drop agent_definition_versions table
DROP TABLE IF EXISTS agent_definition_versions;
