-- Add password_hash to users table for local auth (community edition)
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT;
