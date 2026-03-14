// Package api provides HTTP handlers for the Nexus API Gateway.
package api

import (
	"crypto/rand"
	"encoding/json"
	"math/big"
	"net/http"
	"strings"

	"github.com/daao/nexus/internal/audit"
	"github.com/daao/nexus/internal/auth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserHandler provides HTTP handlers for user management operations.
type UserHandler struct {
	db          *pgxpool.Pool
	store       *auth.UserStore
	auditLogger *audit.AuditLogger
}

// NewUserHandler creates a new UserHandler instance with dependencies injected.
func NewUserHandler(pool *pgxpool.Pool, auditLogger ...*audit.AuditLogger) *UserHandler {
	h := &UserHandler{
		db:    pool,
		store: auth.NewUserStore(pool),
	}
	if len(auditLogger) > 0 {
		h.auditLogger = auditLogger[0]
	}
	return h
}

// ============================================================================
// Request Types
// ============================================================================

// InviteUserRequest represents a request to invite a new user
type InviteUserRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// ChangeRoleRequest represents a request to change a user's role
type ChangeRoleRequest struct {
	Role string `json:"role"`
}

// ============================================================================
// Response Types
// ============================================================================

// UserResponse represents a user in API responses
type UserResponse struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	Name        string  `json:"name"`
	Role        string  `json:"role"`
	AvatarURL   string  `json:"avatar_url"`
	LastLoginAt *string `json:"last_login_at"`
	CreatedAt   string  `json:"created_at"`
}

