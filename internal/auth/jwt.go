package auth

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
)

// ErrTokenExpired is returned when the token has expired
var ErrTokenExpired = errors.New("token has expired")

// ErrTokenInvalid is returned when the token is invalid
var ErrTokenInvalid = errors.New("invalid token")

// UserClaims represents the JWT claims for a user
type UserClaims struct {
	UserID      string `json:"user_id,omitempty"` // Populated from Subject after validation
	Role        string `json:"role"`
	SatelliteID string `json:"satellite_id"`
	jwt.RegisteredClaims
}

// JWTTokenValidator validates JWT tokens for Cockpit (user) authentication
type JWTTokenValidator struct {
	jwtSecret []byte
	issuer    string
}

// NewJWTTokenValidator creates a new JWT validator
func NewJWTTokenValidator(secret string, issuer string) *JWTTokenValidator {
	return &JWTTokenValidator{
		jwtSecret: []byte(secret),
		issuer:    issuer,
	}
}

// Validate validates a JWT token and returns the claims
func (v *JWTTokenValidator) Validate(tokenString string) (*UserClaims, error) {
	if tokenString == "" {
		return nil, ErrTokenInvalid
	}

	// Parse the token without validation first to check the signing method
	token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate the signing algorithm
		switch token.Method {
		case jwt.SigningMethodHS256:
			return v.jwtSecret, nil
		case jwt.SigningMethodRS256:
			// For RS256, we need to handle the public key
			// In production, this would load from JWKS endpoint or configuration
			return nil, ErrTokenInvalid
		default:
			return nil, ErrTokenInvalid
		}
	}, jwt.WithIssuer(v.issuer), jwt.WithExpirationRequired())

	if err != nil {
		// Check if the error is due to expiration
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		// Check for validation errors
		if errors.Is(err, jwt.ErrTokenSignatureInvalid) ||
			errors.Is(err, jwt.ErrTokenMalformed) ||
			errors.Is(err, jwt.ErrTokenInvalidIssuer) ||
			errors.Is(err, jwt.ErrTokenRequiredClaimMissing) {
			return nil, ErrTokenInvalid
		}
		return nil, ErrTokenInvalid
	}

	// Extract claims
	claims, ok := token.Claims.(*UserClaims)
	if !ok || claims.Subject == "" {
		return nil, ErrTokenInvalid
	}

	// Set the UserID from the subject claim
	claims.UserID = claims.Subject

	return claims, nil
}
