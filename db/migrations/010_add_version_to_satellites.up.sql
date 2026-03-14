-- Add version tracking columns to satellites table
ALTER TABLE satellites ADD COLUMN IF NOT EXISTS version VARCHAR(50) DEFAULT '';
ALTER TABLE satellites ADD COLUMN IF NOT EXISTS os VARCHAR(20) DEFAULT '';
ALTER TABLE satellites ADD COLUMN IF NOT EXISTS arch VARCHAR(20) DEFAULT '';
