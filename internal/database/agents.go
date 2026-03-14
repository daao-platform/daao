// Package database provides PostgreSQL connection pool management and query functions.
package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AgentDefinition represents an agent definition stored in the database.
type AgentDefinition struct {
	ID           uuid.UUID
	Name         string
	DisplayName  string
	Description  *string
	Version      string
	Type         string
	Category     string
	Icon         *string
	Provider     string
	Model        string
	SystemPrompt string
	ToolsConfig  string
	Guardrails   *string
	Schedule     *string
	Trigger      *string
	OutputConfig *string
	Routing      *string
	IsBuiltin    bool
	IsEnterprise bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// AgentFilter contains optional filters for listing agent definitions.
type AgentFilter struct {
	Type         *string // Filter by type: 'specialist' or 'autonomous'
	Category     *string // Filter by category
	IsBuiltin    *bool   // Filter by builtin status
	IsEnterprise *bool   // Filter by enterprise status
}

// CreateAgentDefinition inserts a new agent definition into the database.
// Returns the created AgentDefinition or an error.
func CreateAgentDefinition(ctx context.Context, pool *pgxpool.Pool, agent *AgentDefinition) (*AgentDefinition, error) {
	query := `
		INSERT INTO agent_definitions (
			name, display_name, description, version, type, category, icon,
			provider, model, system_prompt, tools_config, guardrails, schedule, trigger,
			output_config, routing, is_builtin, is_enterprise
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		RETURNING id, name, display_name, description, version, type, category, icon,
			provider, model, system_prompt, tools_config, guardrails, schedule, trigger,
			output_config, routing, is_builtin, is_enterprise, created_at, updated_at
	`

	var a AgentDefinition
	err := pool.QueryRow(ctx, query,
		agent.Name, agent.DisplayName, agent.Description, agent.Version, agent.Type,
		agent.Category, agent.Icon, agent.Provider, agent.Model, agent.SystemPrompt,
		agent.ToolsConfig, agent.Guardrails, agent.Schedule, agent.Trigger, agent.OutputConfig,
		agent.Routing, agent.IsBuiltin, agent.IsEnterprise,
	).Scan(
		&a.ID, &a.Name, &a.DisplayName, &a.Description, &a.Version, &a.Type, &a.Category,
		&a.Icon, &a.Provider, &a.Model, &a.SystemPrompt, &a.ToolsConfig, &a.Guardrails,
		&a.Schedule, &a.Trigger, &a.OutputConfig, &a.Routing, &a.IsBuiltin, &a.IsEnterprise, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// GetAgentDefinition retrieves an agent definition by its ID.
// Returns the AgentDefinition or nil if not found, or an error.
func GetAgentDefinition(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*AgentDefinition, error) {
	query := `
		SELECT id, name, display_name, description, version, type, category, icon,
			provider, model, system_prompt, tools_config, guardrails, schedule, trigger,
			output_config, routing, is_builtin, is_enterprise, created_at, updated_at
		FROM agent_definitions
		WHERE id = $1
	`

	var a AgentDefinition
	err := pool.QueryRow(ctx, query, id).Scan(
		&a.ID, &a.Name, &a.DisplayName, &a.Description, &a.Version, &a.Type, &a.Category,
		&a.Icon, &a.Provider, &a.Model, &a.SystemPrompt, &a.ToolsConfig, &a.Guardrails,
		&a.Schedule, &a.Trigger, &a.OutputConfig, &a.Routing, &a.IsBuiltin, &a.IsEnterprise, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

// ListAgentDefinitions retrieves agent definitions with optional filters.
// Returns a slice of AgentDefinition or an error.
func ListAgentDefinitions(ctx context.Context, pool *pgxpool.Pool, filter *AgentFilter) ([]AgentDefinition, error) {
	// Build dynamic query with filters
	baseQuery := `
		SELECT id, name, display_name, description, version, type, category, icon,
			provider, model, system_prompt, tools_config, guardrails, schedule, trigger,
			output_config, routing, is_builtin, is_enterprise, created_at, updated_at
		FROM agent_definitions
		WHERE 1=1
	`

	args := []interface{}{}
	argIndex := 1

	if filter != nil {
		if filter.Type != nil {
			baseQuery += fmt.Sprintf(" AND type = $%d", argIndex)
			args = append(args, *filter.Type)
			argIndex++
		}
		if filter.Category != nil {
			baseQuery += fmt.Sprintf(" AND category = $%d", argIndex)
			args = append(args, *filter.Category)
			argIndex++
		}
		if filter.IsBuiltin != nil {
			baseQuery += fmt.Sprintf(" AND is_builtin = $%d", argIndex)
			args = append(args, *filter.IsBuiltin)
			argIndex++
		}
		if filter.IsEnterprise != nil {
			baseQuery += fmt.Sprintf(" AND is_enterprise = $%d", argIndex)
			args = append(args, *filter.IsEnterprise)
			argIndex++
		}
	}

	baseQuery += " ORDER BY display_name"

	rows, err := pool.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []AgentDefinition
	for rows.Next() {
		var a AgentDefinition
		err := rows.Scan(
			&a.ID, &a.Name, &a.DisplayName, &a.Description, &a.Version, &a.Type, &a.Category,
			&a.Icon, &a.Provider, &a.Model, &a.SystemPrompt, &a.ToolsConfig, &a.Guardrails,
			&a.Schedule, &a.Trigger, &a.OutputConfig, &a.Routing, &a.IsBuiltin, &a.IsEnterprise, &a.CreatedAt, &a.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}

	return agents, rows.Err()
}

// UpdateAgentDefinition updates an agent definition's fields.
// Returns the updated AgentDefinition or an error.
func UpdateAgentDefinition(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, updates *AgentDefinition) (*AgentDefinition, error) {
	// First check if the agent exists
	exists, err := GetAgentDefinition(ctx, pool, id)
	if err != nil {
		return nil, err
	}
	if exists == nil {
		return nil, nil
	}

	query := `
		UPDATE agent_definitions
		SET name = $1, display_name = $2, description = $3, version = $4, type = $5,
			category = $6, icon = $7, provider = $8, model = $9, system_prompt = $10,
			tools_config = $11, guardrails = $12, schedule = $13, trigger = $14, output_config = $15,
			routing = $16, is_builtin = $17, is_enterprise = $18, updated_at = NOW()
		WHERE id = $19
		RETURNING id, name, display_name, description, version, type, category, icon,
			provider, model, system_prompt, tools_config, guardrails, schedule, trigger,
			output_config, routing, is_builtin, is_enterprise, created_at, updated_at
	`

	var a AgentDefinition
	err = pool.QueryRow(ctx, query,
		updates.Name, updates.DisplayName, updates.Description, updates.Version, updates.Type,
		updates.Category, updates.Icon, updates.Provider, updates.Model, updates.SystemPrompt,
		updates.ToolsConfig, updates.Guardrails, updates.Schedule, updates.Trigger, updates.OutputConfig,
		updates.Routing, updates.IsBuiltin, updates.IsEnterprise, id,
	).Scan(
		&a.ID, &a.Name, &a.DisplayName, &a.Description, &a.Version, &a.Type, &a.Category,
		&a.Icon, &a.Provider, &a.Model, &a.SystemPrompt, &a.ToolsConfig, &a.Guardrails,
		&a.Schedule, &a.Trigger, &a.OutputConfig, &a.Routing, &a.IsBuiltin, &a.IsEnterprise, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// DeleteAgentDefinition deletes an agent definition by its ID.
// Returns an error if the deletion fails.
func DeleteAgentDefinition(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	_, err := pool.Exec(ctx, "DELETE FROM agent_definitions WHERE id = $1", id)
	return err
}

// SeedBuiltinAgents creates the built-in Core Pack agents if they don't already exist.
// This seeds 3 agents from PRD §5.2: Log Analyzer, Security Scanner, Virtual Sysadmin.
// Returns an error if seeding fails.
func SeedBuiltinAgents(ctx context.Context, pool *pgxpool.Pool) error {
	// Define the 3 Core Pack agents from PRD §5.2
	builtinAgents := []struct {
		Name         string
		DisplayName  string
		Description  string
		Type         string
		Category     string
		Provider     string
		Model        string
		SystemPrompt string
	}{
		{
			Name:         "log-analyzer",
			DisplayName:  "Log Analyzer",
			Description:  "Analyzes system and application logs to identify errors, anomalies, and security incidents. Provides insights into system behavior and helps troubleshoot issues.",
			Type:         "specialist",
			Category:     "operations",
			Provider:     "openai",
			Model:        "gpt-4o",
			SystemPrompt: "You are a Log Analyzer agent specialized in parsing and analyzing system logs. You identify error patterns, security incidents, and performance issues.",
		},
		{
			Name:         "security-scanner",
			DisplayName:  "Security Scanner",
			Description:  "Scans systems and configurations for security vulnerabilities and compliance issues. Provides recommendations for hardening and remediation.",
			Type:         "specialist",
			Category:     "security",
			Provider:     "openai",
			Model:        "gpt-4o",
			SystemPrompt: "You are a Security Scanner agent specialized in identifying security vulnerabilities and compliance issues. You provide actionable recommendations for remediation.",
		},
		{
			Name:         "virtual-sysadmin",
			DisplayName:  "Virtual Sysadmin",
			Description:  "A comprehensive system administration assistant that helps with server management, configuration, monitoring, and automation tasks.",
			Type:         "autonomous",
			Category:     "operations",
			Provider:     "openai",
			Model:        "gpt-4o",
			SystemPrompt: "You are a Virtual Sysadmin agent that helps manage servers and infrastructure. You can execute system administration tasks, monitor health, and automate operations.",
		},
	}

	for _, agent := range builtinAgents {
		// Check if agent already exists by name
		existing, err := ListAgentsByName(ctx, pool, agent.Name)
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			continue // Agent already exists, skip
		}

		// Create the agent definition
		agentDef := &AgentDefinition{
			Name:         agent.Name,
			DisplayName:  agent.DisplayName,
			Description:  &agent.Description,
			Version:      "1.0.0",
			Type:         agent.Type,
			Category:     agent.Category,
			Provider:     agent.Provider,
			Model:        agent.Model,
			SystemPrompt: agent.SystemPrompt,
			ToolsConfig:  "{}",
			Guardrails:   nil,
			Schedule:     nil,
			Trigger:      nil,
			OutputConfig: nil,
			Routing:      nil,
			IsBuiltin:    true,
			IsEnterprise: false,
		}

		_, err = CreateAgentDefinition(ctx, pool, agentDef)
		if err != nil {
			return err
		}
	}

	return nil
}

// ListAgentsByName retrieves agent definitions by exact name match.
// This is an internal helper function used by SeedBuiltinAgents.
func ListAgentsByName(ctx context.Context, pool *pgxpool.Pool, name string) ([]AgentDefinition, error) {
	query := `
		SELECT id, name, display_name, description, version, type, category, icon,
			provider, model, system_prompt, tools_config, guardrails, schedule, trigger,
			output_config, routing, is_builtin, is_enterprise, created_at, updated_at
		FROM agent_definitions
		WHERE name = $1
	`

	rows, err := pool.Query(ctx, query, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []AgentDefinition
	for rows.Next() {
		var a AgentDefinition
		err := rows.Scan(
			&a.ID, &a.Name, &a.DisplayName, &a.Description, &a.Version, &a.Type, &a.Category,
			&a.Icon, &a.Provider, &a.Model, &a.SystemPrompt, &a.ToolsConfig, &a.Guardrails,
			&a.Schedule, &a.Trigger, &a.OutputConfig, &a.Routing, &a.IsBuiltin, &a.IsEnterprise, &a.CreatedAt, &a.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}

	return agents, rows.Err()
}
