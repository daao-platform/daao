// Package api provides HTTP handlers for the Nexus API Gateway.
package api

import (
	"encoding/json"
	"log/slog"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/daao/nexus/internal/audit"
	"github.com/daao/nexus/internal/database"
	"github.com/daao/nexus/internal/dispatch"
	"github.com/daao/nexus/internal/license"
	"github.com/daao/nexus/internal/stream"
	"github.com/daao/nexus/proto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AgentHandler provides HTTP handlers for agent CRUD and deployment operations.
type AgentHandler struct {
	dbPool         *pgxpool.Pool
	streamRegistry stream.StreamRegistryInterface
	auditLogger    *audit.AuditLogger
	dispatcher     *dispatch.Dispatcher
	licenseMgr     *license.Manager
}

// NewAgentHandler creates a new AgentHandler instance with dependencies injected.
func NewAgentHandler(dbPool *pgxpool.Pool, streamRegistry stream.StreamRegistryInterface, dispatcher *dispatch.Dispatcher, licenseMgr *license.Manager, auditLogger ...*audit.AuditLogger) *AgentHandler {
	h := &AgentHandler{
		dbPool:         dbPool,
		streamRegistry: streamRegistry,
		dispatcher:     dispatcher,
		licenseMgr:     licenseMgr,
	}
	if len(auditLogger) > 0 {
		h.auditLogger = auditLogger[0]
	}
	return h
}

// ============================================================================
// Request Types
// ============================================================================

// CreateAgentRequest represents a request to create a new agent definition
type CreateAgentRequest struct {
	Name         string          `json:"name"`
	DisplayName  string          `json:"display_name"`
	Description  *string         `json:"description"`
	Version      string          `json:"version"`
	Type         string          `json:"type"`
	Category     string          `json:"category"`
	Icon         *string         `json:"icon"`
	Provider     string          `json:"provider"`
	Model        string          `json:"model"`
	SystemPrompt string          `json:"system_prompt"`
	ToolsConfig  json.RawMessage `json:"tools_config"`
	Guardrails   json.RawMessage `json:"guardrails"`
	Schedule     *string         `json:"schedule"`
	Trigger      *string         `json:"trigger"`
	OutputConfig *string         `json:"output_config"`
	Routing      *string         `json:"routing"`
}

// UpdateAgentRequest represents a request to update an agent definition
type UpdateAgentRequest struct {
	Name         *string `json:"name"`
	DisplayName  *string `json:"display_name"`
	Description  *string `json:"description"`
	Version      *string `json:"version"`
	Type         *string `json:"type"`
	Category     *string `json:"category"`
	Icon         *string `json:"icon"`
	Provider     *string `json:"provider"`
	Model        *string `json:"model"`
	SystemPrompt *string `json:"system_prompt"`
	ToolsConfig  *string `json:"tools_config"`
	Guardrails   *string `json:"guardrails"`
	Schedule     *string `json:"schedule"`
	Trigger      *string `json:"trigger"`
	OutputConfig *string `json:"output_config"`
	Routing      *string `json:"routing"`
}

// DeployAgentRequest represents a request to deploy an agent to a satellite
type DeployAgentRequest struct {
	SatelliteID string            `json:"satellite_id"`
	SessionID   *string           `json:"session_id"`
	Config      map[string]string `json:"config"`
	Secrets     map[string]string `json:"secrets"`
}

// ============================================================================
// Response Types
// ============================================================================

// AgentResponse represents an agent definition in API responses
type AgentResponse struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	DisplayName  string    `json:"display_name"`
	Description  *string   `json:"description"`
	Version      string    `json:"version"`
	Type         string    `json:"type"`
	Category     string    `json:"category"`
	Icon         *string   `json:"icon"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	SystemPrompt string    `json:"system_prompt"`
	ToolsConfig  string    `json:"tools_config"`
	Guardrails   *string   `json:"guardrails"`
	Schedule     *string   `json:"schedule"`
	Trigger      *string   `json:"trigger"`
	OutputConfig *string   `json:"output_config"`
	Routing      *string   `json:"routing"`
	IsBuiltin    bool      `json:"is_builtin"`
	IsEnterprise bool      `json:"is_enterprise"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// DeployAgentResponse represents the response after deploying an agent
type DeployAgentResponse struct {
	RunID          uuid.UUID `json:"run_id"`
	AgentID        uuid.UUID `json:"agent_id"`
	SatelliteID    uuid.UUID `json:"satellite_id"`
	SessionID      *string   `json:"session_id,omitempty"`
	Status         string    `json:"status"`
	Dispatched     bool      `json:"dispatched"`
	DispatchScore  float64   `json:"dispatch_score,omitempty"`
}

// databaseAgentToResponse converts a database.AgentDefinition to an AgentResponse
func databaseAgentToResponse(agent *database.AgentDefinition) *AgentResponse {
	resp := &AgentResponse{
		ID:           agent.ID,
		Name:         agent.Name,
		DisplayName:  agent.DisplayName,
		Description:  agent.Description,
		Version:      agent.Version,
		Type:         agent.Type,
		Category:     agent.Category,
		Icon:         agent.Icon,
		Provider:     agent.Provider,
		Model:        agent.Model,
		SystemPrompt: agent.SystemPrompt,
		ToolsConfig:  agent.ToolsConfig,
		Guardrails:   agent.Guardrails,
		Schedule:     agent.Schedule,
		Trigger:      agent.Trigger,
		OutputConfig: agent.OutputConfig,
		Routing:      agent.Routing,
		IsBuiltin:    agent.IsBuiltin,
		IsEnterprise: agent.IsEnterprise,
		CreatedAt:    agent.CreatedAt,
		UpdatedAt:    agent.UpdatedAt,
	}

	return resp
}

// ============================================================================
// Handler Implementations
// ============================================================================

// HandleListAgents handles GET /api/v1/agents - list all agents with pagination and filtering
func (h *AgentHandler) HandleListAgents(w http.ResponseWriter, r *http.Request) {
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	// Parse pagination parameters
	params := ParsePaginationParams(r)

	// Parse filter parameters
	filter := &database.AgentFilter{}

	if filterType := r.URL.Query().Get("type"); filterType != "" {
		filter.Type = &filterType
	}
	if filterCategory := r.URL.Query().Get("category"); filterCategory != "" {
		filter.Category = &filterCategory
	}
	if filterBuiltin := r.URL.Query().Get("is_builtin"); filterBuiltin != "" {
		isBuiltin := filterBuiltin == "true"
		filter.IsBuiltin = &isBuiltin
	}
	if filterEnterprise := r.URL.Query().Get("is_enterprise"); filterEnterprise != "" {
		isEnterprise := filterEnterprise == "true"
		filter.IsEnterprise = &isEnterprise
	}

	// Fetch agents with filters
	agents, err := database.ListAgentDefinitions(r.Context(), h.dbPool, filter)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleListAgents: failed to list agents: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}

	if agents == nil {
		agents = []database.AgentDefinition{}
	}

	// Apply pagination
	var paginatedAgents []database.AgentDefinition
	total := len(agents)
	if params.Cursor != "" {
		// Cursor-based pagination: find cursor position and get remaining items
		cursorIdx := -1
		for i, agent := range agents {
			if agent.ID.String() == params.Cursor {
				cursorIdx = i
				break
			}
		}
		if cursorIdx >= 0 && cursorIdx < len(agents) {
			endIdx := cursorIdx + params.Limit
			if endIdx > len(agents) {
				endIdx = len(agents)
			}
			paginatedAgents = agents[cursorIdx+1 : endIdx]
		} else {
			paginatedAgents = []database.AgentDefinition{}
		}
	} else {
		// Offset-based pagination
		endIdx := params.Limit
		if endIdx > len(agents) {
			endIdx = len(agents)
		}
		paginatedAgents = agents[:endIdx]
	}

	// Convert to response format
	agentResponses := make([]*AgentResponse, len(paginatedAgents))
	for i, agent := range paginatedAgents {
		agentResponses[i] = databaseAgentToResponse(&agent)
	}

	// Determine if there are more results
	hasMore := len(agentResponses) >= params.Limit

	// Build response
	var nextCursor *string
	if hasMore && len(agentResponses) > 0 {
		lastID := agentResponses[len(agentResponses)-1].ID.String()
		nextCursor = &lastID
	}

	response := &PaginatedResponse{
		Items:      agentResponses,
		Count:      len(agentResponses),
		Total:      total,
		NextCursor: nextCursor,
	}

	writeJSON(w, http.StatusOK, response)
}

