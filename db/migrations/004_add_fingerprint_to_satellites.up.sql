-- Add fingerprint column to satellites table for gRPC registration matching
ALTER TABLE satellites ADD COLUMN IF NOT EXISTS fingerprint TEXT;
CREATE INDEX IF NOT EXISTS idx_satellites_fingerprint ON satellites(fingerprint);
