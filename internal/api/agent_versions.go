// Package api provides HTTP handlers for the Nexus API Gateway.
package api

import (
	"encoding/json"
	"log/slog"
	"fmt"
	"net/http"
	"strings"

	"github.com/daao/nexus/internal/audit"
	"github.com/daao/nexus/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ============================================================================
// Request/Response Types
// ============================================================================

// RollbackAgentRequest represents a request to rollback an agent to a previous version
type RollbackAgentRequest struct {
	Version string `json:"version"`
}

// AgentVersionResponse represents an agent version in API responses (lightweight - no snapshot)
type AgentVersionResponse struct {
	ID            uuid.UUID `json:"id"`
	Version       string    `json:"version"`
	ChangeSummary *string   `json:"change_summary"`
	CreatedBy     *string   `json:"created_by"`
	CreatedAt     string    `json:"created_at"`
}

// AgentVersionDetailResponse represents an agent version with full snapshot
type AgentVersionDetailResponse struct {
	ID            uuid.UUID      `json:"id"`
	AgentID       uuid.UUID      `json:"agent_id"`
	Version       string         `json:"version"`
	Snapshot      json.RawMessage `json:"snapshot"`
	ChangeSummary *string        `json:"change_summary"`
	CreatedBy     *string        `json:"created_by"`
	CreatedAt     string         `json:"created_at"`
}

// ============================================================================
// Handler Implementations
// ============================================================================

// HandleListAgentVersions handles GET /api/v1/agents/{id}/versions
// Returns a list of versions for the agent without the snapshot field (lightweight listing)
func (h *AgentHandler) HandleListAgentVersions(w http.ResponseWriter, r *http.Request, agentID string) {
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	// Parse agentID as UUID
	id, err := uuid.Parse(agentID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	// Verify agent exists
	agent, err := database.GetAgentDefinition(r.Context(), h.dbPool, id)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleListAgentVersions: failed to get agent: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to list versions")
		return
	}
	if agent == nil {
		writeJSONError(w, http.StatusNotFound, "agent not found")
		return
	}

	// List versions
	versions, err := database.ListAgentVersions(r.Context(), h.dbPool, id)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleListAgentVersions: failed to list versions: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to list versions")
		return
	}

	// Convert to response format (without snapshot)
	versionResponses := make([]AgentVersionResponse, 0, len(versions))
	for _, v := range versions {
		versionResponses = append(versionResponses, AgentVersionResponse{
			ID:            v.ID,
			Version:       v.Version,
			ChangeSummary: v.ChangeSummary,
			CreatedBy:     v.CreatedBy,
			CreatedAt:     v.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"versions": versionResponses})
}

// HandleGetAgentVersion handles GET /api/v1/agents/{id}/versions/{version}
// Returns a single version including the full snapshot
func (h *AgentHandler) HandleGetAgentVersion(w http.ResponseWriter, r *http.Request, agentID, version string) {
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	// Parse agentID as UUID
	id, err := uuid.Parse(agentID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	// Get version with snapshot
	agentVersion, err := database.GetAgentVersion(r.Context(), h.dbPool, id, version)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleGetAgentVersion: failed to get version: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to get version")
		return
	}
	if agentVersion == nil {
		writeJSONError(w, http.StatusNotFound, "version not found")
		return
	}

	// Build response with full snapshot
	response := AgentVersionDetailResponse{
		ID:            agentVersion.ID,
		AgentID:       agentVersion.AgentID,
		Version:       agentVersion.Version,
		Snapshot:      agentVersion.Snapshot,
		ChangeSummary: agentVersion.ChangeSummary,
		CreatedBy:     agentVersion.CreatedBy,
		CreatedAt:     agentVersion.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"version": response})
}

