-- Migration 026: Pipeline System Tables
-- Creates tables for the pipeline system:
-- - pipelines: defines pipeline configurations
-- - pipeline_steps: defines steps within a pipeline
-- - pipeline_runs: tracks pipeline executions
-- - pipeline_step_runs: tracks step execution within runs

-- Create pipelines table
CREATE TABLE pipelines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT,
    satellite_id UUID NOT NULL REFERENCES satellites(id),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    on_failure TEXT NOT NULL DEFAULT 'stop' CHECK (on_failure IN ('stop', 'skip', 'retry', 'escalate')),
    max_retries INTEGER NOT NULL DEFAULT 3,
    schedule JSONB,
    is_enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Create indexes for pipelines table
CREATE INDEX idx_pipelines_created_by ON pipelines(created_by);
CREATE INDEX idx_pipelines_satellite_id ON pipelines(satellite_id);

-- Create pipeline_steps table
CREATE TABLE pipeline_steps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pipeline_id UUID NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
    step_order INTEGER NOT NULL,
    agent_id UUID NOT NULL REFERENCES agent_definitions(id),
    input_mode TEXT NOT NULL DEFAULT 'none' CHECK (input_mode IN ('none', 'previous_output')),
    output_mode TEXT NOT NULL DEFAULT 'next' CHECK (output_mode IN ('next', 'report', 'next_and_report')),
    config JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (pipeline_id, step_order)
);

-- Create index for pipeline_steps table
CREATE INDEX idx_pipeline_steps_pipeline_id ON pipeline_steps(pipeline_id);

-- Create pipeline_runs table
CREATE TABLE pipeline_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pipeline_id UUID NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
    satellite_id UUID NOT NULL REFERENCES satellites(id),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    current_step INTEGER,
    trigger_source TEXT NOT NULL DEFAULT 'manual' CHECK (trigger_source IN ('manual', 'schedule')),
    started_at TIMESTAMPTZ,
    ended_at TIMESTAMPTZ,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Create indexes for pipeline_runs table
CREATE INDEX idx_pipeline_runs_pipeline_id ON pipeline_runs(pipeline_id);
CREATE INDEX idx_pipeline_runs_status ON pipeline_runs(status);
CREATE INDEX idx_pipeline_runs_created_at ON pipeline_runs(created_at);

-- Create pipeline_step_runs table
CREATE TABLE pipeline_step_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pipeline_run_id UUID NOT NULL REFERENCES pipeline_runs(id) ON DELETE CASCADE,
    step_order INTEGER NOT NULL,
    agent_run_id UUID REFERENCES agent_runs(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed', 'skipped')),
    input_text TEXT,
    output_text TEXT,
    started_at TIMESTAMPTZ,
    ended_at TIMESTAMPTZ,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (pipeline_run_id, step_order)
);

-- Create index for pipeline_step_runs table
CREATE INDEX idx_pipeline_step_runs_pipeline_run_id ON pipeline_step_runs(pipeline_run_id);
