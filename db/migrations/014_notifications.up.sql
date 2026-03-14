-- Notifications & alerts (issue #19)
-- Stores user-facing notification records and per-user delivery preferences.

CREATE TABLE notifications (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL,
    type        VARCHAR(50) NOT NULL,
    priority    VARCHAR(20) NOT NULL DEFAULT 'INFO',
    title       VARCHAR(255) NOT NULL,
    body        TEXT,
    session_id  UUID REFERENCES sessions(id) ON DELETE SET NULL,
    satellite_id UUID REFERENCES satellites(id) ON DELETE SET NULL,
    payload     JSONB DEFAULT '{}',
    read        BOOLEAN DEFAULT FALSE,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Fast lookup: unread notifications for a user (partial index)
CREATE INDEX idx_notifications_user_unread ON notifications(user_id, read) WHERE read = FALSE;

-- Timeline queries: all notifications for a user, newest first
CREATE INDEX idx_notifications_user_created ON notifications(user_id, created_at DESC);

CREATE TABLE notification_preferences (
    user_id             UUID PRIMARY KEY,
    min_priority        VARCHAR(20) DEFAULT 'INFO',
    browser_enabled     BOOLEAN DEFAULT TRUE,
    session_terminated  BOOLEAN DEFAULT TRUE,
    session_error       BOOLEAN DEFAULT TRUE,
    satellite_offline   BOOLEAN DEFAULT TRUE,
    session_suspended   BOOLEAN DEFAULT TRUE,
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ DEFAULT NOW()
);
