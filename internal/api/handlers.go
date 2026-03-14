// Package api provides HTTP handlers for the Nexus API Gateway.
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/daao/nexus/internal/audit"
	"github.com/daao/nexus/internal/auth"
	"github.com/daao/nexus/internal/enterprise/hitl"
	"github.com/daao/nexus/internal/license"
	"github.com/daao/nexus/internal/notification"
	"github.com/daao/nexus/internal/session"
	"github.com/daao/nexus/internal/stream"
	"github.com/daao/nexus/proto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ConfigAccessor provides access to configuration
type ConfigAccessor interface {
	GetServerCert() string
	GetServerKey() string
	GetClientCAs() string
	GetListenAddr() string
	GetGRPCAddr() string
}

// nexusConfig implements ConfigAccessor
type nexusConfig struct {
	ServerCert string
	ServerKey  string
	ClientCAs  string
	ListenAddr string
	GRPCAddr   string
}

func (c *nexusConfig) GetServerCert() string { return c.ServerCert }
func (c *nexusConfig) GetServerKey() string  { return c.ServerKey }
func (c *nexusConfig) GetClientCAs() string  { return c.ClientCAs }
func (c *nexusConfig) GetListenAddr() string { return c.ListenAddr }
func (c *nexusConfig) GetGRPCAddr() string   { return c.GRPCAddr }

// scanSession scans a session row from the database
func scanSession(row interface {
	Scan(dest ...interface{}) error
}) (*session.Session, error) {
	s := &session.Session{}
	var agentArgs []byte
	var osPID *int
	var ptsName *string
	err := row.Scan(
		&s.ID,
		&s.SatelliteID,
		&s.UserID,
		&s.Name,
		&s.AgentBinary,
		&agentArgs,
		&s.State,
		&osPID,
		&ptsName,
		&s.Cols,
		&s.Rows,
		&s.RecordingEnabled,
		&s.LastActivityAt,
		&s.StartedAt,
		&s.DetachedAt,
		&s.SuspendedAt,
		&s.TerminatedAt,
		&s.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	s.OSPID = osPID
	s.PTSName = ptsName
	if len(agentArgs) > 0 {
		json.Unmarshal(agentArgs, &s.AgentArgs)
	}
	return s, nil
}

// Handlers provides HTTP handlers for the Nexus API Gateway.
// All dependencies are injected via the constructor to avoid global state.
type Handlers struct {
	sessionStore        session.Store
	dbPool              *pgxpool.Pool
	readPool           *pgxpool.Pool // nil = use dbPool for all queries
	streamRegistry      stream.StreamRegistryInterface
	ringBufferPool      *session.RingBufferPool
	config              ConfigAccessor
	recordingPool       RecordingPoolInterface
	notificationStore   notification.Store
	notificationService *notification.NotificationService
	licenseManager      *license.Manager
	hitlManager         *hitl.Manager
	auditLogger         *audit.AuditLogger
	healthDeps          *HealthDeps
	timescaleEnabled    bool
}

// RecordingPoolInterface defines the recording pool dependency for handlers.
type RecordingPoolInterface interface {
	StartRecording(sessionID, recordingID string, cols, rows int, title string) error
	StopRecording(sessionID string) (durationMs int64, sizeBytes int64, err error)
	WriteIfRecording(sessionID string, data []byte)
	IsRecording(sessionID string) bool
	DataDir() string
	GetPlaybackURL(filename string) (string, error)
}

// NewHandlers creates a new Handlers instance with all dependencies injected.
// The notificationStore and notificationService parameters are optional (nil-safe).
func NewHandlers(
	sessionStore session.Store,
	dbPool *pgxpool.Pool,
	streamRegistry stream.StreamRegistryInterface,
	ringBufferPool *session.RingBufferPool,
	config ConfigAccessor,
	recordingPool RecordingPoolInterface,
	notifOpts ...interface{},
) *Handlers {
	h := &Handlers{
		sessionStore:   sessionStore,
		dbPool:         dbPool,
		streamRegistry: streamRegistry,
		ringBufferPool: ringBufferPool,
		config:         config,
		recordingPool:  recordingPool,
	}
	// Optional dependencies (variadic to keep existing callers working)
	for _, opt := range notifOpts {
		switch v := opt.(type) {
		case notification.Store:
			h.notificationStore = v
		case *notification.NotificationService:
			h.notificationService = v
		case *license.Manager:
			h.licenseManager = v
		case *hitl.Manager:
			h.hitlManager = v
		case *audit.AuditLogger:
			h.auditLogger = v
		}
	}
	return h
}

// SetReadPool configures an optional read replica pool for list queries.
// If nil (default), all queries use the primary dbPool.
func (h *Handlers) SetReadPool(p *pgxpool.Pool) { h.readPool = p }

// SetTimescaleEnabled configures whether TimescaleDB continuous aggregates are used for telemetry charts.
func (h *Handlers) SetTimescaleEnabled(v bool) { h.timescaleEnabled = v }

// rpool returns the read pool when available, falling back to the primary pool.
func (h *Handlers) rpool() *pgxpool.Pool {
	if h.readPool != nil {
		return h.readPool
	}
	return h.dbPool
}

// ReadPool returns the read pool for testing purposes.
// Exported for testability - returns the read pool when set, otherwise the primary pool.
func (h *Handlers) ReadPool() *pgxpool.Pool { return h.rpool() }

// HandleHealth handles health check requests
func (h *Handlers) HandleHealth(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]interface{})
	dbHealthy := false
	var dbLatency int64

	// Database check with 2s timeout
	if h.healthDeps != nil && h.healthDeps.Pool != nil {
		start := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		var dummy int
		err := h.healthDeps.Pool.QueryRow(ctx, "SELECT 1").Scan(&dummy)
		dbLatency = time.Since(start).Milliseconds()

		if err != nil {
			checks["database"] = HealthCheck{
				Status:   "unhealthy",
				LatencyMs: dbLatency,
			}
		} else {
			dbHealthy = true
			checks["database"] = HealthCheck{
				Status:    "healthy",
				LatencyMs: dbLatency,
			}
		}
	} else {
		// No pool configured - assume healthy for backwards compatibility
		checks["database"] = HealthCheck{
			Status: "healthy",
		}
		dbHealthy = true
	}

	// gRPC check
	if h.healthDeps != nil && h.healthDeps.GRPCServer != nil {
		checks["grpc"] = HealthCheck{
			Status:   "healthy",
			Listener: h.healthDeps.GRPCServer.GetAddr(),
		}
	} else {
		checks["grpc"] = HealthCheck{
			Status: "healthy",
		}
	}

	// Satellite count
	satCount := 0
	if h.healthDeps != nil && h.healthDeps.GetSatelliteCount != nil {
		satCount = h.healthDeps.GetSatelliteCount()
	}
	checks["satellites_connected"] = satCount

	// Session count
	sessCount := 0
	if h.healthDeps != nil && h.healthDeps.GetSessionCount != nil {
		sessCount = h.healthDeps.GetSessionCount()
	}
	checks["sessions_active"] = sessCount

	// Calculate uptime
	uptimeSeconds := int64(0)
	version := "unknown"
	if h.healthDeps != nil && !h.healthDeps.StartTime.IsZero() {
		uptimeSeconds = int64(time.Since(h.healthDeps.StartTime).Seconds())
		version = h.healthDeps.Version
	}

	// Determine overall status
	status := "healthy"
	httpStatus := http.StatusOK
	if !dbHealthy {
		status = "unhealthy"
		httpStatus = http.StatusServiceUnavailable
	}

	response := HealthResponse{
		Status:        status,
		Version:       version,
		UptimeSeconds: uptimeSeconds,
		Checks:        checks,
	}

	writeJSON(w, httpStatus, response)
}

