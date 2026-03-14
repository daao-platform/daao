DROP INDEX IF EXISTS idx_satellites_tags;
ALTER TABLE satellites DROP COLUMN IF EXISTS tags;
