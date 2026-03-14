package ipc

import (
	"testing"
	"time"
)

// TestTokenValidation tests the token generation and validation flow:
// - generate: can create a valid token
// - validate correct: valid token passes validation
// - reject wrong: wrong token fails validation
// - reject expired: expired token fails validation
func TestTokenValidation(t *testing.T) {
	t.Run("generate creates valid 64-char hex token", func(t *testing.T) {
		token, err := GenerateToken()
		if err != nil {
			t.Fatalf("GenerateToken failed: %v", err)
		}
		if len(token) != tokenLength {
			t.Errorf("Expected token length %d, got %d", tokenLength, len(token))
		}
		if !isValidHex(token) {
			t.Error("Token is not valid hex")
		}
	})

	t.Run("validate correct token passes", func(t *testing.T) {
		token, err := GenerateToken()
		if err != nil {
			t.Fatalf("GenerateToken failed: %v", err)
		}

		err = ValidateToken(token, token)
		if err != nil {
			t.Errorf("ValidateToken should pass for correct token, got: %v", err)
		}
	})

	t.Run("reject wrong token", func(t *testing.T) {
		token, err := GenerateToken()
		if err != nil {
			t.Fatalf("GenerateToken failed: %v", err)
		}

		wrongToken := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 64 a's
		err = ValidateToken(token, wrongToken)
		if err != ErrTokenInvalid {
			t.Errorf("Expected ErrTokenInvalid, got: %v", err)
		}
	})

	t.Run("reject empty token", func(t *testing.T) {
		err := ValidateToken("", "somevalue")
		if err != ErrTokenInvalid && err != ErrTokenMissing {
			t.Errorf("Expected ErrTokenInvalid or ErrTokenMissing, got: %v", err)
		}
	})

	t.Run("reject wrong length token", func(t *testing.T) {
		err := ValidateToken("tooshort", "somevalue")
		if err != ErrTokenInvalid {
			t.Errorf("Expected ErrTokenInvalid, got: %v", err)
		}
	})

	t.Run("reject expired token", func(t *testing.T) {
		// Generate a token with immediate expiry - use -1 to ensure it's already expired
		token, err := GenerateTokenWithExpiry(-1 * time.Second)
		if err != nil {
			t.Fatalf("GenerateTokenWithExpiry failed: %v", err)
		}

		// Token should be expired since we used negative duration
		err = ValidateTokenWithExpiry(token, token.Value)
		if err != ErrTokenExpired {
			t.Errorf("Expected ErrTokenExpired, got: %v", err)
		}
	})

	t.Run("reject expired token with short TTL", func(t *testing.T) {
		// Generate a token with very short TTL
		token, err := GenerateTokenWithExpiry(1 * time.Millisecond)
		if err != nil {
			t.Fatalf("GenerateTokenWithExpiry failed: %v", err)
		}

		// Wait for token to expire
		time.Sleep(10 * time.Millisecond)

		err = ValidateTokenWithExpiry(token, token.Value)
		if err != ErrTokenExpired {
			t.Errorf("Expected ErrTokenExpired, got: %v", err)
		}
	})

	t.Run("accept non-expired token", func(t *testing.T) {
		// Generate a token with long TTL
		token, err := GenerateTokenWithExpiry(1 * time.Hour)
		if err != nil {
			t.Fatalf("GenerateTokenWithExpiry failed: %v", err)
		}

		// Token should still be valid
		err = ValidateTokenWithExpiry(token, token.Value)
		if err != nil {
			t.Errorf("Expected no error for non-expired token, got: %v", err)
		}
	})

	t.Run("reject nil token in ValidateTokenWithExpiry", func(t *testing.T) {
		err := ValidateTokenWithExpiry(nil, "somevalue")
		if err != ErrTokenMissing {
			t.Errorf("Expected ErrTokenMissing, got: %v", err)
		}
	})
}
