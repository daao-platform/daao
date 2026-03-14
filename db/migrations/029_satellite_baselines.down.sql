-- Rollback migration 029: Drop satellite baselines & profiles

DROP TRIGGER IF EXISTS trg_satellite_profiles_updated ON satellite_profiles;
DROP FUNCTION IF EXISTS update_satellite_profiles_timestamp();
DROP TABLE IF EXISTS satellite_profiles;
DROP TABLE IF EXISTS satellite_baselines;
