// Package ipc provides inter-process communication via Unix Domain Sockets
// (POSIX) and Named Pipes (Windows).
package ipc

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"os"
	"time"
)

// Token entropy: 32 bytes (256-bit)
const tokenEntropy = 32

// TokenLength is the length of the hex-encoded token (64 hex characters)
const tokenLength = 64

// Token environment variable name
const tokenEnvVar = "DAAO_IPC_TOKEN"

// Errors
var (
	ErrTokenInvalid   = errors.New("token is invalid")
	ErrTokenExpired   = errors.New("token has expired")
	ErrTokenMissing   = errors.New("token is missing")
)

// Token represents an authentication token with expiration.
type Token struct {
	Value   string
	Expires time.Time
}

// GenerateToken generates a new 256-bit (32-byte) random token.
func GenerateToken() (string, error) {
	bytes := make([]byte, tokenEntropy)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// GenerateTokenWithExpiry generates a new token with the given maximum age.
// If maxAge is 0, the token expires immediately.
func GenerateTokenWithExpiry(maxAge time.Duration) (*Token, error) {
	value, err := GenerateToken()
	if err != nil {
		return nil, err
	}

	expires := time.Now().Add(maxAge)
	// If maxAge is 0, the token expires immediately
	if maxAge == 0 {
		expires = time.Now()
	}

	return &Token{
		Value:   value,
		Expires: expires,
	}, nil
}

// ValidateToken validates a token against the expected value and checks expiration.
// Returns ErrTokenInvalid if the token value doesn't match.
// Returns ErrTokenExpired if the token has expired.
// Uses constant-time comparison to prevent timing attacks.
func ValidateToken(token, expected string) error {
	// Check if token is missing
	if token == "" {
		return ErrTokenMissing
	}

	// Check token length - must be 64 hex characters
	if len(token) != tokenLength {
		return ErrTokenInvalid
	}

	// Validate hex characters
	if !isValidHex(token) {
		return ErrTokenInvalid
	}

	// Constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
		return ErrTokenInvalid
	}

	return nil
}

// ValidateTokenWithExpiry validates a token with expiration check.
// Returns ErrTokenInvalid if the token value doesn't match.
// Returns ErrTokenExpired if the token has expired.
// Uses constant-time comparison to prevent timing attacks.
func ValidateTokenWithExpiry(token *Token, expected string) error {
	if token == nil || token.Value == "" {
		return ErrTokenMissing
	}

	// Check expiration first
	if time.Now().After(token.Expires) {
		return ErrTokenExpired
	}

	return ValidateToken(token.Value, expected)
}

// isValidHex checks if a string contains only valid hexadecimal characters.
func isValidHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// GetTokenFromEnv retrieves the token from the DAAO_IPC_TOKEN environment variable.
func GetTokenFromEnv() (string, error) {
	token := os.Getenv(tokenEnvVar)
	if token == "" {
		return "", ErrTokenMissing
	}
	return token, nil
}

// InjectTokenIntoEnv sets the DAAO_IPC_TOKEN environment variable in a copy of
// the current environment, suitable for passing to a child process.
func InjectTokenIntoEnv(token string) []string {
	env := os.Environ()
	found := false
	for i, e := range env {
		if len(e) >= len(tokenEnvVar) && e[:len(tokenEnvVar)] == tokenEnvVar {
			env[i] = tokenEnvVar + "=" + token
			found = true
			break
		}
	}
	if !found {
		env = append(env, tokenEnvVar+"="+token)
	}
	return env
}

// InjectTokenIntoProcessEnv injects the token into the provided environment slice.
func InjectTokenIntoProcessEnv(token string, env []string) []string {
	found := false
	for i, e := range env {
		if len(e) >= len(tokenEnvVar) && e[:len(tokenEnvVar)] == tokenEnvVar {
			env[i] = tokenEnvVar + "=" + token
			found = true
			break
		}
	}
	if !found {
		env = append(env, tokenEnvVar+"="+token)
	}
	return env
}