// SetHealthDeps sets the health check dependencies
func (h *Handlers) SetHealthDeps(deps *HealthDeps) {
	h.healthDeps = deps
}

// HandleListSessions handles listing all active sessions with cursor-based pagination
func (h *Handlers) HandleListSessions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.sessionStore == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "session store not configured"})
		return
	}

	// Extract user from context for resource scoping
	user, _ := auth.UserFromContext(r.Context())
	var userID *uuid.UUID
	var isViewer bool
	if user != nil {
		if user.Role == "viewer" {
			isViewer = true
			uid, err := uuid.Parse(user.ID)
			if err == nil {
				userID = &uid
			}
		}
		// admin/owner: userID stays nil (show all sessions)
	}

	// Parse pagination parameters
	params := ParsePaginationParams(r)

	// Use cursor if provided, otherwise fetch all active sessions
	var sessions []*session.Session
	var err error

	// Get the session columns for queries
	sessionColumns := `id, satellite_id, user_id, name, agent_binary, agent_args, state, os_pid, pts_name, cols, rows,
	       recording_enabled, last_activity_at, started_at, detached_at, suspended_at, terminated_at, created_at`

	// If viewer, use custom query with user_id filter; otherwise use store methods
	if isViewer && userID != nil {
		// Viewer: filter by user_id
		if params.Cursor != "" {
			// Cursor-based pagination - fetch sessions after the cursor
			cursorUUID, err := uuid.Parse(params.Cursor)
			if err == nil {
				query := `
					SELECT ` + sessionColumns + `
					FROM sessions
					WHERE user_id = $1 AND (created_at, id) < (SELECT created_at, id FROM sessions WHERE id = $2)
					ORDER BY created_at DESC, id DESC
					LIMIT $3
				`
				rows, err := h.rpool().Query(r.Context(), query, *userID, cursorUUID, params.Limit)
				if err == nil {
					defer rows.Close()
					for rows.Next() {
						sess, err := scanSession(rows)
						if err == nil {
							sessions = append(sessions, sess)
						}
					}
				}
			}
		} else {
			// First page - fetch with limit filtered by user_id
			query := `
				SELECT ` + sessionColumns + `
				FROM sessions
				WHERE user_id = $1
				ORDER BY created_at DESC, id DESC
				LIMIT $2
			`
			rows, err := h.rpool().Query(r.Context(), query, *userID, params.Limit)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					sess, err := scanSession(rows)
					if err == nil {
						sessions = append(sessions, sess)
					}
				}
			}
		}
	} else {
		// Admin/owner or dev mode (no user in context): show all sessions
		if params.Cursor != "" {
			sessions, err = h.sessionStore.ListActiveSessionsAfter(r.Context(), params.Cursor, params.Limit)
		} else {
			sessions, err = h.sessionStore.ListActiveSessionsWithLimit(r.Context(), params.Limit)
		}
	}

	if err != nil {
		slog.Error(fmt.Sprintf("Failed to list sessions: %v", err), "component", "api")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to list sessions"})
		return
	}

	if sessions == nil {
		sessions = []*session.Session{}
	}

	// Check if there are more results (we fetched exactly the limit)
	hasMore := len(sessions) >= params.Limit

	// Get total count for pagination metadata
	var total int
	var countErr error
	if isViewer && userID != nil {
		countErr = h.rpool().QueryRow(r.Context(),
			`SELECT COUNT(*) FROM sessions WHERE user_id = $1`, *userID,
		).Scan(&total)
	} else {
		total, countErr = h.sessionStore.CountActiveSessions(r.Context())
	}
	if countErr != nil {
		slog.Error(fmt.Sprintf("Failed to count sessions: %v", countErr), "component", "api")
		total = len(sessions)
	}

	// Create paginated response with proper cursor extraction
	var nextCursor *string
	if hasMore && len(sessions) > 0 {
		lastID := sessions[len(sessions)-1].ID.String()
		nextCursor = &lastID
	}

	response := &PaginatedResponse{
		Items:      sessions,
		Count:      len(sessions),
		Total:      total,
		NextCursor: nextCursor,
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// HandleCreateSession handles creating a new session
func (h *Handlers) HandleCreateSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.sessionStore == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "database not configured"})
		return
	}

	var req CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid JSON request body"})
		return
	}

	if req.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "name is required"})
		return
	}

	if len(req.Name) > 255 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "name must not exceed 255 characters"})
		return
	}

	// Reject agent binary paths that attempt directory traversal or contain null bytes
	if req.AgentBinary != "" {
		if strings.Contains(req.AgentBinary, "..") || strings.ContainsRune(req.AgentBinary, 0) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "agent_binary contains invalid path characters"})
			return
		}
	}

	if req.SatelliteID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "satellite_id is required"})
		return
	}

	satelliteUUID, err := uuid.Parse(req.SatelliteID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid satellite_id: must be a valid UUID"})
		return
	}

	// Verify satellite exists and is active
	if h.dbPool != nil {
		var satStatus string
		err := h.dbPool.QueryRow(r.Context(),
			`SELECT status FROM satellites WHERE id = $1`, satelliteUUID,
		).Scan(&satStatus)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "satellite not found"})
			return
		}
		if satStatus != "active" {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("satellite is not active (status: %s) — ensure the daemon is running and connected", satStatus)})
			return
		}
	}

	// Set defaults for optional fields
	if req.AgentArgs == nil {
		req.AgentArgs = []string{}
	}
	if req.Cols == 0 {
		req.Cols = 80
	}
	if req.Rows == 0 {
		req.Rows = 24
	}

	// Extract user from context for resource scoping
	var userID uuid.UUID
	user, _ := auth.UserFromContext(r.Context())
	if user != nil {
		if parsedID, err := uuid.Parse(user.ID); err == nil {
			userID = parsedID
		}
	}

	newSession := &session.Session{
		ID:               uuid.New(),
		SatelliteID:      satelliteUUID,
		UserID:           userID,
		Name:             req.Name,
		AgentBinary:      req.AgentBinary,
		AgentArgs:        req.AgentArgs,
		State:            session.StateProvisioning,
		Cols:             req.Cols,
		Rows:             req.Rows,
		RecordingEnabled: true,
		LastActivityAt:   time.Now().UTC(),
		CreatedAt:        time.Now().UTC(),
	}

	ctx := r.Context()
	if err := h.sessionStore.CreateSession(ctx, newSession); err != nil {
		slog.Error(fmt.Sprintf("Failed to create session: %v", err), "component", "api")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to create session"})
		return
	}

	// Audit log the session creation
	if h.auditLogger != nil {
		ctx = audit.WithClientIP(ctx, audit.ClientIPFromRequest(r))
		h.auditLogger.Log(ctx, "session.create", "session", newSession.ID.String(), map[string]interface{}{
			"satellite_id": req.SatelliteID,
			"name":         req.Name,
		})
	}

	// Dispatch StartSessionCommand to satellite via stream registry
	if h.streamRegistry != nil {
		startCmd := &proto.NexusMessage{
			Payload: &proto.NexusMessage_StartSessionCommand{
				StartSessionCommand: &proto.StartSessionCommand{
					SessionId:   newSession.ID.String(),
					AgentBinary: newSession.AgentBinary,
					AgentArgs:   newSession.AgentArgs,
					Cols:        int32(newSession.Cols),
					Rows:        int32(newSession.Rows),
					WorkingDir:  req.WorkingDir,
				},
			},
		}
		if h.streamRegistry.SendToSatellite(req.SatelliteID, startCmd) {
			slog.Info(fmt.Sprintf("HandleCreateSession: Dispatched StartSessionCommand for session %s to satellite %s", newSession.ID, req.SatelliteID), "component", "api")
		} else {
			slog.Warn(fmt.Sprintf("HandleCreateSession: Warning: could not dispatch StartSessionCommand to satellite %s (stream not found)", req.SatelliteID), "component", "api")
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(newSession)
}

// HandleGetSession handles getting a session by ID
func (h *Handlers) HandleGetSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	w.Header().Set("Content-Type", "application/json")

	if h.sessionStore == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "session store not configured"})
		return
	}

	id, err := uuid.Parse(sessionID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid session_id"})
		return
	}

	sess, err := h.sessionStore.GetSession(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "session not found"})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(sess)
}

