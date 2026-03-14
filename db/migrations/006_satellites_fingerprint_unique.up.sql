-- Add unique partial index on satellites.fingerprint to prevent duplicates
CREATE UNIQUE INDEX IF NOT EXISTS idx_satellites_fingerprint_unique ON satellites (fingerprint) WHERE fingerprint IS NOT NULL;
