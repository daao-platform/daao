-- Migration 026: Pipeline System Tables (down)
-- Drops all pipeline system tables in reverse dependency order

-- Drop pipeline_step_runs table
DROP TABLE IF EXISTS pipeline_step_runs;

-- Drop pipeline_runs table
DROP TABLE IF EXISTS pipeline_runs;

-- Drop pipeline_steps table
DROP TABLE IF EXISTS pipeline_steps;

-- Drop pipelines table
DROP TABLE IF EXISTS pipelines;
