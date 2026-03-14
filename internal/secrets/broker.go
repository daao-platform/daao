// Package secrets provides the secrets broker with pull-on-demand pattern
// and local encrypted backend.
package secrets

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrSecretNotFound is returned when a secret is not found in any scope.
var ErrSecretNotFound = errors.New("secret not found")

// SecretBackend defines the interface for secret storage backends.
type SecretBackend interface {
	// FetchSecret retrieves a secret by its reference.
	// ref is the backend-specific reference (e.g., key name, path).
	// Returns the secret value as a string or an error.
	FetchSecret(ctx context.Context, ref string) (string, error)
}

// SecretScope represents a secret scope configuration from the database.
type SecretScope struct {
	ID          uuid.UUID
	SatelliteID *uuid.UUID // nil for global scope
	Provider    string
	SecretRef   string
	Backend     string
	LeaseTTLMin int
}

// Broker manages secret retrieval with per-satellite scoping.
type Broker struct {
	db           *pgxpool.Pool
	localBackend *LocalBackend // cached backend to avoid re-creation per call
}

// NewBroker creates a new secrets broker with the given database pool.
func NewBroker(db *pgxpool.Pool) *Broker {
	return &Broker{
		db:           db,
		localBackend: NewLocalBackend(db),
	}
}

// FetchSecret retrieves a secret for the given provider and satellite ID.
// It first looks for a satellite-specific secret scope, then falls back
// to a global scope if not found.
// Returns the secret value as a string or an error.
func (b *Broker) FetchSecret(ctx context.Context, provider string, satelliteID uuid.UUID) (string, error) {
	// First, try to find a satellite-specific scope
	scope, err := b.findScope(ctx, provider, &satelliteID)
	if err != nil {
		return "", fmt.Errorf("failed to lookup satellite-specific secret scope: %w", err)
	}

	// If satellite-specific scope not found, try global scope (nil satellite_id)
	if scope == nil {
		scope, err = b.findScope(ctx, provider, nil)
		if err != nil {
			return "", fmt.Errorf("failed to lookup global secret scope: %w", err)
		}
	}

	if scope == nil {
		return "", fmt.Errorf("%w: provider=%q", ErrSecretNotFound, provider)
	}

	// Delegate to the cached local backend
	return b.localBackend.FetchSecret(ctx, scope.SecretRef)
}

// findScope looks up a secret scope by provider and optional satellite ID.
// Returns nil if no matching scope is found.
func (b *Broker) findScope(ctx context.Context, provider string, satelliteID *uuid.UUID) (*SecretScope, error) {
	var scope SecretScope

	var err error
	if satelliteID != nil {
		// Look for satellite-specific scope
		err = b.db.QueryRow(ctx, `
			SELECT id, satellite_id, provider, secret_ref, backend, lease_ttl_min
			FROM secret_scopes
			WHERE provider = $1 AND satellite_id = $2
			ORDER BY satellite_id DESC
			LIMIT 1
		`, provider, satelliteID).Scan(
			&scope.ID,
			&scope.SatelliteID,
			&scope.Provider,
			&scope.SecretRef,
			&scope.Backend,
			&scope.LeaseTTLMin,
		)
	} else {
		// Look for global scope (satellite_id IS NULL)
		err = b.db.QueryRow(ctx, `
			SELECT id, satellite_id, provider, secret_ref, backend, lease_ttl_min
			FROM secret_scopes
			WHERE provider = $1 AND satellite_id IS NULL
			LIMIT 1
		`, provider).Scan(
			&scope.ID,
			&scope.SatelliteID,
			&scope.Provider,
			&scope.SecretRef,
			&scope.Backend,
			&scope.LeaseTTLMin,
		)
	}

	if err != nil {
		// Return nil if no rows found (this is expected for missing scopes)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		// pgx.ErrNoRows or similar - return nil, nil
		return nil, nil
	}

	return &scope, nil
}
