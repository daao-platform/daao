// Package api provides HTTP handlers for the Nexus API Gateway.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/daao/nexus/internal/session"
	"github.com/google/uuid"
)

// ============================================================================
// Mock Implementations
// ============================================================================

// mockSessionStore implements session.Store for testing
type mockSessionStore struct {
	sessions      map[uuid.UUID]*session.Session
	listActiveErr error
	createErr     error
	getErr        error
	updateErr     error
	deleteErr     error
	transitionErr error
	writeEventErr error
}

func newMockSessionStore() *mockSessionStore {
	return &mockSessionStore{
		sessions: make(map[uuid.UUID]*session.Session),
	}
}

func (m *mockSessionStore) CreateSession(ctx context.Context, s *session.Session) error {
	if m.createErr != nil {
		return m.createErr
	}
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	m.sessions[s.ID] = s
	return nil
}

func (m *mockSessionStore) GetSession(ctx context.Context, id uuid.UUID) (*session.Session, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	s, ok := m.sessions[id]
	if !ok {
		return nil, errors.New("session not found")
	}
	return s, nil
}

func (m *mockSessionStore) UpdateSession(ctx context.Context, s *session.Session) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.sessions[s.ID] = s
	return nil
}

func (m *mockSessionStore) DeleteSession(ctx context.Context, id uuid.UUID) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
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
	if m.listActiveErr != nil {
		return nil, m.listActiveErr
	}
	var result []*session.Session
	for _, s := range m.sessions {
		if s.State != session.StateTerminated {
			result = append(result, s)
		}
	}
	return result, nil
}

func (m *mockSessionStore) TransitionSession(ctx context.Context, id uuid.UUID, newState session.SessionState) (*session.Session, error) {
	if m.transitionErr != nil {
		return nil, m.transitionErr
	}
	s, ok := m.sessions[id]
	if !ok {
		return nil, errors.New("session not found")
	}
	s.State = newState
	return s, nil
}

func (m *mockSessionStore) WriteEventLog(ctx context.Context, event *session.EventLog) error {
	return m.writeEventErr
}

func (m *mockSessionStore) GetEventLogs(ctx context.Context, sessionID uuid.UUID, limit int) ([]*session.EventLog, error) {
	return nil, nil
}

