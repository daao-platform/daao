package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/daao/nexus/internal/session"
	"github.com/daao/nexus/internal/stream"
	"github.com/daao/nexus/internal/transport"
	"github.com/daao/nexus/proto"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSessionStore implements session.Store for testing TerminalStreamHandler
type mockSessionStore struct {
	sessions map[uuid.UUID]*session.Session
}

func newMockSessionStore() *mockSessionStore {
	return &mockSessionStore{
		sessions: make(map[uuid.UUID]*session.Session),
	}
}

func (m *mockSessionStore) CreateSession(ctx context.Context, s *session.Session) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	m.sessions[s.ID] = s
	return nil
}

func (m *mockSessionStore) GetSession(ctx context.Context, id uuid.UUID) (*session.Session, error) {
	s, ok := m.sessions[id]
	if !ok {
		return nil, errors.New("session not found")
	}
	return s, nil
}

func (m *mockSessionStore) UpdateSession(ctx context.Context, s *session.Session) error {
	m.sessions[s.ID] = s
	return nil
}

func (m *mockSessionStore) DeleteSession(ctx context.Context, id uuid.UUID) error {
	delete(m.sessions, id)
	return nil
}

func (m *mockSessionStore) ListSessionsBySatellite(ctx context.Context, satelliteID uuid.UUID) ([]*session.Session, error) {
	var result []*session.Session
	for _, s := range m.sessions {
		if s.SatelliteID == satelliteID {
			result = append(result, s)
		}
	}
	return result, nil
}

func (m *mockSessionStore) ListSessionsByUser(ctx context.Context, userID uuid.UUID) ([]*session.Session, error) {
	var result []*session.Session
	for _, s := range m.sessions {
		if s.UserID == userID {
			result = append(result, s)
		}
	}
	return result, nil
}

func (m *mockSessionStore) ListActiveSessions(ctx context.Context) ([]*session.Session, error) {
	var result []*session.Session
	for _, s := range m.sessions {
		if s.State != session.StateTerminated {
			result = append(result, s)
		}
	}
	return result, nil
}

func (m *mockSessionStore) ListActiveSessionsWithLimit(ctx context.Context, limit int) ([]*session.Session, error) {
	var result []*session.Session
	count := 0
	for _, s := range m.sessions {
		if s.State != session.StateTerminated {
			if count >= limit {
				break
			}
			result = append(result, s)
			count++
		}
	}
	return result, nil
}

func (m *mockSessionStore) ListActiveSessionsAfter(ctx context.Context, cursor string, limit int) ([]*session.Session, error) {
	return nil, nil
}

func (m *mockSessionStore) CountActiveSessions(ctx context.Context) (int, error) {
	count := 0
	for _, s := range m.sessions {
		if s.State != session.StateTerminated {
			count++
		}
	}
	return count, nil
}

func (m *mockSessionStore) TransitionSession(ctx context.Context, id uuid.UUID, newState session.SessionState) (*session.Session, error) {
	s, ok := m.sessions[id]
	if !ok {
		return nil, errors.New("session not found")
	}
	s.State = newState
	return s, nil
}

func (m *mockSessionStore) WriteEventLog(ctx context.Context, event *session.EventLog) error {
	return nil
}

func (m *mockSessionStore) GetEventLogs(ctx context.Context, sessionID uuid.UUID, limit int) ([]*session.EventLog, error) {
	return nil, nil
}

// wsTestSetup creates a TerminalStreamHandler with mocked dependencies and an httptest.Server
func wsTestSetup(t *testing.T) (*httptest.Server, *session.RingBufferPool, *stream.StreamRegistry, *mockSessionStore, func()) {
	t.Helper()

	store := newMockSessionStore()
	bufPool := session.NewRingBufferPool()
	registry := stream.NewStreamRegistry()

	// Pass nil for jwtValidator and false for oidcEnabled (dev mode, no auth required)
	handler := transport.NewTerminalStreamHandler(
		store,
		bufPool,
		registry,
		nil, // jwtValidator - nil for dev mode
		false, // oidcEnabled - false for dev mode
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/sessions/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/"), "/")
		if len(parts) >= 2 && parts[1] == "stream" {
			handler.HandleTerminalStream(w, r, parts[0])
		}
	})

	server := httptest.NewServer(mux)
	return server, bufPool, registry, store, func() { server.Close() }
}

