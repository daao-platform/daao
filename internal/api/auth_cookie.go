package api

import (
	"net/http"
	"strings"

	"github.com/daao/nexus/internal/auth"
)

// HandleSetAuthCookie handles POST/DELETE /api/v1/auth/cookie.
// POST validates a Bearer token and sets it as an HttpOnly cookie for SSE auth.
// DELETE clears the cookie (used on logout).
func HandleSetAuthCookie(jwtValidator *auth.JWTTokenValidator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			authHeader := r.Header.Get("Authorization")
			if len(authHeader) < 8 || !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, `{"error":"missing Bearer token"}`, http.StatusUnauthorized)
				return
			}
			token := authHeader[7:]

			if _, err := jwtValidator.Validate(token); err != nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}

			http.SetCookie(w, &http.Cookie{
				Name:     "daao_auth",
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				Secure:   true,
				SameSite: http.SameSiteStrictMode,
				MaxAge:   3600, // 1 hour — match JWT expiry
			})
			w.WriteHeader(http.StatusNoContent)

		case http.MethodDelete:
			http.SetCookie(w, &http.Cookie{
				Name:     "daao_auth",
				Value:    "",
				Path:     "/",
				HttpOnly: true,
				Secure:   true,
				SameSite: http.SameSiteStrictMode,
				MaxAge:   -1, // delete cookie
			})
			w.WriteHeader(http.StatusNoContent)

		default:
			w.Header().Set("Allow", "POST, DELETE")
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	}
}