// HandleAttachSession handles attaching to a session
func (h *Handlers) HandleAttachSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	w.Header().Set("Content-Type", "application/json")

	if h.sessionStore == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "session store not configured"})
		return
	}

	id, err := uuid.Parse(sessionID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid session_id"})
		return
	}

	ctx := r.Context()
	sess, err := h.sessionStore.GetSession(ctx, id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "session not found"})
		return
	}

	// Transition to RUNNING state (skip if already RUNNING)
	var updatedSession *session.Session
	if sess.State == session.StateRunning {
		// Already running — no transition needed, just record the attach event
		updatedSession = sess
	} else {
		var err2 error
		updatedSession, err2 = h.sessionStore.TransitionSession(ctx, id, session.StateRunning)
		if err2 != nil {
			slog.Error(fmt.Sprintf("Failed to transition session to RUNNING: %v", err2), "component", "api")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to attach session"})
			return
		}
	}

	// Create event log
	h.sessionStore.WriteEventLog(ctx, &session.EventLog{
		SessionID: sess.ID,
		EventType: session.EventClientAttach,
		CreatedAt: time.Now(),
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(updatedSession)
}

// HandleDetachSession handles detaching from a session
func (h *Handlers) HandleDetachSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	w.Header().Set("Content-Type", "application/json")

	if h.sessionStore == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "session store not configured"})
		return
	}

	id, err := uuid.Parse(sessionID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid session_id"})
		return
	}

	ctx := r.Context()
	sess, err := h.sessionStore.GetSession(ctx, id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "session not found"})
		return
	}

	// Transition to DETACHED state
	updatedSession, err := h.sessionStore.TransitionSession(ctx, id, session.StateDetached)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to transition session to DETACHED: %v", err), "component", "api")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to detach session"})
		return
	}

	// Create event log
	h.sessionStore.WriteEventLog(ctx, &session.EventLog{
		SessionID: sess.ID,
		EventType: session.EventClientDetach,
		CreatedAt: time.Now(),
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(updatedSession)
}

// HandleSuspendSession handles suspending a session
func (h *Handlers) HandleSuspendSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	w.Header().Set("Content-Type", "application/json")

	if h.sessionStore == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "session store not configured"})
		return
	}

	id, err := uuid.Parse(sessionID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid session_id"})
		return
	}

	ctx := r.Context()
	sess, err := h.sessionStore.GetSession(ctx, id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "session not found"})
		return
	}

	// Check ownership for viewers - viewers can only modify their own sessions
	user, _ := auth.UserFromContext(r.Context())
	if user != nil && user.Role == "viewer" {
		userID, err := uuid.Parse(user.ID)
		if err == nil && sess.UserID != userID {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "you can only suspend your own sessions"})
			return
		}
	}

	// Transition to SUSPENDED state
	updatedSession, err := h.sessionStore.TransitionSession(ctx, id, session.StateSuspended)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to transition session to SUSPENDED: %v", err), "component", "api")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to suspend session"})
		return
	}

	// Dispatch SuspendCommand to the satellite
	if h.streamRegistry != nil {
		suspendCmd := &proto.NexusMessage{
			Payload: &proto.NexusMessage_SuspendCommand{
				SuspendCommand: &proto.SuspendCommand{
					SessionId: sessionID,
				},
			},
		}
		if h.streamRegistry.SendToSatellite(sess.SatelliteID.String(), suspendCmd) {
			slog.Info(fmt.Sprintf("HandleSuspendSession: Dispatched SuspendCommand for session %s to satellite %s", sessionID, sess.SatelliteID), "component", "api")
		} else {
			slog.Warn(fmt.Sprintf("HandleSuspendSession: Warning: could not dispatch SuspendCommand to satellite %s (stream not found)", sess.SatelliteID), "component", "api")
		}
	}

	// Create event log
	h.sessionStore.WriteEventLog(ctx, &session.EventLog{
		SessionID: sess.ID,
		EventType: session.EventDMSTriggered,
		CreatedAt: time.Now(),
	})

	// Emit notification event
	if h.notificationService != nil {
		h.notificationService.Emit(&notification.Event{
			Type:      notification.EventSessionSuspended,
			Priority:  notification.PriorityInfo,
			SessionID: &id,
			Title:     "Session Suspended",
			Body:      fmt.Sprintf("Session %q has been suspended", sess.Name),
		})
	}

	// Audit log the session suspend
	if h.auditLogger != nil {
		ctx = audit.WithClientIP(ctx, audit.ClientIPFromRequest(r))
		h.auditLogger.Log(ctx, "session.suspend", "session", sessionID, map[string]interface{}{
			"session_id": sessionID,
		})
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(updatedSession)
}

