-- 021_context_files_cascade.down.sql
-- Revert ON DELETE CASCADE — restore plain FK constraints

ALTER TABLE context_file_history
    DROP CONSTRAINT IF EXISTS context_file_history_context_file_id_fkey;
ALTER TABLE context_file_history
    ADD CONSTRAINT context_file_history_context_file_id_fkey
    FOREIGN KEY (context_file_id) REFERENCES satellite_context_files(id);

ALTER TABLE satellite_context_files
    DROP CONSTRAINT IF EXISTS satellite_context_files_satellite_id_fkey;
ALTER TABLE satellite_context_files
    ADD CONSTRAINT satellite_context_files_satellite_id_fkey
    FOREIGN KEY (satellite_id) REFERENCES satellites(id);
