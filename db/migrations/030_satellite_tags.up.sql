-- Add tags column to satellites for capability-based agent routing.
-- Tags are admin-defined labels (e.g., 'linux', 'gpu', 'production').
-- Auto-populated tags (os, arch) are set by the gRPC gateway on satellite connect.
ALTER TABLE satellites ADD COLUMN tags TEXT[] NOT NULL DEFAULT '{}';
CREATE INDEX idx_satellites_tags ON satellites USING GIN (tags);
