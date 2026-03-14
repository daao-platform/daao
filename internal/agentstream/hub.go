package agentstream

import (
	"encoding/json"
	"log/slog"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type AgentStreamEvent struct {
	ID             string          `json:"id"`
	RunID          string          `json:"run_id"`
	EventType      string          `json:"event_type"`
	Payload        json.RawMessage `json:"payload"`
	Sequence       int             `json:"sequence"`
	CreatedAt      time.Time       `json:"created_at"`
	OriginInstance string          `json:"-"` // set by NATSRunEventHub to prevent echo
}

// subscriber wraps a channel with a closed flag to prevent
// send-on-closed-channel panics during concurrent Publish/Close.
type subscriber struct {
	ch     chan AgentStreamEvent
	closed bool
}

type RunEventHub struct {
	mu      sync.Mutex
	clients map[uuid.UUID]map[*subscriber]struct{}
}

func NewRunEventHub() *RunEventHub {
	return &RunEventHub{
		clients: make(map[uuid.UUID]map[*subscriber]struct{}),
	}
}

// Subscribe registers a new subscriber for the given run. Returns the
// channel to read events from. Call Unsubscribe when done.
func (h *RunEventHub) Subscribe(runID uuid.UUID) chan AgentStreamEvent {
	sub := &subscriber{
		ch: make(chan AgentStreamEvent, 128),
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[runID] == nil {
		h.clients[runID] = make(map[*subscriber]struct{})
	}
	h.clients[runID][sub] = struct{}{}
	return sub.ch
}

// Unsubscribe removes a subscriber channel. Safe to call concurrently
// with Publish — the closed flag prevents send-on-closed-channel panics.
func (h *RunEventHub) Unsubscribe(runID uuid.UUID, ch chan AgentStreamEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if clients, ok := h.clients[runID]; ok {
		for sub := range clients {
			if sub.ch == ch {
				sub.closed = true
				close(sub.ch)
				delete(clients, sub)
				break
			}
		}
		if len(clients) == 0 {
			delete(h.clients, runID)
		}
	}
}

// Publish sends an event to all subscribers for the given run. Fully safe
// against concurrent Close/Unsubscribe — the mutex is held during iteration
// and closed subscribers are skipped.
func (h *RunEventHub) Publish(runID uuid.UUID, event AgentStreamEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.clients[runID] {
		if sub.closed {
			continue
		}
		select {
		case sub.ch <- event:
		default:
			slog.Info(fmt.Sprintf("RunEventHub: dropped event for slow subscriber (run %s)", runID), "component", "agentstream")
		}
	}
}

// Close closes all subscriber channels for a run and removes the run entry.
// Subscriber channels will return ok=false on receive after this call.
func (h *RunEventHub) Close(runID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.clients[runID] {
		sub.closed = true
		close(sub.ch)
	}
	delete(h.clients, runID)
}

// Compile-time interface compliance check.
var _ RunEventHubInterface = (*RunEventHub)(nil)
