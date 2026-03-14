-- Create encrypted_secrets table for AES-256-GCM encrypted secret storage
CREATE TABLE encrypted_secrets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key_hash TEXT NOT NULL UNIQUE, -- SHA-256 hash of the secret key
    cipher_text BYTEA NOT NULL,    -- AES-256-GCM encrypted value
    nonce BYTEA NOT NULL,           -- Unique nonce for each encryption
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for fast key lookup
CREATE INDEX idx_encrypted_secrets_key_hash ON encrypted_secrets(key_hash);
