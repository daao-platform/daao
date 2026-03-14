package transport

import (
	"context"
	"encoding/json"
	"log/slog"
	"fmt"
	"net/http"
	"time"

	"github.com/daao/nexus/internal/auth"
	"github.com/daao/nexus/internal/session"
	"github.com/daao/nexus/internal/stream"
	"github.com/daao/nexus/internal/transport/cors"
	"github.com/daao/nexus/proto"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// WebSocketHandler handles WebSocket connections for session streaming
type WebSocketHandler struct {
	sessionStore *session.SessionStore
	upgrader     websocket.Upgrader
	jwtValidator *auth.JWTTokenValidator
	oidcEnabled  bool
}

// NewWebSocketHandler creates a new WebSocket handler with the given session store
func NewWebSocketHandler(sessionStore *session.SessionStore, jwtValidator *auth.JWTTokenValidator, oidcEnabled bool) *WebSocketHandler {
	return &WebSocketHandler{
		sessionStore: sessionStore,
		jwtValidator: jwtValidator,
		oidcEnabled:  oidcEnabled,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     CheckOrigin,
		},
	}
}

// CheckOrigin validates the origin header for CORS compliance.
// Delegates to the shared cors package used by both WebSocket and WebTransport.
func CheckOrigin(r *http.Request) bool {
	return cors.CheckOrigin(r)
}

// authenticateWebSocket performs first-message JWT authentication on a WebSocket connection.
// It returns whether authentication succeeded and the first message bytes (for dev mode processing).
// In dev mode (oidcEnabled=false), the first non-auth message is returned for normal processing.
func authenticateWebSocket(conn *websocket.Conn, jwtValidator *auth.JWTTokenValidator, oidcEnabled bool) (bool, []byte) {
	// 1. Set read deadline to 5 seconds
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// 2. Read first message
	_, msg, err := conn.ReadMessage()
	if err != nil {
		if oidcEnabled {
			conn.WriteJSON(map[string]string{"type": "auth_error", "message": "auth timeout"})
			conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(4401, "authentication required"))
			return false, nil
		}
		return true, nil // dev mode — no auth needed
	}

	// 3. Try to parse as auth message
	var authMsg struct {
		Type  string `json:"type"`
		Token string `json:"token"`
	}
	if json.Unmarshal(msg, &authMsg) == nil && authMsg.Type == "auth" {
		if jwtValidator == nil {
			conn.WriteJSON(map[string]string{"type": "auth_ok"})
			conn.SetReadDeadline(time.Time{}) // clear deadline
			return true, nil
		}
		_, err := jwtValidator.Validate(authMsg.Token)
		if err != nil {
			conn.WriteJSON(map[string]string{"type": "auth_error", "message": err.Error()})
			conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(4401, "invalid token"))
			return false, nil
		}
		conn.WriteJSON(map[string]string{"type": "auth_ok"})
		conn.SetReadDeadline(time.Time{}) // clear deadline
		return true, nil
	}

	// 4. First message is NOT auth — in dev mode, process it normally
	if !oidcEnabled {
		conn.SetReadDeadline(time.Time{}) // clear deadline
		return true, msg                  // caller should process msg as normal data
	}

	// 5. OIDC enabled but no auth message
	conn.WriteJSON(map[string]string{"type": "auth_error", "message": "authentication required"})
	conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(4401, "authentication required"))
	return false, nil
}

// HandleSessionStream provides real-time session updates via WebSocket
func (h *WebSocketHandler) HandleSessionStream(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error(fmt.Sprintf("WebSocket upgrade failed: %v", err), "component", "transport")
		return
	}
	defer conn.Close()
	conn.SetReadLimit(8192) // 8 KB — session update JSON only

	slog.Info(fmt.Sprintf("WebSocket session stream connected from %s", r.RemoteAddr), "component", "transport")

	// Authenticate using first-message protocol.
	// Always call authenticateWebSocket so that local-auth clients
	// (which send {"type":"auth","token":"..."}) get an auth_ok reply
	// even when oidcEnabled=false. Without this, the client waits
	// forever for auth_ok and the connection deadlocks.
	if h.oidcEnabled && h.jwtValidator == nil {
		slog.Warn(fmt.Sprintf("WebSocket: WARNING - OIDC enabled but no JWT validator configured"), "component", "transport")
		conn.WriteJSON(map[string]string{"type": "auth_error", "message": "server configuration error"})
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(4401, "authentication required"))
		return
	}
	authOK, _ := authenticateWebSocket(conn, h.jwtValidator, h.oidcEnabled)
	if !authOK {
		return
	}

	// Set up ping/pong for keepalive
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Read pump (detect client disconnect)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	// Send initial session list
	if err := h.sendSessionUpdate(conn); err != nil {
		return
	}

	// Poll and send updates every 2 seconds
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case <-done:
			slog.Info(fmt.Sprintf("WebSocket session stream disconnected from %s", r.RemoteAddr), "component", "transport")
			return
		case <-ticker.C:
			if err := h.sendSessionUpdate(conn); err != nil {
				return
			}
		case <-pingTicker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		}
	}
}

