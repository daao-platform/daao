// Package license provides DAAO license key validation.
//
// License keys are Ed25519-signed JWT tokens. The Nexus binary embeds the
// Ed25519 public key; the private key is kept offline and used only to
// generate license keys.
//
// When no license key is present (or the key is invalid), DAAO runs in
// Community mode with feature gates enforced.
package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Tier represents the license tier.
type Tier string

const (
	TierCommunity  Tier = "community"
	TierTeam       Tier = "team"
	TierEnterprise Tier = "enterprise"
)

// Claims represents the decoded license key payload.
type Claims struct {
	// Subject is the customer UUID.
	Subject string `json:"sub"`
	// Customer is the human-readable customer name.
	Customer string `json:"customer"`
	// Tier is the license tier (community, team, enterprise).
	Tier Tier `json:"tier"`
	// MaxUsers is the maximum number of users allowed.
	MaxUsers int `json:"max_users"`
	// MaxSatellites is the maximum number of satellites allowed.
	MaxSatellites int `json:"max_satellites"`
	// Features is the list of enterprise features enabled by this license.
	Features []string `json:"features"`
	// IssuedAt is the Unix timestamp when the license was issued.
	IssuedAt int64 `json:"iat"`
	// ExpiresAt is the Unix timestamp when the license expires.
	ExpiresAt int64 `json:"exp"`
}

// Validate checks that the claims are internally consistent and not expired.
func (c *Claims) Validate() error {
	if c.Subject == "" {
		return errors.New("license: missing subject")
	}
	if c.Tier == "" {
		return errors.New("license: missing tier")
	}
	if c.ExpiresAt > 0 && time.Now().Unix() > c.ExpiresAt {
		return fmt.Errorf("license: expired at %s", time.Unix(c.ExpiresAt, 0).Format(time.RFC3339))
	}
	if c.IssuedAt > 0 && c.IssuedAt > time.Now().Unix()+300 {
		return errors.New("license: issued in the future")
	}
	return nil
}

// HasFeature checks if the license includes a specific feature.
func (c *Claims) HasFeature(feature string) bool {
	for _, f := range c.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// Manager holds the validated license state. It is safe for concurrent use.
type Manager struct {
	mu        sync.RWMutex
	claims    *Claims
	publicKey ed25519.PublicKey
	loaded    bool
}

// NewManager creates a new license manager with the given Ed25519 public key.
// The publicKey parameter is the base64-encoded Ed25519 public key used to
// verify license key signatures. Pass an empty string to use Community mode
// (no license validation possible).
func NewManager(publicKeyBase64 string) (*Manager, error) {
	m := &Manager{}

	if publicKeyBase64 != "" {
		keyBytes, err := base64.StdEncoding.DecodeString(publicKeyBase64)
		if err != nil {
			return nil, fmt.Errorf("license: invalid public key encoding: %w", err)
		}
		if len(keyBytes) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("license: invalid public key size: got %d, want %d", len(keyBytes), ed25519.PublicKeySize)
		}
		m.publicKey = ed25519.PublicKey(keyBytes)
	}

	return m, nil
}

// LoadKey validates and loads a license key (Ed25519-signed JWT).
// If the key is empty, the manager stays in Community mode.
func (m *Manager) LoadKey(licenseKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if licenseKey == "" {
		m.claims = nil
		m.loaded = false
		return nil
	}

	claims, err := m.parseAndVerify(licenseKey)
	if err != nil {
		return err
	}

	if err := claims.Validate(); err != nil {
		return err
	}

	m.claims = claims
	m.loaded = true
	return nil
}

// Claims returns the current license claims. Returns nil if in Community mode.
func (m *Manager) Claims() *Claims {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.claims
}

// Tier returns the current license tier.
func (m *Manager) Tier() Tier {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.claims == nil {
		return TierCommunity
	}
	return m.claims.Tier
}

// IsEnterprise returns true if an enterprise license is loaded and valid.
func (m *Manager) IsEnterprise() bool {
	return m.Tier() == TierEnterprise
}

// MaxUsers returns the maximum number of users for the current license.
func (m *Manager) MaxUsers() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.claims == nil || m.claims.MaxUsers == 0 {
		return CommunityMaxUsers
	}
	return m.claims.MaxUsers
}

// MaxSatellites returns the maximum number of satellites for the current license.
func (m *Manager) MaxSatellites() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.claims == nil || m.claims.MaxSatellites == 0 {
		return CommunityMaxSatellites
	}
	return m.claims.MaxSatellites
}

// HasFeature checks if a feature is enabled by the current license.
func (m *Manager) HasFeature(feature string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.claims == nil {
		return false
	}
	return m.claims.HasFeature(feature)
}

// parseAndVerify parses a JWT token and verifies its Ed25519 signature.
// JWT format: base64url(header).base64url(payload).base64url(signature)
func (m *Manager) parseAndVerify(token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("license: invalid token format")
	}

	// Verify signature
	if m.publicKey == nil {
		return nil, errors.New("license: no public key configured")
	}

	signingInput := parts[0] + "." + parts[1]
	signature, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, fmt.Errorf("license: invalid signature encoding: %w", err)
	}

	if !ed25519.Verify(m.publicKey, []byte(signingInput), signature) {
		return nil, errors.New("license: invalid signature")
	}

	// Verify header specifies EdDSA
	headerBytes, err := base64URLDecode(parts[0])
	if err != nil {
		return nil, fmt.Errorf("license: invalid header encoding: %w", err)
	}

	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("license: invalid header JSON: %w", err)
	}
	if header.Alg != "EdDSA" {
		return nil, fmt.Errorf("license: unsupported algorithm %q, expected EdDSA", header.Alg)
	}

	// Decode payload
	payloadBytes, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("license: invalid payload encoding: %w", err)
	}

	var claims Claims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("license: invalid payload JSON: %w", err)
	}

	return &claims, nil
}

// base64URLDecode decodes base64url-encoded data (JWT uses base64url without padding).
func base64URLDecode(s string) ([]byte, error) {
	// Add padding if missing
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}