// HandleGetAgent handles GET /api/v1/agents/:id - get a single agent by ID
func (h *AgentHandler) HandleGetAgent(w http.ResponseWriter, r *http.Request, agentID string) {
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	id, err := uuid.Parse(agentID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	agent, err := database.GetAgentDefinition(r.Context(), h.dbPool, id)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleGetAgent: failed to get agent: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to get agent")
		return
	}

	if agent == nil {
		writeJSONError(w, http.StatusNotFound, "agent not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"agent": databaseAgentToResponse(agent)})
}

// HandleCreateAgent handles POST /api/v1/agents - create a new agent
func (h *AgentHandler) HandleCreateAgent(w http.ResponseWriter, r *http.Request) {
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	var req CreateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON request body")
		return
	}

	// Validate required fields
	if req.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.DisplayName == "" {
		writeJSONError(w, http.StatusBadRequest, "display_name is required")
		return
	}
	if req.Type == "" {
		writeJSONError(w, http.StatusBadRequest, "type is required")
		return
	}
	if req.Category == "" {
		writeJSONError(w, http.StatusBadRequest, "category is required")
		return
	}
	if req.Provider == "" {
		writeJSONError(w, http.StatusBadRequest, "provider is required")
		return
	}
	if req.Model == "" {
		writeJSONError(w, http.StatusBadRequest, "model is required")
		return
	}

	// Validate type
	if req.Type != "specialist" && req.Type != "autonomous" {
		writeJSONError(w, http.StatusBadRequest, "type must be 'specialist' or 'autonomous'")
		return
	}

	// Set defaults for optional fields
	if req.Version == "" {
		req.Version = "1.0.0"
	}

	// Convert tools_config from raw JSON to string for storage
	toolsConfigStr := "{}"
	if len(req.ToolsConfig) > 0 {
		toolsConfigStr = string(req.ToolsConfig)
	}

	// Convert guardrails from raw JSON to string pointer for storage
	var guardrailsPtr *string
	if len(req.Guardrails) > 0 && string(req.Guardrails) != "null" {
		s := string(req.Guardrails)
		guardrailsPtr = &s
	}

	// Create agent definition
	agent := &database.AgentDefinition{
		Name:         req.Name,
		DisplayName:  req.DisplayName,
		Description:  req.Description,
		Version:      req.Version,
		Type:         req.Type,
		Category:     req.Category,
		Icon:         req.Icon,
		Provider:     req.Provider,
		Model:        req.Model,
		SystemPrompt: req.SystemPrompt,
		ToolsConfig:  toolsConfigStr,
		Guardrails:   guardrailsPtr,
		Schedule:     req.Schedule,
		Trigger:      req.Trigger,
		OutputConfig: req.OutputConfig,
		Routing:      req.Routing,
		IsBuiltin:    false, // User-created agents are not builtin
		IsEnterprise: false,
	}

	created, err := database.CreateAgentDefinition(r.Context(), h.dbPool, agent)
	if err != nil {
		// Check for duplicate name constraint
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			writeJSONError(w, http.StatusConflict, "agent with this name already exists")
			return
		}
		slog.Error(fmt.Sprintf("HandleCreateAgent: failed to create agent: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to create agent")
		return
	}

	// Audit log the agent creation
	if h.auditLogger != nil {
		ctx := r.Context()
		ctx = audit.WithClientIP(ctx, audit.ClientIPFromRequest(r))
		h.auditLogger.Log(ctx, "agent.create", "agent", created.ID.String(), map[string]interface{}{
			"name":       req.Name,
			"agent_type": req.Type,
		})
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"agent": databaseAgentToResponse(created)})
}

