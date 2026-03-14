package notification

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Mock Store
// ---------------------------------------------------------------------------

type mockStore struct {
	mu            sync.Mutex
	notifications []*Notification
	prefs         map[uuid.UUID]*UserPreferences
	createErr     error
}

func newMockStore() *mockStore {
	return &mockStore{
		prefs: make(map[uuid.UUID]*UserPreferences),
	}
}

func (m *mockStore) Create(ctx context.Context, n *Notification) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications = append(m.notifications, n)
	return nil
}

func (m *mockStore) ListByUser(ctx context.Context, userID uuid.UUID, limit int, cursor *time.Time) ([]*Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*Notification
	for _, n := range m.notifications {
		if n.UserID == userID {
			result = append(result, n)
		}
	}
	return result, nil
}

func (m *mockStore) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, n := range m.notifications {
		if n.UserID == userID && !n.Read {
			count++
		}
	}
	return count, nil
}

func (m *mockStore) MarkRead(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range m.notifications {
		if n.ID == id {
			n.Read = true
		}
	}
	return nil
}

func (m *mockStore) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range m.notifications {
		if n.UserID == userID {
			n.Read = true
		}
	}
	return nil
}

func (m *mockStore) GetPreferences(ctx context.Context, userID uuid.UUID) (*UserPreferences, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.prefs[userID]
	if !ok {
		return nil, nil
	}
	return p, nil
}

func (m *mockStore) UpdatePreferences(ctx context.Context, prefs *UserPreferences) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prefs[prefs.UserID] = prefs
	return nil
}

// ---------------------------------------------------------------------------
// Mock Dispatcher
// ---------------------------------------------------------------------------

type mockDispatcher struct {
	name         string
	dispatched   []*Notification
	mu           sync.Mutex
	shouldReturn bool
	dispatchErr  error
}

func newMockDispatcher(name string) *mockDispatcher {
	return &mockDispatcher{name: name, shouldReturn: true}
}

func (d *mockDispatcher) Name() string { return d.name }

func (d *mockDispatcher) Dispatch(ctx context.Context, n *Notification) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dispatched = append(d.dispatched, n)
	return d.dispatchErr
}

func (d *mockDispatcher) ShouldDispatch(n *Notification, prefs *UserPreferences) bool {
	return d.shouldReturn
}

func (d *mockDispatcher) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.dispatched)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestEventBus_Emit(t *testing.T) {
	bus := NewEventBus()
	received := make(chan *Event, 1)

	bus.Subscribe(func(e *Event) {
		received <- e
	})

	event := &Event{
		Type:     EventSessionTerminated,
		Priority: PriorityInfo,
		Title:    "Session ended",
		Body:     "test-session has terminated",
	}

	bus.Emit(event)

	select {
	case got := <-received:
		if got.Type != EventSessionTerminated {
			t.Errorf("expected type %s, got %s", EventSessionTerminated, got.Type)
		}
		if got.Title != "Session ended" {
			t.Errorf("expected title 'Session ended', got '%s'", got.Title)
		}
		if got.CreatedAt.IsZero() {
			t.Error("expected CreatedAt to be auto-set")
		}
	default:
		t.Fatal("event was not received by subscriber")
	}
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	bus := NewEventBus()
	count := 0
	var mu sync.Mutex

	for i := 0; i < 3; i++ {
		bus.Subscribe(func(e *Event) {
			mu.Lock()
			count++
			mu.Unlock()
		})
	}

	bus.Emit(&Event{Type: EventSatelliteOffline, Priority: PriorityCritical, Title: "test"})

	mu.Lock()
	defer mu.Unlock()
	if count != 3 {
		t.Errorf("expected 3 subscribers to fire, got %d", count)
	}
}

func TestEventBus_PanicRecovery(t *testing.T) {
	bus := NewEventBus()
	called := false

	// First subscriber panics
	bus.Subscribe(func(e *Event) {
		panic("boom")
	})

	// Second subscriber should still be called
	bus.Subscribe(func(e *Event) {
		called = true
	})

	bus.Emit(&Event{Type: EventSessionError, Priority: PriorityWarning, Title: "test"})

	if !called {
		t.Error("second subscriber should have been called despite first panicking")
	}
}

func TestNotificationService_PersistAndDispatch(t *testing.T) {
	store := newMockStore()
	bus := NewEventBus()
	ns := NewNotificationService(bus, store)

	dispatcher := newMockDispatcher("test")
	ns.RegisterDispatcher(dispatcher)

	userID := uuid.MustParse("00000000-0000-0000-0000-000000000000")
	sessionID := uuid.New()

	ns.Emit(&Event{
		Type:      EventSessionTerminated,
		Priority:  PriorityInfo,
		SessionID: &sessionID,
		UserID:    &userID,
		Title:     "Session ended",
		Body:      "test-session terminated normally",
	})

	// Check persistence
	if len(store.notifications) != 1 {
		t.Fatalf("expected 1 notification in store, got %d", len(store.notifications))
	}
	n := store.notifications[0]
	if n.Type != EventSessionTerminated {
		t.Errorf("expected type %s, got %s", EventSessionTerminated, n.Type)
	}
	if n.UserID != userID {
		t.Errorf("expected userID %s, got %s", userID, n.UserID)
	}

	// Check dispatch
	if dispatcher.count() != 1 {
		t.Errorf("expected 1 dispatch, got %d", dispatcher.count())
	}
}

