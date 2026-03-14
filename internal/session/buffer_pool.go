// Package session provides session management with RingBuffer pool for terminal data.
package session

import (
	"sync"

	"github.com/daao/nexus/pkg/buffer"
)

// RingBufferPool manages per-session RingBuffers for terminal data buffering.
// It creates buffers on session creation and destroys them on session termination.
type RingBufferPool struct {
	mu      sync.RWMutex
	buffers map[string]*buffer.RingBuffer
}

// NewRingBufferPool creates a new RingBufferPool.
func NewRingBufferPool() *RingBufferPool {
	return &RingBufferPool{
		buffers: make(map[string]*buffer.RingBuffer),
	}
}

// GetOrCreateBuffer gets an existing RingBuffer for a session or creates a new one.
func (p *RingBufferPool) GetOrCreateBuffer(sessionID string) *buffer.RingBuffer {
	p.mu.Lock()
	defer p.mu.Unlock()

	if rb, exists := p.buffers[sessionID]; exists {
		return rb
	}

	rb := buffer.NewRingBuffer(0) // 0 uses default capacity (5MB)
	p.buffers[sessionID] = rb
	return rb
}

// GetBuffer gets an existing RingBuffer for a session.
// Returns nil if the session doesn't have a buffer.
func (p *RingBufferPool) GetBuffer(sessionID string) *buffer.RingBuffer {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.buffers[sessionID]
}

// RemoveBuffer removes and destroys the RingBuffer for a session.
// This should be called when a session terminates.
func (p *RingBufferPool) RemoveBuffer(sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.buffers, sessionID)
}

// HasBuffer checks if a session has an active RingBuffer.
func (p *RingBufferPool) HasBuffer(sessionID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	_, exists := p.buffers[sessionID]
	return exists
}

// Len returns the number of active buffers in the pool.
func (p *RingBufferPool) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return len(p.buffers)
}

// Clear removes all buffers from the pool.
func (p *RingBufferPool) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.buffers = make(map[string]*buffer.RingBuffer)
}
