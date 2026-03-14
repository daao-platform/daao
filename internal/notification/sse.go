package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// SSEHub – manages per-user SSE connections
// ---------------------------------------------------------------------------

// SSEHub manages Server-Sent Events connections for real-time notification
// delivery to browser clients.
type SSEHub struct {
	mu      sync.RWMutex
	clients map[uuid.UUID]map[chan *Notification]struct{} // userID → set of channels
}

// NewSSEHub creates a new SSE hub.
func NewSSEHub() *SSEHub {
	return &SSEHub{
		clients: make(map[uuid.UUID]map[chan *Notification]struct{}),
	}
}

// Subscribe registers a new SSE client for a user. Returns the channel to
// read notifications from. Call Unsubscribe when the connection closes.
func (h *SSEHub) Subscribe(userID uuid.UUID) chan *Notification {
	ch := make(chan *Notification, 64)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[userID] == nil {
		h.clients[userID] = make(map[chan *Notification]struct{})
	}
	h.clients[userID][ch] = struct{}{}
	slog.Info(fmt.Sprintf("SSEHub: client subscribed for user %s (%d active)", userID, len(h.clients[userID])), "component", "notification")
	return ch
}

// Unsubscribe removes a client channel.
func (h *SSEHub) Unsubscribe(userID uuid.UUID, ch chan *Notification) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if clients, ok := h.clients[userID]; ok {
		delete(clients, ch)
		close(ch)
		if len(clients) == 0 {
			delete(h.clients, userID)
		}
		slog.Info(fmt.Sprintf("SSEHub: client unsubscribed for user %s", userID), "component", "notification")
	}
}

// Broadcast sends a notification to all connected clients for a user.
func (h *SSEHub) Broadcast(userID uuid.UUID, n *Notification) {
	h.mu.RLock()
	clients := h.clients[userID]
	h.mu.RUnlock()

	for ch := range clients {
		select {
		case ch <- n:
		default:
			// Client channel full — drop notification rather than block
			slog.Info(fmt.Sprintf("SSEHub: dropped notification for slow client (user %s)", userID), "component", "notification")
		}
	}
}

// ---------------------------------------------------------------------------
// SSEDispatcher – Dispatcher implementation that pushes through SSEHub
// ---------------------------------------------------------------------------

// SSEDispatcher implements Dispatcher by pushing notifications through the
// SSEHub to all connected browser tabs.
type SSEDispatcher struct {
	hub *SSEHub
}

// NewSSEDispatcher creates a dispatcher backed by the given SSEHub.
func NewSSEDispatcher(hub *SSEHub) *SSEDispatcher {
	return &SSEDispatcher{hub: hub}
}

func (d *SSEDispatcher) Name() string { return "browser" }

func (d *SSEDispatcher) Dispatch(ctx context.Context, n *Notification) error {
	d.hub.Broadcast(n.UserID, n)
	return nil
}

func (d *SSEDispatcher) ShouldDispatch(n *Notification, prefs *UserPreferences) bool {
	if !prefs.BrowserEnabled {
		return false
	}
	// Check per-event-type toggles
	switch n.Type {
	case EventSessionTerminated:
		if !prefs.SessionTerminated {
			return false
		}
	case EventSessionError:
		if !prefs.SessionError {
			return false
		}
	case EventSatelliteOffline:
		if !prefs.SatelliteOffline {
			return false
		}
	case EventSessionSuspended:
		if !prefs.SessionSuspended {
			return false
		}
	}
	// Check minimum priority
	return priorityLevel(n.Priority) >= priorityLevel(prefs.MinPriority)
}

// priorityLevel returns a numeric level for priority comparison.
func priorityLevel(p Priority) int {
	switch p {
	case PriorityCritical:
		return 3
	case PriorityWarning:
		return 2
	case PriorityInfo:
		return 1
	default:
		return 0
	}
}

// ---------------------------------------------------------------------------
// SSE HTTP Handler
// ---------------------------------------------------------------------------

// HandleSSEStream is the HTTP handler for GET /api/v1/notifications/stream.
// It establishes an SSE connection for real-time notification delivery.
func HandleSSEStream(hub *SSEHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// SSE requires streaming support
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		// Extract user ID — in v1 without multi-user auth, use default user.
		// When #23 lands, extract from JWT claims.
		userID := uuid.MustParse("00000000-0000-0000-0000-000000000000")

		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// Subscribe
		ch := hub.Subscribe(userID)
		defer hub.Unsubscribe(userID, ch)

		// Send initial connected event
		fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
		flusher.Flush()

		// Heartbeat to keep connection alive
		heartbeat := time.NewTicker(30 * time.Second)
		defer heartbeat.Stop()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case n, ok := <-ch:
				if !ok {
					return
				}
				data, err := json.Marshal(n)
				if err != nil {
					slog.Error(fmt.Sprintf("SSE: failed to marshal notification: %v", err), "component", "notification")
					continue
				}
				fmt.Fprintf(w, "event: notification\ndata: %s\n\n", data)
				flusher.Flush()
			case <-heartbeat.C:
				fmt.Fprintf(w, ": heartbeat\n\n")
				flusher.Flush()
			}
		}
	}
}
