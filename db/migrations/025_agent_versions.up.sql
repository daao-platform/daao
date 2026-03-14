-- Migration 025: Agent Version History
-- Creates agent_definition_versions table for tracking agent definition snapshots

-- Create agent_definition_versions table
CREATE TABLE agent_definition_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agent_definitions(id) ON DELETE CASCADE,
    version TEXT NOT NULL,
    snapshot JSONB NOT NULL,
    change_summary TEXT,
    created_by TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(agent_id, version)
);

-- Create index for efficient queries by agent_id with time ordering
CREATE INDEX idx_agent_def_versions_agent_id ON agent_definition_versions(agent_id, created_at DESC);

-- Add author column to agent_definitions for tracking creator
ALTER TABLE agent_definitions ADD COLUMN author TEXT;
