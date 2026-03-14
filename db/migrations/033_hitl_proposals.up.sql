-- Migration 031: HITL Proposals
-- Creates the hitl_proposals table for Human-in-the-Loop command interception.
-- Agents submit proposals for high-risk actions that require human approval.
-- Enterprise-gated feature (FeatureHITL).

CREATE TABLE IF NOT EXISTS hitl_proposals (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id  UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    agent_name  TEXT NOT NULL,
    action      TEXT NOT NULL,
    description TEXT NOT NULL,
    risk_level  TEXT NOT NULL CHECK (risk_level IN ('low', 'medium', 'high', 'critical')),
    payload     JSONB,
    status      TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'APPROVED', 'DENIED', 'EXPIRED')),
    reviewed_by TEXT,
    reviewed_at TIMESTAMPTZ,
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_hitl_proposals_session_id ON hitl_proposals(session_id);
CREATE INDEX idx_hitl_proposals_status ON hitl_proposals(status);
CREATE INDEX idx_hitl_proposals_created_at ON hitl_proposals(created_at DESC);
