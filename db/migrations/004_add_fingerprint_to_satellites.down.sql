DROP INDEX IF EXISTS idx_satellites_fingerprint;
ALTER TABLE satellites DROP COLUMN IF EXISTS fingerprint;