func TestTerminalStream_FlushesRingBuffer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server, bufPool, _, store, cleanup := wsTestSetup(t)
	defer cleanup()

	sessID := uuid.New()
	store.sessions[sessID] = &session.Session{
		ID:    sessID,
		State: session.StateRunning,
	}

	// Pre-write data to ring buffer
	buf := bufPool.GetOrCreateBuffer(sessID.String())
	expectedData := []byte("pre-existing terminal output\r\n")
	buf.Write(expectedData)

	// Connect via WebSocket
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/sessions/" + sessID.String() + "/stream"
	ws, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	require.NoError(t, err)
	defer ws.Close()

	// Read the first binary frame — should contain ring buffer data
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	msgType, data, err := ws.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.BinaryMessage, msgType)
	assert.Contains(t, string(data), "pre-existing terminal output")
}

func TestTerminalStream_StreamsNewData(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server, bufPool, _, store, cleanup := wsTestSetup(t)
	defer cleanup()

	sessID := uuid.New()
	store.sessions[sessID] = &session.Session{
		ID:    sessID,
		State: session.StateRunning,
	}

	buf := bufPool.GetOrCreateBuffer(sessID.String())

	// Connect via WebSocket
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/sessions/" + sessID.String() + "/stream"
	ws, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	require.NoError(t, err)
	defer ws.Close()

	// Write new data after connection
	go func() {
		time.Sleep(10 * time.Millisecond)
		buf.Write([]byte("new live data\r\n"))
	}()

	// Read — should receive the new data within 50ms
	ws.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, data, err := ws.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(data), "new live data")
}

func TestTerminalStream_InputForwardedToStream(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server, _, registry, store, cleanup := wsTestSetup(t)
	defer cleanup()

	sessID := uuid.New()
	store.sessions[sessID] = &session.Session{
		ID:    sessID,
		State: session.StateRunning,
	}

	// Register a session stream to receive forwarded input
	satCh := make(chan *proto.NexusMessage, 10)
	registry.RegisterStream(sessID.String(), satCh)
	defer registry.UnregisterStream(sessID.String())

	// Connect via WebSocket
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/sessions/" + sessID.String() + "/stream"
	ws, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	require.NoError(t, err)
	defer ws.Close()

	// Send text input via WS
	err = ws.WriteMessage(websocket.TextMessage, []byte("ls -la\n"))
	require.NoError(t, err)

	// Assert input was forwarded to the stream registry
	select {
	case msg := <-satCh:
		assert.NotNil(t, msg, "expected a message forwarded to session stream")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for forwarded input on session stream")
	}
}

func TestTerminalStream_TerminatedSession_NoConnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server, _, _, store, cleanup := wsTestSetup(t)
	defer cleanup()

	sessID := uuid.New()
	store.sessions[sessID] = &session.Session{
		ID:    sessID,
		State: session.StateTerminated,
	}

	// Connect via WebSocket
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/sessions/" + sessID.String() + "/stream"
	ws, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	require.NoError(t, err)
	defer ws.Close()

	// Should immediately receive a terminated message
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := ws.ReadMessage()
	require.NoError(t, err)

	var msg map[string]string
	err = json.Unmarshal(data, &msg)
	require.NoError(t, err)
	assert.Equal(t, "terminated", msg["type"])
}

func TestTerminalStream_StatePolling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	server, _, _, store, cleanup := wsTestSetup(t)
	defer cleanup()

	sessID := uuid.New()
	store.sessions[sessID] = &session.Session{
		ID:    sessID,
		State: session.StateRunning,
	}

	// Connect via WebSocket
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/sessions/" + sessID.String() + "/stream"
	ws, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	require.NoError(t, err)
	defer ws.Close()

	// Transition to TERMINATED mid-stream
	go func() {
		time.Sleep(500 * time.Millisecond)
		store.sessions[sessID].State = session.StateTerminated
	}()

	// Read messages until we get the terminated notification
	ws.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			t.Fatalf("WS read error before receiving terminated: %v", err)
		}
		var msg map[string]string
		if json.Unmarshal(data, &msg) == nil && msg["type"] == "terminated" {
			// Success
			return
		}
	}
}

