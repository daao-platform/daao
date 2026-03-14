// Package api provides HTTP handlers for the Nexus API Gateway.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/daao/nexus/internal/audit"
	"github.com/daao/nexus/internal/database"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// ============================================================================
// YAML Types - Portable format for agent import/export
// ============================================================================

// AgentYAML represents the portable YAML format for agent definitions.
// Matches PRODUCT_SPEC §11 schema.
type AgentYAML struct {
	Name         string                 `yaml:"name"`
	DisplayName  string                 `yaml:"display_name"`
	Description  string                 `yaml:"description,omitempty"`
	Version      string                 `yaml:"version"`
	Type         string                 `yaml:"type"`
	Category     string                 `yaml:"category"`
	Icon         string                 `yaml:"icon,omitempty"`
	Provider     string                 `yaml:"provider"`
	Model        string                 `yaml:"model"`
	SystemPrompt string                 `yaml:"system_prompt"`
	Tools        *AgentYAMLTools        `yaml:"tools,omitempty"`
	Guardrails   *AgentYAMLGuardrails  `yaml:"guardrails,omitempty"`
	Output       map[string]interface{} `yaml:"output,omitempty"`
	Schedule     interface{}           `yaml:"schedule,omitempty"`
	Trigger      interface{}           `yaml:"trigger,omitempty"`
}

// AgentYAMLTools represents tool configuration in YAML format
type AgentYAMLTools struct {
	Allow []string `yaml:"allow,omitempty"`
	Deny  []string `yaml:"deny,omitempty"`
}

// AgentYAMLGuardrails represents guardrails configuration in YAML format
type AgentYAMLGuardrails struct {
	HITL           bool `yaml:"hitl"`
	ReadOnly       bool `yaml:"read_only"`
	TimeoutMinutes int  `yaml:"timeout_minutes,omitempty"`
	MaxToolCalls   int  `yaml:"max_tool_calls,omitempty"`
}

// ============================================================================
// Handler Implementations
// ============================================================================

// HandleExportAgent handles GET /api/v1/agents/{id}/export - export an agent as YAML
func (h *AgentHandler) HandleExportAgent(w http.ResponseWriter, r *http.Request, agentID string) {
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
		slog.Error(fmt.Sprintf("HandleExportAgent: failed to get agent: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to get agent")
		return
	}

	if agent == nil {
		writeJSONError(w, http.StatusNotFound, "agent not found")
		return
	}

	// Convert to AgentYAML (exclude: id, is_builtin, is_enterprise, created_at, updated_at)
	agentYAML := convertAgentToYAML(agent)

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(agentYAML)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleExportAgent: failed to marshal YAML: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to export agent")
		return
	}

	// Set headers
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.agent.yaml\"", agent.Name))

	// Write YAML bytes
	_, err = w.Write(yamlBytes)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleExportAgent: failed to write response: %v", err), "component", "api")
	}

	// Audit log the agent export
	if h.auditLogger != nil {
		ctx := r.Context()
		ctx = audit.WithClientIP(ctx, audit.ClientIPFromRequest(r))
		h.auditLogger.Log(ctx, "agent.export", "agent", agentID, map[string]interface{}{
			"agent_id": agentID,
			"name":     agent.Name,
		})
	}
}