// userToResponse converts an auth.User to a UserResponse
func userToResponse(u *auth.User) *UserResponse {
	var lastLoginAt *string
	if u.LastLoginAt != nil {
		s := u.LastLoginAt.Format("2006-01-02T15:04:05Z07:00")
		lastLoginAt = &s
	}
	return &UserResponse{
		ID:          u.ID.String(),
		Email:       u.Email,
		Name:        u.Name,
		Role:        u.Role,
		AvatarURL:   u.AvatarURL,
		LastLoginAt: lastLoginAt,
		CreatedAt:   u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ============================================================================
// Route Registration
// ============================================================================

// RegisterUserRoutes registers user routes to the given mux.
// This function is called by the integration task to wire up the routes.
func RegisterUserRoutes(mux *http.ServeMux, h *UserHandler) {
	mux.HandleFunc("/api/v1/users", h.handleUsers)
	mux.HandleFunc("/api/v1/users/invite", h.handleInviteUser)
	mux.HandleFunc("/api/v1/users/", h.handleUsersSubpath)
}

// handleUsers handles /api/v1/users and /api/v1/users/invite
func (h *UserHandler) handleUsers(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	p := r.URL.Path
	method := r.Method

	// Route: GET /api/v1/users - list all users (admin only)
	if method == http.MethodGet && p == "/api/v1/users" {
		h.handleListUsers(w, r)
		return
	}

	// Route: POST /api/v1/users/invite - invite a new user (owner only)
	if method == http.MethodPost && p == "/api/v1/users/invite" {
		h.handleInviteUser(w, r)
		return
	}

	// No match
	http.NotFound(w, r)
}

// handleUsersSubpath handles /api/v1/users/me, /api/v1/users/{id}/role, /api/v1/users/{id}
func (h *UserHandler) handleUsersSubpath(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	p := r.URL.Path
	method := r.Method

	// Route: POST /api/v1/users/invite - create a new user with temp password (owner only)
	if method == http.MethodPost && p == "/api/v1/users/invite" {
		h.handleInviteUser(w, r)
		return
	}

	// Route: GET /api/v1/users/me - get current user (viewer minimum)
	if method == http.MethodGet && p == "/api/v1/users/me" {
		h.handleGetCurrentUser(w, r)
		return
	}

	// Check for /api/v1/users/{id}/role
	if strings.HasSuffix(p, "/role") && (method == http.MethodPatch || method == http.MethodPost) {
		h.handleChangeRole(w, r)
		return
	}

	// Otherwise, treat as /api/v1/users/{id} - get or delete user
	if (method == http.MethodGet || method == http.MethodDelete) && strings.HasPrefix(p, "/api/v1/users/") {
		h.handleGetUser(w, r)
		return
	}

	// No match
	http.NotFound(w, r)
}

// ============================================================================
// Handler Implementations
// ============================================================================

// handleListUsers handles GET /api/v1/users - list all users (admin only)
func (h *UserHandler) handleListUsers(w http.ResponseWriter, r *http.Request) {
	// Check authentication and role
	user, ok := auth.UserFromContext(r.Context())
	if !ok || user == nil {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Check admin role
	if !auth.HasPermission(user.Role, "admin") {
		writeJSONError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	// List users from database
	users, err := h.store.List(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list users")
		return
	}

	// Convert to response format
	var userResponses []*UserResponse
	for _, u := range users {
		userResponses = append(userResponses, userToResponse(u))
	}

	writeJSON(w, http.StatusOK, userResponses)
}

// handleGetCurrentUser handles GET /api/v1/users/me - get current user (viewer minimum)
func (h *UserHandler) handleGetCurrentUser(w http.ResponseWriter, r *http.Request) {
	// Check authentication
	user, ok := auth.UserFromContext(r.Context())
	if !ok || user == nil {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Get user from database
	u, err := h.store.GetByID(r.Context(), uuid.MustParse(user.ID))
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "user not found")
		return
	}

	writeJSON(w, http.StatusOK, userToResponse(u))
}

// handleInviteUser handles POST /api/v1/users/invite - create a new user with temp password (owner only)
func (h *UserHandler) handleInviteUser(w http.ResponseWriter, r *http.Request) {
	// Check authentication and role
	user, ok := auth.UserFromContext(r.Context())
	if !ok || user == nil {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Check owner role
	if !auth.HasPermission(user.Role, "owner") {
		writeJSONError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	// Parse request body
	var req InviteUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate email and role
	if req.Email == "" || req.Role == "" {
		writeJSONError(w, http.StatusBadRequest, "email and role are required")
		return
	}

	// Validate role
	validRole := false
	for _, r := range auth.ValidRoles {
		if req.Role == r {
			validRole = true
			break
		}
	}
	if !validRole {
		writeJSONError(w, http.StatusBadRequest, "invalid role")
		return
	}

	// Generate a random temporary password
	tempPassword, err := generateTempPassword(16)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to generate password")
		return
	}

	// Hash the password
	hashedPassword, err := auth.HashPassword(tempPassword)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	// Create the user with password
	name := strings.Split(req.Email, "@")[0]
	createdUser, err := h.store.CreateWithPassword(r.Context(), req.Email, name, req.Role, hashedPassword)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			writeJSONError(w, http.StatusConflict, "email already exists")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	// Audit log the user creation
	if h.auditLogger != nil {
		ctx := r.Context()
		ctx = audit.WithClientIP(ctx, audit.ClientIPFromRequest(r))
		h.auditLogger.Log(ctx, "user.create", "user", createdUser.ID.String(), map[string]interface{}{
			"email": req.Email,
			"role":  req.Role,
		})
	}

	// Return user info + temporary password (shown once to the owner)
	resp := userToResponse(createdUser)
	respMap := map[string]interface{}{
		"user":               resp,
		"temporary_password": tempPassword,
	}
	writeJSON(w, http.StatusCreated, respMap)
}

// generateTempPassword creates a random alphanumeric password of the given length
func generateTempPassword(length int) (string, error) {
	const chars = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			return "", err
		}
		b[i] = chars[n.Int64()]
	}
	return string(b), nil
}

// handleGetUser handles GET /api/v1/users/{id} - get a user by ID (admin only)
// handleGetUser also handles DELETE /api/v1/users/{id} - delete a user (owner only)
func (h *UserHandler) handleGetUser(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from path
	idStr := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
	userID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	// Handle DELETE
	if r.Method == http.MethodDelete {
		h.handleDeleteUser(w, r, userID)
		return
	}

	// Handle GET
	// Check authentication and role
	user, ok := auth.UserFromContext(r.Context())
	if !ok || user == nil {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Check admin role
	if !auth.HasPermission(user.Role, "admin") {
		writeJSONError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	// Get user from database
	u, err := h.store.GetByID(r.Context(), userID)
	if err != nil {
		if err == auth.ErrUserNotFound {
			writeJSONError(w, http.StatusNotFound, "user not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to get user")
		return
	}

	writeJSON(w, http.StatusOK, userToResponse(u))
}

// handleChangeRole handles PATCH /api/v1/users/{id}/role - change a user's role (owner only)
func (h *UserHandler) handleChangeRole(w http.ResponseWriter, r *http.Request) {
	// Check authentication and role
	user, ok := auth.UserFromContext(r.Context())
	if !ok || user == nil {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Check owner role
	if !auth.HasPermission(user.Role, "owner") {
		writeJSONError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	// Extract user ID from path
	// Path is like /api/v1/users/{id}/role
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/users/"), "/")
	if len(parts) < 2 {
		writeJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}
	userID, err := uuid.Parse(parts[0])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	// Parse request body
	var req ChangeRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate role
	validRole := false
	for _, r := range auth.ValidRoles {
		if req.Role == r {
			validRole = true
			break
		}
	}
	if !validRole {
		writeJSONError(w, http.StatusBadRequest, "invalid role")
		return
	}

	// Update the role
	err = h.store.UpdateRole(r.Context(), userID, req.Role)
	if err != nil {
		if err == auth.ErrUserNotFound {
			writeJSONError(w, http.StatusNotFound, "user not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to update role")
		return
	}

	// Audit log the role change
	if h.auditLogger != nil {
		ctx := r.Context()
		ctx = audit.WithClientIP(ctx, audit.ClientIPFromRequest(r))
		h.auditLogger.Log(ctx, "user.role_change", "user", userID.String(), map[string]interface{}{
			"user_id":   userID.String(),
			"new_role":  req.Role,
		})
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleDeleteUser handles DELETE /api/v1/users/{id} - delete a user (owner only)
func (h *UserHandler) handleDeleteUser(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	// Check authentication and role
	user, ok := auth.UserFromContext(r.Context())
	if !ok || user == nil {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Check owner role
	if !auth.HasPermission(user.Role, "owner") {
		writeJSONError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	// Check if trying to delete self
	currentUserID, err := uuid.Parse(user.ID)
	if err == nil && currentUserID == userID {
		writeJSONError(w, http.StatusBadRequest, "cannot delete self")
		return
	}

	// Delete the user
	err = h.store.Delete(r.Context(), userID)
	if err != nil {
		if err == auth.ErrUserNotFound {
			writeJSONError(w, http.StatusNotFound, "user not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}

	// Audit log the user delete
	if h.auditLogger != nil {
		ctx := r.Context()
		ctx = audit.WithClientIP(ctx, audit.ClientIPFromRequest(r))
		h.auditLogger.Log(ctx, "user.delete", "user", userID.String(), map[string]interface{}{
			"user_id": userID.String(),
		})
	}

	w.WriteHeader(http.StatusNoContent)
}
