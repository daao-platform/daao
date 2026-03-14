CREATE TABLE admin_audit_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id    UUID REFERENCES users(id) ON DELETE SET NULL,
    actor_email TEXT NOT NULL DEFAULT '',
    action      TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT,
    details     JSONB,
    ip_address  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_admin_audit_log_actor_id ON admin_audit_log(actor_id);
CREATE INDEX idx_admin_audit_log_action ON admin_audit_log(action);
CREATE INDEX idx_admin_audit_log_resource_type ON admin_audit_log(resource_type);
CREATE INDEX idx_admin_audit_log_created_at ON admin_audit_log(created_at);
