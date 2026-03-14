// Package database provides PostgreSQL connection pool management and query functions.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AgentRunEvent represents an event associated with an agent run.
type AgentRunEvent struct {
	ID        uuid.UUID
	RunID     uuid.UUID
	EventType string
	Payload   []byte
	Sequence  int
	CreatedAt time.Time
}

// InsertAgentRunEvent inserts a new event for an agent run into the database.
// Returns the created AgentRunEvent or an error.
func InsertAgentRunEvent(ctx context.Context, pool *pgxpool.Pool, runID uuid.UUID, eventType string, payload []byte, sequence int) (*AgentRunEvent, error) {
	query := `INSERT INTO agent_run_events (run_id, event_type, payload, sequence)
			  VALUES ($1, $2, $3, $4)
			  RETURNING id, run_id, event_type, payload, sequence, created_at`
	var e AgentRunEvent
	err := pool.QueryRow(ctx, query, runID, eventType, payload, sequence).Scan(
		&e.ID, &e.RunID, &e.EventType, &e.Payload, &e.Sequence, &e.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("InsertAgentRunEvent: %w", err)
	}
	return &e, nil
}

// ListAgentRunEvents retrieves all events for a given agent run ordered by sequence ASC.
// Returns a slice of AgentRunEvent or an error.
// Returns an empty slice (not nil) when no events exist.
func ListAgentRunEvents(ctx context.Context, pool *pgxpool.Pool, runID uuid.UUID) ([]AgentRunEvent, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, run_id, event_type, payload, sequence, created_at
		 FROM agent_run_events WHERE run_id = $1 ORDER BY sequence ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("ListAgentRunEvents: %w", err)
	}
	defer rows.Close()
	var events []AgentRunEvent
	for rows.Next() {
		var e AgentRunEvent
		if err := rows.Scan(&e.ID, &e.RunID, &e.EventType, &e.Payload, &e.Sequence, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("ListAgentRunEvents scan: %w", err)
		}
		events = append(events, e)
	}
	if events == nil {
		events = []AgentRunEvent{} // never return nil slice
	}
	return events, rows.Err()
}

// ListAgentRunEventsAfter retrieves events for a given agent run with sequence > afterSequence.
// Ordered by sequence ASC.
// Returns a slice of AgentRunEvent or an error.
func ListAgentRunEventsAfter(ctx context.Context, pool *pgxpool.Pool, runID uuid.UUID, afterSequence int) ([]AgentRunEvent, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, run_id, event_type, payload, sequence, created_at
		 FROM agent_run_events WHERE run_id = $1 AND sequence > $2 ORDER BY sequence ASC`,
		runID, afterSequence)
	if err != nil {
		return nil, fmt.Errorf("ListAgentRunEventsAfter: %w", err)
	}
	defer rows.Close()
	var events []AgentRunEvent
	for rows.Next() {
		var e AgentRunEvent
		if err := rows.Scan(&e.ID, &e.RunID, &e.EventType, &e.Payload, &e.Sequence, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("ListAgentRunEventsAfter scan: %w", err)
		}
		events = append(events, e)
	}
	if events == nil {
		events = []AgentRunEvent{} // never return nil slice
	}
	return events, rows.Err()
}
