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

// AuditLogEntry represents an audit log entry stored in the database.
type AuditLogEntry struct {
	ID           uuid.UUID
	ActorID      *uuid.UUID
	ActorEmail   string
	Action       string
	ResourceType string
	ResourceID   *string
	Details      interface{}
	IPAddress    *string
	CreatedAt    time.Time
}

// AuditLogFilters contains optional filters for listing audit log entries.
type AuditLogFilters struct {
	ActorID      *uuid.UUID
	Action       *string
	ResourceType *string
	Since        *time.Time
	Until        *time.Time
}

// WriteAuditLog inserts a new audit log entry into the database.
func WriteAuditLog(ctx context.Context, pool *pgxpool.Pool, entry *AuditLogEntry) error {
	var details interface{}
	if entry.Details == nil {
		details = "{}"
	} else {
		details = entry.Details
	}

	query := `
		INSERT INTO admin_audit_log (actor_id, actor_email, action, resource_type, resource_id, details, ip_address)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := pool.Exec(ctx, query,
		entry.ActorID,
		entry.ActorEmail,
		entry.Action,
		entry.ResourceType,
		entry.ResourceID,
		details,
		entry.IPAddress,
	)
	if err != nil {
		return err
	}

	return nil
}

// ListAuditLogs retrieves audit log entries filtered by the provided filters.
// Returns a slice of AuditLogEntry, total count, and any error.
func ListAuditLogs(ctx context.Context, pool *pgxpool.Pool, filters AuditLogFilters, limit, offset int) ([]AuditLogEntry, int, error) {
	// Apply default limit and cap
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	// Build dynamic WHERE clause
	whereClause := "WHERE 1=1"
	args := []interface{}{}
	argIndex := 1

	if filters.ActorID != nil {
		whereClause += " AND actor_id = " + fmt.Sprintf("$%d", argIndex)
		args = append(args, *filters.ActorID)
		argIndex++
	}

	if filters.Action != nil {
		whereClause += " AND action = " + fmt.Sprintf("$%d", argIndex)
		args = append(args, *filters.Action)
		argIndex++
	}

	if filters.ResourceType != nil {
		whereClause += " AND resource_type = " + fmt.Sprintf("$%d", argIndex)
		args = append(args, *filters.ResourceType)
		argIndex++
	}

	if filters.Since != nil {
		whereClause += " AND created_at >= " + fmt.Sprintf("$%d", argIndex)
		args = append(args, *filters.Since)
		argIndex++
	}

	if filters.Until != nil {
		whereClause += " AND created_at <= " + fmt.Sprintf("$%d", argIndex)
		args = append(args, *filters.Until)
		argIndex++
	}

	// Build COUNT query
	countQuery := "SELECT COUNT(*) FROM admin_audit_log " + whereClause
	var totalCount int
	err := pool.QueryRow(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		return nil, 0, err
	}

	// Build main query with ORDER BY, LIMIT, OFFSET
	query := fmt.Sprintf(`
		SELECT id, actor_id, actor_email, action, resource_type, resource_id, details, ip_address, created_at
		FROM admin_audit_log
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)

	args = append(args, limit, offset)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []AuditLogEntry
	for rows.Next() {
		var entry AuditLogEntry
		err := rows.Scan(
			&entry.ID,
			&entry.ActorID,
			&entry.ActorEmail,
			&entry.Action,
			&entry.ResourceType,
			&entry.ResourceID,
			&entry.Details,
			&entry.IPAddress,
			&entry.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		entries = append(entries, entry)
	}

	if entries == nil {
		entries = []AuditLogEntry{}
	}

	return entries, totalCount, rows.Err()
}

// GetAuditLog retrieves a single audit log entry by its ID.
func GetAuditLog(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*AuditLogEntry, error) {
	query := `
		SELECT id, actor_id, actor_email, action, resource_type, resource_id, details, ip_address, created_at
		FROM admin_audit_log
		WHERE id = $1
	`

	var entry AuditLogEntry
	err := pool.QueryRow(ctx, query, id).Scan(
		&entry.ID,
		&entry.ActorID,
		&entry.ActorEmail,
		&entry.Action,
		&entry.ResourceType,
		&entry.ResourceID,
		&entry.Details,
		&entry.IPAddress,
		&entry.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &entry, nil
}
