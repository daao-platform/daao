// Package main provides the satellite daemon control API.
//
// The ControlServer exposes a local-only JSON protocol over a Unix domain
// socket (POSIX) or Named Pipe (Windows) that lets the `daao sessions` and
// `daao attach` CLI commands communicate with the running daemon.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ControlRequest is the envelope for CLI → daemon requests.
type ControlRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// ControlResponse is the envelope for daemon → CLI responses.
type ControlResponse struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// SessionInfo is the JSON-serialisable summary of a session.
type SessionInfo struct {
	ID          string `json:"id"`
	State       string `json:"state"`
	AgentBinary string `json:"agent_binary,omitempty"`
	PID         int    `json:"pid,omitempty"`
	BufferLen   int    `json:"buffer_len"`
}

// AttachParams carries the session_id for an attach request.
type AttachParams struct {
	SessionID string `json:"session_id"`
}

// ControlServer listens on the daemon control socket and serves requests.
type ControlServer struct {
	daemon   *Daemon
	listener net.Listener
	wg       sync.WaitGroup
	done     chan struct{}
}

// NewControlServer creates a new control server for the given daemon.
func NewControlServer(daemon *Daemon) *ControlServer {
	return &ControlServer{
		daemon: daemon,
		done:   make(chan struct{}),
	}
}

// Start begins listening for control connections.
func (cs *ControlServer) Start() error {
	sockPath := getDaemonSocketPath()

	// Ensure parent directory exists
	if dir := filepath.Dir(sockPath); dir != "" {
		os.MkdirAll(dir, 0700)
	}

	ln, err := controlListen(sockPath)
	if err != nil {
		return fmt.Errorf("control server listen: %w", err)
	}
	cs.listener = ln

	cs.wg.Add(1)
	go cs.acceptLoop()

	log.Printf("ControlServer: listening on %s", sockPath)
	return nil
}

// Stop shuts down the control server.
func (cs *ControlServer) Stop() {
	close(cs.done)
	if cs.listener != nil {
		cs.listener.Close()
	}
	cs.wg.Wait()
	controlCleanup(getDaemonSocketPath())
}

// acceptLoop accepts new control connections.
func (cs *ControlServer) acceptLoop() {
	defer cs.wg.Done()
	for {
		conn, err := cs.listener.Accept()
		if err != nil {
			select {
			case <-cs.done:
				return
			default:
			}
			// Transient error, retry
			time.Sleep(50 * time.Millisecond)
			continue
		}
		cs.wg.Add(1)
		go func() {
			defer cs.wg.Done()
			cs.handleConnection(conn)
		}()
	}
}

// handleConnection reads one request, dispatches, and may enter I/O bridge
// mode for attach.
func (cs *ControlServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Read one line of JSON (the request).
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return
	}

	var req ControlRequest
	if err := json.Unmarshal(line, &req); err != nil {
		cs.sendError(conn, "invalid request JSON")
		return
	}

	switch req.Method {
	case "list_sessions":
		cs.handleListSessions(conn)
	case "attach_session":
		cs.handleAttachSession(conn, reader, req.Params)
	default:
		cs.sendError(conn, fmt.Sprintf("unknown method: %s", req.Method))
	}
}

// handleListSessions returns a JSON array of active sessions.
func (cs *ControlServer) handleListSessions(conn net.Conn) {
	cs.daemon.sessionsMu.RLock()
	sessions := make([]SessionInfo, 0, len(cs.daemon.sessions))
	for _, sess := range cs.daemon.sessions {
		info := SessionInfo{
			ID:    sess.ID,
			State: string(sess.State),
		}
		if sess.Process != nil {
			info.PID = sess.Process.Pid
		}
		if sess.RingBuffer != nil {
			info.BufferLen = sess.RingBuffer.Len()
		}
		// Try to get agent binary from the session's IPC server config
		// (not directly available — we'll store it separately)
		info.AgentBinary = sess.AgentBinary
		sessions = append(sessions, info)
	}
	cs.daemon.sessionsMu.RUnlock()

	data, _ := json.Marshal(sessions)
	resp := ControlResponse{OK: true, Data: data}
	respBytes, _ := json.Marshal(resp)
	conn.Write(append(respBytes, '\n'))
}

// handleAttachSession bridges the control connection to a session's PTY.
func (cs *ControlServer) handleAttachSession(conn net.Conn, reader *bufio.Reader, paramsRaw json.RawMessage) {
	var params AttachParams
	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		cs.sendError(conn, "invalid attach params")
		return
	}

	cs.daemon.sessionsMu.RLock()
	sess, ok := cs.daemon.sessions[params.SessionID]
	cs.daemon.sessionsMu.RUnlock()

	if !ok {
		// Try partial match
		cs.daemon.sessionsMu.RLock()
		for id, s := range cs.daemon.sessions {
			if len(params.SessionID) >= 4 && id[:len(params.SessionID)] == params.SessionID {
				sess = s
				ok = true
				break
			}
		}
		cs.daemon.sessionsMu.RUnlock()
	}

	if !ok {
		cs.sendError(conn, fmt.Sprintf("session %s not found", params.SessionID))
		return
	}

	if sess.Pty == nil {
		cs.sendError(conn, "session has no PTY (may be terminated)")
		return
	}

	// Send OK + scrollback hydration as the initial response.
	// Format: {"ok":true,"data":{"session_id":"...", "hydration_len": N}}\n<hydration bytes>
	var hydration []byte
	if sess.RingBuffer != nil {
		hydration = sess.RingBuffer.Snapshot()
	}

	meta := map[string]interface{}{
		"session_id":    sess.ID,
		"hydration_len": len(hydration),
	}
	metaBytes, _ := json.Marshal(meta)
	resp := ControlResponse{OK: true, Data: metaBytes}
	respBytes, _ := json.Marshal(resp)
	if _, err := conn.Write(append(respBytes, '\n')); err != nil {
		return
	}

	// Send hydration data
	if len(hydration) > 0 {
		if _, err := conn.Write(hydration); err != nil {
			return
		}
	}

	// Record DMS activity to keep the session alive
	if sess.DMS != nil {
		sess.DMS.RecordActivity()
	}

	// Register as a local client for live output fan-out.
	localWriter := &connWriter{conn: conn}
	sess.addLocalClient(localWriter)
	defer sess.removeLocalClient(localWriter)

	log.Printf("ControlServer: local attach to session %s", sess.ID)

	// Bidirectional I/O bridge:
	// conn stdin → PTY (in this goroutine)
	// PTY → conn stdout (handled by forwardPtyOutput fan-out via localClients)
	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("ControlServer: attach read error for session %s: %v", sess.ID, err)
			}
			break
		}
		if n > 0 {
			if _, werr := sess.Pty.Write(buf[:n]); werr != nil {
				log.Printf("ControlServer: PTY write error for session %s: %v", sess.ID, werr)
				break
			}
			// Record DMS activity on every input
			if sess.DMS != nil {
				sess.DMS.RecordActivity()
			}
		}
	}

	log.Printf("ControlServer: local detach from session %s", sess.ID)
}

// sendError sends a JSON error response.
func (cs *ControlServer) sendError(conn net.Conn, msg string) {
	resp := ControlResponse{OK: false, Error: msg}
	respBytes, _ := json.Marshal(resp)
	conn.Write(append(respBytes, '\n'))
}

// connWriter wraps a net.Conn as an io.Writer for the local client fan-out.
type connWriter struct {
	conn net.Conn
}

func (cw *connWriter) Write(p []byte) (int, error) {
	return cw.conn.Write(p)
}
