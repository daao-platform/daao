-- Reverse migration 032
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
        -- Remove continuous aggregate
        DROP MATERIALIZED VIEW IF EXISTS satellite_telemetry_hourly CASCADE;

        -- Remove policies - use if_exists parameter (renamed from if_not_exists in newer TimescaleDB)
        PERFORM remove_retention_policy('satellite_telemetry', if_exists => TRUE);
        PERFORM remove_compression_policy('satellite_telemetry', if_exists => TRUE);

        -- Note: TimescaleDB does not support converting a hypertable back to a
        -- regular table without data loss. Down migration only removes policies.
        RAISE NOTICE 'TimescaleDB policies removed. Hypertable structure retained (cannot revert without data loss).';
    END IF;
END;
$$;
