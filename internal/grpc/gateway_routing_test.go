package grpc

import (
	"context"
	"io"
	"sync"
	"testing"

	"github.com/daao/nexus/internal/session"
	"github.com/daao/nexus/pkg/buffer"
	"github.com/daao/nexus/proto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// --- Mock implementations ---

type mockDB struct {
	mu       sync.Mutex
	execLog  []string
	queryRow func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (m *mockDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	m.mu.Lock()
	m.execLog = append(m.execLog, sql)
	m.mu.Unlock()
	return pgconn.CommandTag{}, nil
}

func (m *mockDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRow != nil {
		return m.queryRow(ctx, sql, args...)
	}
	return &mockRow{err: pgx.ErrNoRows}
}

func (m *mockDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return &mockRows{}, nil
}

// mockRows implements pgx.Rows with no results (empty result set).
type mockRows struct{}

func (r *mockRows) Close()                                       {}
func (r *mockRows) Err() error                                   { return nil }
func (r *mockRows) Next() bool                                   { return false }
func (r *mockRows) Scan(dest ...any) error                       { return nil }
func (r *mockRows) Values() ([]any, error)                       { return nil, nil }
func (r *mockRows) RawValues() [][]byte                          { return nil }
func (r *mockRows) Conn() *pgx.Conn                              { return nil }
func (r *mockRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *mockRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

type mockRow struct {
	err error
}

func (r *mockRow) Scan(dest ...any) error { return r.err }

type mockSessionStore struct {
	sessions map[uuid.UUID]*session.Session
	mu       sync.Mutex
}

func (m *mockSessionStore) GetSession(ctx context.Context, id uuid.UUID) (*session.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		return s, nil
	}
	return nil, session.ErrSessionNotFound
}

func (m *mockSessionStore) UpdateState(ctx context.Context, id uuid.UUID, state session.SessionState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		s.State = state
		return nil
	}
	return session.ErrSessionNotFound
}

func (m *mockSessionStore) TransitionSession(ctx context.Context, id uuid.UUID, state session.SessionState) (*session.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		s.State = state
		return s, nil
	}
	return nil, session.ErrSessionNotFound
}

func (m *mockSessionStore) WriteEventLog(ctx context.Context, event *session.EventLog) error {
	return nil
}

type mockStreamRegistry struct {
	registered map[string]bool
	mu         sync.Mutex
}

func newMockStreamRegistry() *mockStreamRegistry {
	return &mockStreamRegistry{registered: make(map[string]bool)}
}

func (m *mockStreamRegistry) RegisterStream(sessionID string, ch chan<- *proto.NexusMessage) {
	m.mu.Lock()
	m.registered[sessionID] = true
	m.mu.Unlock()
}

func (m *mockStreamRegistry) UnregisterStream(sessionID string) {
	m.mu.Lock()
	delete(m.registered, sessionID)
	m.mu.Unlock()
}

func (m *mockStreamRegistry) RegisterSatelliteStream(satelliteID string, ch chan<- *proto.NexusMessage) {
	m.mu.Lock()
	m.registered["sat:"+satelliteID] = true
	m.mu.Unlock()
}

func (m *mockStreamRegistry) UnregisterSatelliteStream(satelliteID string) {
	m.mu.Lock()
	delete(m.registered, "sat:"+satelliteID)
	m.mu.Unlock()
}

func (m *mockStreamRegistry) SendToSession(sessionID string, msg *proto.NexusMessage) bool {
	return true
}

func (m *mockStreamRegistry) SendToSatellite(satelliteID string, msg *proto.NexusMessage) bool {
	return true
}

type mockRingBufferPool struct {
	buffers map[string]*buffer.RingBuffer
	mu      sync.Mutex
}

func newMockRingBufferPool() *mockRingBufferPool {
	return &mockRingBufferPool{buffers: make(map[string]*buffer.RingBuffer)}
}

func (m *mockRingBufferPool) GetOrCreateBuffer(sessionID string) *buffer.RingBuffer {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok := m.buffers[sessionID]; ok {
		return b
	}
	b := buffer.NewRingBuffer(4096)
	m.buffers[sessionID] = b
	return b
}

func (m *mockRingBufferPool) RemoveBuffer(sessionID string) {
	m.mu.Lock()
	delete(m.buffers, sessionID)
	m.mu.Unlock()
}

// mockConnectStream simulates a gRPC bidirectional stream for testing
type mockConnectStream struct {
	grpc.ServerStream
	ctx      context.Context
	messages []*proto.SatelliteMessage
	idx      int
	sent     []*proto.NexusMessage
	mu       sync.Mutex
}

func (s *mockConnectStream) Context() context.Context { return s.ctx }

func (s *mockConnectStream) Send(msg *proto.NexusMessage) error {
	s.mu.Lock()
	s.sent = append(s.sent, msg)
	s.mu.Unlock()
	return nil
}

func (s *mockConnectStream) Recv() (*proto.SatelliteMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idx >= len(s.messages) {
		return nil, io.EOF
	}
	msg := s.messages[s.idx]
	s.idx++
	return msg, nil
}

func (s *mockConnectStream) SetHeader(md metadata.MD) error  { return nil }
func (s *mockConnectStream) SendHeader(md metadata.MD) error { return nil }
func (s *mockConnectStream) SetTrailer(md metadata.MD)       {}
func (s *mockConnectStream) SendMsg(m any) error             { return nil }
func (s *mockConnectStream) RecvMsg(m any) error             { return nil }

