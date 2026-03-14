// Package notification provides the event bus, notification service, and
// dispatcher interface for the DAAO notification system.
//
// Architecture:
//
//	EventBus  – pub/sub hub; system components emit events, subscribers react.
//	NotificationService – converts events → persisted Notification rows, then
//	                      fans out to all registered Dispatchers.
//	Dispatcher – interface for delivery channels (SSE/browser, Slack, webhook, …).
package notification

import (
	"context"
	"log/slog"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// EventType classifies what happened.
type EventType string

const (
	EventSessionTerminated   EventType = "SESSION_TERMINATED"
	EventSessionSuspended   EventType = "SESSION_SUSPENDED"
	EventSessionError       EventType = "SESSION_ERROR"
	EventSatelliteOffline   EventType = "SATELLITE_OFFLINE"
	EventAgentTriggerFired  EventType = "AGENT_TRIGGER_FIRED"
)

// Priority controls delivery urgency and filtering.
type Priority string

const (
	PriorityInfo     Priority = "INFO"
	PriorityWarning  Priority = "WARNING"
	PriorityCritical Priority = "CRITICAL"
)

// Event represents something that happened in the system.
type Event struct {
	Type        EventType
	Priority    Priority
	SessionID   *uuid.UUID
	SatelliteID *uuid.UUID
	UserID      *uuid.UUID // nil = broadcast to all users
	Title       string
	Body        string
	Payload     map[string]interface{}
	CreatedAt   time.Time
}

// Notification is the persisted, user-facing record of an Event.
type Notification struct {
	ID          uuid.UUID              `json:"id"`
	UserID      uuid.UUID              `json:"user_id"`
	Type        EventType              `json:"type"`
	Priority    Priority               `json:"priority"`
	Title       string                 `json:"title"`
	Body        string                 `json:"body"`
	SessionID   *uuid.UUID             `json:"session_id,omitempty"`
	SatelliteID *uuid.UUID             `json:"satellite_id,omitempty"`
	Payload     map[string]interface{} `json:"payload,omitempty"`
	Read        bool                   `json:"read"`
	CreatedAt   time.Time              `json:"created_at"`
}

// UserPreferences stores per-user notification settings.
type UserPreferences struct {
	UserID            uuid.UUID `json:"user_id"`
	MinPriority       Priority  `json:"min_priority"`
	BrowserEnabled    bool      `json:"browser_enabled"`
	SessionTerminated bool      `json:"session_terminated"`
	SessionError      bool      `json:"session_error"`
	SatelliteOffline  bool      `json:"satellite_offline"`
	SessionSuspended  bool      `json:"session_suspended"`
}

// DefaultPreferences returns sensible defaults for a new user.
func DefaultPreferences(userID uuid.UUID) *UserPreferences {
	return &UserPreferences{
		UserID:            userID,
		MinPriority:       PriorityInfo,
		BrowserEnabled:    true,
		SessionTerminated: true,
		SessionError:      true,
		SatelliteOffline:  true,
		SessionSuspended:  true,
	}
}

// ---------------------------------------------------------------------------
// Dispatcher interface – implement this to add new delivery channels
// ---------------------------------------------------------------------------

// Dispatcher is the extension point for notification delivery channels.
// Implement this interface to add Slack, Teams, webhook, email, etc.
type Dispatcher interface {
	// Name returns a human-readable channel identifier (e.g. "browser", "slack").
	Name() string
	// Dispatch delivers the notification to its channel.
	Dispatch(ctx context.Context, notification *Notification) error
	// ShouldDispatch checks whether this dispatcher should fire for the given
	// notification + user preferences combination.
	ShouldDispatch(notification *Notification, prefs *UserPreferences) bool
}

// ---------------------------------------------------------------------------
// EventBus – pub/sub for system events
// ---------------------------------------------------------------------------

// EventHandler is a callback that processes an Event.
type EventHandler func(event *Event)

// EventBus is a simple synchronous pub/sub hub.
type EventBus struct {
	mu       sync.RWMutex
	handlers []EventHandler
}

// NewEventBus creates a new EventBus.
func NewEventBus() *EventBus {
	return &EventBus{}
}

// Subscribe registers a handler that will be called for every emitted event.
func (b *EventBus) Subscribe(h EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = append(b.handlers, h)
}

// Emit publishes an event to all subscribers. Handlers run synchronously in
// the caller's goroutine to keep ordering simple; dispatchers should be fast
// (they just enqueue to a channel or write to SSE).
func (b *EventBus) Emit(event *Event) {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	b.mu.RLock()
	handlers := make([]EventHandler, len(b.handlers))
	copy(handlers, b.handlers)
	b.mu.RUnlock()

	for _, h := range handlers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Info(fmt.Sprintf("NotificationBus: handler panicked: %v", r), "component", "notification")
				}
			}()
			h(event)
		}()
	}
}

