-- Add available_agents column to track which agent binaries are installed on each satellite
ALTER TABLE satellites ADD COLUMN IF NOT EXISTS available_agents JSONB DEFAULT '[]';
