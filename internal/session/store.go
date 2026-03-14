// Package session provides session persistence with state machine management.
//
// This package implements PostgreSQL session persistence with state machine
// transitions. Sessions track state (PROVISIONING, RUNNING, DETACHED, RE_ATTACHING,
// SUSPENDED, TERMINATED) and enforce valid state transitions.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Session states as defined in the PRD
const (
	StateProvisioning = "PROVISIONING"
	StateRunning      = "RUNNING"
	StateDetached     = "DETACHED"
	StateReAttaching  = "RE_ATTACHING"
	StateSuspended    = "SUSPENDED"
	StateTerminated   = "TERMINATED"
)

// AllSessionStates contains all valid session states
var AllSessionStates = []string{
	StateProvisioning,
	StateRunning,
	StateDetached,
	StateReAttaching,
	StateSuspended,
	StateTerminated,
}

// SessionState represents a session state
type SessionState string

// IsValid checks if the state is a valid session state
func (s SessionState) IsValid() bool {
	for _, state := range AllSessionStates {
		if string(s) == state {
			return true
		}
	}
	return false
}

// ValidTransitions defines the valid state transitions for the session state machine
// Based on the PRD state machine diagram:
// - PROVISIONING -> RUNNING (PTY acquired, process started)
// - RUNNING -> DETACHED (all clients disconnect)
// - RUNNING -> SUSPENDED (Dead Man's Switch fires)
// - DETACHED -> RUNNING (client connects)
// - DETACHED -> TERMINATED (explicit kill/process exit/TTL)
// - RE_ATTACHING -> RUNNING (buffer hydrated, stream established)
// - SUSPENDED -> RUNNING (SIGCONT/resumed)
// - SUSPENDED -> TERMINATED (max suspension exceeded)
var ValidTransitions = map[SessionState][]SessionState{
	StateProvisioning: {StateRunning, StateTerminated},
	StateRunning:      {StateDetached, StateSuspended, StateTerminated},
	StateDetached:     {StateRunning, StateReAttaching, StateTerminated},
	StateReAttaching:  {StateRunning, StateDetached, StateTerminated},
	StateSuspended:    {StateRunning, StateTerminated},
	StateTerminated:   {}, // Terminal state - no transitions out
}

// IsTransitionValid checks if a transition from one state to another is valid
func IsTransitionValid(from, to SessionState) bool {
	validTargets, exists := ValidTransitions[from]
	if !exists {
		return false
	}
	for _, target := range validTargets {
		if target == to {
			return true
		}
	}
	return false
}

// TransitionError represents an invalid state transition error
type TransitionError struct {
	From SessionState
	To   SessionState
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("invalid state transition from %s to %s", e.From, e.To)
}

// InvalidTransitionError creates a new TransitionError
func InvalidTransitionError(from, to SessionState) *TransitionError {
	return &TransitionError{From: from, To: to}
}

// Session represents a session in the database
type Session struct {
	ID               uuid.UUID    `json:"id"`
	SatelliteID      uuid.UUID    `json:"satellite_id"`
	UserID           uuid.UUID    `json:"user_id"`
	Name             string       `json:"name"`
	AgentBinary      string       `json:"agent_binary"`
	AgentArgs        []string     `json:"agent_args"`
	State            SessionState `json:"state"`
	OSPID            *int         `json:"os_pid,omitempty"`
	PTSName          *string      `json:"pts_name,omitempty"`
	Cols             int16        `json:"cols"`
	Rows             int16        `json:"rows"`
	RecordingEnabled bool         `json:"recording_enabled"`
	LastActivityAt   time.Time    `json:"last_activity_at"`
	StartedAt        *time.Time   `json:"started_at,omitempty"`
	DetachedAt       *time.Time   `json:"detached_at,omitempty"`
	SuspendedAt      *time.Time   `json:"suspended_at,omitempty"`
	TerminatedAt     *time.Time   `json:"terminated_at,omitempty"`
	CreatedAt        time.Time    `json:"created_at"`
}

// EventType represents the type of event logged
type EventType string