// HandleRollbackAgent handles POST /api/v1/agents/{id}/rollback
// Restores an agent to a previous version's snapshot
func (h *AgentHandler) HandleRollbackAgent(w http.ResponseWriter, r *http.Request, agentID string) {
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	// Parse agentID as UUID
	id, err := uuid.Parse(agentID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	// Verify agent exists
	existing, err := database.GetAgentDefinition(r.Context(), h.dbPool, id)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleRollbackAgent: failed to get agent: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to rollback agent")
		return
	}
	if existing == nil {
		writeJSONError(w, http.StatusNotFound, "agent not found")
		return
	}

	// Prevent rolling back builtin agents
	if existing.IsBuiltin {
		writeJSONError(w, http.StatusForbidden, "cannot rollback builtin agents")
		return
	}

	// Parse request body
	var req RollbackAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON request body")
		return
	}

	// Validate version
	if req.Version == "" {
		writeJSONError(w, http.StatusBadRequest, "version is required")
		return
	}

	// Get the version snapshot
	versionSnapshot, err := database.GetAgentVersion(r.Context(), h.dbPool, id, req.Version)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleRollbackAgent: failed to get version: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to rollback agent")
		return
	}
	if versionSnapshot == nil {
		writeJSONError(w, http.StatusNotFound, "version not found")
		return
	}

	// Unmarshal snapshot JSONB into agent fields
	// The snapshot contains the agent fields as JSON
	var snapshotData struct {
		Name         string  `json:"name"`
		DisplayName  string  `json:"display_name"`
		Description  *string `json:"description"`
		Version      string  `json:"version"`
		Type         string  `json:"type"`
		Category     string  `json:"category"`
		Icon         *string `json:"icon"`
		Provider     string  `json:"provider"`
		Model        string  `json:"model"`
		SystemPrompt string  `json:"system_prompt"`
		ToolsConfig  string  `json:"tools_config"`
		Guardrails   *string `json:"guardrails"`
		Schedule     *string `json:"schedule"`
		Trigger      *string `json:"trigger"`
		OutputConfig *string `json:"output_config"`
	}

	if err := json.Unmarshal(versionSnapshot.Snapshot, &snapshotData); err != nil {
		slog.Error(fmt.Sprintf("HandleRollbackAgent: failed to unmarshal snapshot: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to parse version snapshot")
		return
	}

	// Build updates from snapshot
	updates := &database.AgentDefinition{
		Name:         snapshotData.Name,
		DisplayName:  snapshotData.DisplayName,
		Description:  snapshotData.Description,
		Version:      snapshotData.Version,
		Type:         snapshotData.Type,
		Category:     snapshotData.Category,
		Icon:         snapshotData.Icon,
		Provider:     snapshotData.Provider,
		Model:        snapshotData.Model,
		SystemPrompt: snapshotData.SystemPrompt,
		ToolsConfig:  snapshotData.ToolsConfig,
		Guardrails:   snapshotData.Guardrails,
		Schedule:     snapshotData.Schedule,
		Trigger:      snapshotData.Trigger,
		OutputConfig: snapshotData.OutputConfig,
		// Preserve immutable fields
		IsBuiltin:    existing.IsBuiltin,
		IsEnterprise: existing.IsEnterprise,
	}

	// Store current version before rollback for audit log
	fromVersion := existing.Version

	// Update agent
	updated, err := database.UpdateAgentDefinition(r.Context(), h.dbPool, id, updates)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleRollbackAgent: failed to update agent: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to rollback agent")
		return
	}

	// Emit audit log
	if h.auditLogger != nil {
		ctx := r.Context()
		ctx = audit.WithClientIP(ctx, audit.ClientIPFromRequest(r))
		h.auditLogger.Log(ctx, "agent.rollback", "agent", agentID, map[string]interface{}{
			"agent_id":     agentID,
			"from_version": fromVersion,
			"to_version":   req.Version,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"agent": databaseAgentToResponse(updated)})
}

// RegisterAgentVersionRoutes registers agent version routes to the given mux.
func RegisterAgentVersionRoutes(mux *http.ServeMux, handler *AgentHandler) {
	// Version routes
	mux.HandleFunc("/api/v1/agents/", handler.handleAgentVersionsAPI)
}

// handleAgentVersionsAPI handles all agent version API requests
func (h *AgentHandler) handleAgentVersionsAPI(w http.ResponseWriter, r *http.Request) {
	// Check if database is configured
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	p := r.URL.Path
	method := r.Method

	// Route: GET /api/v1/agents/{id}/versions - list versions
	if strings.HasPrefix(p, "/api/v1/agents/") && strings.HasSuffix(p, "/versions") && method == "GET" {
		agentID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/agents/"), "/versions")
		h.HandleListAgentVersions(w, r, agentID)
		return
	}

	// Route: GET /api/v1/agents/{id}/versions/{version} - get single version
	if strings.HasPrefix(p, "/api/v1/agents/") && method == "GET" {
		// Check if path matches /api/v1/agents/{id}/versions/{version}
		parts := strings.Split(strings.TrimPrefix(p, "/api/v1/agents/"), "/")
		if len(parts) == 3 && parts[1] == "versions" {
			agentID := parts[0]
			version := parts[2]
			h.HandleGetAgentVersion(w, r, agentID, version)
			return
		}
	}

	// Route: POST /api/v1/agents/{id}/rollback - rollback agent
	if strings.HasPrefix(p, "/api/v1/agents/") && strings.HasSuffix(p, "/rollback") && method == "POST" {
		agentID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/agents/"), "/rollback")
		h.HandleRollbackAgent(w, r, agentID)
		return
	}

	// 404 for unknown routes
	http.NotFound(w, r)
}

// Ensure AgentHandler has dbPool and auditLogger fields (may be shadowed from agents.go)
var _ = func() {
	// This function validates that AgentHandler has the required fields
	var h *AgentHandler
	_ = h.dbPool
	_ = h.auditLogger
	_ = (*pgxpool.Pool)(nil)
	_ = (*audit.AuditLogger)(nil)
}
