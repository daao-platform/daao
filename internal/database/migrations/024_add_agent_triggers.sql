-- Migration: Add trigger column and trigger_source
-- Adds trigger JSONB column to agent_definitions and trigger_source to agent_runs
-- Idempotent: uses IF NOT EXISTS / ADD COLUMN IF NOT EXISTS patterns

-- Add trigger JSONB column to agent_definitions table (nullable)
ALTER TABLE agent_definitions ADD COLUMN IF NOT EXISTS trigger JSONB;

-- Add trigger_source TEXT column to agent_runs table with default 'manual' and CHECK constraint
ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS trigger_source TEXT NOT NULL DEFAULT 'manual'
    CHECK (trigger_source IN ('manual', 'schedule', 'trigger'));
