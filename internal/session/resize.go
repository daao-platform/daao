// Package session provides session persistence with state machine management.
//
// This package implements the Minimum Bounding Box Consensus for terminal
// dimension enforcement across multiple attached clients.
package session

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// SatellitePusher defines the interface for pushing commands to the satellite
type SatellitePusher interface {
	// PushResize sends a ResizeCommand to the satellite
	PushResize(ctx context.Context, cmd *ResizeCommand) error
}

// Client represents a client attached to a session
type Client struct {
	ID       uuid.UUID
	Cols     int16
	Rows     int16
	SendChan chan interface{} // Channel to send messages to this client
}

// BoundingBox represents the minimum dimensions enforced across all clients
type BoundingBox struct {
	Cols int16
	Rows int16
}

// ResizeManager handles terminal dimension consensus across multiple clients
type ResizeManager struct {
	mu            sync.RWMutex
	clients       map[uuid.UUID]*Client
	sessionID     uuid.UUID
	satellitePusher SatellitePusher
}

// NewResizeManager creates a new resize manager for a session
func NewResizeManager(sessionID uuid.UUID) *ResizeManager {
	return &ResizeManager{
		clients:  make(map[uuid.UUID]*Client),
		sessionID: sessionID,
	}
}

// SetSatellitePusher sets the satellite pusher for sending resize commands
func (rm *ResizeManager) SetSatellitePusher(pusher SatellitePusher) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.satellitePusher = pusher
}

// AddClient adds a client to the resize manager
func (rm *ResizeManager) AddClient(client *Client) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.clients[client.ID] = client
}

// RemoveClient removes a client from the resize manager
func (rm *ResizeManager) RemoveClient(clientID uuid.UUID) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	delete(rm.clients, clientID)
}

// UpdateClientDimensions updates a client's terminal dimensions
func (rm *ResizeManager) UpdateClientDimensions(clientID uuid.UUID, cols, rows int16) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if client, ok := rm.clients[clientID]; ok {
		client.Cols = cols
		client.Rows = rows
	}
}

// CalculateBoundingBox calculates the minimum dimensions across all clients
// Effective dimensions = min(all client cols) x min(all client rows)
func (rm *ResizeManager) CalculateBoundingBox() BoundingBox {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if len(rm.clients) == 0 {
		// Default minimum dimensions when no clients are attached
		return BoundingBox{Cols: 80, Rows: 24}
	}

	minCols := int16(0)
	minRows := int16(0)

	for _, client := range rm.clients {
		if minCols == 0 || client.Cols < minCols {
			minCols = client.Cols
		}
		if minRows == 0 || client.Rows < minRows {
			minRows = client.Rows
		}
	}

	return BoundingBox{Cols: minCols, Rows: minRows}
}

// HandleResize processes a RESIZE command from a client
// Returns the effective dimensions after applying bounding box consensus
func (rm *ResizeManager) HandleResize(ctx context.Context, clientID uuid.UUID, cols, rows int16) (BoundingBox, error) {
	// Update the client's dimensions
	rm.UpdateClientDimensions(clientID, cols, rows)

	// Calculate the bounding box (minimum across all clients)
	effectiveDimensions := rm.CalculateBoundingBox()

	// Push ResizeCommand to satellite with enforced dimensions
	rm.pushResizeToSatellite(ctx, effectiveDimensions)

	// Broadcast RESIZE_ACK to all clients with effective dimensions
	rm.broadcastResizeAck(effectiveDimensions)

	return effectiveDimensions, nil
}

// pushResizeToSatellite sends the ResizeCommand to the satellite via the pusher
func (rm *ResizeManager) pushResizeToSatellite(ctx context.Context, dimensions BoundingBox) {
	rm.mu.RLock()
	pusher := rm.satellitePusher
	rm.mu.RUnlock()

	if pusher == nil {
		// No satellite pusher configured, skip pushing
		return
	}

	cmd := CreateResizeCommand(rm.sessionID, dimensions)
	if err := pusher.PushResize(ctx, cmd); err != nil {
		// Log error but don't fail the resize - clients still get the ACK
		// In production, this would be logged via proper logging
		return
	}
}

// broadcastResizeAck sends RESIZE_ACK to all connected clients
func (rm *ResizeManager) broadcastResizeAck(dimensions BoundingBox) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	resizeAck := ResizeAck{
		SessionID: rm.sessionID,
		Cols:      dimensions.Cols,
		Rows:      dimensions.Rows,
	}

	for _, client := range rm.clients {
		select {
		case client.SendChan <- resizeAck:
		default:
			// Client channel is full or blocked, skip this client
		}
	}
}

// ResizeAck represents the acknowledgment sent to clients after resize
type ResizeAck struct {
	SessionID uuid.UUID
	Cols      int16
	Rows      int16
}

// ResizeCommand represents a resize command sent to the satellite
type ResizeCommand struct {
	SessionID   uuid.UUID
	Cols        int16
	Rows        int16
	PixelWidth  int32
	PixelHeight int32
}

// GetEffectiveDimensions returns the current effective dimensions
func (rm *ResizeManager) GetEffectiveDimensions() BoundingBox {
	return rm.CalculateBoundingBox()
}

// ClientCount returns the number of connected clients
func (rm *ResizeManager) ClientCount() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.clients)
}

// HandleClientAttach handles a new client attaching to the session
// Recalculates bounding box and broadcasts if dimensions change
func (rm *ResizeManager) HandleClientAttach(ctx context.Context, client *Client) BoundingBox {
	rm.mu.Lock()
	rm.clients[client.ID] = client
	rm.mu.Unlock()

	// Calculate new bounding box after adding client
	newBoundingBox := rm.CalculateBoundingBox()

	// Push ResizeCommand to satellite with new effective dimensions
	rm.pushResizeToSatellite(ctx, newBoundingBox)

	// Broadcast the new effective dimensions to all clients
	rm.broadcastResizeAck(newBoundingBox)

	return newBoundingBox
}

// HandleClientDetach handles a client detaching from the session
// Recalculates bounding box and broadcasts if dimensions change
func (rm *ResizeManager) HandleClientDetach(ctx context.Context, clientID uuid.UUID) BoundingBox {
	rm.mu.Lock()
	delete(rm.clients, clientID)
	rm.mu.Unlock()

	// Calculate new bounding box after removing client
	newBoundingBox := rm.CalculateBoundingBox()

	// Push ResizeCommand to satellite with new effective dimensions
	rm.pushResizeToSatellite(ctx, newBoundingBox)

	// Broadcast the new effective dimensions to all clients
	rm.broadcastResizeAck(newBoundingBox)

	return newBoundingBox
}

// CreateResizeCommand creates a ResizeCommand for the satellite
// This is called when pushing the command to the satellite
func CreateResizeCommand(sessionID uuid.UUID, dimensions BoundingBox) *ResizeCommand {
	return &ResizeCommand{
		SessionID:   sessionID,
		Cols:        dimensions.Cols,
		Rows:        dimensions.Rows,
		PixelWidth:  0, // Default, can be calculated based on font size
		PixelHeight: 0,
	}
}