// ---------------------------------------------------------------------------
// NotificationService – event → persist → dispatch
// ---------------------------------------------------------------------------

// NotificationService converts events into persisted notifications and
// dispatches them through registered channels.
type NotificationService struct {
	bus         *EventBus
	store       Store
	dispatchers []Dispatcher
	mu          sync.RWMutex
}

// NewNotificationService creates a NotificationService, subscribes to the
// EventBus, and returns it ready to use.
func NewNotificationService(bus *EventBus, store Store) *NotificationService {
	ns := &NotificationService{
		bus:   bus,
		store: store,
	}
	bus.Subscribe(ns.handleEvent)
	return ns
}

// RegisterDispatcher adds a delivery channel.
func (ns *NotificationService) RegisterDispatcher(d Dispatcher) {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	ns.dispatchers = append(ns.dispatchers, d)
	slog.Info(fmt.Sprintf("Notifications: registered dispatcher %q", d.Name()), "component", "notification")
}

// Bus returns the underlying EventBus so callers can emit events.
func (ns *NotificationService) Bus() *EventBus {
	return ns.bus
}

// handleEvent converts an Event into Notification(s) and dispatches them.
func (ns *NotificationService) handleEvent(event *Event) {
	ctx := context.Background()

	// Determine target user(s).
	var userIDs []uuid.UUID
	if event.UserID != nil {
		userIDs = []uuid.UUID{*event.UserID}
	} else {
		// Broadcast: in v1 without multi-user, target the single default user.
		// When #23 (multi-user) lands, query all users from the org/session owner.
		defaultUser := uuid.MustParse("00000000-0000-0000-0000-000000000000")
		userIDs = []uuid.UUID{defaultUser}
	}

	for _, userID := range userIDs {
		n := &Notification{
			ID:          uuid.New(),
			UserID:      userID,
			Type:        event.Type,
			Priority:    event.Priority,
			Title:       event.Title,
			Body:        event.Body,
			SessionID:   event.SessionID,
			SatelliteID: event.SatelliteID,
			Payload:     event.Payload,
			Read:        false,
			CreatedAt:   event.CreatedAt,
		}

		// Persist
		if ns.store != nil {
			if err := ns.store.Create(ctx, n); err != nil {
				slog.Error(fmt.Sprintf("Notifications: failed to persist notification: %v", err), "component", "notification")
				// Continue with dispatch even if persistence fails
			}
		}

		// Load user preferences (fall back to defaults if unavailable)
		prefs := DefaultPreferences(userID)
		if ns.store != nil {
			if p, err := ns.store.GetPreferences(ctx, userID); err == nil && p != nil {
				prefs = p
			}
		}

		// Fan-out to dispatchers
		ns.mu.RLock()
		dispatchers := make([]Dispatcher, len(ns.dispatchers))
		copy(dispatchers, ns.dispatchers)
		ns.mu.RUnlock()

		for _, d := range dispatchers {
			if d.ShouldDispatch(n, prefs) {
				if err := d.Dispatch(ctx, n); err != nil {
					slog.Error(fmt.Sprintf("Notifications: dispatcher %q failed: %v", d.Name(), err), "component", "notification")
				}
			}
		}
	}
}

// Emit is a convenience method to emit an event through the bus.
func (ns *NotificationService) Emit(event *Event) {
	ns.bus.Emit(event)
}
