package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-key-for-ci"
const testIssuer = "daao"

// createTestToken creates a signed JWT with the given claims map.
// We use MapClaims to avoid JSON tag conflicts in UserClaims (UserID has json:"sub"
// which conflicts with RegisteredClaims.Subject).
func createTestToken(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("Failed to sign token: %v", err)
	}
	return tokenString
}

func TestJWTValidator_ValidToken(t *testing.T) {
	v := NewJWTTokenValidator(testSecret, testIssuer)

	token := createTestToken(t, testSecret, jwt.MapClaims{
		"sub":  "user-123",
		"role": "admin",
		"iss":  testIssuer,
		"exp":  time.Now().Add(1 * time.Hour).Unix(),
	})

	claims, err := v.Validate(token)
	if err != nil {
		t.Fatalf("Expected valid token, got error: %v", err)
	}
	if claims.UserID != "user-123" {
		t.Errorf("Expected UserID 'user-123', got '%s'", claims.UserID)
	}
}

func TestJWTValidator_ExpiredToken(t *testing.T) {
	v := NewJWTTokenValidator(testSecret, testIssuer)

	token := createTestToken(t, testSecret, jwt.MapClaims{
		"sub": "user-123",
		"iss": testIssuer,
		"exp": jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
	})

	_, err := v.Validate(token)
	if err != ErrTokenExpired {
		t.Errorf("Expected ErrTokenExpired, got: %v", err)
	}
}

func TestJWTValidator_WrongIssuer(t *testing.T) {
	v := NewJWTTokenValidator(testSecret, testIssuer)

	token := createTestToken(t, testSecret, jwt.MapClaims{
		"sub": "user-123",
		"iss": "wrong-issuer",
		"exp": jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
	})

	_, err := v.Validate(token)
	if err == nil {
		t.Error("Expected error for wrong issuer, got nil")
	}
}

func TestJWTValidator_WrongSecret(t *testing.T) {
	v := NewJWTTokenValidator(testSecret, testIssuer)

	token := createTestToken(t, "wrong-secret", jwt.MapClaims{
		"sub": "user-123",
		"iss": testIssuer,
		"exp": jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
	})

	_, err := v.Validate(token)
	if err == nil {
		t.Error("Expected error for wrong secret, got nil")
	}
}

func TestJWTValidator_MalformedToken(t *testing.T) {
	v := NewJWTTokenValidator(testSecret, testIssuer)

	_, err := v.Validate("not.a.valid.jwt.token")
	if err == nil {
		t.Error("Expected error for malformed token, got nil")
	}
}

func TestJWTValidator_EmptyToken(t *testing.T) {
	v := NewJWTTokenValidator(testSecret, testIssuer)

	_, err := v.Validate("")
	if err != ErrTokenInvalid {
		t.Errorf("Expected ErrTokenInvalid for empty token, got: %v", err)
	}
}

func TestJWTValidator_MissingSubject(t *testing.T) {
	v := NewJWTTokenValidator(testSecret, testIssuer)

	token := createTestToken(t, testSecret, jwt.MapClaims{
		// No "sub" claim
		"iss": testIssuer,
		"exp": jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
	})

	_, err := v.Validate(token)
	if err == nil {
		t.Error("Expected error for missing subject, got nil")
	}
}

func TestJWTValidator_NoExpiration(t *testing.T) {
	v := NewJWTTokenValidator(testSecret, testIssuer)

	token := createTestToken(t, testSecret, jwt.MapClaims{
		"sub": "user-123",
		"iss": testIssuer,
		// No "exp" claim
	})

	_, err := v.Validate(token)
	if err == nil {
		t.Error("Expected error for missing expiration, got nil")
	}
}