// HandleResumeSession handles resuming a suspended session
func (h *Handlers) HandleResumeSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	w.Header().Set("Content-Type", "application/json")

	if h.sessionStore == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "session store not configured"})
		return
	}

	id, err := uuid.Parse(sessionID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid session_id"})
		return
	}

	ctx := r.Context()
	sess, err := h.sessionStore.GetSession(ctx, id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "session not found"})
		return
	}

	// Transition to RUNNING state
	updatedSession, err := h.sessionStore.TransitionSession(ctx, id, session.StateRunning)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to transition session to RUNNING: %v", err), "component", "api")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to resume session"})
		return
	}

	// Dispatch ResumeCommand to the satellite
	if h.streamRegistry != nil {
		resumeCmd := &proto.NexusMessage{
			Payload: &proto.NexusMessage_ResumeCommand{
				ResumeCommand: &proto.ResumeCommand{
					SessionId: sessionID,
				},
			},
		}
		if h.streamRegistry.SendToSatellite(sess.SatelliteID.String(), resumeCmd) {
			slog.Info(fmt.Sprintf("HandleResumeSession: Dispatched ResumeCommand for session %s to satellite %s", sessionID, sess.SatelliteID), "component", "api")
		} else {
			slog.Warn(fmt.Sprintf("HandleResumeSession: Warning: could not dispatch ResumeCommand to satellite %s (stream not found)", sess.SatelliteID), "component", "api")
		}
	}

	// Create event log
	h.sessionStore.WriteEventLog(ctx, &session.EventLog{
		SessionID: sess.ID,
		EventType: session.EventDMSResumed,
		CreatedAt: time.Now(),
	})

	// Audit log the session resume
	if h.auditLogger != nil {
		ctx = audit.WithClientIP(ctx, audit.ClientIPFromRequest(r))
		h.auditLogger.Log(ctx, "session.resume", "session", sessionID, map[string]interface{}{
			"session_id": sessionID,
		})
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(updatedSession)
}

// HandleKillSession handles killing a session
func (h *Handlers) HandleKillSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	w.Header().Set("Content-Type", "application/json")

	if h.sessionStore == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "session store not configured"})
		return
	}

	id, err := uuid.Parse(sessionID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid session_id"})
		return
	}

	ctx := r.Context()
	sess, err := h.sessionStore.GetSession(ctx, id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "session not found"})
		return
	}

	// Check ownership for viewers - viewers can only modify their own sessions
	user, _ := auth.UserFromContext(r.Context())
	if user != nil && user.Role == "viewer" {
		userID, err := uuid.Parse(user.ID)
		if err == nil && sess.UserID != userID {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "you can only kill your own sessions"})
			return
		}
	}

	// Transition to TERMINATED state
	updatedSession, err := h.sessionStore.TransitionSession(ctx, id, session.StateTerminated)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to transition session to TERMINATED: %v", err), "component", "api")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to kill session"})
		return
	}

	// Dispatch KillCommand to the satellite
	if h.streamRegistry != nil {
		killCmd := &proto.NexusMessage{
			Payload: &proto.NexusMessage_KillCommand{
				KillCommand: &proto.KillCommand{
					SessionId: sessionID,
					ExitCode:  -1,
				},
			},
		}
		if h.streamRegistry.SendToSatellite(sess.SatelliteID.String(), killCmd) {
			slog.Info(fmt.Sprintf("HandleKillSession: Dispatched KillCommand for session %s to satellite %s", sessionID, sess.SatelliteID), "component", "api")
		} else {
			slog.Warn(fmt.Sprintf("HandleKillSession: Warning: could not dispatch KillCommand to satellite %s (stream not found)", sess.SatelliteID), "component", "api")
		}
	}

	// Create event log
	h.sessionStore.WriteEventLog(ctx, &session.EventLog{
		SessionID: sess.ID,
		EventType: session.EventProcessExit,
		CreatedAt: time.Now(),
	})

	// Clean up ring buffer if exists
	if h.ringBufferPool != nil {
		h.ringBufferPool.RemoveBuffer(sessionID)
	}

	// Emit notification event
	if h.notificationService != nil {
		h.notificationService.Emit(&notification.Event{
			Type:      notification.EventSessionTerminated,
			Priority:  notification.PriorityInfo,
			SessionID: &id,
			Title:     "Session Terminated",
			Body:      fmt.Sprintf("Session %q has been terminated", sess.Name),
		})
	}

	// Audit log the session kill
	if h.auditLogger != nil {
		ctx = audit.WithClientIP(ctx, audit.ClientIPFromRequest(r))
		h.auditLogger.Log(ctx, "session.kill", "session", sessionID, map[string]interface{}{
			"session_id": sessionID,
		})
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(updatedSession)
}

