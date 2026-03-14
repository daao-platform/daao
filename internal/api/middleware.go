package api

import (
	"crypto/x509"
	"log/slog"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/daao/nexus/internal/auth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CertValidator defines the interface for validating certificates
type CertValidator interface {
	Validate(clientCert *x509.Certificate) error
}

// AuthMiddleware creates authentication middleware that handles both
// mTLS (for Satellite) and JWT/OIDC (for Cockpit/User) authentication.
// It enforces per-satellite and per-user rate limits.
// If dbPool is provided and not nil, it will inject authenticated user identity
// into request context after successful JWT validation.
func AuthMiddleware(
	jwtValidator *auth.JWTTokenValidator,
	certValidator CertValidator,
	rateLimiter auth.RateLimiterInterface,
	dbPool *pgxpool.Pool,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Set Content-Type: application/json on all responses
			w.Header().Set("Content-Type", "application/json")

			// Exempt health check, metrics, and login from auth
			if r.URL.Path == "/health" || r.URL.Path == "/metrics" || r.URL.Path == "/api/v1/auth/login" {
				next.ServeHTTP(w, r)
				return
			}

			// Exempt WebSocket upgrade requests — authentication is handled
			// by the first-message protocol in the handler itself
			if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
				next.ServeHTTP(w, r)
				return
			}

			// Check for client certificate (mTLS for Satellite)
			if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
				clientCert := r.TLS.PeerCertificates[0]
				if err := certValidator.Validate(clientCert); err != nil {
					http.Error(w, `{"error":"invalid satellite certificate"}`, http.StatusUnauthorized)
					return
				}

				// Rate limit satellite requests
				satelliteID := clientCert.Subject.CommonName
				if rateLimiter != nil && !rateLimiter.AllowSatellite(satelliteID) {
					http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
					return
				}
			} else {
				// Check for JWT/OIDC token (Cockpit/User auth)
				authHeader := r.Header.Get("Authorization")

				// EventSource (SSE) cannot send Authorization headers.
				// Fall back to HttpOnly cookie set by POST /api/v1/auth/cookie.
				if authHeader == "" {
					if cookie, err := r.Cookie("daao_auth"); err == nil {
						authHeader = "Bearer " + cookie.Value
					}
				}

				oidcIssuer := os.Getenv("OIDC_ISSUER_URL")

				// Normalize broken auth headers sent by frontend when token is null
				if authHeader == "Bearer null" || authHeader == "Bearer undefined" {
					authHeader = ""
				}

				if authHeader == "" || len(authHeader) < 8 {
					if oidcIssuer != "" {
						// OIDC configured — authentication required
						http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
						return
					}
					// No OIDC configured — single-user mode.
					// Inject owner user from DB so RBAC works without SSO.
					if dbPool != nil {
						userStore := auth.NewUserStore(dbPool)
						users, err := userStore.List(r.Context())
						if err == nil {
							for _, u := range users {
								if u.Role == "owner" {
									ctx := r.Context()
									ctx = auth.WithUser(ctx, &auth.AuthUser{
										ID:        u.ID.String(),
										Email:     u.Email,
										Name:      u.Name,
										Role:      u.Role,
										AvatarURL: u.AvatarURL,
									})
									r = r.WithContext(ctx)
									break
								}
							}
						}
					}
					next.ServeHTTP(w, r)
					return
				}

				token := authHeader[7:] // Remove "Bearer " prefix

				// Validate JWT token
				claims, err := jwtValidator.Validate(token)
				if err != nil {
					if oidcIssuer != "" {
						// OIDC configured — reject invalid tokens
						http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
						return
					}
					http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
					return
				}

				// Rate limit user requests
				if rateLimiter != nil && claims != nil && !rateLimiter.AllowUser(claims.UserID) {
					http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
					return
				}

				// Inject authenticated user into context if dbPool is available
				if dbPool != nil && claims != nil {
					userStore := auth.NewUserStore(dbPool)
					var user *auth.User
					var err error

					if oidcIssuer != "" {
						// OIDC mode — upsert from OIDC claims
						user, err = userStore.UpsertFromOIDC(r.Context(), "oidc", claims.Subject, "", "", "")
					} else {
						// Local auth mode — look up user by ID from JWT subject
						userID, parseErr := uuid.Parse(claims.Subject)
						if parseErr == nil {
							user, err = userStore.GetByID(r.Context(), userID)
						}
					}

					if err != nil {
						// Log warning but don't block request
						slog.Error(fmt.Sprintf("warning: failed to resolve user from token: %v", err), "component", "api")
					} else if user != nil {
						// Inject user into request context
						ctx := r.Context()
						ctx = auth.WithUser(ctx, &auth.AuthUser{
							ID:        user.ID.String(),
							Email:     user.Email,
							Name:      user.Name,
							Role:      user.Role,
							AvatarURL: user.AvatarURL,
						})
						r = r.WithContext(ctx)
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
