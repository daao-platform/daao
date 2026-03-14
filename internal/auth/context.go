package auth

import (
	"context"
	"net/http"
)

type contextKey string

const userContextKey = contextKey("user")

// AuthUser represents an authenticated user extracted from JWT claims and DB lookup.
type AuthUser struct {
	ID        string
	Email     string
	Name      string
	Role      string // "owner", "admin", "viewer"
	AvatarURL string
}

// WithUser injects a user into context.
func WithUser(ctx context.Context, user *AuthUser) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

// UserFromContext extracts a user from context.
func UserFromContext(ctx context.Context) (*AuthUser, bool) {
	user, ok := ctx.Value(userContextKey).(*AuthUser)
	return user, ok && user != nil
}

// RequireRole returns middleware that checks minimum role.
// Returns 401 if no user in context, 403 if insufficient role.
func RequireRole(minRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := UserFromContext(r.Context())
			if !ok || user == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}

			if !HasPermission(user.Role, minRole) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error":"forbidden"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
