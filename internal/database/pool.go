// Package database provides PostgreSQL connection pool management.
package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a new PostgreSQL connection pool with the specified database URL.
// It configures the pool with minimum 2 and maximum 20 connections.
// A health check query (SELECT 1) is executed on startup to verify connectivity.
func NewPool(databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	// Configure connection pool limits
	config.MinConns = 2
	config.MaxConns = 20

	// Create the pool
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Health check - verify connectivity with SELECT 1
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database health check failed: %w", err)
	}

	return pool, nil
}
