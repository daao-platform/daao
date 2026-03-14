-- Create agent_runs table per PRD §10.2
CREATE TABLE agent_runs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id        UUID NOT NULL REFERENCES agent_definitions(id),
    satellite_id    UUID NOT NULL REFERENCES satellites(id),
    session_id      UUID REFERENCES sessions(id),
    status          TEXT NOT NULL DEFAULT 'running'
                    CHECK (status IN ('running', 'completed', 'failed', 'timeout', 'killed')),
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at        TIMESTAMPTZ,
    total_tokens    INTEGER DEFAULT 0,
    estimated_cost  DECIMAL(10, 6) DEFAULT 0,
    tool_call_count INTEGER DEFAULT 0,
    result          TEXT,
    error           TEXT,
    metadata        JSONB DEFAULT '{}'
);

-- Create indexes for agent_runs table
CREATE INDEX idx_agent_runs_agent_id ON agent_runs(agent_id);
CREATE INDEX idx_agent_runs_satellite_id ON agent_runs(satellite_id);
CREATE INDEX idx_agent_runs_session_id ON agent_runs(session_id);
CREATE INDEX idx_agent_runs_status ON agent_runs(status);
CREATE INDEX idx_agent_runs_started_at ON agent_runs(started_at);
