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

// AgentRun represents an agent execution run stored in the database.
type AgentRun struct {
	ID            uuid.UUID
	AgentID       uuid.UUID
	SatelliteID   uuid.UUID
	SessionID     *uuid.UUID
	Status        string
	TriggerSource string
	StartedAt     time.Time
	EndedAt       *time.Time
	TotalTokens   int
	EstimatedCost float64
	ToolCallCount int
	Result        *string
	Error         *string
	Metadata      interface{}
}

// AgentRunFilters contains optional filters for listing agent runs.
type AgentRunFilters struct {
	AgentID     *uuid.UUID
	SatelliteID *uuid.UUID
	Status      *string
}

// CreateAgentRun inserts a new agent run into the database.
// Returns the created AgentRun or an error.
func CreateAgentRun(ctx context.Context, pool *pgxpool.Pool, agentID, satelliteID uuid.UUID, sessionID *uuid.UUID, triggerSource string) (*AgentRun, error) {
	query := `
		INSERT INTO agent_runs (agent_id, satellite_id, session_id, trigger_source, status, started_at, total_tokens, estimated_cost, tool_call_count, metadata)
		VALUES ($1, $2, $3, $4, 'running', NOW(), 0, 0, 0, '{}')
		RETURNING id, agent_id, satellite_id, session_id, trigger_source, status, started_at, ended_at, total_tokens, estimated_cost, tool_call_count, result, error, metadata
	`

	var run AgentRun
	var sessionIDPtr *uuid.UUID
	var endedAtPtr *time.Time
	var resultPtr *string
	var errorPtr *string

	err := pool.QueryRow(ctx, query, agentID, satelliteID, sessionID, triggerSource).Scan(
		&run.ID, &run.AgentID, &run.SatelliteID, &sessionIDPtr, &run.TriggerSource, &run.Status, &run.StartedAt, &endedAtPtr, &run.TotalTokens, &run.EstimatedCost, &run.ToolCallCount, &resultPtr, &errorPtr, &run.Metadata,
	)
	if err != nil {
		return nil, err
	}
	run.SessionID = sessionIDPtr
	run.EndedAt = endedAtPtr
	run.Result = resultPtr
	run.Error = errorPtr
	return &run, nil
}

