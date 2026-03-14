ALTER TABLE users ADD COLUMN IF NOT EXISTS role VARCHAR(50) NOT NULL DEFAULT 'viewer';
ALTER TABLE users ADD COLUMN IF NOT EXISTS provider VARCHAR(50);
ALTER TABLE users ADD COLUMN IF NOT EXISTS provider_id VARCHAR(255);
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_url TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_provider_id ON users (provider, provider_id) WHERE provider IS NOT NULL;
DO $$ BEGIN
  ALTER TABLE users ADD CONSTRAINT chk_users_role CHECK (role IN ('owner', 'admin', 'viewer'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;