func TestSSEDispatcher_ShouldDispatch_Preferences(t *testing.T) {
	hub := NewSSEHub()
	d := NewSSEDispatcher(hub)

	tests := []struct {
		name     string
		notif    *Notification
		prefs    *UserPreferences
		expected bool
	}{
		{
			name:  "all enabled, info priority",
			notif: &Notification{Type: EventSessionTerminated, Priority: PriorityInfo},
			prefs: &UserPreferences{
				BrowserEnabled: true, SessionTerminated: true,
				SessionError: true, SatelliteOffline: true, SessionSuspended: true,
				MinPriority: PriorityInfo,
			},
			expected: true,
		},
		{
			name:  "browser disabled",
			notif: &Notification{Type: EventSessionTerminated, Priority: PriorityInfo},
			prefs: &UserPreferences{
				BrowserEnabled: false, SessionTerminated: true,
				MinPriority: PriorityInfo,
			},
			expected: false,
		},
		{
			name:  "event type disabled",
			notif: &Notification{Type: EventSessionTerminated, Priority: PriorityInfo},
			prefs: &UserPreferences{
				BrowserEnabled: true, SessionTerminated: false,
				MinPriority: PriorityInfo,
			},
			expected: false,
		},
		{
			name:  "priority too low",
			notif: &Notification{Type: EventSessionTerminated, Priority: PriorityInfo},
			prefs: &UserPreferences{
				BrowserEnabled: true, SessionTerminated: true,
				MinPriority: PriorityWarning,
			},
			expected: false,
		},
		{
			name:  "critical meets warning threshold",
			notif: &Notification{Type: EventSatelliteOffline, Priority: PriorityCritical},
			prefs: &UserPreferences{
				BrowserEnabled: true, SatelliteOffline: true,
				MinPriority: PriorityWarning,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.ShouldDispatch(tt.notif, tt.prefs)
			if got != tt.expected {
				t.Errorf("ShouldDispatch() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSSEHub_SubscribeAndBroadcast(t *testing.T) {
	hub := NewSSEHub()
	userID := uuid.New()

	ch := hub.Subscribe(userID)
	defer hub.Unsubscribe(userID, ch)

	n := &Notification{
		ID:       uuid.New(),
		UserID:   userID,
		Type:     EventSatelliteOffline,
		Priority: PriorityCritical,
		Title:    "Satellite offline",
	}

	hub.Broadcast(userID, n)

	select {
	case got := <-ch:
		if got.ID != n.ID {
			t.Errorf("expected notification ID %s, got %s", n.ID, got.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notification")
	}
}

func TestSSEHub_MultipleClients(t *testing.T) {
	hub := NewSSEHub()
	userID := uuid.New()

	ch1 := hub.Subscribe(userID)
	ch2 := hub.Subscribe(userID)
	defer hub.Unsubscribe(userID, ch1)
	defer hub.Unsubscribe(userID, ch2)

	n := &Notification{ID: uuid.New(), UserID: userID, Title: "test"}
	hub.Broadcast(userID, n)

	// Both clients should receive
	for i, ch := range []chan *Notification{ch1, ch2} {
		select {
		case got := <-ch:
			if got.ID != n.ID {
				t.Errorf("client %d: expected ID %s, got %s", i, n.ID, got.ID)
			}
		case <-time.After(time.Second):
			t.Fatalf("client %d: timed out", i)
		}
	}
}

func TestSSEHub_NoLeakOnUnsubscribe(t *testing.T) {
	hub := NewSSEHub()
	userID := uuid.New()

	ch := hub.Subscribe(userID)
	hub.Unsubscribe(userID, ch)

	// After unsubscribe, the user should have no clients
	hub.mu.RLock()
	_, exists := hub.clients[userID]
	hub.mu.RUnlock()

	if exists {
		t.Error("expected user to be removed from clients map after last unsubscribe")
	}
}

func TestDefaultPreferences(t *testing.T) {
	uid := uuid.New()
	p := DefaultPreferences(uid)

	if p.UserID != uid {
		t.Errorf("expected UserID %s, got %s", uid, p.UserID)
	}
	if !p.BrowserEnabled {
		t.Error("expected BrowserEnabled to default true")
	}
	if !p.SessionTerminated {
		t.Error("expected SessionTerminated to default true")
	}
	if p.MinPriority != PriorityInfo {
		t.Errorf("expected MinPriority INFO, got %s", p.MinPriority)
	}
}