// GetAgentRun retrieves an agent run by its ID.
// Returns the AgentRun or an error.
func GetAgentRun(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*AgentRun, error) {
	query := `
		SELECT id, agent_id, satellite_id, session_id, trigger_source, status, started_at, ended_at, total_tokens, estimated_cost, tool_call_count, result, error, metadata
		FROM agent_runs
		WHERE id = $1
	`

	var run AgentRun
	var sessionIDPtr *uuid.UUID
	var endedAtPtr *time.Time
	var resultPtr *string
	var errorPtr *string

	err := pool.QueryRow(ctx, query, id).Scan(
		&run.ID, &run.AgentID, &run.SatelliteID, &sessionIDPtr, &run.TriggerSource, &run.Status, &run.StartedAt, &endedAtPtr, &run.TotalTokens, &run.EstimatedCost, &run.ToolCallCount, &resultPtr, &errorPtr, &run.Metadata,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	run.SessionID = sessionIDPtr
	run.EndedAt = endedAtPtr
	run.Result = resultPtr
	run.Error = errorPtr
	return &run, nil
}

// ListAgentRuns retrieves agent runs filtered by the provided filters.
// Supports filtering by agent_id, satellite_id, and status.
// Returns a slice of AgentRun or an error.
func ListAgentRuns(ctx context.Context, pool *pgxpool.Pool, filters AgentRunFilters) ([]AgentRun, error) {
	// Build dynamic query based on filters
	query := `
		SELECT id, agent_id, satellite_id, session_id, trigger_source, status, started_at, ended_at, total_tokens, estimated_cost, tool_call_count, result, error, metadata
		FROM agent_runs
		WHERE 1=1
	`
	args := []interface{}{}
	argIndex := 1

	if filters.AgentID != nil {
		query += ` AND agent_id = ` + fmt.Sprintf("$%d", argIndex)
		args = append(args, *filters.AgentID)
		argIndex++
	}

	if filters.SatelliteID != nil {
		query += ` AND satellite_id = ` + fmt.Sprintf("$%d", argIndex)
		args = append(args, *filters.SatelliteID)
		argIndex++
	}

	if filters.Status != nil {
		query += ` AND status = ` + fmt.Sprintf("$%d", argIndex)
		args = append(args, *filters.Status)
		argIndex++
	}

	query += ` ORDER BY started_at DESC`

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []AgentRun
	for rows.Next() {
		var run AgentRun
		var sessionIDPtr *uuid.UUID
		var endedAtPtr *time.Time
		var resultPtr *string
		var errorPtr *string

		err := rows.Scan(
			&run.ID, &run.AgentID, &run.SatelliteID, &sessionIDPtr, &run.TriggerSource, &run.Status, &run.StartedAt, &endedAtPtr, &run.TotalTokens, &run.EstimatedCost, &run.ToolCallCount, &resultPtr, &errorPtr, &run.Metadata,
		)
		if err != nil {
			return nil, err
		}
		run.SessionID = sessionIDPtr
		run.EndedAt = endedAtPtr
		run.Result = resultPtr
		run.Error = errorPtr
		runs = append(runs, run)
	}

	return runs, rows.Err()
}

// AgentRunWithContext extends AgentRun with display names from related tables.
// Used by the "list all runs" endpoint to avoid N+1 queries.
type AgentRunWithContext struct {
	AgentRun
	AgentName     string     `json:"agent_name"`
	SatelliteName string     `json:"satellite_name"`
	PipelineRunID *uuid.UUID `json:"pipeline_run_id,omitempty"`
	PipelineName  *string    `json:"pipeline_name,omitempty"`
	StepOrder     *int       `json:"step_order,omitempty"`
}

// ListAgentRunsWithContext retrieves agent runs with agent/satellite/pipeline display names.
// Supports limit/offset pagination. Returns runs ordered by started_at DESC.
func ListAgentRunsWithContext(ctx context.Context, pool *pgxpool.Pool, filters AgentRunFilters, limit, offset int) ([]AgentRunWithContext, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	// Build WHERE clause
	where := "WHERE 1=1"
	args := []interface{}{}
	argIndex := 1

	if filters.AgentID != nil {
		where += fmt.Sprintf(" AND ar.agent_id = $%d", argIndex)
		args = append(args, *filters.AgentID)
		argIndex++
	}
	if filters.SatelliteID != nil {
		where += fmt.Sprintf(" AND ar.satellite_id = $%d", argIndex)
		args = append(args, *filters.SatelliteID)
		argIndex++
	}
	if filters.Status != nil {
		where += fmt.Sprintf(" AND ar.status = $%d", argIndex)
		args = append(args, *filters.Status)
		argIndex++
	}

	// Count query
	countQuery := `SELECT COUNT(*) FROM agent_runs ar ` + where
	var total int
	err := pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Data query with JOINs
	query := `
		SELECT ar.id, ar.agent_id, ar.satellite_id, ar.session_id, ar.trigger_source,
		       ar.status, ar.started_at, ar.ended_at, ar.total_tokens, ar.estimated_cost,
		       ar.tool_call_count, ar.result, ar.error, ar.metadata,
		       COALESCE(ad.display_name, ad.name, '') AS agent_name,
		       COALESCE(s.name, '') AS satellite_name,
		       psr.pipeline_run_id,
		       p.name AS pipeline_name,
		       psr.step_order
		FROM agent_runs ar
		LEFT JOIN agent_definitions ad ON ad.id = ar.agent_id
		LEFT JOIN satellites s ON s.id = ar.satellite_id
		LEFT JOIN pipeline_step_runs psr ON psr.agent_run_id = ar.id
		LEFT JOIN pipeline_runs pr ON pr.id = psr.pipeline_run_id
		LEFT JOIN pipelines p ON p.id = pr.pipeline_id
		` + where + `
		ORDER BY ar.started_at DESC
		LIMIT ` + fmt.Sprintf("$%d", argIndex) + ` OFFSET ` + fmt.Sprintf("$%d", argIndex+1)

	args = append(args, limit, offset)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var runs []AgentRunWithContext
	for rows.Next() {
		var r AgentRunWithContext
		var sessionIDPtr *uuid.UUID
		var endedAtPtr *time.Time
		var resultPtr *string
		var errorPtr *string

		err := rows.Scan(
			&r.ID, &r.AgentID, &r.SatelliteID, &sessionIDPtr, &r.TriggerSource,
			&r.Status, &r.StartedAt, &endedAtPtr, &r.TotalTokens, &r.EstimatedCost,
			&r.ToolCallCount, &resultPtr, &errorPtr, &r.Metadata,
			&r.AgentName, &r.SatelliteName,
			&r.PipelineRunID, &r.PipelineName, &r.StepOrder,
		)
		if err != nil {
			return nil, 0, err
		}
		r.SessionID = sessionIDPtr
		r.EndedAt = endedAtPtr
		r.Result = resultPtr
		r.Error = errorPtr
		runs = append(runs, r)
	}

	return runs, total, rows.Err()
}

// GetAgentRunIDBySessionID returns the most recent agent run ID for a given session.
// Returns nil if no agent run is associated with the session.
func GetAgentRunIDBySessionID(ctx context.Context, pool *pgxpool.Pool, sessionID uuid.UUID) (*uuid.UUID, error) {
	var runID uuid.UUID
	err := pool.QueryRow(ctx,
		`SELECT id FROM agent_runs WHERE session_id = $1 ORDER BY started_at DESC LIMIT 1`,
		sessionID,
	).Scan(&runID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &runID, nil
}

// AgentRunUpdates contains fields that can be updated on an agent run.
type AgentRunUpdates struct {
	Status        *string
	EndedAt       *time.Time
	TotalTokens   *int
	EstimatedCost *float64
	ToolCallCount *int
	Result        *string
	Error         *string
	Metadata      interface{}
}

// UpdateAgentRun updates an agent run's fields.
// Used for status transitions (running → completed|failed|timeout|killed) and other updates.
// Returns the updated AgentRun or an error.
func UpdateAgentRun(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, updates AgentRunUpdates) (*AgentRun, error) {
	// Build dynamic update query
	query := `UPDATE agent_runs SET `
	args := []interface{}{}
	argIndex := 1
	hasUpdates := false

	if updates.Status != nil {
		query += `status = ` + fmt.Sprintf("$%d", argIndex)
		args = append(args, *updates.Status)
		argIndex++
		hasUpdates = true
	}

	if updates.EndedAt != nil {
		if hasUpdates {
			query += `, `
		}
		query += `ended_at = ` + fmt.Sprintf("$%d", argIndex)
		args = append(args, *updates.EndedAt)
		argIndex++
		hasUpdates = true
	}

	if updates.TotalTokens != nil {
		if hasUpdates {
			query += `, `
		}
		query += `total_tokens = ` + fmt.Sprintf("$%d", argIndex)
		args = append(args, *updates.TotalTokens)
		argIndex++
		hasUpdates = true
	}

	if updates.EstimatedCost != nil {
		if hasUpdates {
			query += `, `
		}
		query += `estimated_cost = ` + fmt.Sprintf("$%d", argIndex)
		args = append(args, *updates.EstimatedCost)
		argIndex++
		hasUpdates = true
	}

	if updates.ToolCallCount != nil {
		if hasUpdates {
			query += `, `
		}
		query += `tool_call_count = ` + fmt.Sprintf("$%d", argIndex)
		args = append(args, *updates.ToolCallCount)
		argIndex++
		hasUpdates = true
	}

	if updates.Result != nil {
		if hasUpdates {
			query += `, `
		}
		query += `result = ` + fmt.Sprintf("$%d", argIndex)
		args = append(args, *updates.Result)
		argIndex++
		hasUpdates = true
	}

	if updates.Error != nil {
		if hasUpdates {
			query += `, `
		}
		query += `error = ` + fmt.Sprintf("$%d", argIndex)
		args = append(args, *updates.Error)
		argIndex++
		hasUpdates = true
	}

	if updates.Metadata != nil {
		if hasUpdates {
			query += `, `
		}
		query += `metadata = ` + fmt.Sprintf("$%d", argIndex)
		args = append(args, updates.Metadata)
		argIndex++
		hasUpdates = true
	}

	if !hasUpdates {
		return GetAgentRun(ctx, pool, id)
	}

	query += ` WHERE id = ` + fmt.Sprintf("$%d", argIndex) + ` RETURNING id, agent_id, satellite_id, session_id, trigger_source, status, started_at, ended_at, total_tokens, estimated_cost, tool_call_count, result, error, metadata`
	args = append(args, id)

	var run AgentRun
	var sessionIDPtr *uuid.UUID
	var endedAtPtr *time.Time
	var resultPtr *string
	var errorPtr *string

	err := pool.QueryRow(ctx, query, args...).Scan(
		&run.ID, &run.AgentID, &run.SatelliteID, &sessionIDPtr, &run.TriggerSource, &run.Status, &run.StartedAt, &endedAtPtr, &run.TotalTokens, &run.EstimatedCost, &run.ToolCallCount, &resultPtr, &errorPtr, &run.Metadata,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	run.SessionID = sessionIDPtr
	run.EndedAt = endedAtPtr
	run.Result = resultPtr
	run.Error = errorPtr
	return &run, nil
}