// HandleImportAgent handles POST /api/v1/agents/import - import an agent from YAML
func (h *AgentHandler) HandleImportAgent(w http.ResponseWriter, r *http.Request) {
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	// Parse multipart form data (1MB max)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to parse multipart form")
		return
	}

	// Get file from form
	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "no file provided in form field 'file'")
		return
	}
	defer file.Close()

	// Read file content
	fileContent, err := io.ReadAll(file)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to read file")
		return
	}

	// Unmarshal YAML
	var agentYAML AgentYAML
	if err := yaml.Unmarshal(fileContent, &agentYAML); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid YAML: %v", err))
		return
	}

	// Validate required fields
	if agentYAML.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "name is required")
		return
	}
	if agentYAML.DisplayName == "" {
		writeJSONError(w, http.StatusBadRequest, "display_name is required")
		return
	}
	if agentYAML.Provider == "" {
		writeJSONError(w, http.StatusBadRequest, "provider is required")
		return
	}
	if agentYAML.Model == "" {
		writeJSONError(w, http.StatusBadRequest, "model is required")
		return
	}
	if agentYAML.SystemPrompt == "" {
		writeJSONError(w, http.StatusBadRequest, "system_prompt is required")
		return
	}

	// Check for name conflict
	existing, err := database.ListAgentsByName(r.Context(), h.dbPool, agentYAML.Name)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleImportAgent: failed to check name conflict: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to import agent")
		return
	}
	if len(existing) > 0 {
		writeJSONError(w, http.StatusConflict, "agent with this name already exists")
		return
	}

	// Convert AgentYAML to database.AgentDefinition
	agent := convertYAMLToAgent(&agentYAML)

	// Create the agent
	created, err := database.CreateAgentDefinition(r.Context(), h.dbPool, agent)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleImportAgent: failed to create agent: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to import agent")
		return
	}

	// Audit log the agent import
	if h.auditLogger != nil {
		ctx := r.Context()
		ctx = audit.WithClientIP(ctx, audit.ClientIPFromRequest(r))
		h.auditLogger.Log(ctx, "agent.import", "agent", created.ID.String(), map[string]interface{}{
			"name":       created.Name,
			"agent_type": created.Type,
		})
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"agent": databaseAgentToResponse(created)})
}

// ============================================================================
// Conversion Helpers
// ============================================================================

// convertAgentToYAML converts a database.AgentDefinition to an AgentYAML struct
func convertAgentToYAML(agent *database.AgentDefinition) *AgentYAML {
	yaml := &AgentYAML{
		Name:         agent.Name,
		DisplayName:  agent.DisplayName,
		Version:      agent.Version,
		Type:         agent.Type,
		Category:     agent.Category,
		Provider:     agent.Provider,
		Model:        agent.Model,
		SystemPrompt: agent.SystemPrompt,
	}

	// Optional fields
	if agent.Description != nil && *agent.Description != "" {
		yaml.Description = *agent.Description
	}
	if agent.Icon != nil && *agent.Icon != "" {
		yaml.Icon = *agent.Icon
	}

	// Parse tools_config JSON to YAML structure
	if agent.ToolsConfig != "" && agent.ToolsConfig != "{}" {
		var toolsConfig map[string]interface{}
		if err := json.Unmarshal([]byte(agent.ToolsConfig), &toolsConfig); err == nil {
			var tools AgentYAMLTools
			if allow, ok := toolsConfig["allow"].([]interface{}); ok {
				tools.Allow = sliceIfaceToStrings(allow)
			}
			if deny, ok := toolsConfig["deny"].([]interface{}); ok {
				tools.Deny = sliceIfaceToStrings(deny)
			}
			if len(tools.Allow) > 0 || len(tools.Deny) > 0 {
				yaml.Tools = &tools
			}
		}
	}

	// Parse guardrails JSON to YAML structure
	if agent.Guardrails != nil && *agent.Guardrails != "" && *agent.Guardrails != "{}" {
		var guardrailsJSON map[string]interface{}
		if err := json.Unmarshal([]byte(*agent.Guardrails), &guardrailsJSON); err == nil {
			var guardrails AgentYAMLGuardrails
			if v, ok := guardrailsJSON["hitl"].(bool); ok {
				guardrails.HITL = v
			}
			if v, ok := guardrailsJSON["read_only"].(bool); ok {
				guardrails.ReadOnly = v
			}
			if v, ok := guardrailsJSON["timeout_minutes"].(float64); ok {
				guardrails.TimeoutMinutes = int(v)
			}
			if v, ok := guardrailsJSON["max_tool_calls"].(float64); ok {
				guardrails.MaxToolCalls = int(v)
			}
			yaml.Guardrails = &guardrails
		}
	}

	// Parse output_config JSON if present
	if agent.OutputConfig != nil && *agent.OutputConfig != "" && *agent.OutputConfig != "{}" {
		var outputConfig map[string]interface{}
		if err := json.Unmarshal([]byte(*agent.OutputConfig), &outputConfig); err == nil {
			yaml.Output = outputConfig
		}
	}

	// Include schedule and trigger if set
	if agent.Schedule != nil && *agent.Schedule != "" && *agent.Schedule != "null" {
		yaml.Schedule = *agent.Schedule
	}
	if agent.Trigger != nil && *agent.Trigger != "" && *agent.Trigger != "null" {
		yaml.Trigger = *agent.Trigger
	}

	return yaml
}