// Event types as defined in the PRD
const (
	EventStateChange    EventType = "STATE_CHANGE"
	EventResize         EventType = "RESIZE"
	EventClientAttach   EventType = "CLIENT_ATTACH"
	EventClientDetach   EventType = "CLIENT_DETACH"
	EventIPCStateChange EventType = "IPC_STATE_CHANGE"
	EventIPCOOBUI       EventType = "IPC_OOB_UI"
	EventDMSTriggered   EventType = "DMS_TRIGGERED"
	EventDMSResumed     EventType = "DMS_RESUMED"
	EventProcessExit    EventType = "PROCESS_EXIT"
	EventError          EventType = "ERROR"
)

// EventLog represents an event log entry
type EventLog struct {
	ID          int64                  `json:"id"`
	SessionID   uuid.UUID              `json:"session_id"`
	SatelliteID *uuid.UUID             `json:"satellite_id,omitempty"`
	UserID      *uuid.UUID             `json:"user_id,omitempty"`
	EventType   EventType              `json:"event_type"`
	Payload     map[string]interface{} `json:"payload,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

// Store defines the interface for session persistence
type Store interface {
	// Session operations
	CreateSession(ctx context.Context, session *Session) error
	GetSession(ctx context.Context, id uuid.UUID) (*Session, error)
	UpdateSession(ctx context.Context, session *Session) error
	DeleteSession(ctx context.Context, id uuid.UUID) error
	ListSessionsBySatellite(ctx context.Context, satelliteID uuid.UUID) ([]*Session, error)
	ListSessionsByUser(ctx context.Context, userID uuid.UUID) ([]*Session, error)
	ListActiveSessions(ctx context.Context) ([]*Session, error)

	// Pagination operations
	ListActiveSessionsWithLimit(ctx context.Context, limit int) ([]*Session, error)
	ListActiveSessionsAfter(ctx context.Context, cursor string, limit int) ([]*Session, error)
	CountActiveSessions(ctx context.Context) (int, error)

	// State machine operations
	TransitionSession(ctx context.Context, id uuid.UUID, newState SessionState) (*Session, error)

	// Event log operations
	WriteEventLog(ctx context.Context, event *EventLog) error
	GetEventLogs(ctx context.Context, sessionID uuid.UUID, limit int) ([]*EventLog, error)
}

// SessionStore implements the Store interface
type SessionStore struct {
	pool *pgxpool.Pool
}

// NewSessionStore creates a new session store with the given connection pool
func NewSessionStore(pool *pgxpool.Pool) *SessionStore {
	return &SessionStore{
		pool: pool,
	}
}

// sessionColumns is the canonical column list for session queries
const sessionColumns = `id, satellite_id, user_id, name, agent_binary, agent_args, state, os_pid, pts_name, cols, rows,
	       recording_enabled, last_activity_at, started_at, detached_at, suspended_at, terminated_at, created_at`

// scanSession scans a session row into a Session struct.
// This eliminates the 17-column scan duplication across all query methods.
func scanSession(row interface {
	Scan(dest ...interface{}) error
}) (*Session, error) {
	var session Session
	err := row.Scan(
		&session.ID,
		&session.SatelliteID,
		&session.UserID,
		&session.Name,
		&session.AgentBinary,
		&session.AgentArgs,
		&session.State,
		&session.OSPID,
		&session.PTSName,
		&session.Cols,
		&session.Rows,
		&session.RecordingEnabled,
		&session.LastActivityAt,
		&session.StartedAt,
		&session.DetachedAt,
		&session.SuspendedAt,
		&session.TerminatedAt,
		&session.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// CreateSession creates a new session in the database using parameterized queries
func (s *SessionStore) CreateSession(ctx context.Context, session *Session) error {
	query := `
		INSERT INTO sessions (id, satellite_id, user_id, name, agent_binary, agent_args, state, cols, rows, recording_enabled, last_activity_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	_, err := s.pool.Exec(ctx, query,
		session.ID,
		session.SatelliteID,
		session.UserID,
		session.Name,
		session.AgentBinary,
		session.AgentArgs,
		session.State,
		session.Cols,
		session.Rows,
		session.RecordingEnabled,
		session.LastActivityAt,
		session.CreatedAt,
	)
	return err
}

// GetSession retrieves a session by ID from the database using parameterized queries
func (s *SessionStore) GetSession(ctx context.Context, id uuid.UUID) (*Session, error) {
	query := `
		SELECT ` + sessionColumns + `
		FROM sessions WHERE id = $1
	`
	sess, err := scanSession(s.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("get session %s: %w", id, err)
	}
	return sess, nil
}

// UpdateState updates the state of a session using parameterized queries
func (s *SessionStore) UpdateState(ctx context.Context, id uuid.UUID, state SessionState) error {
	query := `
		UPDATE sessions
		SET state = $1, last_activity_at = $2
		WHERE id = $3
	`
	result, err := s.pool.Exec(ctx, query, state, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// TransitionSession performs a state transition on a session
// It validates the transition and updates the session in the database
// Uses SELECT ... FOR UPDATE to prevent concurrent transition race conditions
func (s *SessionStore) TransitionSession(ctx context.Context, id uuid.UUID, newState SessionState) (*Session, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock the row with FOR UPDATE to prevent concurrent transitions
	query := `
		SELECT ` + sessionColumns + `
		FROM sessions WHERE id = $1 FOR UPDATE
	`
	session, err := scanSession(tx.QueryRow(ctx, query, id))
	if err != nil {
		return nil, ErrSessionNotFound
	}

	// Use the in-memory TransitionSession function to validate and update
	updatedSession, err := TransitionSession(session, newState)
	if err != nil {
		return nil, err
	}

	// Update in database within the same transaction
	updateQuery := `
		UPDATE sessions
		SET state = $1, last_activity_at = $2, started_at = $3, detached_at = $4, suspended_at = $5, terminated_at = $6
		WHERE id = $7
	`
	result, err := tx.Exec(ctx, updateQuery,
		updatedSession.State,
		updatedSession.LastActivityAt,
		updatedSession.StartedAt,
		updatedSession.DetachedAt,
		updatedSession.SuspendedAt,
		updatedSession.TerminatedAt,
		id,
	)
	if err != nil {
		return nil, err
	}
	if result.RowsAffected() == 0 {
		return nil, ErrSessionNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return updatedSession, nil
}

// ListSessions lists all sessions from the database using parameterized queries
func (s *SessionStore) ListSessions(ctx context.Context) ([]*Session, error) {
	query := `
		SELECT ` + sessionColumns + `
		FROM sessions
		ORDER BY created_at DESC
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

// ListSessionsByUser retrieves all sessions for a specific user
func (s *SessionStore) ListSessionsByUser(ctx context.Context, userID uuid.UUID) ([]*Session, error) {
	query := `
		SELECT ` + sessionColumns + `
		FROM sessions
		WHERE user_id = $1
		ORDER BY created_at DESC
	`
	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

// ListActiveSessions retrieves all non-terminated sessions
func (s *SessionStore) ListActiveSessions(ctx context.Context) ([]*Session, error) {
	query := `
		SELECT ` + sessionColumns + `
		FROM sessions
		WHERE terminated_at IS NULL
		ORDER BY created_at DESC
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

// ListActiveSessionsWithLimit retrieves sessions with a limit (includes all states)
func (s *SessionStore) ListActiveSessionsWithLimit(ctx context.Context, limit int) ([]*Session, error) {
	query := `
		SELECT ` + sessionColumns + `
		FROM sessions
		ORDER BY created_at DESC, id DESC
		LIMIT $1
	`
	rows, err := s.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

// ListActiveSessionsAfter retrieves active sessions after a cursor (for cursor-based pagination)
// Cursor is the UUID of the last item from the previous page. Uses composite
// (created_at, id) ordering to ensure deterministic keyset pagination.
func (s *SessionStore) ListActiveSessionsAfter(ctx context.Context, cursor string, limit int) ([]*Session, error) {
	cursorUUID, err := uuid.Parse(cursor)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT ` + sessionColumns + `
		FROM sessions
		WHERE (created_at, id) < (SELECT created_at, id FROM sessions WHERE id = $1)
		ORDER BY created_at DESC, id DESC
		LIMIT $2
	`
	rows, err := s.pool.Query(ctx, query, cursorUUID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

// CountActiveSessions returns the total count of all sessions
func (s *SessionStore) CountActiveSessions(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM sessions`
	var count int
	err := s.pool.QueryRow(ctx, query).Scan(&count)
	return count, err
}

// ListSessionsBySatellite retrieves all sessions for a specific satellite
func (s *SessionStore) ListSessionsBySatellite(ctx context.Context, satelliteID uuid.UUID) ([]*Session, error) {
	query := `
		SELECT ` + sessionColumns + `
		FROM sessions
		WHERE satellite_id = $1
		ORDER BY created_at DESC
	`
	rows, err := s.pool.Query(ctx, query, satelliteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

// UpdateSession updates a session in the database
func (s *SessionStore) UpdateSession(ctx context.Context, sess *Session) error {
	query := `
		UPDATE sessions
		SET name = $1, state = $2, os_pid = $3, pts_name = $4, cols = $5, rows = $6,
		    last_activity_at = $7, started_at = $8, detached_at = $9, suspended_at = $10, terminated_at = $11
		WHERE id = $12
	`
	result, err := s.pool.Exec(ctx, query,
		sess.Name, sess.State, sess.OSPID, sess.PTSName, sess.Cols, sess.Rows,
		sess.LastActivityAt, sess.StartedAt, sess.DetachedAt, sess.SuspendedAt, sess.TerminatedAt,
		sess.ID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// DeleteSession deletes a session from the database
func (s *SessionStore) DeleteSession(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// WriteEventLog writes an event log entry to the database
func (s *SessionStore) WriteEventLog(ctx context.Context, event *EventLog) error {
	query := `
		INSERT INTO event_logs (session_id, satellite_id, event_type, event_data, created_at)
		VALUES ($1, $2, $3, $4, NOW())
	`
	var satelliteID *uuid.UUID
	if event.SatelliteID != nil {
		satelliteID = event.SatelliteID
	}

	var payload []byte
	if event.Payload != nil {
		var err error
		payload, err = json.Marshal(event.Payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}
	}

	_, err := s.pool.Exec(ctx, query, event.SessionID, satelliteID, event.EventType, payload)
	return err
}

// GetEventLogs retrieves event logs for a session
func (s *SessionStore) GetEventLogs(ctx context.Context, sessionID uuid.UUID, limit int) ([]*EventLog, error) {
	query := `
		SELECT id, session_id, satellite_id, event_type, event_data, created_at
		FROM event_logs
		WHERE session_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := s.pool.Query(ctx, query, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*EventLog
	for rows.Next() {
		var event EventLog
		var payload []byte
		err := rows.Scan(
			&event.ID,
			&event.SessionID,
			&event.SatelliteID,
			&event.EventType,
			&payload,
			&event.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		if payload != nil {
			if err := json.Unmarshal(payload, &event.Payload); err != nil {
				slog.Error(fmt.Sprintf("Warning: failed to unmarshal event payload: %v", err), "component", "session")
			}
		}
		events = append(events, &event)
	}
	return events, nil
}

// SessionMigrations contains the SQL for creating the sessions table
const SessionMigrations = `
-- Sessions table as defined in the PRD
-- Tracks session lifecycle with state machine
CREATE TABLE IF NOT EXISTS sessions (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    satellite_id    UUID        NOT NULL REFERENCES satellites(id) ON DELETE CASCADE,
    user_id         UUID        NOT NULL REFERENCES users(id),
    -- Human-readable session name, unique per satellite while active
    name            TEXT        NOT NULL,
    -- The agent binary being run (informational, set at creation)
    agent_binary    TEXT        NOT NULL,                -- e.g. "claude-code", "gemini-cli"
    agent_args      TEXT[]      NOT NULL DEFAULT '{}',
    -- State machine
    state           TEXT        NOT NULL DEFAULT 'PROVISIONING'
                                CHECK (state IN (
                                    'PROVISIONING', 'RUNNING', 'DETACHED',
                                    'RE_ATTACHING', 'SUSPENDED', 'TERMINATED'
                                )),
    -- OS-level process info (set by satellite after fork)
    os_pid          INTEGER,
    pts_name        TEXT,                               -- PTY slave: /dev/pts/3 or \\.\pipe\conpty-xxx
    -- Terminal dimensions (Nexus enforces Minimum Bounding Box)
    cols            SMALLINT    NOT NULL DEFAULT 80,
    rows            SMALLINT    NOT NULL DEFAULT 24,
    -- Recording toggle (per-session)
    recording_enabled BOOLEAN   NOT NULL DEFAULT TRUE,
    -- Lifecycle timestamps
    last_activity_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at      TIMESTAMPTZ,
    detached_at     TIMESTAMPTZ,
    suspended_at    TIMESTAMPTZ,
    terminated_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Enforce unique active session names per satellite
    CONSTRAINT uq_active_session_name UNIQUE NULLS NOT DISTINCT (satellite_id, name, terminated_at)
);

-- Indexes as defined in the PRD
CREATE INDEX IF NOT EXISTS idx_sessions_satellite_id ON sessions (satellite_id);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id      ON sessions (user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_state        ON sessions (state) WHERE terminated_at IS NULL;
`

// EventLogMigrations contains the SQL for creating the event_logs table
const EventLogMigrations = `
-- Event logs table for session auditing and telemetry
-- Partitioned by month as defined in the PRD
CREATE TABLE IF NOT EXISTS event_logs (
    id              BIGSERIAL   PRIMARY KEY,
    session_id      UUID        NOT NULL REFERENCES sessions(id),
    satellite_id    UUID        REFERENCES satellites(id),
    user_id         UUID        REFERENCES users(id),
    -- Event classification
    event_type      TEXT        NOT NULL,
    -- Values: STATE_CHANGE | RESIZE | CLIENT_ATTACH | CLIENT_DETACH |
    --         IPC_STATE_CHANGE | IPC_OOB_UI | DMS_TRIGGERED |
    --         DMS_RESUMED | PROCESS_EXIT | ERROR
    payload         JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
) PARTITION BY RANGE (created_at);

-- Monthly partitions (managed by pg_partman in production)
-- Example partition for February 2026
CREATE TABLE IF NOT EXISTS event_logs_2026_02 PARTITION OF event_logs
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');

CREATE TABLE IF NOT EXISTS event_logs_2026_03 PARTITION OF event_logs
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');

-- Indexes as defined in the PRD
CREATE INDEX IF NOT EXISTS idx_event_logs_session_created ON event_logs (session_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_event_logs_type            ON event_logs (event_type, created_at DESC);
`

// TransitionSession performs a state transition on a session
// It validates the transition and updates the session accordingly
func TransitionSession(session *Session, newState SessionState) (*Session, error) {
	if !newState.IsValid() {
		return nil, fmt.Errorf("invalid session state: %s", newState)
	}

	if !IsTransitionValid(session.State, newState) {
		return nil, InvalidTransitionError(session.State, newState)
	}

	now := time.Now().UTC()
	session.State = newState
	session.LastActivityAt = now

	// Update state-specific timestamps
	switch newState {
	case StateRunning:
		if session.StartedAt == nil {
			session.StartedAt = &now
		}
		// Clear detached/suspended if transitioning back to running
		session.DetachedAt = nil
		session.SuspendedAt = nil
	case StateDetached:
		session.DetachedAt = &now
	case StateSuspended:
		session.SuspendedAt = &now
	case StateTerminated:
		session.TerminatedAt = &now
	}

	return session, nil
}

// ValidateSessionName checks if a session name is valid
func ValidateSessionName(name string) error {
	if name == "" {
		return errors.New("session name cannot be empty")
	}
	if len(name) > 255 {
		return errors.New("session name cannot exceed 255 characters")
	}
	return nil
}

// ValidateTransition checks if a transition is valid and returns an error if not
func ValidateTransition(from, to SessionState) error {
	if !to.IsValid() {
		return fmt.Errorf("invalid target state: %s", to)
	}
	if !IsTransitionValid(from, to) {
		return InvalidTransitionError(from, to)
	}
	return nil
}

// GetValidTransitions returns the valid transitions from a given state
func GetValidTransitions(state SessionState) []SessionState {
	return ValidTransitions[state]
}

// SessionStoreErrors contains error definitions
var (
	ErrSessionNotFound     = errors.New("session not found")
	ErrInvalidState        = errors.New("invalid session state")
	ErrInvalidTransition   = errors.New("invalid state transition")
	ErrSessionNameConflict = errors.New("session name already exists for this satellite")
)
