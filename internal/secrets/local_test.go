package secrets

import (
	"os"
	"testing"
)

// =============================================================================
// deriveMasterKey tests
// =============================================================================

func TestDeriveMasterKey_Deterministic(t *testing.T) {
	key1 := deriveMasterKey("test-key")
	key2 := deriveMasterKey("test-key")
	if len(key1) != 32 {
		t.Errorf("deriveMasterKey length = %d, want 32", len(key1))
	}
	for i := range key1 {
		if key1[i] != key2[i] {
			t.Fatal("deriveMasterKey is not deterministic")
		}
	}
}

func TestDeriveMasterKey_DifferentInputs(t *testing.T) {
	key1 := deriveMasterKey("input-a")
	key2 := deriveMasterKey("input-b")
	same := true
	for i := range key1 {
		if key1[i] != key2[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("Different inputs produced the same key")
	}
}

// =============================================================================
// hashKey tests
// =============================================================================

func TestHashKey_Deterministic(t *testing.T) {
	h1 := hashKey("my-secret-ref")
	h2 := hashKey("my-secret-ref")
	if h1 != h2 {
		t.Errorf("hashKey is not deterministic: %q vs %q", h1, h2)
	}
}

func TestHashKey_DifferentInputs(t *testing.T) {
	h1 := hashKey("ref-a")
	h2 := hashKey("ref-b")
	if h1 == h2 {
		t.Error("Different inputs produced the same hash")
	}
}

// =============================================================================
// encrypt / decrypt round-trip
// =============================================================================

func TestEncryptDecryptRoundTrip(t *testing.T) {
	lb := &LocalBackend{
		masterKey: deriveMasterKey("test-master-key"),
	}

	plaintext := "super-secret-api-key-sk-12345"
	cipherText, nonce, err := lb.encrypt([]byte(plaintext))
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	if len(cipherText) == 0 {
		t.Fatal("cipherText is empty")
	}
	if len(nonce) == 0 {
		t.Fatal("nonce is empty")
	}

	// Decrypt
	decrypted, err := lb.decrypt(cipherText, nonce)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if string(decrypted) != plaintext {
		t.Errorf("decrypt = %q, want %q", string(decrypted), plaintext)
	}
}

func TestEncryptProducesUniqueNonces(t *testing.T) {
	lb := &LocalBackend{
		masterKey: deriveMasterKey("test-master-key"),
	}

	_, nonce1, err := lb.encrypt([]byte("same-data"))
	if err != nil {
		t.Fatalf("encrypt #1 failed: %v", err)
	}
	_, nonce2, err := lb.encrypt([]byte("same-data"))
	if err != nil {
		t.Fatalf("encrypt #2 failed: %v", err)
	}

	// Nonces must be different
	same := true
	for i := range nonce1 {
		if nonce1[i] != nonce2[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("Two encryptions produced the same nonce — nonce reuse is a critical vulnerability")
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	lb1 := &LocalBackend{masterKey: deriveMasterKey("key-1")}
	lb2 := &LocalBackend{masterKey: deriveMasterKey("key-2")}

	cipherText, nonce, err := lb1.encrypt([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = lb2.decrypt(cipherText, nonce)
	if err == nil {
		t.Error("Expected error decrypting with wrong key")
	}
}

// =============================================================================
// NewLocalBackend with DAAO_SECRET_KEY env var — H3 fix
// =============================================================================

func TestNewLocalBackend_WithEnvVar(t *testing.T) {
	// Save and restore env
	orig := os.Getenv("DAAO_SECRET_KEY")
	defer os.Setenv("DAAO_SECRET_KEY", orig)

	os.Setenv("DAAO_SECRET_KEY", "test-env-secret-key")

	// NewLocalBackend requires a db pool, but we pass nil since we only
	// test the key derivation logic here
	lb := NewLocalBackend(nil)
	if lb == nil {
		t.Fatal("NewLocalBackend returned nil")
	}
	if len(lb.masterKey) != 32 {
		t.Errorf("masterKey length = %d, want 32", len(lb.masterKey))
	}

	// Key should be derived from our env var, not random
	expected := deriveMasterKey("test-env-secret-key")
	for i := range expected {
		if lb.masterKey[i] != expected[i] {
			t.Error("masterKey doesn't match expected derived key from DAAO_SECRET_KEY")
			break
		}
	}
}

func TestNewLocalBackend_WithoutEnvVar(t *testing.T) {
	orig := os.Getenv("DAAO_SECRET_KEY")
	defer os.Setenv("DAAO_SECRET_KEY", orig)

	os.Unsetenv("DAAO_SECRET_KEY")

	lb := NewLocalBackend(nil)
	if lb == nil {
		t.Fatal("NewLocalBackend returned nil with no env var")
	}
	if len(lb.masterKey) != 32 {
		t.Errorf("masterKey length = %d, want 32 (random key)", len(lb.masterKey))
	}
}

// =============================================================================
// NewLocalBackendWithKey tests
// =============================================================================

func TestNewLocalBackendWithKey_Exact32(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	lb := NewLocalBackendWithKey(nil, key)
	if len(lb.masterKey) != 32 {
		t.Errorf("masterKey length = %d, want 32", len(lb.masterKey))
	}
	// Should be the exact same key (not re-hashed)
	for i := range key {
		if lb.masterKey[i] != key[i] {
			t.Error("masterKey differs from input for 32-byte key")
			break
		}
	}
}

func TestNewLocalBackendWithKey_NonStandardLength(t *testing.T) {
	// Non-32-byte key should be hashed to 32 bytes
	key := []byte("short-key")
	lb := NewLocalBackendWithKey(nil, key)
	if len(lb.masterKey) != 32 {
		t.Errorf("masterKey length = %d, want 32", len(lb.masterKey))
	}
}