// HandleUpdateAgent handles PUT /api/v1/agents/:id - update an existing agent
func (h *AgentHandler) HandleUpdateAgent(w http.ResponseWriter, r *http.Request, agentID string) {
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	id, err := uuid.Parse(agentID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	// Snapshot current version before update
	existingAgent, err := database.GetAgentDefinition(r.Context(), h.dbPool, id)
	if err == nil && existingAgent != nil {
		snapshot, _ := json.Marshal(existingAgent)
		database.CreateAgentVersion(r.Context(), h.dbPool, id, existingAgent.Version, snapshot, nil, nil)
	}

	// Check if agent exists
	existing, err := database.GetAgentDefinition(r.Context(), h.dbPool, id)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleUpdateAgent: failed to get agent: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to update agent")
		return
	}
	if existing == nil {
		writeJSONError(w, http.StatusNotFound, "agent not found")
		return
	}

	// Prevent updating builtin agents
	if existing.IsBuiltin {
		writeJSONError(w, http.StatusForbidden, "cannot update builtin agents")
		return
	}

	var req UpdateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON request body")
		return
	}

	// Build updates - only include non-nil fields
	updates := &database.AgentDefinition{}

	if req.Name != nil {
		if *req.Name == "" {
			writeJSONError(w, http.StatusBadRequest, "name cannot be empty")
			return
		}
		updates.Name = *req.Name
	} else {
		updates.Name = existing.Name
	}

	if req.DisplayName != nil {
		updates.DisplayName = *req.DisplayName
	} else {
		updates.DisplayName = existing.DisplayName
	}

	if req.Description != nil {
		updates.Description = req.Description
	} else {
		updates.Description = existing.Description
	}

	if req.Version != nil {
		updates.Version = *req.Version
	} else {
		updates.Version = existing.Version
	}

	if req.Type != nil {
		if *req.Type != "specialist" && *req.Type != "autonomous" {
			writeJSONError(w, http.StatusBadRequest, "type must be 'specialist' or 'autonomous'")
			return
		}
		updates.Type = *req.Type
	} else {
		updates.Type = existing.Type
	}

	if req.Category != nil {
		updates.Category = *req.Category
	} else {
		updates.Category = existing.Category
	}

	if req.Icon != nil {
		updates.Icon = req.Icon
	} else {
		updates.Icon = existing.Icon
	}

	if req.Provider != nil {
		updates.Provider = *req.Provider
	} else {
		updates.Provider = existing.Provider
	}

	if req.Model != nil {
		updates.Model = *req.Model
	} else {
		updates.Model = existing.Model
	}

	if req.SystemPrompt != nil {
		updates.SystemPrompt = *req.SystemPrompt
	} else {
		updates.SystemPrompt = existing.SystemPrompt
	}

	if req.ToolsConfig != nil {
		updates.ToolsConfig = *req.ToolsConfig
	} else {
		updates.ToolsConfig = existing.ToolsConfig
	}

	if req.Guardrails != nil {
		updates.Guardrails = req.Guardrails
	} else {
		updates.Guardrails = existing.Guardrails
	}

	if req.Schedule != nil {
		updates.Schedule = req.Schedule
	} else {
		updates.Schedule = existing.Schedule
	}

	if req.Trigger != nil {
		updates.Trigger = req.Trigger
	} else {
		updates.Trigger = existing.Trigger
	}

	if req.OutputConfig != nil {
		updates.OutputConfig = req.OutputConfig
	} else {
		updates.OutputConfig = existing.OutputConfig
	}

	if req.Routing != nil {
		updates.Routing = req.Routing
	} else {
		updates.Routing = existing.Routing
	}

	// Preserve immutable fields
	updates.IsBuiltin = existing.IsBuiltin
	updates.IsEnterprise = existing.IsEnterprise

	updated, err := database.UpdateAgentDefinition(r.Context(), h.dbPool, id, updates)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleUpdateAgent: failed to update agent: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to update agent")
		return
	}

	// Audit log the agent update
	if h.auditLogger != nil {
		ctx := r.Context()
		ctx = audit.WithClientIP(ctx, audit.ClientIPFromRequest(r))
		h.auditLogger.Log(ctx, "agent.update", "agent", agentID, map[string]interface{}{
			"agent_id": agentID,
		})
	}

	writeJSON(w, http.StatusOK, databaseAgentToResponse(updated))
}

