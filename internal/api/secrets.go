// Package api provides HTTP handlers for the Nexus API Gateway.
package api

import (
	"encoding/json"
	"log/slog"
	"fmt"
	"net/http"

	"github.com/daao/nexus/internal/secrets"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SecretHandler provides HTTP handlers for secret management operations.
type SecretHandler struct {
	dbPool *pgxpool.Pool
	broker *secrets.Broker
}

// NewSecretHandler creates a new SecretHandler instance.
func NewSecretHandler(pool *pgxpool.Pool, broker *secrets.Broker) *SecretHandler {
	return &SecretHandler{
		dbPool: pool,
		broker: broker,
	}
}

// SecretScopeResponse represents a secret scope in API responses
type SecretScopeResponse struct {
	ID          string  `json:"id"`
	SatelliteID *string `json:"satellite_id,omitempty"`
	Provider    string  `json:"provider"`
	Backend     string  `json:"backend"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// SecretListResponse represents a list of secrets
type SecretListResponse struct {
	Secrets []string `json:"secrets"`
	Count   int      `json:"count"`
}

// RegisterSecretRoutes registers secret routes to the given mux.
// This function is called by the integration task to wire up the routes.
func RegisterSecretRoutes(mux *http.ServeMux, handler *SecretHandler) {
	// Secret routes
	mux.HandleFunc("/api/v1/secrets", handler.handleSecretsAPI)
}

// handleSecretsAPI handles all secret API requests
func (h *SecretHandler) handleSecretsAPI(w http.ResponseWriter, r *http.Request) {
	// Check if broker is configured
	if h.broker == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "secrets not configured")
		return
	}

	p := r.URL.Path
	method := r.Method

	// Route: GET /api/v1/secrets - list secret scopes
	if p == "/api/v1/secrets" && method == "GET" {
		h.HandleListSecretScopes(w, r)
		return
	}

	// Route: GET /api/v1/secrets - create secret scope
	if p == "/api/v1/secrets" && method == "POST" {
		h.HandleCreateSecretScope(w, r)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"service": "Nexus Secrets API", "version": "0.1.0"})
}

// HandleListSecretScopes handles GET /api/v1/secrets
func (h *SecretHandler) HandleListSecretScopes(w http.ResponseWriter, r *http.Request) {
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	ctx := r.Context()
	rows, err := h.dbPool.Query(ctx, `
		SELECT id, satellite_id, provider, backend, created_at, updated_at 
		FROM secret_scopes 
		ORDER BY created_at DESC
	`)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to list secret scopes: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to list secret scopes")
		return
	}
	defer rows.Close()

	var scopes []SecretScopeResponse
	for rows.Next() {
		var scope SecretScopeResponse
		if err := rows.Scan(&scope.ID, &scope.SatelliteID, &scope.Provider, &scope.Backend, &scope.CreatedAt, &scope.UpdatedAt); err != nil {
			slog.Error(fmt.Sprintf("Failed to scan secret scope: %v", err), "component", "api")
			continue
		}
		scopes = append(scopes, scope)
	}

	writeJSON(w, http.StatusOK, scopes)
}

// HandleCreateSecretScope handles POST /api/v1/secrets
func (h *SecretHandler) HandleCreateSecretScope(w http.ResponseWriter, r *http.Request) {
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	var req struct {
		SatelliteID *string `json:"satellite_id"`
		Provider    string  `json:"provider"`
		Backend     string  `json:"backend"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Provider == "" {
		writeJSONError(w, http.StatusBadRequest, "provider is required")
		return
	}

	if req.Backend == "" {
		req.Backend = "local"
	}

	ctx := r.Context()
	var id string
	err := h.dbPool.QueryRow(ctx, `
		INSERT INTO secret_scopes (satellite_id, provider, backend, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		RETURNING id
	`, req.SatelliteID, req.Provider, req.Backend).Scan(&id)

	if err != nil {
		slog.Error(fmt.Sprintf("Failed to create secret scope: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to create secret scope")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":       id,
		"provider": req.Provider,
		"backend":  req.Backend,
	})
}
