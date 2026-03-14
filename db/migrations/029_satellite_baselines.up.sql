-- Migration 029: Satellite Baselines & Profiles
-- Supports the Infrastructure Discovery Agent's deterministic baseline collection.
-- Phase 1 uses regular JSONB tables; Phase 2 migrates to TimescaleDB hypertables.

-- ============================================================================
-- satellite_baselines — Time-series snapshots of system state
-- ============================================================================
CREATE TABLE IF NOT EXISTS satellite_baselines (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    satellite_id   UUID NOT NULL REFERENCES satellites(id) ON DELETE CASCADE,
    tier           SMALLINT NOT NULL CHECK (tier BETWEEN 1 AND 5),
    snapshot_type  TEXT NOT NULL,          -- 'os_profile', 'services', 'network', 'hardware', 'security', 'storage', 'processes'
    snapshot_data  JSONB NOT NULL,         -- Structured JSON from monitor script
    agent_run_id   UUID,                   -- Which discovery run produced this (NULL for scheduled pulses)
    is_baseline    BOOLEAN DEFAULT FALSE,  -- Marked as the "golden" baseline
    drift_score    REAL,                   -- 0.0 (no drift) to 1.0 (complete drift from baseline)
    drift_summary  JSONB,                  -- {"added": [...], "removed": [...], "changed": [...]}
    collected_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for common query patterns
CREATE INDEX idx_baselines_satellite_type ON satellite_baselines(satellite_id, snapshot_type, collected_at DESC);
CREATE INDEX idx_baselines_satellite_tier ON satellite_baselines(satellite_id, tier, collected_at DESC);
CREATE INDEX idx_baselines_golden ON satellite_baselines(satellite_id, snapshot_type) WHERE is_baseline = TRUE;
CREATE INDEX idx_baselines_collected ON satellite_baselines(collected_at DESC);

-- ============================================================================
-- satellite_profiles — Machine classification and agent recommendations
-- ============================================================================
CREATE TABLE IF NOT EXISTS satellite_profiles (
    satellite_id       UUID PRIMARY KEY REFERENCES satellites(id) ON DELETE CASCADE,
    os_family          TEXT NOT NULL DEFAULT 'unknown',    -- 'linux', 'windows', 'macos', 'unraid'
    os_distro          TEXT,                                -- 'ubuntu-22.04', 'windows-server-2022', etc.
    os_version         TEXT,                                -- '22.04', '10.0.20348', etc.
    arch               TEXT,                                -- 'amd64', 'arm64'
    machine_roles      TEXT[] DEFAULT '{}',                 -- ['web-server', 'database', 'ci-runner']
    detected_services  TEXT[] DEFAULT '{}',                 -- ['nginx', 'postgresql', 'docker']
    detected_containers TEXT[] DEFAULT '{}',                -- ['nexus', 'cockpit', 'postgres']
    listening_ports    INTEGER[] DEFAULT '{}',              -- [80, 443, 5432, 8443]
    risk_level         TEXT NOT NULL DEFAULT 'unknown'
                       CHECK (risk_level IN ('low', 'medium', 'high', 'critical', 'unknown')),
    recommended_agents TEXT[] DEFAULT '{}',                 -- ['log-analyzer', 'security-scanner']
    last_discovery     TIMESTAMPTZ,
    last_pulse         TIMESTAMPTZ,                         -- Last Tier 2 service pulse
    profile_data       JSONB DEFAULT '{}',                  -- Full classification details
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Trigger: auto-update updated_at
CREATE OR REPLACE FUNCTION update_satellite_profiles_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_satellite_profiles_updated
    BEFORE UPDATE ON satellite_profiles
    FOR EACH ROW
    EXECUTE FUNCTION update_satellite_profiles_timestamp();
