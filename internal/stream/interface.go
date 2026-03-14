package stream

import (
	"github.com/daao/nexus/proto"
	"github.com/google/uuid"
)

// StreamRegistryInterface is satisfied by both the in-memory StreamRegistry
// and the NATS-backed NATSStreamRegistry.
type StreamRegistryInterface interface {
	RegisterStream(sessionID string, ch chan<- *proto.NexusMessage)
	UnregisterStream(sessionID string)
	RegisterSatelliteStream(satelliteID string, ch chan<- *proto.NexusMessage)
	UnregisterSatelliteStream(satelliteID string)
	GetStream(sessionID string) (chan<- *proto.NexusMessage, bool)
	GetSatelliteStream(satelliteID string) (chan<- *proto.NexusMessage, bool)
	SendToSession(sessionID string, msg *proto.NexusMessage) bool
	SendToSatellite(satelliteID string, msg *proto.NexusMessage) bool
	HasStream(satelliteID uuid.UUID) bool
	ConnectedCount() int
}