// HandleListSatellites returns list of satellites from the database
func (h *Handlers) HandleListSatellites(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.dbPool == nil {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]"))
		return
	}

	// Extract user from context for resource scoping
	user, _ := auth.UserFromContext(r.Context())
	var userID *uuid.UUID
	var isViewer bool
	if user != nil {
		if user.Role == "viewer" {
			isViewer = true
			uid, err := uuid.Parse(user.ID)
			if err == nil {
				userID = &uid
			}
		}
		// admin/owner: userID stays nil (show all satellites)
	}

	// Build query based on user role
	var query string
	var rows interface {
		Close()
		Next() bool
		Scan(dest ...interface{}) error
	}
	var err error

	if isViewer && userID != nil {
		// Viewer: filter by owner_id
		query = `SELECT id, name, owner_id, status, COALESCE(os,''), COALESCE(arch,''), COALESCE(version,''), COALESCE(tags,'{}'), COALESCE(available_agents,'[]'), created_at, updated_at FROM satellites WHERE owner_id = $1 ORDER BY created_at DESC`
		rows, err = h.rpool().Query(r.Context(), query, *userID)
	} else {
		// Admin/owner or dev mode: show all satellites
		query = `SELECT id, name, owner_id, status, COALESCE(os,''), COALESCE(arch,''), COALESCE(version,''), COALESCE(tags,'{}'), COALESCE(available_agents,'[]'), created_at, updated_at FROM satellites ORDER BY created_at DESC`
		rows, err = h.rpool().Query(r.Context(), query)
	}

	if err != nil {
		slog.Error(fmt.Sprintf("Failed to query satellites: %v", err), "component", "api")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]"))
		return
	}
	defer rows.Close()

	type SatelliteRow struct {
		ID              string    `json:"id"`
		Name            string    `json:"name"`
		OwnerID         string    `json:"owner_id"`
		Status          string    `json:"status"`
		Os              string    `json:"os,omitempty"`
		Arch            string    `json:"arch,omitempty"`
		Version         string    `json:"version,omitempty"`
		Tags            []string  `json:"tags"`
		AvailableAgents []string  `json:"available_agents"`
		CreatedAt       time.Time `json:"created_at"`
		UpdatedAt       time.Time `json:"updated_at"`
	}

	var satellites []SatelliteRow
	for rows.Next() {
		var sat SatelliteRow
		var agentsJSON []byte
		if err := rows.Scan(&sat.ID, &sat.Name, &sat.OwnerID, &sat.Status, &sat.Os, &sat.Arch, &sat.Version, &sat.Tags, &agentsJSON, &sat.CreatedAt, &sat.UpdatedAt); err != nil {
			slog.Error(fmt.Sprintf("Failed to scan satellite row: %v", err), "component", "api")
			continue
		}
		if len(agentsJSON) > 0 {
			json.Unmarshal(agentsJSON, &sat.AvailableAgents)
		}
		if sat.AvailableAgents == nil {
			sat.AvailableAgents = []string{}
		}
		if sat.Tags == nil {
			sat.Tags = []string{}
		}
		satellites = append(satellites, sat)
	}

	if satellites == nil {
		satellites = []SatelliteRow{}
	}

	data, err := json.Marshal(satellites)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to marshal satellites: %v", err), "component", "api")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]"))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// HandleCreateSatellite handles creating a new satellite
func (h *Handlers) HandleCreateSatellite(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req CreateSatelliteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid JSON request body"})
		return
	}

	if req.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "name is required"})
		return
	}

	if len(req.Name) > 255 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "name must not exceed 255 characters"})
		return
	}

	if h.dbPool == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "database not configured"})
		return
	}

	ownerID := uuid.Nil
	now := time.Now().UTC()

	// Upsert by name: if a satellite with this name already exists, reset it to
	// pending (new install) and return the same ID so the daemon reuses the record.
	// This prevents duplicate records on reinstall.
	var returnedID string
	err := h.dbPool.QueryRow(r.Context(),
		`INSERT INTO satellites (id, name, owner_id, status, fingerprint, created_at, updated_at)
		 VALUES ($1, $2, $3, 'pending', NULL, $4, $4)
		 ON CONFLICT (name) DO UPDATE
		   SET status = 'pending', fingerprint = NULL, updated_at = $4
		 RETURNING id::text`,
		uuid.New(), req.Name, ownerID, now,
	).Scan(&returnedID)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to upsert satellite: %v", err), "component", "api")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to create satellite"})
		return
	}
	newID, _ := uuid.Parse(returnedID)

	type SatelliteResponse struct {
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		OwnerID   string    `json:"owner_id"`
		Status    string    `json:"status"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}

	response := SatelliteResponse{
		ID:        newID.String(),
		Name:      req.Name,
		OwnerID:   ownerID.String(),
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, _ := json.Marshal(response)
	w.WriteHeader(http.StatusCreated)
	w.Write(data)
}

// HandleRenameSession handles renaming a session via PATCH /api/v1/sessions/{id}/name
func (h *Handlers) HandleRenameSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	w.Header().Set("Content-Type", "application/json")

	// Validate inputs before touching the DB.
	id, err := uuid.Parse(sessionID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid session_id"})
		return
	}

	var req RenameSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid JSON request body"})
		return
	}

	if req.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "name is required"})
		return
	}

	if len(req.Name) > 255 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "name must not exceed 255 characters"})
		return
	}

	if h.dbPool == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "database not configured"})
		return
	}

	result, err := h.dbPool.Exec(r.Context(),
		`UPDATE sessions SET name = $1, updated_at = NOW() WHERE id = $2 AND terminated_at IS NULL`,
		req.Name, id,
	)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleRenameSession: failed to update session name: %v", err), "component", "api")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to rename session"})
		return
	}

	if result.RowsAffected() == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "session not found or already terminated"})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"id": id.String(), "name": req.Name})
}

// HandleDeleteSession deletes a terminated session by ID
func (h *Handlers) HandleDeleteSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	w.Header().Set("Content-Type", "application/json")

	if h.sessionStore == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "database not configured"})
		return
	}

	parsedUUID, err := uuid.Parse(sessionID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid session ID format"})
		return
	}

	ctx := r.Context()

	// Check session exists and is terminated
	sess, err := h.sessionStore.GetSession(ctx, parsedUUID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "session not found"})
		return
	}

	// Check ownership for viewers - viewers can only delete their own sessions
	user, _ := auth.UserFromContext(r.Context())
	if user != nil && user.Role == "viewer" {
		userID, err := uuid.Parse(user.ID)
		if err == nil && sess.UserID != userID {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "you can only delete your own sessions"})
			return
		}
	}

	// Only allow deletion of non-active sessions (terminated or failed provisioning)
	if sess.State != session.StateTerminated && sess.State != "PROVISIONING" {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "only terminated or provisioning sessions can be deleted"})
		return
	}

	if err := h.sessionStore.DeleteSession(ctx, parsedUUID); err != nil {
		slog.Error(fmt.Sprintf("Failed to delete session: %v", err), "component", "api")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to delete session"})
		return
	}

	// Clean up ring buffer if exists
	if h.ringBufferPool != nil {
		h.ringBufferPool.RemoveBuffer(sessionID)
	}

	// Audit log the session delete
	if h.auditLogger != nil {
		ctx = audit.WithClientIP(ctx, audit.ClientIPFromRequest(r))
		h.auditLogger.Log(ctx, "session.delete", "session", sessionID, map[string]interface{}{
			"session_id": sessionID,
		})
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"deleted"}`))
}