// sendSessionUpdate sends the current session list to a WebSocket client
func (h *WebSocketHandler) sendSessionUpdate(conn *websocket.Conn) error {
	var sessions []*session.Session

	if h.sessionStore != nil {
		var err error
		sessions, err = h.sessionStore.ListActiveSessions(context.Background())
		if err != nil {
			slog.Error(fmt.Sprintf("Failed to list sessions for stream: %v", err), "component", "transport")
			sessions = []*session.Session{}
		}
	}

	if sessions == nil {
		sessions = []*session.Session{}
	}

	msg := map[string]interface{}{
		"type":     "session_update",
		"sessions": sessions,
	}

	return conn.WriteJSON(msg)
}

// TerminalStreamHandler handles per-session WebSocket connections for terminal I/O.
// It streams ring-buffer output to the browser and forwards keyboard input to the satellite.
type TerminalStreamHandler struct {
	sessionStore   session.Store
	ringBufferPool *session.RingBufferPool
	streamRegistry stream.StreamRegistryInterface
	upgrader       websocket.Upgrader
	jwtValidator   *auth.JWTTokenValidator
	oidcEnabled    bool
}

// NewTerminalStreamHandler creates a new TerminalStreamHandler.
func NewTerminalStreamHandler(
	sessionStore session.Store,
	ringBufferPool *session.RingBufferPool,
	streamRegistry stream.StreamRegistryInterface,
	jwtValidator *auth.JWTTokenValidator,
	oidcEnabled bool,
) *TerminalStreamHandler {
	return &TerminalStreamHandler{
		sessionStore:   sessionStore,
		ringBufferPool: ringBufferPool,
		streamRegistry: streamRegistry,
		jwtValidator:   jwtValidator,
		oidcEnabled:    oidcEnabled,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin:     CheckOrigin,
		},
	}
}

