-- Create satellites table
CREATE TABLE satellites (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    owner_id UUID NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    status VARCHAR(50) NOT NULL DEFAULT 'active'
);

-- Create sessions table with all 6 PRD states
CREATE TABLE sessions (
    id UUID PRIMARY KEY,
    satellite_id UUID NOT NULL REFERENCES satellites(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    state VARCHAR(50) NOT NULL CHECK (state IN ('PROVISIONING', 'RUNNING', 'DETACHED', 'RE_ATTACHING', 'SUSPENDED', 'TERMINATED')),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_heartbeat TIMESTAMP WITH TIME ZONE,
    connection_info JSONB
);

-- Create indexes for satellites table
CREATE INDEX idx_satellites_owner_id ON satellites(owner_id);
CREATE INDEX idx_satellites_status ON satellites(status);

-- Create indexes for sessions table
CREATE INDEX idx_sessions_satellite_id ON sessions(satellite_id);
CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_state ON sessions(state);
CREATE INDEX idx_sessions_created_at ON sessions(created_at);