// ListActiveSessionsWithLimit retrieves active sessions with a limit
func (m *mockSessionStore) ListActiveSessionsWithLimit(ctx context.Context, limit int) ([]*session.Session, error) {
	if m.listActiveErr != nil {
		return nil, m.listActiveErr
	}
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

// ListActiveSessionsAfter retrieves active sessions after a cursor
func (m *mockSessionStore) ListActiveSessionsAfter(ctx context.Context, cursor string, limit int) ([]*session.Session, error) {
	if m.listActiveErr != nil {
		return nil, m.listActiveErr
	}
	cursorUUID, err := uuid.Parse(cursor)
	if err != nil {
		return nil, err
	}
	var result []*session.Session
	count := 0
	for _, s := range m.sessions {
		if s.State != session.StateTerminated && s.ID.String() < cursorUUID.String() {
			if count >= limit {
				break
			}
			result = append(result, s)
			count++
		}
	}
	return result, nil
}

// CountActiveSessions returns the count of active sessions
func (m *mockSessionStore) CountActiveSessions(ctx context.Context) (int, error) {
	count := 0
	for _, s := range m.sessions {
		if s.State != session.StateTerminated {
			count++
		}
	}
	return count, nil
}

// mockConfig implements ConfigAccessor for testing
type mockConfig struct {
	serverCert string
	serverKey  string
	clientCAs  string
	listenAddr string
	grpcAddr   string
}

func (m *mockConfig) GetServerCert() string { return m.serverCert }
func (m *mockConfig) GetServerKey() string  { return m.serverKey }
func (m *mockConfig) GetClientCAs() string  { return m.clientCAs }
func (m *mockConfig) GetListenAddr() string { return m.listenAddr }
func (m *mockConfig) GetGRPCAddr() string   { return m.grpcAddr }

// ============================================================================
// Health Handler Tests
// ============================================================================

func TestHandleHealth_Success(t *testing.T) {
	handlers := NewHandlers(nil, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("GET", "/health", nil)

	handlers.HandleHealth(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp HealthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Status != "healthy" {
		t.Errorf("expected status 'healthy', got '%s'", resp.Status)
	}
}

func TestHandleHealth_ContentType(t *testing.T) {
	handlers := NewHandlers(nil, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("GET", "/health", nil)

	handlers.HandleHealth(w, r)

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", contentType)
	}
}

// ============================================================================
// ListSessions Handler Tests
// ============================================================================

func TestHandleListSessions_Success(t *testing.T) {
	mockStore := newMockSessionStore()
	satID := uuid.New()
	userID := uuid.New()
	mockStore.sessions[uuid.New()] = &session.Session{
		ID:          uuid.New(),
		SatelliteID: satID,
		UserID:      userID,
		Name:        "test-session",
		State:       session.StateRunning,
		CreatedAt:   time.Now(),
	}

	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("GET", "/sessions", nil)

	handlers.HandleListSessions(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleListSessions_Empty(t *testing.T) {
	mockStore := newMockSessionStore()
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("GET", "/sessions", nil)

	handlers.HandleListSessions(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleListSessions_StoreNotConfigured(t *testing.T) {
	handlers := NewHandlers(nil, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("GET", "/sessions", nil)

	handlers.HandleListSessions(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

func TestHandleListSessions_StoreError(t *testing.T) {
	mockStore := newMockSessionStore()
	mockStore.listActiveErr = errors.New("database error")
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("GET", "/sessions", nil)

	handlers.HandleListSessions(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

// ============================================================================
// CreateSession Handler Tests
// ============================================================================

func TestHandleCreateSession_Success(t *testing.T) {
	mockStore := newMockSessionStore()
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)

	body := `{"satellite_id":"` + uuid.New().String() + `","name":"test-session","agent_binary":"/bin/bash"}`
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/sessions", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")

	handlers.HandleCreateSession(w, r)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}
}

func TestHandleCreateSession_InvalidJSON(t *testing.T) {
	mockStore := newMockSessionStore()
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)

	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/sessions", bytes.NewBufferString("invalid json"))
	r.Header.Set("Content-Type", "application/json")

	handlers.HandleCreateSession(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleCreateSession_InvalidSatelliteID(t *testing.T) {
	mockStore := newMockSessionStore()
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)

	body := `{"satellite_id":"invalid-uuid","name":"test-session"}`
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/sessions", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")

	handlers.HandleCreateSession(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleCreateSession_MissingName(t *testing.T) {
	mockStore := newMockSessionStore()
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)

	body := `{"satellite_id":"` + uuid.New().String() + `"}`
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/sessions", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")

	handlers.HandleCreateSession(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleCreateSession_StoreError(t *testing.T) {
	mockStore := newMockSessionStore()
	mockStore.createErr = errors.New("database error")
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)

	body := `{"satellite_id":"` + uuid.New().String() + `","name":"test-session"}`
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/sessions", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")

	handlers.HandleCreateSession(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

func TestHandleCreateSession_StoreNotConfigured(t *testing.T) {
	handlers := NewHandlers(nil, nil, nil, nil, nil, nil)

	body := `{"satellite_id":"` + uuid.New().String() + `","name":"test-session"}`
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/sessions", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")

	handlers.HandleCreateSession(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

// ============================================================================
// GetSession Handler Tests
// ============================================================================

func TestHandleGetSession_Success(t *testing.T) {
	mockStore := newMockSessionStore()
	sessID := uuid.New()
	satID := uuid.New()
	userID := uuid.New()
	mockStore.sessions[sessID] = &session.Session{
		ID:          sessID,
		SatelliteID: satID,
		UserID:      userID,
		Name:        "test-session",
		State:       session.StateRunning,
		CreatedAt:   time.Now(),
	}

	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("GET", "/sessions/"+sessID.String(), nil)

	handlers.HandleGetSession(w, r, sessID.String())

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleGetSession_NotFound(t *testing.T) {
	mockStore := newMockSessionStore()
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("GET", "/sessions/"+uuid.New().String(), nil)

	handlers.HandleGetSession(w, r, uuid.New().String())

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestHandleGetSession_InvalidID(t *testing.T) {
	mockStore := newMockSessionStore()
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("GET", "/sessions/invalid-uuid", nil)

	handlers.HandleGetSession(w, r, "invalid-uuid")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleGetSession_StoreNotConfigured(t *testing.T) {
	handlers := NewHandlers(nil, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("GET", "/sessions/"+uuid.New().String(), nil)

	handlers.HandleGetSession(w, r, uuid.New().String())

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

// ============================================================================
// AttachSession Handler Tests
// ============================================================================

func TestHandleAttachSession_Success(t *testing.T) {
	mockStore := newMockSessionStore()
	sessID := uuid.New()
	satID := uuid.New()
	userID := uuid.New()
	mockStore.sessions[sessID] = &session.Session{
		ID:          sessID,
		SatelliteID: satID,
		UserID:      userID,
		Name:        "test-session",
		State:       session.StateProvisioning,
		CreatedAt:   time.Now(),
	}

	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/sessions/"+sessID.String()+"/attach", nil)

	handlers.HandleAttachSession(w, r, sessID.String())

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleAttachSession_NotFound(t *testing.T) {
	mockStore := newMockSessionStore()
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/sessions/"+uuid.New().String()+"/attach", nil)

	handlers.HandleAttachSession(w, r, uuid.New().String())

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestHandleAttachSession_InvalidID(t *testing.T) {
	mockStore := newMockSessionStore()
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/sessions/invalid-uuid/attach", nil)

	handlers.HandleAttachSession(w, r, "invalid-uuid")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleAttachSession_TransitionError(t *testing.T) {
	mockStore := newMockSessionStore()
	sessID := uuid.New()
	mockStore.sessions[sessID] = &session.Session{
		ID:          sessID,
		SatelliteID: uuid.New(),
		UserID:      uuid.New(),
		Name:        "test-session",
		State:       session.StateProvisioning,
		CreatedAt:   time.Now(),
	}
	mockStore.transitionErr = errors.New("invalid transition")

	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/sessions/"+sessID.String()+"/attach", nil)

	handlers.HandleAttachSession(w, r, sessID.String())

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

// ============================================================================
// DetachSession Handler Tests
// ============================================================================

func TestHandleDetachSession_Success(t *testing.T) {
	mockStore := newMockSessionStore()
	sessID := uuid.New()
	satID := uuid.New()
	userID := uuid.New()
	mockStore.sessions[sessID] = &session.Session{
		ID:          sessID,
		SatelliteID: satID,
		UserID:      userID,
		Name:        "test-session",
		State:       session.StateRunning,
		CreatedAt:   time.Now(),
	}

	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/sessions/"+sessID.String()+"/detach", nil)

	handlers.HandleDetachSession(w, r, sessID.String())

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleDetachSession_NotFound(t *testing.T) {
	mockStore := newMockSessionStore()
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/sessions/"+uuid.New().String()+"/detach", nil)

	handlers.HandleDetachSession(w, r, uuid.New().String())

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestHandleDetachSession_InvalidID(t *testing.T) {
	mockStore := newMockSessionStore()
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/sessions/invalid-uuid/detach", nil)

	handlers.HandleDetachSession(w, r, "invalid-uuid")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// ============================================================================
// SuspendSession Handler Tests
// ============================================================================

func TestHandleSuspendSession_Success(t *testing.T) {
	mockStore := newMockSessionStore()
	sessID := uuid.New()
	satID := uuid.New()
	userID := uuid.New()
	mockStore.sessions[sessID] = &session.Session{
		ID:          sessID,
		SatelliteID: satID,
		UserID:      userID,
		Name:        "test-session",
		State:       session.StateRunning,
		CreatedAt:   time.Now(),
	}

	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/sessions/"+sessID.String()+"/suspend", nil)

	handlers.HandleSuspendSession(w, r, sessID.String())

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleSuspendSession_NotFound(t *testing.T) {
	mockStore := newMockSessionStore()
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/sessions/"+uuid.New().String()+"/suspend", nil)

	handlers.HandleSuspendSession(w, r, uuid.New().String())

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestHandleSuspendSession_InvalidID(t *testing.T) {
	mockStore := newMockSessionStore()
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/sessions/invalid-uuid/suspend", nil)

	handlers.HandleSuspendSession(w, r, "invalid-uuid")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// ============================================================================
// ResumeSession Handler Tests
// ============================================================================

func TestHandleResumeSession_Success(t *testing.T) {
	mockStore := newMockSessionStore()
	sessID := uuid.New()
	satID := uuid.New()
	userID := uuid.New()
	mockStore.sessions[sessID] = &session.Session{
		ID:          sessID,
		SatelliteID: satID,
		UserID:      userID,
		Name:        "test-session",
		State:       session.StateSuspended,
		CreatedAt:   time.Now(),
	}

	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/sessions/"+sessID.String()+"/resume", nil)

	handlers.HandleResumeSession(w, r, sessID.String())

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleResumeSession_NotFound(t *testing.T) {
	mockStore := newMockSessionStore()
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/sessions/"+uuid.New().String()+"/resume", nil)

	handlers.HandleResumeSession(w, r, uuid.New().String())

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestHandleResumeSession_InvalidID(t *testing.T) {
	mockStore := newMockSessionStore()
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/sessions/invalid-uuid/resume", nil)

	handlers.HandleResumeSession(w, r, "invalid-uuid")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// ============================================================================
// KillSession Handler Tests
// ============================================================================

func TestHandleKillSession_Success(t *testing.T) {
	mockStore := newMockSessionStore()
	sessID := uuid.New()
	satID := uuid.New()
	userID := uuid.New()
	mockStore.sessions[sessID] = &session.Session{
		ID:          sessID,
		SatelliteID: satID,
		UserID:      userID,
		Name:        "test-session",
		State:       session.StateRunning,
		CreatedAt:   time.Now(),
	}

	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("DELETE", "/sessions/"+sessID.String(), nil)

	handlers.HandleKillSession(w, r, sessID.String())

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleKillSession_NotFound(t *testing.T) {
	mockStore := newMockSessionStore()
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("DELETE", "/sessions/"+uuid.New().String(), nil)

	handlers.HandleKillSession(w, r, uuid.New().String())

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestHandleKillSession_InvalidID(t *testing.T) {
	mockStore := newMockSessionStore()
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("DELETE", "/sessions/invalid-uuid", nil)

	handlers.HandleKillSession(w, r, "invalid-uuid")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleKillSession_TransitionError(t *testing.T) {
	mockStore := newMockSessionStore()
	sessID := uuid.New()
	mockStore.sessions[sessID] = &session.Session{
		ID:          sessID,
		SatelliteID: uuid.New(),
		UserID:      uuid.New(),
		Name:        "test-session",
		State:       session.StateRunning,
		CreatedAt:   time.Now(),
	}
	mockStore.transitionErr = errors.New("invalid transition")

	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("DELETE", "/sessions/"+sessID.String(), nil)

	handlers.HandleKillSession(w, r, sessID.String())

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

// ============================================================================
// ListSatellites Handler Tests
// ============================================================================

func TestHandleListSatellites_Success(t *testing.T) {
	handlers := NewHandlers(nil, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("GET", "/satellites", nil)

	handlers.HandleListSatellites(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// ============================================================================
// CreateSatellite Handler Tests
// ============================================================================

func TestHandleCreateSatellite_Success(t *testing.T) {
	handlers := NewHandlers(nil, nil, nil, nil, nil, nil)

	body := `{"name":"test-satellite","region":"us-east-1","endpoint":"http://localhost:8080"}`
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/satellites", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")

	handlers.HandleCreateSatellite(w, r)

	// No DB pool injected, so should return 503
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestHandleCreateSatellite_InvalidJSON(t *testing.T) {
	handlers := NewHandlers(nil, nil, nil, nil, nil, nil)

	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/satellites", bytes.NewBufferString("invalid json"))
	r.Header.Set("Content-Type", "application/json")

	handlers.HandleCreateSatellite(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleCreateSatellite_MissingName(t *testing.T) {
	handlers := NewHandlers(nil, nil, nil, nil, nil, nil)

	body := `{"region":"us-east-1"}`
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/satellites", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")

	handlers.HandleCreateSatellite(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// ============================================================================
// DeleteSession Handler Tests
// ============================================================================

func TestHandleDeleteSession_Success(t *testing.T) {
	mockStore := newMockSessionStore()
	sessID := uuid.New()
	satID := uuid.New()
	userID := uuid.New()
	mockStore.sessions[sessID] = &session.Session{
		ID:          sessID,
		SatelliteID: satID,
		UserID:      userID,
		Name:        "test-session",
		State:       session.StateTerminated,
		CreatedAt:   time.Now(),
	}

	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("DELETE", "/sessions/"+sessID.String(), nil)

	handlers.HandleDeleteSession(w, r, sessID.String())

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleDeleteSession_NotFound(t *testing.T) {
	mockStore := newMockSessionStore()
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("DELETE", "/sessions/"+uuid.New().String(), nil)

	handlers.HandleDeleteSession(w, r, uuid.New().String())

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestHandleDeleteSession_InvalidID(t *testing.T) {
	mockStore := newMockSessionStore()
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("DELETE", "/sessions/invalid-uuid", nil)

	handlers.HandleDeleteSession(w, r, "invalid-uuid")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleDeleteSession_DeleteError(t *testing.T) {
	mockStore := newMockSessionStore()
	sessID := uuid.New()
	mockStore.sessions[sessID] = &session.Session{
		ID:          sessID,
		SatelliteID: uuid.New(),
		UserID:      uuid.New(),
		Name:        "test-session",
		State:       session.StateTerminated,
		CreatedAt:   time.Now(),
	}
	mockStore.deleteErr = errors.New("database error")

	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("DELETE", "/sessions/"+sessID.String(), nil)

	handlers.HandleDeleteSession(w, r, sessID.String())

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

func TestHandleDeleteSession_StoreNotConfigured(t *testing.T) {
	handlers := NewHandlers(nil, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("DELETE", "/sessions/"+uuid.New().String(), nil)

	handlers.HandleDeleteSession(w, r, uuid.New().String())

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

// ============================================================================
// DeleteSatellite Handler Tests
// ============================================================================

func TestHandleDeleteSatellite_NoDB(t *testing.T) {
	handlers := NewHandlers(nil, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("DELETE", "/satellites/"+uuid.New().String(), nil)

	handlers.HandleDeleteSatellite(w, r, uuid.New().String())

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

// ============================================================================
// GetConfig Handler Tests
// ============================================================================

func TestHandleGetConfig_Success(t *testing.T) {
	handlers := NewHandlers(nil, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("GET", "/config", nil)

	handlers.HandleGetConfig(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp ConfigResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.DMSTTLMinutes != 60 {
		t.Errorf("expected dms_ttl_minutes 60, got %d", resp.DMSTTLMinutes)
	}
	if resp.HeartbeatIntervalSeconds != 30 {
		t.Errorf("expected heartbeat_interval_seconds 30, got %d", resp.HeartbeatIntervalSeconds)
	}
}

// ============================================================================
// SatelliteHeartbeat Handler Tests
// ============================================================================

func TestHandleSatelliteHeartbeat_NoDB(t *testing.T) {
	handlers := NewHandlers(nil, nil, nil, nil, nil, nil)

	body := `{"satellite_id":"` + uuid.New().String() + `"}`
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/satellites/heartbeat", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")

	handlers.HandleSatelliteHeartbeat(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestHandleSatelliteHeartbeat_InvalidJSON(t *testing.T) {
	handlers := NewHandlers(nil, nil, nil, nil, nil, nil)

	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/satellites/heartbeat", bytes.NewBufferString("invalid json"))
	r.Header.Set("Content-Type", "application/json")

	handlers.HandleSatelliteHeartbeat(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleSatelliteHeartbeat_MissingSatelliteID(t *testing.T) {
	handlers := NewHandlers(nil, nil, nil, nil, nil, nil)

	body := `{"status":"active"}`
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/satellites/heartbeat", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")

	handlers.HandleSatelliteHeartbeat(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// ============================================================================
// HandleRenameSession Tests
// ============================================================================

func TestHandleRenameSession_InvalidUUID(t *testing.T) {
	h := &Handlers{} // dbPool nil — validation runs before DB check
	body := `{"name":"new-name"}`
	r := httptest.NewRequest("PATCH", "/api/v1/sessions/not-a-uuid/name", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleRenameSession(w, r, "not-a-uuid")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleRenameSession_EmptyName(t *testing.T) {
	h := &Handlers{}
	body := `{"name":""}`
	r := httptest.NewRequest("PATCH", "/api/v1/sessions/"+uuid.New().String()+"/name", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleRenameSession(w, r, uuid.New().String())
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleRenameSession_NameTooLong(t *testing.T) {
	h := &Handlers{}
	longName := strings.Repeat("a", 256)
	body := `{"name":"` + longName + `"}`
	r := httptest.NewRequest("PATCH", "/api/v1/sessions/"+uuid.New().String()+"/name", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleRenameSession(w, r, uuid.New().String())
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleRenameSession_NoDB(t *testing.T) {
	// Validation passes, but no DB — expect 503
	h := &Handlers{}
	id := uuid.New().String()
	body := `{"name":"valid-name"}`
	r := httptest.NewRequest("PATCH", "/api/v1/sessions/"+id+"/name", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleRenameSession(w, r, id)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

// ============================================================================
// WriteJSON Helper Tests
// ============================================================================

func TestWriteJSON_Success(t *testing.T) {
	w := httptest.NewRecorder()
	resp := HealthResponse{Status: "healthy"}

	WriteJSON(w, http.StatusOK, resp)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", contentType)
	}
}

// ============================================================================
// GetContext Helper Tests
// ============================================================================

func TestGetContext_Success(t *testing.T) {
	ctx, cancel := GetContext(time.Second)
	defer cancel()

	if ctx == nil {
		t.Error("expected context to not be nil")
	}
}

// ============================================================================
// Input Validation / Security Tests
// ============================================================================

func TestCreateSession_NameTooLong(t *testing.T) {
	mockStore := newMockSessionStore()
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)

	longName := strings.Repeat("a", 256)
	body := `{"satellite_id":"` + uuid.New().String() + `","name":"` + longName + `"}`
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/sessions", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")

	handlers.HandleCreateSession(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestCreateSatellite_NameTooLong(t *testing.T) {
	handlers := NewHandlers(nil, nil, nil, nil, nil, nil)

	longName := strings.Repeat("a", 256)
	body := `{"name":"` + longName + `"}`
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/satellites", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")

	handlers.HandleCreateSatellite(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestCreateSession_AgentPathTraversal(t *testing.T) {
	mockStore := newMockSessionStore()
	handlers := NewHandlers(mockStore, nil, nil, nil, nil, nil)

	body := `{"satellite_id":"` + uuid.New().String() + `","name":"test","agent_binary":"../../etc/passwd"}`
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/sessions", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")

	handlers.HandleCreateSession(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for path traversal, got %d", w.Code)
	}
}
