package agentstream

import "github.com/google/uuid"

// RunEventHubInterface is satisfied by both the in-memory RunEventHub
// and the NATS-backed NATSRunEventHub.
type RunEventHubInterface interface {
	Subscribe(runID uuid.UUID) chan AgentStreamEvent
	Unsubscribe(runID uuid.UUID, ch chan AgentStreamEvent)
	Publish(runID uuid.UUID, event AgentStreamEvent)
	Close(runID uuid.UUID)
}
