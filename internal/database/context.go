// Package database provides PostgreSQL connection pool management and query functions.
package database

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ContextFile represents a context file stored for a satellite.
type ContextFile struct {
	ID             uuid.UUID
	SatelliteID    uuid.UUID
	FilePath       string
	Content        string
	Version        int
	LastModifiedBy string
	CreatedAt      interface{}
	UpdatedAt      interface{}
}

// ContextFileHistory represents a historical version of a context file.
type ContextFileHistory struct {
	ID            uuid.UUID
	ContextFileID uuid.UUID
	Version       int
	Content       string
	ModifiedBy    string
	ModifiedAt    interface{}
}

// CreateContextFile inserts a new context file into the database.
// Returns the created ContextFile or an error.
func CreateContextFile(ctx context.Context, pool *pgxpool.Pool, satelliteID uuid.UUID, filePath, content, lastModifiedBy string) (*ContextFile, error) {
	query := `
		INSERT INTO satellite_context_files (satellite_id, file_path, content, version, last_modified_by)
		VALUES ($1, $2, $3, 1, $4)
		RETURNING id, satellite_id, file_path, content, version, last_modified_by, created_at, updated_at
	`

	var cf ContextFile
	err := pool.QueryRow(ctx, query, satelliteID, filePath, content, lastModifiedBy).Scan(
		&cf.ID, &cf.SatelliteID, &cf.FilePath, &cf.Content, &cf.Version, &cf.LastModifiedBy, &cf.CreatedAt, &cf.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &cf, nil
}

// GetContextFile retrieves a context file by its ID.
// Returns the ContextFile or an error.
func GetContextFile(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*ContextFile, error) {
	query := `
		SELECT id, satellite_id, file_path, content, version, last_modified_by, created_at, updated_at
		FROM satellite_context_files
		WHERE id = $1
	`

	var cf ContextFile
	err := pool.QueryRow(ctx, query, id).Scan(
		&cf.ID, &cf.SatelliteID, &cf.FilePath, &cf.Content, &cf.Version, &cf.LastModifiedBy, &cf.CreatedAt, &cf.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &cf, nil
}

// ListContextFiles retrieves all context files for a given satellite ID.
// Returns a slice of ContextFile or an error.
func ListContextFiles(ctx context.Context, pool *pgxpool.Pool, satelliteID uuid.UUID) ([]ContextFile, error) {
	query := `
		SELECT id, satellite_id, file_path, content, version, last_modified_by, created_at, updated_at
		FROM satellite_context_files
		WHERE satellite_id = $1
		ORDER BY file_path
	`

	rows, err := pool.Query(ctx, query, satelliteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []ContextFile
	for rows.Next() {
		var cf ContextFile
		err := rows.Scan(
			&cf.ID, &cf.SatelliteID, &cf.FilePath, &cf.Content, &cf.Version, &cf.LastModifiedBy, &cf.CreatedAt, &cf.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		files = append(files, cf)
	}

	return files, rows.Err()
}

// UpdateContextFile updates a context file's content and increments its version.
// It also creates a history entry for the previous version.
// Returns the updated ContextFile or an error.
func UpdateContextFile(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, content, lastModifiedBy string) (*ContextFile, error) {
	// First, get the current version
	var currentVersion int
	err := pool.QueryRow(ctx, "SELECT version FROM satellite_context_files WHERE id = $1", id).Scan(&currentVersion)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	// Create history entry for the current version before updating
	historyQuery := `
		INSERT INTO context_file_history (context_file_id, version, content, modified_by)
		SELECT id, version, content, last_modified_by
		FROM satellite_context_files
		WHERE id = $1
	`
	_, err = pool.Exec(ctx, historyQuery, id)
	if err != nil {
		return nil, err
	}

	// Update the context file with incremented version
	updateQuery := `
		UPDATE satellite_context_files
		SET content = $1, version = version + 1, last_modified_by = $2, updated_at = NOW()
		WHERE id = $3
		RETURNING id, satellite_id, file_path, content, version, last_modified_by, created_at, updated_at
	`

	var cf ContextFile
	err = pool.QueryRow(ctx, updateQuery, content, lastModifiedBy, id).Scan(
		&cf.ID, &cf.SatelliteID, &cf.FilePath, &cf.Content, &cf.Version, &cf.LastModifiedBy, &cf.CreatedAt, &cf.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &cf, nil
}

// DeleteContextFile deletes a context file by its ID.
// History entries are removed automatically via ON DELETE CASCADE.
// Returns an error if the deletion fails.
func DeleteContextFile(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	_, err := pool.Exec(ctx, "DELETE FROM satellite_context_files WHERE id = $1", id)
	return err
}

// CreateContextFileHistory creates a new history entry for a context file.
// Returns the created ContextFileHistory or an error.
func CreateContextFileHistory(ctx context.Context, pool *pgxpool.Pool, contextFileID uuid.UUID, version int, content, modifiedBy string) (*ContextFileHistory, error) {
	query := `
		INSERT INTO context_file_history (context_file_id, version, content, modified_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, context_file_id, version, content, modified_by, modified_at
	`

	var cfh ContextFileHistory
	err := pool.QueryRow(ctx, query, contextFileID, version, content, modifiedBy).Scan(
		&cfh.ID, &cfh.ContextFileID, &cfh.Version, &cfh.Content, &cfh.ModifiedBy, &cfh.ModifiedAt,
	)
	if err != nil {
		return nil, err
	}
	return &cfh, nil
}

// ListContextFileHistory retrieves all history entries for a context file,
// ordered by modified_at descending (most recent first).
// Returns a slice of ContextFileHistory or an error.
func ListContextFileHistory(ctx context.Context, pool *pgxpool.Pool, contextFileID uuid.UUID) ([]ContextFileHistory, error) {
	query := `
		SELECT id, context_file_id, version, content, modified_by, modified_at
		FROM context_file_history
		WHERE context_file_id = $1
		ORDER BY modified_at DESC
	`

	rows, err := pool.Query(ctx, query, contextFileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []ContextFileHistory
	for rows.Next() {
		var cfh ContextFileHistory
		err := rows.Scan(
			&cfh.ID, &cfh.ContextFileID, &cfh.Version, &cfh.Content, &cfh.ModifiedBy, &cfh.ModifiedAt,
		)
		if err != nil {
			return nil, err
		}
		history = append(history, cfh)
	}

	return history, rows.Err()
}
