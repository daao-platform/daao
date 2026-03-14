-- Remove version tracking columns from satellites table
ALTER TABLE satellites DROP COLUMN IF EXISTS version;
ALTER TABLE satellites DROP COLUMN IF EXISTS os;
ALTER TABLE satellites DROP COLUMN IF EXISTS arch;
