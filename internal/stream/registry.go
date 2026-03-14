package stream

import (
	"sync"

	"github.com/daao/nexus/proto"
	"github.com/google/uuid"
)

// StreamRegistry manages active gRPC streams for sending messages to satellites and sessions
type StreamRegistry struct {
	mu               sync.RWMutex
	sessionStreams   map[string]chan<- *proto.NexusMessage
	satelliteStreams map[string]chan<- *proto.NexusMessage
}

// NewStreamRegistry creates a new stream registry
func NewStreamRegistry() *StreamRegistry {
	return &StreamRegistry{
		sessionStreams:   make(map[string]chan<- *proto.NexusMessage),
		satelliteStreams: make(map[string]chan<- *proto.NexusMessage),
	}
}

// RegisterStream registers a stream for a session
func (r *StreamRegistry) RegisterStream(sessionID string, ch chan<- *proto.NexusMessage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessionStreams[sessionID] = ch
}

// UnregisterStream removes a stream for a session
func (r *StreamRegistry) UnregisterStream(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessionStreams, sessionID)
}

// RegisterSatelliteStream registers a stream for a satellite
func (r *StreamRegistry) RegisterSatelliteStream(satelliteID string, ch chan<- *proto.NexusMessage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.satelliteStreams[satelliteID] = ch
}

// UnregisterSatelliteStream removes a stream for a satellite
func (r *StreamRegistry) UnregisterSatelliteStream(satelliteID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.satelliteStreams, satelliteID)
}

// GetStream returns the channel for a session
func (r *StreamRegistry) GetStream(sessionID string) (chan<- *proto.NexusMessage, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ch, ok := r.sessionStreams[sessionID]
	return ch, ok
}

// GetSatelliteStream returns the channel for a satellite
func (r *StreamRegistry) GetSatelliteStream(satelliteID string) (chan<- *proto.NexusMessage, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ch, ok := r.satelliteStreams[satelliteID]
	return ch, ok
}

// SendToSession sends a message to a specific session
func (r *StreamRegistry) SendToSession(sessionID string, msg *proto.NexusMessage) bool {
	r.mu.RLock()
	ch, ok := r.sessionStreams[sessionID]
	r.mu.RUnlock()

	if ok && ch != nil {
		select {
		case ch <- msg:
			return true
		default:
			// Channel is full or blocked, message dropped
			return false
		}
	}
	return false
}

// SendToSatellite sends a message to a specific satellite
func (r *StreamRegistry) SendToSatellite(satelliteID string, msg *proto.NexusMessage) bool {
	r.mu.RLock()
	ch, ok := r.satelliteStreams[satelliteID]
	r.mu.RUnlock()

	if ok && ch != nil {
		select {
		case ch <- msg:
			return true
		default:
			// Channel is full or blocked, message dropped
			return false
		}
	}
	return false
}

// HasStream returns true if the satellite has an active stream registered.
func (r *StreamRegistry) HasStream(satelliteID uuid.UUID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.satelliteStreams[satelliteID.String()]
	return ok
}

// ConnectedCount returns the number of active satellite connections.
func (r *StreamRegistry) ConnectedCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.satelliteStreams)
}

// Compile-time interface compliance check.
var _ StreamRegistryInterface = (*StreamRegistry)(nil)