// convertYAMLToAgent converts an AgentYAML struct to a database.AgentDefinition
func convertYAMLToAgent(yaml *AgentYAML) *database.AgentDefinition {
	agent := &database.AgentDefinition{
		Name:         yaml.Name,
		DisplayName:  yaml.DisplayName,
		Version:      yaml.Version,
		Type:         yaml.Type,
		Category:     yaml.Category,
		Provider:     yaml.Provider,
		Model:        yaml.Model,
		SystemPrompt: yaml.SystemPrompt,
		IsBuiltin:    false,
		IsEnterprise: false,
	}

	// Optional string fields
	if yaml.Description != "" {
		agent.Description = &yaml.Description
	}
	if yaml.Icon != "" {
		agent.Icon = &yaml.Icon
	}

	// Convert tools to JSON string
	if yaml.Tools != nil {
		toolsJSON := map[string]interface{}{}
		if len(yaml.Tools.Allow) > 0 {
			toolsJSON["allow"] = yaml.Tools.Allow
		}
		if len(yaml.Tools.Deny) > 0 {
			toolsJSON["deny"] = yaml.Tools.Deny
		}
		if len(toolsJSON) > 0 {
			toolsBytes, _ := json.Marshal(toolsJSON)
			agent.ToolsConfig = string(toolsBytes)
		}
	}
	if agent.ToolsConfig == "" {
		agent.ToolsConfig = "{}"
	}

	// Convert guardrails to JSON string
	if yaml.Guardrails != nil {
		guardrailsJSON := map[string]interface{}{
			"hitl":            yaml.Guardrails.HITL,
			"read_only":       yaml.Guardrails.ReadOnly,
			"timeout_minutes": yaml.Guardrails.TimeoutMinutes,
			"max_tool_calls":  yaml.Guardrails.MaxToolCalls,
		}
		guardrailsBytes, _ := json.Marshal(guardrailsJSON)
		guardrailsStr := string(guardrailsBytes)
		agent.Guardrails = &guardrailsStr
	}

	// Convert output to JSON string
	if yaml.Output != nil {
		outputBytes, _ := json.Marshal(yaml.Output)
		outputStr := string(outputBytes)
		agent.OutputConfig = &outputStr
	}

	// Handle schedule - convert interface{} to *string
	if yaml.Schedule != nil {
		switch v := yaml.Schedule.(type) {
		case string:
			if v != "" {
				agent.Schedule = &v
			}
		case map[string]interface{}:
			scheduleBytes, _ := json.Marshal(v)
			scheduleStr := string(scheduleBytes)
			agent.Schedule = &scheduleStr
		}
	}

	// Handle trigger - convert interface{} to *string
	if yaml.Trigger != nil {
		switch v := yaml.Trigger.(type) {
		case string:
			if v != "" {
				agent.Trigger = &v
			}
		case map[string]interface{}:
			triggerBytes, _ := json.Marshal(v)
			triggerStr := string(triggerBytes)
			agent.Trigger = &triggerStr
		}
	}

	return agent
}

// sliceIfaceToStrings converts []interface{} to []string
func sliceIfaceToStrings(slice []interface{}) []string {
	result := make([]string, len(slice))
	for i, v := range slice {
		if s, ok := v.(string); ok {
			result[i] = s
		}
	}
	return result
}

// RegisterAgentImportExportRoutes registers the import/export routes to the given mux.
func RegisterAgentImportExportRoutes(mux *http.ServeMux, handler *AgentHandler) {
	// Export route: GET /api/v1/agents/{id}/export
	mux.HandleFunc("/api/v1/agents/", handler.handleAgentExportImport)
}

// handleAgentExportImport handles agent export and import routes
func (h *AgentHandler) handleAgentExportImport(w http.ResponseWriter, r *http.Request) {
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	p := r.URL.Path
	method := r.Method

	// Route: GET /api/v1/agents/{id}/export - export agent as YAML
	if strings.HasSuffix(p, "/export") && method == "GET" {
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

	// Not found for other paths
	http.NotFound(w, r)
}
