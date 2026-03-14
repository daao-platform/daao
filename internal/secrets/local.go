// Package secrets provides the secrets broker with pull-on-demand pattern
// and local encrypted backend.
package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LocalBackend implements SecretBackend using AES-256-GCM encryption
// with storage in the Nexus database.
type LocalBackend struct {
	db        *pgxpool.Pool
	masterKey []byte // 32-byte key for AES-256
}

// EncryptedSecret represents an encrypted secret stored in the database.
type EncryptedSecret struct {
	ID         uuid.UUID
	KeyHash    string // SHA-256 hash of the secret key for lookup
	CipherText []byte // AES-256-GCM encrypted value
	Nonce      []byte // Unique nonce for each encryption
	CreatedAt  []byte // Timestamp
	UpdatedAt  []byte // Timestamp
}

// NewLocalBackend creates a new local encrypted backend.
// The masterKey should be a 32-byte key for AES-256.
// In production, this should be loaded from a secure configuration.
func NewLocalBackend(db *pgxpool.Pool) *LocalBackend {
	// Load master key from environment variable
	keyStr := os.Getenv("DAAO_SECRET_KEY")
	var masterKey []byte
	if keyStr != "" {
		masterKey = deriveMasterKey(keyStr)
	} else {
		slog.Info("WARNING: DAAO_SECRET_KEY not set — using randomly generated key (secrets will not persist across restarts)", "component", "secrets")
		randomKey := make([]byte, 32)
		if _, err := rand.Read(randomKey); err != nil {
			slog.Error(fmt.Sprintf("Failed to generate random secret key: %v", err), "component", "secrets")
			os.Exit(1)
		}
		masterKey = randomKey
	}
	return &LocalBackend{
		db:        db,
		masterKey: masterKey,
	}
}

// NewLocalBackendWithKey creates a new local encrypted backend with a custom master key.
func NewLocalBackendWithKey(db *pgxpool.Pool, masterKey []byte) *LocalBackend {
	// Ensure key is exactly 32 bytes for AES-256
	if len(masterKey) != 32 {
		// Hash the key to get 32 bytes
		h := sha256.Sum256(masterKey)
		masterKey = h[:]
	}
	return &LocalBackend{
		db:        db,
		masterKey: masterKey,
	}
}

// deriveMasterKey derives a 32-byte key from input using SHA-256.
func deriveMasterKey(input string) []byte {
	h := sha256.Sum256([]byte(input))
	return h[:]
}

// hashKey generates a SHA-256 hash of the secret key for database lookup.
func hashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return base64.StdEncoding.EncodeToString(h[:])
}

// FetchSecret retrieves and decrypts a secret by its reference (key).
// Returns the decrypted secret value as a string.
func (lb *LocalBackend) FetchSecret(ctx context.Context, ref string) (string, error) {
	if ref == "" {
		return "", errors.New("secret reference cannot be empty")
	}

	keyHash := hashKey(ref)

	var cipherText, nonce []byte
	err := lb.db.QueryRow(ctx, `
		SELECT cipher_text, nonce
		FROM encrypted_secrets
		WHERE key_hash = $1
		LIMIT 1
	`, keyHash).Scan(&cipherText, &nonce)

	if err != nil {
		return "", fmt.Errorf("secret not found: %w", err)
	}

	// Decrypt the secret
	plainText, err := lb.decrypt(cipherText, nonce)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt secret: %w", err)
	}

	return string(plainText), nil
}

// StoreSecret encrypts and stores a secret with the given reference (key).
// If a secret with the same key exists, it will be updated.
func (lb *LocalBackend) StoreSecret(ctx context.Context, ref string, value string) error {
	if ref == "" {
		return errors.New("secret reference cannot be empty")
	}
	if value == "" {
		return errors.New("secret value cannot be empty")
	}

	keyHash := hashKey(ref)

	// Encrypt the secret
	cipherText, nonce, err := lb.encrypt([]byte(value))
	if err != nil {
		return fmt.Errorf("failed to encrypt secret: %w", err)
	}

	// Upsert the secret
	_, err = lb.db.Exec(ctx, `
		INSERT INTO encrypted_secrets (id, key_hash, cipher_text, nonce, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, NOW(), NOW())
		ON CONFLICT (key_hash) DO UPDATE SET
			cipher_text = EXCLUDED.cipher_text,
			nonce = EXCLUDED.nonce,
			updated_at = NOW()
	`, keyHash, cipherText, nonce)

	if err != nil {
		return fmt.Errorf("failed to store encrypted secret: %w", err)
	}

	return nil
}

// DeleteSecret removes a secret by its reference.
func (lb *LocalBackend) DeleteSecret(ctx context.Context, ref string) error {
	if ref == "" {
		return errors.New("secret reference cannot be empty")
	}

	keyHash := hashKey(ref)

	result, err := lb.db.Exec(ctx, `
		DELETE FROM encrypted_secrets
		WHERE key_hash = $1
	`, keyHash)

	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	if result.RowsAffected() == 0 {
		return errors.New("secret not found")
	}

	return nil
}

// encrypt encrypts plaintext using AES-256-GCM.
// Returns cipherText and nonce.
func (lb *LocalBackend) encrypt(plaintext []byte) (cipherText, nonce []byte, err error) {
	block, err := aes.NewCipher(lb.masterKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate a random nonce
	nonce = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the plaintext
	cipherText = gcm.Seal(nil, nonce, plaintext, nil)

	return cipherText, nonce, nil
}

// decrypt decrypts cipherText using AES-256-GCM with the given nonce.
func (lb *LocalBackend) decrypt(cipherText, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(lb.masterKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt the ciphertext
	plaintext, err := gcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}
