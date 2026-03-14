package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/daao/nexus/internal/audit"
	"github.com/daao/nexus/internal/auth"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuthHandler provides HTTP handlers for local authentication.
type AuthHandler struct {
	db          *pgxpool.Pool
	store       *auth.UserStore
	jwtSecret   []byte
	jwtIssuer   string
	auditLogger *audit.AuditLogger
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(pool *pgxpool.Pool, jwtSecret, jwtIssuer string, auditLogger ...*audit.AuditLogger) *AuthHandler {
	h := &AuthHandler{
		db:        pool,
		store:     auth.NewUserStore(pool),
		jwtSecret: []byte(jwtSecret),
		jwtIssuer: jwtIssuer,
	}
	if len(auditLogger) > 0 {
		h.auditLogger = auditLogger[0]
	}
	return h
}

// RegisterAuthRoutes registers local auth routes.
func RegisterAuthRoutes(mux *http.ServeMux, h *AuthHandler) {
	mux.HandleFunc("/api/v1/auth/login", h.handleLogin)
	mux.HandleFunc("/api/v1/auth/change-password", h.handleChangePassword)
}

// loginRequest represents a login request body.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// loginResponse represents a successful login response.
type loginResponse struct {
	Token string        `json:"token"`
	User  *UserResponse `json:"user"`
}

// handleLogin handles POST /api/v1/auth/login
func (h *AuthHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		writeJSONError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	// Verify credentials
	user, err := h.store.VerifyPassword(r.Context(), req.Email, req.Password)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	// Issue JWT
	now := time.Now()
	claims := &auth.UserClaims{
		Role: user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			Issuer:    h.jwtIssuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(h.jwtSecret)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	// Audit log
	if h.auditLogger != nil {
		ctx := audit.WithClientIP(r.Context(), audit.ClientIPFromRequest(r))
		h.auditLogger.Log(ctx, "user.login", "user", user.ID.String(), map[string]interface{}{
			"email":    user.Email,
			"provider": "local",
		})
	}

	resp := &loginResponse{
		Token: tokenStr,
		User: &UserResponse{
			ID:        user.ID.String(),
			Email:     user.Email,
			Name:      user.Name,
			Role:      user.Role,
			AvatarURL: user.AvatarURL,
			CreatedAt: user.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		},
	}
	if user.LastLoginAt != nil {
		s := user.LastLoginAt.Format("2006-01-02T15:04:05Z07:00")
		resp.User.LastLoginAt = &s
	}

	writeJSON(w, http.StatusOK, resp)
}

// changePasswordRequest represents a password change request.
type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// handleChangePassword handles POST /api/v1/auth/change-password
func (h *AuthHandler) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Require authentication
	user, ok := auth.UserFromContext(r.Context())
	if !ok || user == nil {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeJSONError(w, http.StatusBadRequest, "current_password and new_password are required")
		return
	}

	// Verify current password
	_, err := h.store.VerifyPassword(r.Context(), user.Email, req.CurrentPassword)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	// Set new password
	userID, err := uuid.Parse(user.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "invalid user ID")
		return
	}
	if err := h.store.SetPassword(r.Context(), userID, req.NewPassword); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Audit log
	if h.auditLogger != nil {
		ctx := audit.WithClientIP(r.Context(), audit.ClientIPFromRequest(r))
		h.auditLogger.Log(ctx, "user.password_change", "user", user.ID, nil)
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