// TestTerminalStream_PingPong verifies that {"type":"ping"} is answered with
// {"type":"pong"} and is NOT forwarded to the satellite stream.
func TestTerminalStream_PingPong(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server, _, registry, store, cleanup := wsTestSetup(t)
	defer cleanup()

	sessID := uuid.New()
	store.sessions[sessID] = &session.Session{ID: sessID, State: session.StateRunning}

	// Register a channel to capture anything forwarded to the session
	satCh := make(chan *proto.NexusMessage, 8)
	registry.RegisterStream(sessID.String(), satCh)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/sessions/" + sessID.String() + "/stream"
	ws, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	require.NoError(t, err)
	defer ws.Close()

	// Send ping
	require.NoError(t, ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"ping"}`)))

	// Expect pong back as a JSON text frame
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := ws.ReadMessage()
	require.NoError(t, err)
	var msg map[string]string
	require.NoError(t, json.Unmarshal(data, &msg))
	assert.Equal(t, "pong", msg["type"], "server should respond with pong")

	// Nothing should have been forwarded to the satellite
	select {
	case fwd := <-satCh:
		t.Fatalf("ping was incorrectly forwarded to satellite: %v", fwd)
	case <-time.After(100 * time.Millisecond):
		// Correct — nothing forwarded
	}
}

// TestTerminalStream_ResizeRoutedAsResizeCommand verifies that a resize JSON
// message is routed as a ResizeCommand proto, not as raw TerminalInput.
func TestTerminalStream_ResizeRoutedAsResizeCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server, _, registry, store, cleanup := wsTestSetup(t)
	defer cleanup()

	sessID := uuid.New()
	store.sessions[sessID] = &session.Session{ID: sessID, State: session.StateRunning}

	satCh := make(chan *proto.NexusMessage, 8)
	registry.RegisterStream(sessID.String(), satCh)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/sessions/" + sessID.String() + "/stream"
	ws, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	require.NoError(t, err)
	defer ws.Close()

	// Send resize message
	require.NoError(t, ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"resize","cols":120,"rows":40}`)))

	// Expect a ResizeCommand (not TerminalInput) on the satellite channel
	select {
	case msg := <-satCh:
		rc := msg.GetResizeCommand()
		require.NotNil(t, rc, "expected ResizeCommand, got: %T", msg.Payload)
		assert.Equal(t, int32(120), rc.Width)
		assert.Equal(t, int32(40), rc.Height)
		assert.Equal(t, sessID.String(), rc.SessionId)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for ResizeCommand on session stream")
	}
}

// TestTerminalStream_KeystrokeRoutedAsTerminalInput verifies that plain
// keystrokes are forwarded as TerminalInput, not as ResizeCommand.
func TestTerminalStream_KeystrokeRoutedAsTerminalInput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server, _, registry, store, cleanup := wsTestSetup(t)
	defer cleanup()

	sessID := uuid.New()
	store.sessions[sessID] = &session.Session{ID: sessID, State: session.StateRunning}

	satCh := make(chan *proto.NexusMessage, 8)
	registry.RegisterStream(sessID.String(), satCh)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/sessions/" + sessID.String() + "/stream"
	ws, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	require.NoError(t, err)
	defer ws.Close()

	// Send raw keystroke (binary)
	require.NoError(t, ws.WriteMessage(websocket.BinaryMessage, []byte("hello")))

	select {
	case msg := <-satCh:
		ti := msg.GetTerminalInput()
		require.NotNil(t, ti, "expected TerminalInput, got: %T", msg.Payload)
		assert.Equal(t, []byte("hello"), ti.Data)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for TerminalInput on session stream")
	}
}
