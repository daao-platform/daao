// Package api provides HTTP handlers for the Nexus API Gateway.
package api

import (
	"encoding/json"
	"log/slog"
	"fmt"
	"net/http"
	"strings"

	"github.com/daao/nexus/internal/audit"
	"github.com/daao/nexus/internal/secrets"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProviderConfigHandler handles provider configuration API requests.
// Uses the LocalBackend directly for encrypted storage of API keys in the Nexus DB.
// This aligns with PRD §8.1: Nexus as Secrets Broker, §8.2: Local backend (community tier).
type ProviderConfigHandler struct {
	backend     *secrets.LocalBackend
	auditLogger *audit.AuditLogger
}

// NewProviderConfigHandler creates a new ProviderConfigHandler.
func NewProviderConfigHandler(db *pgxpool.Pool, auditLogger ...*audit.AuditLogger) *ProviderConfigHandler {
	h := &ProviderConfigHandler{
		backend: secrets.NewLocalBackend(db),
	}
	if len(auditLogger) > 0 {
		h.auditLogger = auditLogger[0]
	}
	return h
}

// ProviderConfigRequest represents a single provider config in a save request.
type ProviderConfigRequest struct {
	ID     string `json:"id"`
	APIKey string `json:"api_key"`
}

// ProviderConfigResponse represents a single provider config in a response.
// API keys are always masked — the server never returns the full key.
type ProviderConfigResponse struct {
	ID        string `json:"id"`
	MaskedKey string `json:"masked_key"`
	HasKey    bool   `json:"has_key"`
}

// SaveProvidersRequest is the request body for saving provider configurations.
type SaveProvidersRequest struct {
	Providers []ProviderConfigRequest `json:"providers"`
}

// SaveProvidersResponse is the response after saving provider configurations.
type SaveProvidersResponse struct {
	Providers []ProviderConfigResponse `json:"providers"`
}

// providerSecretRef returns the encrypted_secrets key for a provider API key.
func providerSecretRef(providerID string) string {
	return "provider_api_key:" + providerID
}

// maskAPIKey returns a masked version of an API key (e.g., "sk-...abc1").
func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return strings.Repeat("•", len(key))
	}
	prefix := key[:3]
	suffix := key[len(key)-4:]
	return prefix + "..." + suffix
}

// RegisterProviderConfigRoutes registers provider config routes.
func RegisterProviderConfigRoutes(mux *http.ServeMux, handler *ProviderConfigHandler) {
	mux.HandleFunc("/api/v1/config/providers", handler.handleProviderConfig)
}

// handleProviderConfig routes GET and PUT requests.
func (h *ProviderConfigHandler) handleProviderConfig(w http.ResponseWriter, r *http.Request) {
	if h.backend == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "secrets backend not configured")
		return
	}

	switch r.Method {
	case "GET":
		h.handleGetProviders(w, r)
	case "PUT":
		h.handleSaveProviders(w, r)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleGetProviders returns the list of configured providers with masked keys.
// GET /api/v1/config/providers
func (h *ProviderConfigHandler) handleGetProviders(w http.ResponseWriter, r *http.Request) {
	knownProviders := []string{"anthropic", "google", "openai", "ollama"}
	ctx := r.Context()

	var providers []ProviderConfigResponse
	for _, pid := range knownProviders {
		ref := providerSecretRef(pid)
		value, err := h.backend.FetchSecret(ctx, ref)
		if err != nil || value == "" {
			providers = append(providers, ProviderConfigResponse{
				ID:        pid,
				MaskedKey: "",
				HasKey:    false,
			})
		} else {
			providers = append(providers, ProviderConfigResponse{
				ID:        pid,
				MaskedKey: maskAPIKey(value),
				HasKey:    true,
			})
		}
	}

	writeJSON(w, http.StatusOK, SaveProvidersResponse{Providers: providers})
}

// handleSaveProviders saves provider API keys encrypted in the Nexus database.
// PUT /api/v1/config/providers
// Only non-empty keys are stored; empty keys are skipped (preserving existing values).
func (h *ProviderConfigHandler) handleSaveProviders(w http.ResponseWriter, r *http.Request) {
	var req SaveProvidersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()
	var savedProviders []ProviderConfigResponse

	for _, p := range req.Providers {
		ref := providerSecretRef(p.ID)

		// Only update if a new key was provided (non-empty)
		if p.APIKey != "" {
			if err := h.backend.StoreSecret(ctx, ref, p.APIKey); err != nil {
				slog.Error(fmt.Sprintf("Failed to store secret for provider %s: %v", p.ID, err), "component", "api")
				writeJSONError(w, http.StatusInternalServerError, "failed to save provider key: "+p.ID)
				return
			}
			savedProviders = append(savedProviders, ProviderConfigResponse{
				ID:        p.ID,
				MaskedKey: maskAPIKey(p.APIKey),
				HasKey:    true,
			})
		} else {
			// Check if key already exists
			value, err := h.backend.FetchSecret(ctx, ref)
			if err != nil || value == "" {
				savedProviders = append(savedProviders, ProviderConfigResponse{
					ID:        p.ID,
					MaskedKey: "",
					HasKey:    false,
				})
			} else {
				savedProviders = append(savedProviders, ProviderConfigResponse{
					ID:        p.ID,
					MaskedKey: maskAPIKey(value),
					HasKey:    true,
				})
			}
		}
	}

	// Audit log the provider config update (no secrets logged)
	if h.auditLogger != nil {
		providersUpdated := make([]string, len(req.Providers))
		for i, p := range req.Providers {
			providersUpdated[i] = p.ID
		}
		ctx = audit.WithClientIP(ctx, audit.ClientIPFromRequest(r))
		h.auditLogger.Log(ctx, "provider.update", "provider_config", "providers", map[string]interface{}{
			"provider": strings.Join(providersUpdated, ","),
		})
	}

	writeJSON(w, http.StatusOK, SaveProvidersResponse{Providers: savedProviders})
}