// HandleDeleteAgent handles DELETE /api/v1/agents/:id - delete an agent
func (h *AgentHandler) HandleDeleteAgent(w http.ResponseWriter, r *http.Request, agentID string) {
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	id, err := uuid.Parse(agentID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	// Check if agent exists
	existing, err := database.GetAgentDefinition(r.Context(), h.dbPool, id)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleDeleteAgent: failed to get agent: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to delete agent")
		return
	}
	if existing == nil {
		writeJSONError(w, http.StatusNotFound, "agent not found")
		return
	}

	// Reject deletion of builtin agents
	if existing.IsBuiltin {
		writeJSONError(w, http.StatusForbidden, "cannot delete builtin agents")
		return
	}

	err = database.DeleteAgentDefinition(r.Context(), h.dbPool, id)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleDeleteAgent: failed to delete agent: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to delete agent")
		return
	}

	// Audit log the agent delete
	if h.auditLogger != nil {
		ctx := r.Context()
		ctx = audit.WithClientIP(ctx, audit.ClientIPFromRequest(r))
		h.auditLogger.Log(ctx, "agent.delete", "agent", agentID, map[string]interface{}{
			"agent_id": agentID,
		})
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// Local structs for parsing guardrails and tools config JSON
type guardrailsJSON struct {
	Hitl           bool `json:"hitl"`
	ReadOnly       bool `json:"read_only"`
	TimeoutMinutes int  `json:"timeout_minutes"`
	Timeout        int  `json:"timeout"` // seconds — fallback when timeout_minutes is absent
	MaxToolCalls   int  `json:"max_tool_calls"`
	MaxTurns       int  `json:"max_turns"`
}

type toolsConfigJSON struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

// HandleDeployAgent handles POST /api/v1/agents/:id/deploy - deploy an agent to a satellite
func (h *AgentHandler) HandleDeployAgent(w http.ResponseWriter, r *http.Request, agentID string) {
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	// Parse agent ID
	id, err := uuid.Parse(agentID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	// Get agent definition
	agent, err := database.GetAgentDefinition(r.Context(), h.dbPool, id)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleDeployAgent: failed to get agent: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to deploy agent")
		return
	}
	if agent == nil {
		writeJSONError(w, http.StatusNotFound, "agent not found")
		return
	}

	// Parse request
	var req DeployAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON request body")
		return
	}

	// Helper to parse routing JSON
	parseRouting := func(routingJSON string) *dispatch.DispatchOptions {
		if routingJSON == "" {
			return nil
		}
		var opts dispatch.DispatchOptions
		if err := json.Unmarshal([]byte(routingJSON), &opts); err != nil {
			return nil
		}
		return &opts
	}

	var satelliteID uuid.UUID
	var dispatched bool
	var dispatchScore float64

	// If satellite_id is empty, check for auto-dispatch routing
	if req.SatelliteID == "" {
		routingStr := ""
		if agent.Routing != nil {
			routingStr = *agent.Routing
		}
		routing := parseRouting(routingStr)
		if routing != nil && routing.Mode == "auto-dispatch" {
			// Check enterprise license
			if h.licenseMgr == nil || !h.licenseMgr.HasFeature(license.FeatureAgentRouting) {
				writeJSONError(w, http.StatusForbidden, "agent routing requires enterprise license")
				return
			}
			// Check dispatcher is available
			if h.dispatcher == nil {
				writeJSONError(w, http.StatusServiceUnavailable, "dispatcher not configured")
				return
			}
			result, err := h.dispatcher.Dispatch(r.Context(), dispatch.DispatchOptions{
				RequireTags: routing.RequireTags,
				PreferTags:  routing.PreferTags,
			})
			if err != nil {
				writeJSONError(w, http.StatusConflict, err.Error())
				return
			}
			satelliteID = result.SatelliteID
			dispatched = true
			dispatchScore = result.Score
		} else {
			writeJSONError(w, http.StatusBadRequest, "satellite_id is required")
			return
		}
	} else {
		satelliteID, err = uuid.Parse(req.SatelliteID)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid satellite_id")
			return
		}
	}

	// Verify satellite exists and is active
	var satStatus string
	err = h.dbPool.QueryRow(r.Context(),
		`SELECT status FROM satellites WHERE id = $1`, satelliteID,
	).Scan(&satStatus)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "satellite not found")
		return
	}
	if satStatus != "active" {
		writeJSONError(w, http.StatusConflict, "satellite is not active")
		return
	}

	// Parse optional session ID
	var sessionID *uuid.UUID
	if req.SessionID != nil && *req.SessionID != "" {
		sid, err := uuid.Parse(*req.SessionID)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid session_id")
			return
		}
		sessionID = &sid
	} else {
		// Create a new session row in the database for this agent deployment.
		// The agent_runs.session_id has a FK constraint to sessions(id),
		// so we must insert a real session first.
		newSessionID := uuid.New()
		now := time.Now().UTC()
		devUserID := uuid.MustParse("00000000-0000-0000-0000-000000000000")
		sessionName := "agent-" + agent.Name + "-" + newSessionID.String()[:8]
		_, err := h.dbPool.Exec(r.Context(),
			`INSERT INTO sessions (id, satellite_id, user_id, name, agent_binary, agent_args, state, cols, rows, last_activity_at, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, 'PROVISIONING', 80, 24, $7, $7)`,
			newSessionID, satelliteID, devUserID, sessionName, agent.Name, []string{}, now,
		)
		if err != nil {
			slog.Error(fmt.Sprintf("HandleDeployAgent: failed to create session: %v", err), "component", "api")
			writeJSONError(w, http.StatusInternalServerError, "failed to create session for agent deployment")
			return
		}
		sessionID = &newSessionID
	}

	// Create agent run record
	agentRun, err := database.CreateAgentRun(r.Context(), h.dbPool, id, satelliteID, sessionID, "manual")
	if err != nil {
		slog.Error(fmt.Sprintf("HandleDeployAgent: failed to create agent run: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to deploy agent")
		return
	}

	// Build agent definition proto
	toolsConfig := make(map[string]string)
	if agent.ToolsConfig != "" {
		json.Unmarshal([]byte(agent.ToolsConfig), &toolsConfig)
	}
	if req.Config != nil {
		for k, v := range req.Config {
			toolsConfig[k] = v
		}
	}

	// Inject agent-level fields so the PiBridge can pass them as CLI flags
	if agent.Provider != "" {
		toolsConfig["provider"] = agent.Provider
	}
	if agent.Model != "" {
		toolsConfig["model"] = agent.Model
	}
	if agent.SystemPrompt != "" {
		toolsConfig["system_prompt"] = agent.SystemPrompt
	}

	// Parse guardrails from agent.Guardrails and inject into config
	if agent.Guardrails != nil && *agent.Guardrails != "" && *agent.Guardrails != "{}" {
		var gr guardrailsJSON
		if err := json.Unmarshal([]byte(*agent.Guardrails), &gr); err == nil {
			// Only inject non-zero values
			if gr.ReadOnly {
				toolsConfig["guardrails.read_only"] = strconv.FormatBool(gr.ReadOnly)
			}
			if gr.TimeoutMinutes > 0 {
				toolsConfig["guardrails.timeout_minutes"] = strconv.Itoa(gr.TimeoutMinutes)
			} else if gr.Timeout > 0 {
				minutes := gr.Timeout / 60
				if minutes < 1 {
					minutes = 1
				}
				toolsConfig["guardrails.timeout_minutes"] = strconv.Itoa(minutes)
			}
			if gr.MaxToolCalls > 0 {
				toolsConfig["guardrails.max_tool_calls"] = strconv.Itoa(gr.MaxToolCalls)
			}
			if gr.MaxTurns > 0 {
				toolsConfig["guardrails.max_turns"] = strconv.Itoa(gr.MaxTurns)
			}
			if gr.Hitl {
				toolsConfig["guardrails.hitl"] = strconv.FormatBool(gr.Hitl)
			}
		}
	}

	// Parse tools_config for allow/deny arrays and inject into config
	if agent.ToolsConfig != "" {
		var tc toolsConfigJSON
		if err := json.Unmarshal([]byte(agent.ToolsConfig), &tc); err == nil {
			if len(tc.Allow) > 0 {
				toolsConfig["tools.allow"] = strings.Join(tc.Allow, ",")
			}
			if len(tc.Deny) > 0 {
				toolsConfig["tools.deny"] = strings.Join(tc.Deny, ",")
			}
		}
	}

	agentDefProto := &proto.AgentDefinitionProto{
		Name:         agent.Name,
		Version:      agent.Version,
		Image:        agent.Name, // Use name as image/binary identifier
		Config:       toolsConfig,
		Capabilities: []string{agent.Type, agent.Category},
		Entrypoint:   agent.Name,
	}

	// Send deploy command to satellite via stream registry
	if h.streamRegistry != nil {
		deployCmd := &proto.NexusMessage{
			Payload: &proto.NexusMessage_DeployAgentCommand{
				DeployAgentCommand: &proto.DeployAgentCommand{
					SessionId:       sessionID.String(),
					AgentDefinition: agentDefProto,
					Secrets:         req.Secrets,
				},
			},
		}
		if h.streamRegistry.SendToSatellite(satelliteID.String(), deployCmd) {
			slog.Info(fmt.Sprintf("HandleDeployAgent: Dispatched DeployAgentCommand for agent %s to satellite %s", agentID, satelliteID), "component", "api")
		} else {
			slog.Warn(fmt.Sprintf("HandleDeployAgent: Warning: could not dispatch DeployAgentCommand to satellite %s (stream not found)", satelliteID), "component", "api")
		}
	}

	// Build response
	sessionIDStr := sessionID.String()
	response := &DeployAgentResponse{
		RunID:         agentRun.ID,
		AgentID:       id,
		SatelliteID:   satelliteID,
		SessionID:     &sessionIDStr,
		Status:        "running",
		Dispatched:    dispatched,
		DispatchScore: dispatchScore,
	}

	// Audit log the agent deployment
	if h.auditLogger != nil {
		ctx := r.Context()
		ctx = audit.WithClientIP(ctx, audit.ClientIPFromRequest(r))
		h.auditLogger.Log(ctx, "agent.deploy", "agent", agentID, map[string]interface{}{
			"agent_id":     agentID,
			"satellite_id": req.SatelliteID,
		})
	}

	writeJSON(w, http.StatusCreated, response)
}