// HandleDeleteSatellite deletes a satellite by ID
func (h *Handlers) HandleDeleteSatellite(w http.ResponseWriter, r *http.Request, satelliteID string) {
	w.Header().Set("Content-Type", "application/json")

	parsedUUID, err := uuid.Parse(satelliteID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid satellite ID format"})
		return
	}

	if h.dbPool == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "database not configured"})
		return
	}

	// Check ownership for viewers - fetch satellite first to get owner_id
	user, _ := auth.UserFromContext(r.Context())
	if user != nil && user.Role == "viewer" {
		var ownerID string
		err := h.dbPool.QueryRow(r.Context(), `SELECT owner_id FROM satellites WHERE id = $1`, parsedUUID).Scan(&ownerID)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "satellite not found"})
			return
		}
		if ownerID != user.ID {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "you can only delete your own satellites"})
			return
		}
	}

	result, err := h.dbPool.Exec(r.Context(), `DELETE FROM satellites WHERE id = $1`, parsedUUID)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to delete satellite: %v", err), "component", "api")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to delete satellite"})
		return
	}

	if result.RowsAffected() == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "satellite not found"})
		return
	}

	// Audit log the satellite delete
	if h.auditLogger != nil {
		ctx := r.Context()
		ctx = audit.WithClientIP(ctx, audit.ClientIPFromRequest(r))
		h.auditLogger.Log(ctx, "satellite.delete", "satellite", satelliteID, map[string]interface{}{
			"satellite_id": satelliteID,
		})
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"deleted"}`))
}

// HandleRenameSatellite handles renaming a satellite via PATCH /api/v1/satellites/{id}/name
func (h *Handlers) HandleRenameSatellite(w http.ResponseWriter, r *http.Request, satelliteID string) {
	w.Header().Set("Content-Type", "application/json")

	parsedUUID, err := uuid.Parse(satelliteID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid satellite ID format"})
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid JSON request body"})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "name is required"})
		return
	}

	if len(req.Name) > 255 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "name must not exceed 255 characters"})
		return
	}

	if h.dbPool == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "database not configured"})
		return
	}

	result, err := h.dbPool.Exec(r.Context(),
		`UPDATE satellites SET name = $1, updated_at = NOW() WHERE id = $2`,
		req.Name, parsedUUID,
	)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleRenameSatellite: failed to update satellite name: %v", err), "component", "api")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to rename satellite"})
		return
	}

	if result.RowsAffected() == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "satellite not found"})
		return
	}

	// Audit log the satellite rename
	if h.auditLogger != nil {
		ctx := r.Context()
		ctx = audit.WithClientIP(ctx, audit.ClientIPFromRequest(r))
		h.auditLogger.Log(ctx, "satellite.rename", "satellite", satelliteID, map[string]interface{}{
			"satellite_id": satelliteID,
			"new_name":     req.Name,
		})
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"id": parsedUUID.String(), "name": req.Name})
}

// HandleUpdateSatelliteTags handles updating satellite tags via PATCH /api/v1/satellites/{id}/tags
func (h *Handlers) HandleUpdateSatelliteTags(w http.ResponseWriter, r *http.Request, satelliteID string) {
	w.Header().Set("Content-Type", "application/json")

	parsedUUID, err := uuid.Parse(satelliteID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid satellite ID format"})
		return
	}

	var req struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid JSON request body"})
		return
	}

	// Validate tags: lowercase alphanumeric + hyphens, max 50 chars, max 20 tags
	if len(req.Tags) > 20 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "maximum 20 tags allowed"})
		return
	}

	tagRegex := regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	for _, tag := range req.Tags {
		if len(tag) > 50 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "tag must not exceed 50 characters"})
			return
		}
		if !tagRegex.MatchString(tag) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "tag must be lowercase alphanumeric with hyphens"})
			return
		}
	}

	if h.dbPool == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "database not configured"})
		return
	}

	result, err := h.dbPool.Exec(r.Context(),
		`UPDATE satellites SET tags = $1, updated_at = NOW() WHERE id = $2`,
		req.Tags, parsedUUID,
	)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleUpdateSatelliteTags: failed to update satellite tags: %v", err), "component", "api")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to update satellite tags"})
		return
	}

	if result.RowsAffected() == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "satellite not found"})
		return
	}

	// Audit log the satellite tag update
	if h.auditLogger != nil {
		ctx := r.Context()
		ctx = audit.WithClientIP(ctx, audit.ClientIPFromRequest(r))
		h.auditLogger.Log(ctx, "satellite.update_tags", "satellite", satelliteID, map[string]interface{}{
			"satellite_id": satelliteID,
			"tags":         req.Tags,
		})
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"id": parsedUUID.String(), "tags": req.Tags})
}

// HandleGetConfig returns the DMS/heartbeat configuration
func (h *Handlers) HandleGetConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	dmsTTL := 60 // default
	if ttlStr := os.Getenv("DAAO_DMS_TTL"); ttlStr != "" {
		var ttl int
		if _, err := fmt.Sscanf(ttlStr, "%d", &ttl); err == nil && ttl > 0 {
			dmsTTL = ttl
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ConfigResponse{
		DMSTTLMinutes:            dmsTTL,
		HeartbeatIntervalSeconds: 30,
	})
}

// HandleSatelliteHeartbeat handles satellite heartbeat pings to update status
func (h *Handlers) HandleSatelliteHeartbeat(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid request body"}`))
		return
	}

	// Try to match by satellite UUID first, then by fingerprint
	identifier := req.SatelliteID
	if identifier == "" {
		identifier = req.Fingerprint
	}

	if identifier == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"fingerprint or satellite_id required"}`))
		return
	}

	if h.dbPool == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":"database not configured"}`))
		return
	}

	// Update status — try UUID match first, fall back to partial name match
	result, err := h.dbPool.Exec(r.Context(),
		`UPDATE satellites SET status = 'active', updated_at = NOW() 
		 WHERE id::text = $1 OR name = $1`,
		identifier,
	)
	if err != nil {
		slog.Error(fmt.Sprintf("Heartbeat: Failed to update satellite status: %v", err), "component", "api")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"failed to update status"}`))
		return
	}

	if result.RowsAffected() == 0 {
		// No exact match — activate most recently created pending satellite
		result, err = h.dbPool.Exec(r.Context(),
			`UPDATE satellites SET status = 'active', updated_at = NOW()
			 WHERE id = (SELECT id FROM satellites WHERE status = 'pending' ORDER BY created_at DESC LIMIT 1)`,
		)
		if err != nil {
			slog.Error(fmt.Sprintf("Heartbeat: Failed to update pending satellite: %v", err), "component", "api")
		}
		if result.RowsAffected() > 0 {
			slog.Info(fmt.Sprintf("Heartbeat: Activated most recent pending satellite for fingerprint %s", identifier), "component", "api")
		}
	} else {
		slog.Info(fmt.Sprintf("Heartbeat: Satellite %s is active", identifier), "component", "api")
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// HandleGetTelemetry returns the latest telemetry data for a satellite
func (h *Handlers) HandleGetTelemetry(w http.ResponseWriter, r *http.Request, satelliteID string) {
	w.Header().Set("Content-Type", "application/json")

	if h.dbPool == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "database not configured"})
		return
	}

	// Get latest telemetry row for this satellite
	var resp TelemetryResponse
	var gpuData []byte
	err := h.rpool().QueryRow(r.Context(),
		`SELECT satellite_id::text, cpu_percent, memory_percent, memory_used_bytes, memory_total_bytes,
		        disk_percent, disk_used_bytes, disk_total_bytes, gpu_data, active_sessions, created_at
		 FROM satellite_telemetry
		 WHERE satellite_id::text = $1
		 ORDER BY created_at DESC
		 LIMIT 1`,
		satelliteID,
	).Scan(
		&resp.SatelliteID, &resp.CPUPercent, &resp.MemoryPercent,
		&resp.MemoryUsed, &resp.MemoryTotal,
		&resp.DiskPercent, &resp.DiskUsed, &resp.DiskTotal,
		&gpuData, &resp.ActiveSessions, &resp.CollectedAt,
	)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "no telemetry data available for this satellite"})
		return
	}

	if gpuData != nil {
		resp.GPUs = json.RawMessage(gpuData)
	} else {
		resp.GPUs = json.RawMessage("[]")
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// HandleGetTelemetryHistory returns telemetry history for a satellite (for sparklines/graphs)
func (h *Handlers) HandleGetTelemetryHistory(w http.ResponseWriter, r *http.Request, satelliteID string) {
	w.Header().Set("Content-Type", "application/json")

	if h.dbPool == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "database not configured"})
		return
	}

	type TelemetryPoint struct {
		CPUPercent     float64         `json:"cpu_percent"`
		MemoryPercent  float64         `json:"memory_percent"`
		DiskPercent    float64         `json:"disk_percent"`
		GPUs           json.RawMessage `json:"gpus"`
		ActiveSessions int             `json:"active_sessions"`
		Timestamp      time.Time       `json:"timestamp"`
	}

	var points []TelemetryPoint

	if h.timescaleEnabled {
		// Use TimescaleDB continuous aggregate for better performance
		// Get last 60 hours of aggregated data (roughly equivalent time range to 60 raw points at 1/min)
		rows, err := h.rpool().Query(r.Context(),
			`SELECT hour, avg_cpu, avg_mem, avg_disk, peak_mem_bytes
			 FROM satellite_telemetry_hourly
			 WHERE satellite_id::text = $1
			 ORDER BY hour DESC
			 LIMIT 60`,
			satelliteID,
		)
		if err != nil {
			slog.Error(fmt.Sprintf("HandleGetTelemetryHistory: timescale query failed: %v", err), "component", "api")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to query telemetry history"})
			return
		}
		defer rows.Close()

		for rows.Next() {
			var pt TelemetryPoint
			var hour time.Time
			var avgCPU, avgMem, avgDisk sql.NullFloat64
			var peakMemBytes sql.NullInt64
			if err := rows.Scan(&hour, &avgCPU, &avgMem, &avgDisk, &peakMemBytes); err != nil {
				continue
			}
			pt.Timestamp = hour
			if avgCPU.Valid {
				pt.CPUPercent = avgCPU.Float64
			}
			if avgMem.Valid {
				pt.MemoryPercent = avgMem.Float64
			}
			if avgDisk.Valid {
				pt.DiskPercent = avgDisk.Float64
			}
			pt.GPUs = json.RawMessage("[]")
			pt.ActiveSessions = 0 // Not available in hourly aggregate
			points = append(points, pt)
		}
	} else {
		// Get last 60 data points (for ~1 hour at 1 report/min)
		rows, err := h.rpool().Query(r.Context(),
			`SELECT cpu_percent, memory_percent, disk_percent, gpu_data, active_sessions, created_at
			 FROM satellite_telemetry
			 WHERE satellite_id::text = $1
			 ORDER BY created_at DESC
			 LIMIT 60`,
			satelliteID,
		)
		if err != nil {
			slog.Error(fmt.Sprintf("HandleGetTelemetryHistory: query failed: %v", err), "component", "api")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to query telemetry history"})
			return
		}
		defer rows.Close()

		for rows.Next() {
			var pt TelemetryPoint
			var gpuData []byte
			if err := rows.Scan(&pt.CPUPercent, &pt.MemoryPercent, &pt.DiskPercent, &gpuData, &pt.ActiveSessions, &pt.Timestamp); err != nil {
				continue
			}
			if gpuData != nil {
				pt.GPUs = json.RawMessage(gpuData)
			} else {
				pt.GPUs = json.RawMessage("[]")
			}
			points = append(points, pt)
		}
	}

	if points == nil {
		points = []TelemetryPoint{}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(points)
}

// WriteJSON writes a JSON response
func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error(fmt.Sprintf("Failed to encode JSON response: %v", err), "component", "api")
	}
}

// GetContext returns a context with timeout
func GetContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

// HandleLicenseInfo returns the current license tier, limits, and available enterprise features.
// This endpoint is used by the Cockpit UI to show/hide enterprise features and display
// upgrade nudges for locked functionality.
func (h *Handlers) HandleLicenseInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	type LicenseResponse struct {
		Tier                    string                `json:"tier"`
		MaxUsers                int                   `json:"max_users"`
		MaxSatellites           int                   `json:"max_satellites"`
		MaxRecordings           int                   `json:"max_recordings"`
		TelemetryRetentionHours int                   `json:"telemetry_retention_hours"`
		Customer                string                `json:"customer,omitempty"`
		ExpiresAt               int64                 `json:"expires_at,omitempty"`
		EnterpriseFeatures      []license.FeatureInfo `json:"enterprise_features"`
	}

	resp := LicenseResponse{
		EnterpriseFeatures: license.AllEnterpriseFeatures(),
	}

	if h.licenseManager != nil {
		resp.Tier = string(h.licenseManager.Tier())
		resp.MaxUsers = h.licenseManager.MaxUsers()
		resp.MaxSatellites = h.licenseManager.MaxSatellites()
		resp.MaxRecordings = h.licenseManager.MaxRecordings()
		resp.TelemetryRetentionHours = h.licenseManager.TelemetryRetentionHours()
		if claims := h.licenseManager.Claims(); claims != nil {
			resp.Customer = claims.Customer
			resp.ExpiresAt = claims.ExpiresAt
		}
	} else {
		resp.Tier = string(license.TierCommunity)
		resp.MaxUsers = license.CommunityMaxUsers
		resp.MaxSatellites = license.CommunityMaxSatellites
		resp.MaxRecordings = license.CommunityMaxRecordings
		resp.TelemetryRetentionHours = license.CommunityTelemetryRetention
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// ansiRegex strips ANSI escape sequences for clean text preview
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07]*\x07|\x1b[()][A-B0-2]|\x1b[PX^_][^\x1b]*\x1b\\|\x1b.`)

