-- Create agent_definitions table per PRD §10.1
CREATE TABLE agent_definitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT,
    version TEXT NOT NULL DEFAULT '1.0.0',
    type TEXT NOT NULL CHECK (type IN ('specialist', 'autonomous')),
    category TEXT NOT NULL DEFAULT 'operations',
    icon TEXT,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    system_prompt TEXT NOT NULL,
    tools_config JSONB NOT NULL DEFAULT '{}',
    guardrails JSONB NOT NULL DEFAULT '{}',
    schedule JSONB,
    output_config JSONB,
    is_builtin BOOLEAN NOT NULL DEFAULT FALSE,
    is_enterprise BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Create indexes for agent_definitions table
CREATE INDEX idx_agent_definitions_name ON agent_definitions(name);
CREATE INDEX idx_agent_definitions_type ON agent_definitions(type);
CREATE INDEX idx_agent_definitions_category ON agent_definitions(category);
CREATE INDEX idx_agent_definitions_is_builtin ON agent_definitions(is_builtin);
CREATE INDEX idx_agent_definitions_is_enterprise ON agent_definitions(is_enterprise);
CREATE INDEX idx_agent_definitions_provider ON agent_definitions(provider);