// HandleListAgentRuns handles GET /api/v1/agents/:id/runs - list runs for a specific agent
func (h *AgentHandler) HandleListAgentRuns(w http.ResponseWriter, r *http.Request, agentID string) {
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	id, err := uuid.Parse(agentID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	runs, err := database.ListAgentRuns(r.Context(), h.dbPool, database.AgentRunFilters{AgentID: &id})
	if err != nil {
		slog.Error(fmt.Sprintf("HandleListAgentRuns: failed to list runs: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}

	// Convert to response format
	type RunResponse struct {
		ID            uuid.UUID  `json:"id"`
		AgentID       uuid.UUID  `json:"agent_id"`
		Status        string     `json:"status"`
		StartedAt     time.Time  `json:"started_at"`
		EndedAt       *time.Time `json:"ended_at,omitempty"`
		TotalTokens   int        `json:"total_tokens"`
		EstimatedCost float64    `json:"estimated_cost"`
		ToolCallCount int        `json:"tool_call_count"`
	}

	runResponses := make([]RunResponse, 0, len(runs))
	for _, run := range runs {
		runResponses = append(runResponses, RunResponse{
			ID:            run.ID,
			AgentID:       run.AgentID,
			Status:        run.Status,
			StartedAt:     run.StartedAt,
			EndedAt:       run.EndedAt,
			TotalTokens:   run.TotalTokens,
			EstimatedCost: run.EstimatedCost,
			ToolCallCount: run.ToolCallCount,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"runs": runResponses})
}

// RegisterAgentRoutes registers agent routes to the given mux.
// This function is called by the integration task to wire up the routes.
func RegisterAgentRoutes(mux *http.ServeMux, handler *AgentHandler) {
	// Agent routes - use catch-all handlers for /api/v1/agents and /api/v1/agents/
	mux.HandleFunc("/api/v1/agents", handler.handleAgentsAPI)
	mux.HandleFunc("/api/v1/agents/", handler.handleAgentsAPI)
	// Dispatch preview route
	mux.HandleFunc("/api/v1/dispatch/preview", handler.HandleDispatchPreview)
}

// handleAgentsAPI handles all agent API requests
func (h *AgentHandler) handleAgentsAPI(w http.ResponseWriter, r *http.Request) {
	// Check if database is configured
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	p := r.URL.Path
	method := r.Method

	// Route: GET /api/v1/agents - list agents
	if p == "/api/v1/agents" && method == "GET" {
		h.HandleListAgents(w, r)
		return
	}

	// Route: POST /api/v1/agents - create agent
	if p == "/api/v1/agents" && method == "POST" {
		h.HandleCreateAgent(w, r)
		return
	}

	// Route: GET /api/v1/agents/{id} - get agent
	if strings.HasPrefix(p, "/api/v1/agents/") && method == "GET" {
		agentID := strings.TrimPrefix(p, "/api/v1/agents/")
		// Check if this is a sub-path (like /history)
		if !strings.Contains(agentID, "/") {
			h.HandleGetAgent(w, r, agentID)
			return
		}
	}

	// Route: PUT /api/v1/agents/{id} - update agent
	if strings.HasPrefix(p, "/api/v1/agents/") && method == "PUT" {
		agentID := strings.TrimPrefix(p, "/api/v1/agents/")
		if !strings.Contains(agentID, "/") {
			h.HandleUpdateAgent(w, r, agentID)
			return
		}
	}

	// Route: DELETE /api/v1/agents/{id} - delete agent
	if strings.HasPrefix(p, "/api/v1/agents/") && method == "DELETE" {
		agentID := strings.TrimPrefix(p, "/api/v1/agents/")
		if !strings.Contains(agentID, "/") {
			h.HandleDeleteAgent(w, r, agentID)
			return
		}
	}

	// Route: POST /api/v1/agents/{id}/deploy - deploy agent
	if strings.HasPrefix(p, "/api/v1/agents/") && strings.HasSuffix(p, "/deploy") && method == "POST" {
		agentID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/agents/"), "/deploy")
		h.HandleDeployAgent(w, r, agentID)
		return
	}

	// Route: GET /api/v1/agents/{id}/runs - list agent runs
	if strings.HasPrefix(p, "/api/v1/agents/") && strings.HasSuffix(p, "/runs") && method == "GET" {
		agentID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/agents/"), "/runs")
		h.HandleListAgentRuns(w, r, agentID)
		return
	}

	// ---- Agent Version Routes (merged from agent_versions.go) ----

	// Route: GET /api/v1/agents/{id}/versions - list versions
	if strings.HasPrefix(p, "/api/v1/agents/") && strings.HasSuffix(p, "/versions") && method == "GET" {
		agentID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/agents/"), "/versions")
		h.HandleListAgentVersions(w, r, agentID)
		return
	}

	// Route: GET /api/v1/agents/{id}/versions/{version} - get single version
	if strings.HasPrefix(p, "/api/v1/agents/") && method == "GET" {
		parts := strings.Split(strings.TrimPrefix(p, "/api/v1/agents/"), "/")
		if len(parts) == 3 && parts[1] == "versions" {
			h.HandleGetAgentVersion(w, r, parts[0], parts[2])
			return
		}
	}

	// Route: POST /api/v1/agents/{id}/rollback - rollback agent
	if strings.HasPrefix(p, "/api/v1/agents/") && strings.HasSuffix(p, "/rollback") && method == "POST" {
		agentID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/agents/"), "/rollback")
		h.HandleRollbackAgent(w, r, agentID)
		return
	}

	// ---- Agent Import/Export Routes (merged from agent_import_export.go) ----

	// Route: GET /api/v1/agents/{id}/export - export agent as YAML
	if strings.HasPrefix(p, "/api/v1/agents/") && strings.HasSuffix(p, "/export") && method == "GET" {
		agentID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/agents/"), "/export")
		if agentID != "" && !strings.Contains(agentID, "/") {
			h.HandleExportAgent(w, r, agentID)
			return
		}
	}

	// Route: POST /api/v1/agents/import - import agent from YAML
	if p == "/api/v1/agents/import" && method == "POST" {
		h.HandleImportAgent(w, r)
		return
	}

	// 404 for unknown routes
	http.NotFound(w, r)
}

// HandleDispatchPreview handles GET /api/v1/dispatch/preview - returns ranked satellite list for given agent
func (h *AgentHandler) HandleDispatchPreview(w http.ResponseWriter, r *http.Request) {
	if h.dispatcher == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "dispatcher not configured")
		return
	}

	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		writeJSONError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	id, err := uuid.Parse(agentID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	// Get agent definition to get routing config
	agent, err := database.GetAgentDefinition(r.Context(), h.dbPool, id)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleDispatchPreview: failed to get agent: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to get agent")
		return
	}
	if agent == nil {
		writeJSONError(w, http.StatusNotFound, "agent not found")
		return
	}

	// Parse routing JSON
	var opts dispatch.DispatchOptions
	if agent.Routing != nil && *agent.Routing != "" {
		if err := json.Unmarshal([]byte(*agent.Routing), &opts); err != nil {
			slog.Error(fmt.Sprintf("HandleDispatchPreview: failed to parse routing: %v", err), "component", "api")
			writeJSONError(w, http.StatusBadRequest, "invalid routing configuration")
			return
		}
	}

	// Get preview results
	results, err := h.dispatcher.Preview(r.Context(), opts)
	if err != nil {
		writeJSONError(w, http.StatusConflict, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"candidates": results,
		"count":       len(results),
	})
}
