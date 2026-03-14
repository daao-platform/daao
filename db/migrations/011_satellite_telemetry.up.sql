CREATE TABLE satellite_telemetry (
  id BIGSERIAL PRIMARY KEY,
  satellite_id UUID NOT NULL REFERENCES satellites(id) ON DELETE CASCADE,
  cpu_percent DOUBLE PRECISION,
  memory_percent DOUBLE PRECISION,
  memory_used_bytes BIGINT,
  memory_total_bytes BIGINT,
  disk_percent DOUBLE PRECISION,
  disk_used_bytes BIGINT,
  disk_total_bytes BIGINT,
  gpu_data JSONB,
  active_sessions INTEGER DEFAULT 0,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_telemetry_satellite_id ON satellite_telemetry(satellite_id);
CREATE INDEX idx_telemetry_created_at ON satellite_telemetry(created_at);
