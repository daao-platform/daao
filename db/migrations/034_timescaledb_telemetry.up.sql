-- Migration 032: TimescaleDB hypertable for satellite_telemetry
-- Idempotent: wraps TimescaleDB calls so plain PostgreSQL skips them gracefully.

DO $$
BEGIN
    -- Install TimescaleDB extension if available
    IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'timescaledb') THEN
        CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;

        -- Convert to hypertable (partitioned by created_at, 1-day chunks)
        PERFORM create_hypertable(
            'satellite_telemetry',
            'created_at',
            if_not_exists => TRUE,
            migrate_data  => TRUE
        );

        -- Compress chunks older than 7 days
        ALTER TABLE satellite_telemetry
            SET (timescaledb.compress = true,
                 timescaledb.compress_segmentby = 'satellite_id');

        PERFORM add_compression_policy(
            'satellite_telemetry',
            compress_after => INTERVAL '7 days',
            if_not_exists  => TRUE
        );

        -- Drop rows older than 90 days
        PERFORM add_retention_policy(
            'satellite_telemetry',
            drop_after    => INTERVAL '90 days',
            if_not_exists => TRUE
        );

    ELSE
        RAISE NOTICE 'TimescaleDB extension not available — skipping hypertable conversion. Install timescaledb for optimal telemetry performance.';
    END IF;
END;
$$;

-- Continuous aggregate: hourly rollup (created only when TimescaleDB is present)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
        -- Drop if exists to allow re-run
        DROP MATERIALIZED VIEW IF EXISTS satellite_telemetry_hourly;

        CREATE MATERIALIZED VIEW satellite_telemetry_hourly
        WITH (timescaledb.continuous) AS
        SELECT
            satellite_id,
            time_bucket('1 hour', created_at) AS hour,
            AVG(cpu_percent)::DOUBLE PRECISION       AS avg_cpu,
            AVG(memory_percent)::DOUBLE PRECISION    AS avg_mem,
            MAX(memory_used_bytes)                   AS peak_mem_bytes,
            AVG(disk_percent)::DOUBLE PRECISION      AS avg_disk,
            COUNT(*)::INTEGER                        AS sample_count
        FROM satellite_telemetry
        GROUP BY satellite_id, hour
        WITH NO DATA;

        PERFORM add_continuous_aggregate_policy(
            'satellite_telemetry_hourly',
            start_offset      => INTERVAL '3 hours',
            end_offset        => INTERVAL '1 hour',
            schedule_interval => INTERVAL '1 hour',
            if_not_exists     => TRUE
        );
    END IF;
END;
$$;