// HandleSessionPreview returns a plain-text preview of the last terminal output
// for a session, sourced from the in-memory ring buffer.
func (h *Handlers) HandleSessionPreview(w http.ResponseWriter, r *http.Request, sessionID string) {
	w.Header().Set("Content-Type", "application/json")

	if h.ringBufferPool == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"text": "", "has_content": false})
		return
	}

	buf := h.ringBufferPool.GetBuffer(sessionID)
	if buf == nil || buf.Len() == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{"text": "", "has_content": false})
		return
	}

	snapshot := buf.Snapshot()

	// Take last 2KB max
	const maxPreview = 2048
	if len(snapshot) > maxPreview {
		snapshot = snapshot[len(snapshot)-maxPreview:]
	}

	// Strip ANSI escape sequences
	clean := ansiRegex.ReplaceAll(snapshot, nil)

	// Convert to string and take last 12 lines
	text := string(clean)
	lines := strings.Split(text, "\n")
	if len(lines) > 12 {
		lines = lines[len(lines)-12:]
	}

	// Trim empty trailing lines
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	previewText := strings.Join(lines, "\n")

	json.NewEncoder(w).Encode(map[string]interface{}{
		"text":        previewText,
		"has_content": len(previewText) > 0,
	})
}
func (h *Handlers) HandleListProposals(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.hitlManager == nil {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
		return
	}
	var statusFilter *hitl.ProposalStatus
	if s := r.URL.Query().Get("status"); s != "" {
		st := hitl.ProposalStatus(s)
		statusFilter = &st
	}
	proposals, err := h.hitlManager.Store().List(r.Context(), statusFilter, 100)
	if err != nil {
		slog.Info(fmt.Sprintf("HandleListProposals: %v", err), "component", "api")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to list proposals"})
		return
	}
	if proposals == nil {
		proposals = []*hitl.Proposal{}
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(proposals)
}

// HandleProposalCount returns the count of pending proposals.
func (h *Handlers) HandleProposalCount(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.hitlManager == nil {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"count":0}`))
		return
	}
	count, err := h.hitlManager.Store().CountPending(r.Context())
	if err != nil {
		slog.Info(fmt.Sprintf("HandleProposalCount: %v", err), "component", "api")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to count proposals"})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]int{"count": count})
}

// HandleGetProposal returns a single proposal by ID.
func (h *Handlers) HandleGetProposal(w http.ResponseWriter, r *http.Request, proposalID string) {
	w.Header().Set("Content-Type", "application/json")
	if h.hitlManager == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "HITL not enabled"})
		return
	}
	id, err := uuid.Parse(proposalID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid proposal ID"})
		return
	}
	p, err := h.hitlManager.Store().Get(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "proposal not found"})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(p)
}

// HandleApproveProposal approves a pending proposal.
func (h *Handlers) HandleApproveProposal(w http.ResponseWriter, r *http.Request, proposalID string) {
	w.Header().Set("Content-Type", "application/json")
	if h.hitlManager == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "HITL not enabled"})
		return
	}
	id, err := uuid.Parse(proposalID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid proposal ID"})
		return
	}
	if err := h.hitlManager.Approve(r.Context(), id, "operator"); err != nil {
		slog.Info(fmt.Sprintf("HandleApproveProposal: %v", err), "component", "api")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to approve proposal"})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "approved"})
}

// HandleDenyProposal denies a pending proposal.
func (h *Handlers) HandleDenyProposal(w http.ResponseWriter, r *http.Request, proposalID string) {
	w.Header().Set("Content-Type", "application/json")
	if h.hitlManager == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "HITL not enabled"})
		return
	}
	id, err := uuid.Parse(proposalID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid proposal ID"})
		return
	}
	if err := h.hitlManager.Deny(r.Context(), id, "operator"); err != nil {
		slog.Info(fmt.Sprintf("HandleDenyProposal: %v", err), "component", "api")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to deny proposal"})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "denied"})
}