// HandleTerminalStream handles the WebSocket connection for a specific session's terminal.
func (h *TerminalStreamHandler) HandleTerminalStream(w http.ResponseWriter, r *http.Request, sessionID string) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error(fmt.Sprintf("TerminalStream: upgrade failed for session %s: %v", sessionID, err), "component", "transport")
		return
	}
	defer conn.Close()
	conn.SetReadLimit(65536) // 64 KB — keyboard input + resize JSON

	slog.Info(fmt.Sprintf("TerminalStream: client connected for session %s", sessionID), "component", "transport")

	// Authenticate using first-message protocol.
	// Always call authenticateWebSocket so that local-auth clients
	// (which send {"type":"auth","token":"..."}) get an auth_ok reply
	// even when oidcEnabled=false. Without this, the client waits
	// forever for auth_ok and the connection deadlocks.
	if h.oidcEnabled && h.jwtValidator == nil {
		slog.Warn(fmt.Sprintf("TerminalStream: WARNING - OIDC enabled but no JWT validator configured for session %s", sessionID), "component", "transport")
		conn.WriteJSON(map[string]string{"type": "auth_error", "message": "server configuration error"})
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(4401, "authentication required"))
		return
	}
	authOK, firstMsg := authenticateWebSocket(conn, h.jwtValidator, h.oidcEnabled)
	if !authOK {
		return
	}
	// If first message was not auth, process it as normal input
	if len(firstMsg) > 0 && h.streamRegistry != nil {
		h.processTerminalMessage(sessionID, firstMsg, conn)
	}

	// Validate session ID and check it exists
	parsedID, err := uuid.Parse(sessionID)
	if err != nil {
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "invalid session ID"))
		return
	}

	if h.sessionStore != nil {
		sess, err := h.sessionStore.GetSession(r.Context(), parsedID)
		if err != nil {
			conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "session not found"))
			return
		}
		// If already terminated, tell the client and close cleanly
		if sess.State == session.StateTerminated {
			_ = conn.WriteJSON(map[string]string{"type": "terminated"})
			return
		}
	}

	// Get ring buffer — use GetBuffer (not GetOrCreateBuffer) to avoid
	// creating a fresh empty buffer for a session that already ended.
	// Note: We check HasBuffer to avoid the Go interface nil gotcha where
	// assigning a nil pointer to an interface creates a non-nil interface
	// containing a nil pointer.
	var rb interface {
		Snapshot() []byte
		SnapshotWithLen() ([]byte, int)
		Len() int
	}
	if h.ringBufferPool != nil && h.ringBufferPool.HasBuffer(sessionID) {
		rb = h.ringBufferPool.GetBuffer(sessionID)
	}

	// Flush existing scrollback to client
	var lastLen int
	if rb != nil {
		snapshot, sLen := rb.SnapshotWithLen()
		if sLen > 0 {
			if err := conn.WriteMessage(websocket.BinaryMessage, snapshot); err != nil {
				return
			}
			lastLen = sLen
		}
	}

	// Read input from browser → forward to satellite via session stream.
	// Two message types arrive over this WebSocket:
	//   1. Raw keystrokes (binary)  → TerminalInput proto
	//   2. Resize JSON {"type":"resize","cols":N,"rows":N} → ResizeCommand proto
	inputDone := make(chan struct{})
	go func() {
		defer close(inputDone)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			// Use the helper to process the message
			h.processTerminalMessage(sessionID, msg, conn)
		}
	}()

	// Poll ring buffer and push new bytes to browser
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	// Also periodically check session state so we close when terminated
	stateTicker := time.NewTicker(2 * time.Second)
	defer stateTicker.Stop()

	for {
		select {
		case <-inputDone:
			slog.Info(fmt.Sprintf("TerminalStream: client disconnected for session %s", sessionID), "component", "transport")
			return

		case <-ticker.C:
			if rb == nil {
				// Try to get the buffer if it appeared after initial connect
				// Use HasBuffer to avoid the interface nil gotcha
				if h.ringBufferPool != nil && h.ringBufferPool.HasBuffer(sessionID) {
					rb = h.ringBufferPool.GetBuffer(sessionID)
				}
				continue
			}
			// Atomically get snapshot and length to avoid race with concurrent writes
			snapshot, sLen := rb.SnapshotWithLen()
			if sLen == lastLen {
				continue
			}
			var toSend []byte
			if sLen > lastLen {
				// Buffer grew — send only new bytes
				toSend = snapshot[lastLen:]
			} else {
				// Buffer wrapped (eviction) — resend full current content
				toSend = snapshot
			}
			if len(toSend) > 0 {
				if err := conn.WriteMessage(websocket.BinaryMessage, toSend); err != nil {
					return
				}
				lastLen = sLen
			}

		case <-stateTicker.C:
			// Check if session has terminated; if so, notify and close
			if h.sessionStore == nil {
				continue
			}
			sess, err := h.sessionStore.GetSession(context.Background(), parsedID)
			if err != nil || sess.State == session.StateTerminated {
				_ = conn.WriteJSON(map[string]string{"type": "terminated"})
				slog.Info(fmt.Sprintf("TerminalStream: session %s terminated, closing WebSocket", sessionID), "component", "transport")
				return
			}
		}
	}
}

// processTerminalMessage processes a terminal message (ping, resize, or input).
// It is used for both the initial message after auth and subsequent messages from the read loop.
// The conn parameter is optional - if provided, ping messages will receive a pong response.
func (h *TerminalStreamHandler) processTerminalMessage(sessionID string, msg []byte, conn *websocket.Conn) {
	if h.streamRegistry == nil || len(msg) == 0 {
		return
	}

	// Respond to ping messages with pong — do not forward to satellite.
	var pingCheck struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(msg, &pingCheck) == nil && pingCheck.Type == "ping" {
		if conn != nil {
			if err := conn.WriteJSON(map[string]string{"type": "pong"}); err != nil {
				slog.Error(fmt.Sprintf("TerminalStream: failed to send pong for session %s: %v", sessionID, err), "component", "transport")
			}
		}
		return
	}

	// Attempt to detect a resize control message.
	var ctrl struct {
		Type string `json:"type"`
		Cols int32  `json:"cols"`
		Rows int32  `json:"rows"`
	}
	if json.Unmarshal(msg, &ctrl) == nil && ctrl.Type == "resize" && ctrl.Cols > 0 && ctrl.Rows > 0 {
		h.streamRegistry.SendToSession(sessionID, &proto.NexusMessage{
			Payload: &proto.NexusMessage_ResizeCommand{
				ResizeCommand: &proto.ResizeCommand{
					SessionId: sessionID,
					Width:     ctrl.Cols,
					Height:    ctrl.Rows,
				},
			},
		})
		slog.Info(fmt.Sprintf("TerminalStream: resize %dx%d for session %s", ctrl.Cols, ctrl.Rows, sessionID), "component", "transport")
		return
	}

	// Plain keystroke / paste data — forward as TerminalInput.
	h.streamRegistry.SendToSession(sessionID, &proto.NexusMessage{
		Payload: &proto.NexusMessage_TerminalInput{
			TerminalInput: &proto.TerminalInput{
				SessionId: sessionID,
				Data:      msg,
			},
		},
	})
}
