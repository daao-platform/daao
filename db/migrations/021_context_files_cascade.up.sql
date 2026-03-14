-- 021_context_files_cascade.up.sql
-- Add ON DELETE CASCADE to context file FK constraints (PRD §13.3)

-- Drop existing FK on satellite_context_files.satellite_id and re-add with CASCADE
ALTER TABLE satellite_context_files
    DROP CONSTRAINT IF EXISTS satellite_context_files_satellite_id_fkey;
ALTER TABLE satellite_context_files
    ADD CONSTRAINT satellite_context_files_satellite_id_fkey
    FOREIGN KEY (satellite_id) REFERENCES satellites(id) ON DELETE CASCADE;

-- Drop existing FK on context_file_history.context_file_id and re-add with CASCADE
ALTER TABLE context_file_history
    DROP CONSTRAINT IF EXISTS context_file_history_context_file_id_fkey;
ALTER TABLE context_file_history
    ADD CONSTRAINT context_file_history_context_file_id_fkey
    FOREIGN KEY (context_file_id) REFERENCES satellite_context_files(id) ON DELETE CASCADE;
