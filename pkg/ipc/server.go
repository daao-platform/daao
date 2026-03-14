// Package ipc provides inter-process communication via Unix Domain Sockets
// (POSIX) and Named Pipes (Windows).
package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// MaxConnections is the maximum number of concurrent connections the server
// supports to prevent file descriptor exhaustion.
const MaxConnections = 1000

// ErrMaxConnections returned when server has reached capacity.
var ErrMaxConnections = errors.New("max connections reached")

// ErrServerClosed returned when operations are attempted on a closed server.
var ErrServerClosed = errors.New("server closed")

// ServerConfig holds configuration for the IPC server.
type ServerConfig struct {
	// SessionID is the unique identifier for this IPC session.
	// If empty, a new UUID will be generated.
	SessionID string

	// OnConnect is called when a new client connects.
	OnConnect func(conn *Conn)

	// OnDisconnect is called when a client disconnects.
	OnDisconnect func(conn *Conn, err error)

	// OnMessage is called when a JSON-RPC message is received.
	// If not set, messages are handled internally.
	OnMessage func(conn *Conn, msg *JSONRPCMessage) (response interface{}, err error)
}

// Server represents an IPC server that accepts connections from AI processes.
type Server struct {
	config      ServerConfig
	sessionID   string
	socketPath  string
	listener    net.Listener
	conns       map[*Conn]struct{}
	connsMu     sync.RWMutex
	closeOnce   sync.Once
	closed      atomic.Bool
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	
	// Connection counting
	activeConns atomic.Int32
}

// NewServer creates a new IPC server with the given configuration.
// The server creates a Unix domain socket (POSIX) or Named Pipe (Windows)
// at /tmp/daao-sess-<uuid>.sock or the Windows equivalent.
func NewServer(config ServerConfig) (*Server, error) {
	sessionID := config.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	socketPath := socketPathForSession(sessionID)

	ctx, cancel := context.WithCancel(context.Background())

	s := &Server{
		config:     config,
		sessionID:  sessionID,
		socketPath: socketPath,
		conns:      make(map[*Conn]struct{}),
		ctx:        ctx,
		cancel:     cancel,
	}

	return s, nil
}

// SessionID returns the unique session identifier for this server.
func (s *Server) SessionID() string {
	return s.sessionID
}

// SocketPath returns the filesystem path (Unix) or pipe name (Windows)
// for the IPC endpoint.
func (s *Server) SocketPath() string {
	return s.socketPath
}

// Addr returns the network address of the server's listener.
func (s *Server) Addr() net.Addr {
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

// activeConnections returns the current number of active connections.
func (s *Server) activeConnections() int32 {
	return s.activeConns.Load()
}

// canAccept returns true if the server can accept a new connection.
func (s *Server) canAccept() bool {
	return s.activeConns.Load() < MaxConnections
}

// Start starts the IPC server and begins accepting connections.
// It blocks until the server is closed or an error occurs.
func (s *Server) Start() error {
	if s.closed.Load() {
		return ErrServerClosed
	}

	// Create the listener
	ln, err := createListener(s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to create IPC listener: %w", err)
	}

	s.listener = ln

	// Start the accept loop
	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// Close closes the server and all active connections.
// It cleans up the socket file on Unix systems.
func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		s.cancel()

		// Close the listener
		if s.listener != nil {
			s.listener.Close()
		}

		// Close all connections
		s.connsMu.RLock()
		for conn := range s.conns {
			conn.Close()
		}
		s.connsMu.RUnlock()

		// Wait for all goroutines to complete
		s.wg.Wait()

		// Clean up socket file on Unix
		cleanupSocket(s.socketPath)
	})

	return nil
}

// acceptLoop runs in a goroutine and accepts incoming connections.
func (s *Server) acceptLoop() {
	defer s.wg.Done()
	defer s.listener.Close()

	for {
		// Check for context cancellation
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// Set a deadline to allow checking context periodically
		if ul, ok := s.listener.(*net.UnixListener); ok {
			ul.SetDeadline(timeNow().Add(acceptLoopInterval))
		}

		conn, err := s.listener.Accept()
		if err != nil {
			// Check if we should continue
			select {
			case <-s.ctx.Done():
				return
			default:
			}

			// Check if it's a timeout error (expected for periodic checks)
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue
			}

			// Log other errors but continue
			continue
		}

		// Check connection limit
		if !s.canAccept() {
			conn.Close()
			continue
		}

		// Handle the connection
		s.handleConn(conn)
	}
}

// handleConn handles a new connection.
func (s *Server) handleConn(nc net.Conn) {
	conn := newConn(nc, s)

	s.connsMu.Lock()
	s.conns[conn] = struct{}{}
	s.connsMu.Unlock()

	s.activeConns.Add(1)

	// Call onConnect callback
	if s.config.OnConnect != nil {
		s.config.OnConnect(conn)
	}

	// Handle connection in goroutine
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.removeConn(conn)

		s.serveConn(conn)
	}()
}

// removeConn removes a connection from the server's tracking.
func (s *Server) removeConn(conn *Conn) {
	s.connsMu.Lock()
	delete(s.conns, conn)
	s.connsMu.Unlock()

	s.activeConns.Add(-1)

	if s.config.OnDisconnect != nil {
		s.config.OnDisconnect(conn, conn.err)
	}
}

// serveConn reads and processes messages from a connection.
func (s *Server) serveConn(conn *Conn) {
	dec := json.NewDecoder(conn)
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// Set read deadline
		conn.SetReadDeadline(timeNow().Add(readDeadline))

		var msg JSONRPCMessage
		if err := dec.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return
			}
			// Send error response
			_ = conn.SendResponse(NewErrorResponse(nil, -32700, err.Error()))
			continue
		}

		// Handle message
		if s.config.OnMessage != nil {
			resp, err := s.config.OnMessage(conn, &msg)
			if err != nil {
				_ = conn.SendResponse(NewErrorResponse(msg.ID, -32603, err.Error()))
				continue
			}
			if resp != nil {
				if r, err := NewResponse(msg.ID, resp); err == nil {
					_ = conn.SendResponse(r)
				}
			}
		} else {
			// Default handling
			s.handleMessage(conn, &msg)
		}
	}
}

// handleMessage provides default message handling.
func (s *Server) handleMessage(conn *Conn, msg *JSONRPCMessage) {
	if msg.Method == "" {
		_ = conn.SendResponse(NewErrorResponse(msg.ID, -32600, "Method not found"))
		return
	}

	// Default: echo method name as acknowledgment
	if r, err := NewResponse(msg.ID, map[string]string{
		"method": msg.Method,
		"status": "ok",
	}); err == nil {
		_ = conn.SendResponse(r)
	}
}

// timeNow returns current time for testing.
var timeNow = func() time.Time {
	return time.Now()
}

var acceptLoopInterval = 100 * time.Millisecond
var readDeadline = 30 * time.Second
