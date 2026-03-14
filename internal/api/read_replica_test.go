package api

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
)

// mockPool is a minimal mock implementation of *pgxpool.Pool for testing.
// Since we only test pointer identity, we don't need full implementation.
type mockPool struct{}

func (m *mockPool) Close() {}

func TestHandlers_RpoolFallsBackToPrimary(t *testing.T) {
	// Create a mock primary pool
	mockPrimary := &pgxpool.Pool{}
	
	// Create handlers with primary pool but no read pool set
	h := NewHandlers(nil, mockPrimary, nil, nil, nil, nil)
	
	// rpool() should return the primary pool when readPool is nil
	assert.Equal(t, mockPrimary, h.ReadPool())
}

func TestHandlers_RpoolUsesReplicaWhenSet(t *testing.T) {
	// Create mock pools
	mockPrimary := &pgxpool.Pool{}
	mockReplica := &pgxpool.Pool{}
	
	// Create handlers with primary pool
	h := NewHandlers(nil, mockPrimary, nil, nil, nil, nil)
	
	// Initially should return primary
	assert.Equal(t, mockPrimary, h.ReadPool())
	
	// Set the read pool
	h.SetReadPool(mockReplica)
	
	// Now should return the replica
	assert.Equal(t, mockReplica, h.ReadPool())
}

func TestHandlers_SetReadPoolNilFallsBack(t *testing.T) {
	// Create mock pools
	mockPrimary := &pgxpool.Pool{}
	mockReplica := &pgxpool.Pool{}
	
	// Create handlers with primary pool
	h := NewHandlers(nil, mockPrimary, nil, nil, nil, nil)
	
	// Set the read pool to replica
	h.SetReadPool(mockReplica)
	assert.Equal(t, mockReplica, h.ReadPool())
	
	// Set the read pool back to nil
	h.SetReadPool(nil)
	
	// Should fall back to primary
	assert.Equal(t, mockPrimary, h.ReadPool())
}