// --- Tests ---

func TestConnect_RegisterRequest(t *testing.T) {
	db := &mockDB{}
	sr := newMockStreamRegistry()
	gw := NewSatelliteGatewayServerImpl(nil, sr, db, nil, nil, nil, nil, nil)

	stream := &mockConnectStream{
		ctx: context.Background(),
		messages: []*proto.SatelliteMessage{
			{Payload: &proto.SatelliteMessage_RegisterRequest{
				RegisterRequest: &proto.RegisterRequest{
					SatelliteId: "sat-test-123",
					Fingerprint: "fp-abc",
					Version:     "1.0.0",
					Os:          "linux",
					Arch:        "amd64",
				},
			}},
		},
	}

	err := gw.Connect(stream)
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected error: %v", err)
	}

	// The stream registry gets cleaned up in Connect's defer, so we verify
	// via the DB exec log — RegisterRequest does a QueryRow (UPDATE...RETURNING)
	// and on disconnect it does an Exec (UPDATE...SET status='offline').
	db.mu.Lock()
	defer db.mu.Unlock()
	if len(db.execLog) == 0 {
		t.Error("Expected at least one DB exec call from register/disconnect")
	}
}

func TestConnect_TerminalData_WritesToRingBuffer(t *testing.T) {
	rbp := newMockRingBufferPool()
	sr := newMockStreamRegistry()
	gw := NewSatelliteGatewayServerImpl(nil, sr, nil, rbp, nil, nil, nil, nil)

	sessID := uuid.New().String()
	stream := &mockConnectStream{
		ctx: context.Background(),
		messages: []*proto.SatelliteMessage{
			{Payload: &proto.SatelliteMessage_TerminalData{
				TerminalData: &proto.TerminalData{
					SessionId: sessID,
					Data:      []byte("hello from satellite"),
				},
			}},
		},
	}

	err := gw.Connect(stream)
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected error: %v", err)
	}

	// Ring buffer should have the data
	rbp.mu.Lock()
	defer rbp.mu.Unlock()
	rb, ok := rbp.buffers[sessID]
	if !ok {
		t.Fatal("Expected ring buffer to be created for session")
	}
	data := rb.Snapshot()
	if string(data) != "hello from satellite" {
		t.Errorf("Expected 'hello from satellite' in buffer, got '%s'", string(data))
	}
}

func TestConnect_HeartbeatPing_UpdatesDB(t *testing.T) {
	db := &mockDB{}
	sr := newMockStreamRegistry()
	gw := NewSatelliteGatewayServerImpl(nil, sr, db, nil, nil, nil, nil, nil)

	stream := &mockConnectStream{
		ctx: context.Background(),
		messages: []*proto.SatelliteMessage{
			// Register first to set satelliteID
			{Payload: &proto.SatelliteMessage_RegisterRequest{
				RegisterRequest: &proto.RegisterRequest{
					SatelliteId: "sat-heartbeat-test",
				},
			}},
			// Then send heartbeat
			{Payload: &proto.SatelliteMessage_HeartbeatPing{
				HeartbeatPing: &proto.HeartbeatPing{},
			}},
		},
	}

	err := gw.Connect(stream)
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected error: %v", err)
	}

	// DB should have received exec calls (register + heartbeat)
	db.mu.Lock()
	defer db.mu.Unlock()
	if len(db.execLog) == 0 {
		t.Error("Expected at least one DB exec call for heartbeat")
	}
}

func TestConnect_UnknownMessage_DoesNotCrash(t *testing.T) {
	sr := newMockStreamRegistry()
	gw := NewSatelliteGatewayServerImpl(nil, sr, nil, nil, nil, nil, nil, nil)

	// Send a message with nil payload — this exercises the default case
	stream := &mockConnectStream{
		ctx: context.Background(),
		messages: []*proto.SatelliteMessage{
			{}, // empty payload
		},
	}

	err := gw.Connect(stream)
	// Should not crash, just return cleanly on EOF
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConnect_RegisterRequest_SendsReconciliation(t *testing.T) {
	db := &mockDB{}
	sr := newMockStreamRegistry()
	gw := NewSatelliteGatewayServerImpl(nil, sr, db, nil, nil, nil, nil, nil)

	stream := &mockConnectStream{
		ctx: context.Background(),
		messages: []*proto.SatelliteMessage{
			{Payload: &proto.SatelliteMessage_RegisterRequest{
				RegisterRequest: &proto.RegisterRequest{
					SatelliteId: "sat-reconcile-test",
					Fingerprint: "fp-reconcile",
				},
			}},
		},
	}

	err := gw.Connect(stream)
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that a SessionReconciliation message was sent
	stream.mu.Lock()
	defer stream.mu.Unlock()
	found := false
	for _, msg := range stream.sent {
		if msg.GetSessionReconciliation() != nil {
			found = true
			// With mockRows returning no rows, active list should be empty
			if len(msg.GetSessionReconciliation().ActiveSessionIds) != 0 {
				t.Errorf("expected 0 active sessions, got %d", len(msg.GetSessionReconciliation().ActiveSessionIds))
			}
			break
		}
	}
	if !found {
		t.Error("Expected SessionReconciliation message to be sent after registration")
	}
}
