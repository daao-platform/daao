package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

// generateTestKey creates an Ed25519 keypair for testing.
func generateTestKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	return pub, priv
}

// signJWT creates a minimal Ed25519-signed JWT for testing.
func signJWT(t *testing.T, privKey ed25519.PrivateKey, claims Claims) string {
	t.Helper()

	header := map[string]string{"alg": "EdDSA", "typ": "JWT"}
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	signingInput := headerB64 + "." + claimsB64
	signature := ed25519.Sign(privKey, []byte(signingInput))
	sigB64 := base64.RawURLEncoding.EncodeToString(signature)

	return signingInput + "." + sigB64
}

func TestManager_CommunityMode(t *testing.T) {
	m, err := NewManager("")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	if m.Tier() != TierCommunity {
		t.Errorf("expected community tier, got %s", m.Tier())
	}
	if m.IsEnterprise() {
		t.Error("expected not enterprise in community mode")
	}
	if m.MaxUsers() != CommunityMaxUsers {
		t.Errorf("expected %d max users, got %d", CommunityMaxUsers, m.MaxUsers())
	}
	if m.MaxSatellites() != CommunityMaxSatellites {
		t.Errorf("expected %d max satellites, got %d", CommunityMaxSatellites, m.MaxSatellites())
	}
	if m.MaxRecordings() != CommunityMaxRecordings {
		t.Errorf("expected %d max recordings, got %d", CommunityMaxRecordings, m.MaxRecordings())
	}
	if m.HasFeature(FeatureHITL) {
		t.Error("expected HITL feature to be disabled in community mode")
	}
}

func TestManager_ValidEnterpriseLicense(t *testing.T) {
	pub, priv := generateTestKey(t)
	pubB64 := base64.StdEncoding.EncodeToString(pub)

	m, err := NewManager(pubB64)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	claims := Claims{
		Subject:       "customer-123",
		Customer:      "Acme Corp",
		Tier:          TierEnterprise,
		MaxUsers:      50,
		MaxSatellites: 100,
		Features:      []string{FeatureHITL, FeatureSIEM, FeatureRBAC, FeatureAdvancedTelemetry, FeatureAdvancedRecordings},
		IssuedAt:      time.Now().Unix(),
		ExpiresAt:     time.Now().Add(365 * 24 * time.Hour).Unix(),
	}

	token := signJWT(t, priv, claims)
	if err := m.LoadKey(token); err != nil {
		t.Fatalf("LoadKey failed: %v", err)
	}

	if m.Tier() != TierEnterprise {
		t.Errorf("expected enterprise tier, got %s", m.Tier())
	}
	if !m.IsEnterprise() {
		t.Error("expected IsEnterprise() to be true")
	}
	if m.MaxUsers() != 50 {
		t.Errorf("expected 50 max users, got %d", m.MaxUsers())
	}
	if m.MaxSatellites() != 100 {
		t.Errorf("expected 100 max satellites, got %d", m.MaxSatellites())
	}
	if !m.HasFeature(FeatureHITL) {
		t.Error("expected HITL feature to be enabled")
	}
	if !m.HasFeature(FeatureSIEM) {
		t.Error("expected SIEM feature to be enabled")
	}
	if m.HasFeature(FeatureDiscovery) {
		t.Error("expected discovery feature to be disabled (not in license)")
	}

	// Advanced recordings should give unlimited
	if m.MaxRecordings() != 0 {
		t.Errorf("expected unlimited recordings (0), got %d", m.MaxRecordings())
	}
	// Advanced telemetry should give 30 days
	if m.TelemetryRetentionHours() != 720 {
		t.Errorf("expected 720 hours telemetry retention, got %d", m.TelemetryRetentionHours())
	}
}

func TestManager_ExpiredLicense(t *testing.T) {
	pub, priv := generateTestKey(t)
	pubB64 := base64.StdEncoding.EncodeToString(pub)

	m, err := NewManager(pubB64)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	claims := Claims{
		Subject:   "customer-expired",
		Customer:  "Old Corp",
		Tier:      TierEnterprise,
		IssuedAt:  time.Now().Add(-2 * 365 * 24 * time.Hour).Unix(),
		ExpiresAt: time.Now().Add(-24 * time.Hour).Unix(), // expired yesterday
	}

	token := signJWT(t, priv, claims)
	err = m.LoadKey(token)
	if err == nil {
		t.Fatal("expected error for expired license, got nil")
	}
	if m.Tier() != TierCommunity {
		t.Errorf("expected community tier after failed load, got %s", m.Tier())
	}
}

func TestManager_InvalidSignature(t *testing.T) {
	pub, _ := generateTestKey(t)
	_, wrongPriv := generateTestKey(t) // different key
	pubB64 := base64.StdEncoding.EncodeToString(pub)

	m, err := NewManager(pubB64)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	claims := Claims{
		Subject:   "customer-tampered",
		Tier:      TierEnterprise,
		ExpiresAt: time.Now().Add(365 * 24 * time.Hour).Unix(),
	}

	// Sign with the WRONG key
	token := signJWT(t, wrongPriv, claims)
	err = m.LoadKey(token)
	if err == nil {
		t.Fatal("expected error for invalid signature, got nil")
	}
}

func TestManager_EmptyKeyIsCommunity(t *testing.T) {
	pub, _ := generateTestKey(t)
	pubB64 := base64.StdEncoding.EncodeToString(pub)

	m, err := NewManager(pubB64)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Load empty key — should stay in community mode
	if err := m.LoadKey(""); err != nil {
		t.Fatalf("LoadKey('') should not fail: %v", err)
	}

	if m.Tier() != TierCommunity {
		t.Errorf("expected community tier, got %s", m.Tier())
	}
	if m.Claims() != nil {
		t.Error("expected nil claims in community mode")
	}
}

func TestManager_MalformedToken(t *testing.T) {
	pub, _ := generateTestKey(t)
	pubB64 := base64.StdEncoding.EncodeToString(pub)

	m, err := NewManager(pubB64)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	cases := []struct {
		name  string
		token string
	}{
		{"no dots", "notavalidtoken"},
		{"one dot", "header.payload"},
		{"four dots", "a.b.c.d"},
		{"empty parts", ".."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := m.LoadKey(tc.token); err == nil {
				t.Error("expected error for malformed token")
			}
		})
	}
}

func TestAllEnterpriseFeatures(t *testing.T) {
	features := AllEnterpriseFeatures()
	if len(features) < 5 {
		t.Errorf("expected at least 5 enterprise features, got %d", len(features))
	}

	// Check that all features have non-empty fields
	for _, f := range features {
		if f.ID == "" || f.Name == "" || f.Description == "" {
			t.Errorf("feature %+v has empty fields", f)
		}
	}
}
