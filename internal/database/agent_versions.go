// Package database provides PostgreSQL connection pool management and query functions.
package database

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AgentVersion represents a version snapshot of an agent definition stored in the database.
type AgentVersion struct {
	ID            uuid.UUID
	AgentID       uuid.UUID
	Version       string
	Snapshot      json.RawMessage
	ChangeSummary *string
	CreatedBy     *string
	CreatedAt     time.Time
}

// CreateAgentVersion inserts a new agent version snapshot into the database.
// Returns the created AgentVersion or an error.
func CreateAgentVersion(ctx context.Context, pool *pgxpool.Pool, agentID uuid.UUID, version string, snapshot json.RawMessage, changeSummary, createdBy *string) (*AgentVersion, error) {
	query := `
		INSERT INTO agent_definition_versions (agent_id, version, snapshot, change_summary, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, agent_id, version, snapshot, change_summary, created_by, created_at
	`

	var v AgentVersion
	err := pool.QueryRow(ctx, query, agentID, version, snapshot, changeSummary, createdBy).Scan(
		&v.ID, &v.AgentID, &v.Version, &v.Snapshot, &v.ChangeSummary, &v.CreatedBy, &v.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// ListAgentVersions retrieves all versions for a given agent ordered by created_at DESC.
// Returns a slice of AgentVersion or an error.
func ListAgentVersions(ctx context.Context, pool *pgxpool.Pool, agentID uuid.UUID) ([]AgentVersion, error) {
	query := `
		SELECT id, agent_id, version, snapshot, change_summary, created_by, created_at
		FROM agent_definition_versions
		WHERE agent_id = $1
		ORDER BY created_at DESC
	`

	rows, err := pool.Query(ctx, query, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []AgentVersion
	for rows.Next() {
		var v AgentVersion
		err := rows.Scan(
			&v.ID, &v.AgentID, &v.Version, &v.Snapshot, &v.ChangeSummary, &v.CreatedBy, &v.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}

	return versions, rows.Err()
}

// GetAgentVersion retrieves a single version by agent_id and version string.
// Returns the AgentVersion or nil if not found (like GetAgentRun pattern).
func GetAgentVersion(ctx context.Context, pool *pgxpool.Pool, agentID uuid.UUID, version string) (*AgentVersion, error) {
	query := `
		SELECT id, agent_id, version, snapshot, change_summary, created_by, created_at
		FROM agent_definition_versions
		WHERE agent_id = $1 AND version = $2
	`

	var v AgentVersion
	err := pool.QueryRow(ctx, query, agentID, version).Scan(
		&v.ID, &v.AgentID, &v.Version, &v.Snapshot, &v.ChangeSummary, &v.CreatedBy, &v.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &v, nil
}
