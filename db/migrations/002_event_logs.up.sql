-- Create event_logs table with range partitioning by created_at
CREATE TABLE event_logs (
    id BIGSERIAL,
    satellite_id UUID NOT NULL,
    session_id UUID,
    event_type VARCHAR(100) NOT NULL,
    event_data JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
) PARTITION BY RANGE (created_at);

-- Create index on event_logs
CREATE INDEX idx_event_logs_satellite_id ON event_logs(satellite_id);
CREATE INDEX idx_event_logs_session_id ON event_logs(session_id);
CREATE INDEX idx_event_logs_event_type ON event_logs(event_type);
CREATE INDEX idx_event_logs_created_at ON event_logs(created_at);

-- Create default partition for event_logs (catches any data that doesn't match other partitions)
CREATE TABLE event_logs_default PARTITION OF event_logs DEFAULT;
