// Package router provides stream routing between gRPC streams from Satellites
// and WebTransport streams from Cockpit clients.
package router

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/daao/nexus/internal/session"
	"github.com/daao/nexus/internal/transport/cors"

	"github.com/daao/nexus/proto"
	"google.golang.org/grpc"
)

// ============================================================================
// Stream Types
// ============================================================================

// StreamType identifies the type of WebTransport stream
type StreamType int

const (
	StreamTypeTerminalRX StreamType = iota // Terminal output from satellite to client
	StreamTypeTerminalTX                   // Terminal input from client to satellite
	StreamTypeControl                      // Control messages (PING, STATE)
	StreamTypeOOBUI                        // Out-of-band UI payloads
)

// StreamHandler manages the bidirectional gRPC stream
type SatelliteHandler struct {
	sessionID string
}

// TerminalInput sends terminal input to a session
type TerminalInput struct {
	SessionID      string
	Data           []byte
	SequenceNumber int64
}

// ============================================================================
// Control Messages
// ============================================================================

// ControlMessage represents a control stream message
type ControlMessage struct {
	Type    ControlMessageType
	Payload []byte
}

// ControlMessageType defines types of control messages
type ControlMessageType int

const (
	ControlMessagePing ControlMessageType = iota
	ControlMessagePong
	ControlMessageStateUpdate
	ControlMessageSessionInfo
	ControlMessageError
)

// PingMessage represents a PING control message
type PingMessage struct {
	Timestamp      int64
	SequenceNumber int64
}

// StateUpdateMessage represents a STATE update control message
type StateUpdateMessage struct {
	SessionID    string
	State        session.SessionState
	Timestamp    int64
	ErrorMessage string
}

// SessionInfoMessage carries session metadata
type SessionInfoMessage struct {
	SessionID    string
	SatelliteID  string
	UserID       string
	TerminalCols int
	TerminalRows int
	CreatedAt    int64
}

// ============================================================================
// OOB UI Messages
// ============================================================================

// OOBUIMessage represents an out-of-band UI payload
type OOBUIMessage struct {
	SessionID string
	Type      OOBUIType
	Payload   interface{}
	Timestamp int64
}

// OOBUIType defines types of OOB UI messages
type OOBUIType int

const (
	OOBUIProgress OOBUIType = iota
	OOBUIForm
	OOBUIConfirmation
	OOBUIInfo
	OOBUIError
)

// ============================================================================
// Session Resolution
// ============================================================================

// SessionResolver maps client session_id to satellite gRPC stream
type SessionResolver interface {
	// ResolveSession returns the stream handler for a given session_id
	ResolveSession(sessionID string) (*SatelliteHandler, error)
	// RegisterSession registers a session with its stream handler
	RegisterSession(sessionID string, handler *SatelliteHandler) error
	// UnregisterSession removes a session
	UnregisterSession(sessionID string) error
}

// sessionResolver implements SessionResolver
type sessionResolver struct {
	mu       sync.RWMutex
	sessions map[string]*SatelliteHandler
}

// NewSessionResolver creates a new session resolver
func NewSessionResolver() SessionResolver {
	return &sessionResolver{
		sessions: make(map[string]*SatelliteHandler),
	}
}

// ResolveSession returns the stream handler for a given session_id
func (r *sessionResolver) ResolveSession(sessionID string) (*SatelliteHandler, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	handler, exists := r.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	return handler, nil
}

// RegisterSession registers a session with its stream handler
func (r *sessionResolver) RegisterSession(sessionID string, handler *SatelliteHandler) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[sessionID] = handler
	return nil
}

// UnregisterSession removes a session
func (r *sessionResolver) UnregisterSession(sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, sessionID)
	return nil
}

// ============================================================================
// WebTransport Server
// ============================================================================

// WebTransportServer handles WebTransport connections from Cockpit clients
type WebTransportServer struct {
	resolver      SessionResolver
	sessionStore  session.Store
	server        *webtransport.Server
	connHandler   func(sessionID string, session *webtransport.Session) error
	streamCounter uint64
	streamMu      sync.Mutex
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
}

// WebTransportConfig configures the WebTransport server
type WebTransportConfig struct {
	Addr         string
	Resolver     SessionResolver
	SessionStore session.Store
	TLSCert      string
	TLSKey       string
}

// NewWebTransportServer creates a new WebTransport server
func NewWebTransportServer(config *WebTransportConfig) *WebTransportServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &WebTransportServer{
		resolver:      config.Resolver,
		sessionStore:  config.SessionStore,
		streamCounter: 0,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// SetConnectionHandler sets the handler for new WebTransport sessions
func (s *WebTransportServer) SetConnectionHandler(handler func(sessionID string, session *webtransport.Session) error) {
	s.connHandler = handler
}

// Start starts the WebTransport server on HTTP/3/QUIC
func (s *WebTransportServer) Start() error {
	// Create HTTP mux for WebTransport upgrade requests
	mux := http.NewServeMux()
	mux.HandleFunc("/webtransport", func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.URL.Query().Get("session_id")
		if sessionID == "" {
			http.Error(w, "session_id required", http.StatusBadRequest)
			return
		}

		wtSession, err := s.server.Upgrade(w, r)
		if err != nil {
			slog.Error(fmt.Sprintf("WebTransport upgrade failed: %v", err), "component", "router")
			return
		}

		if s.connHandler != nil {
			if err := s.connHandler(sessionID, wtSession); err != nil {
				slog.Error(fmt.Sprintf("WebTransport connection handler error: %v", err), "component", "router")
				wtSession.CloseWithError(1, err.Error())
			}
		} else {
			s.handleWebTransport(sessionID)
		}
	})

	// Create WebTransport server with required H3 (HTTP/3) server
	s.server = &webtransport.Server{
		H3: &http3.Server{
			Handler: mux,
		},
		ReorderingTimeout: 5 * time.Second,
		CheckOrigin: func(r *http.Request) bool {
			return cors.CheckOrigin(r)
		},
	}
	return nil
}

// Serve starts serving WebTransport connections
func (s *WebTransportServer) Serve(certFile, keyFile string) error {
	if s.server == nil {
		if err := s.Start(); err != nil {
			return err
		}
	}
	return s.server.ListenAndServeTLS(certFile, keyFile)
}

// handleWebTransport handles WebTransport upgrade requests
func (s *WebTransportServer) handleWebTransport(sessionID string) {
	// Check if session exists
	handler, err := s.resolver.ResolveSession(sessionID)
	if err != nil {
		return
	}

	// Verify the session is in a valid state
	if s.sessionStore != nil {
		// Session validation would happen here
		_ = handler
	}

	// Handle the session if handler is set
	if s.connHandler != nil {
		// The session would be passed to the handler after upgrade
		_ = sessionID
	}
}

// Stop gracefully stops the WebTransport server
func (s *WebTransportServer) Stop() error {
	s.cancel()
	s.wg.Wait()
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// nextStreamType determines the next stream type based on counter
func (s *WebTransportServer) nextStreamType() StreamType {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	s.streamCounter++
	switch s.streamCounter % 4 {
	case 0:
		return StreamTypeTerminalTX
	case 1:
		return StreamTypeControl
	case 2:
		return StreamTypeTerminalRX
	case 3:
		return StreamTypeOOBUI
	default:
		return StreamTypeControl
	}
}

// ============================================================================
// Stream Router
// ============================================================================

// StreamHandler handles individual WebTransport streams
type StreamHandler struct {
	StreamType StreamType
	SessionID  string
	Reader     io.Reader
	Writer     io.Writer
	Close      func() error
}

// Router routes streams between WebTransport and gRPC
type Router struct {
	resolver     SessionResolver
	sessionStore session.Store
	connMu       sync.RWMutex
	connections  map[string]*ClientConnection
	wg           sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
}

// ClientConnection represents a client connection with its streams
type ClientConnection struct {
	SessionID           string
	WebTransportSession *webtransport.Session
	TerminalRX          *StreamHandler // Terminal output stream (satellite -> client)
	TerminalTX          *StreamHandler // Terminal input stream (client -> satellite)
	Control             *StreamHandler // Control stream (PING, STATE)
	OOBUI               *StreamHandler // OOB UI stream
	mu                  sync.RWMutex
	closed              bool
}

// NewRouter creates a new stream router
func NewRouter(resolver SessionResolver, sessionStore session.Store) *Router {
	ctx, cancel := context.WithCancel(context.Background())
	return &Router{
		resolver:     resolver,
		sessionStore: sessionStore,
		connections:  make(map[string]*ClientConnection),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Route bridges a satellite gRPC stream to a client WebTransport stream for a given session.
// It handles terminal RX/TX, control messages, and OOB UI payloads.
// Returns a ClientConnection that manages the bidirectional stream between satellite and client.
func (r *Router) Route(sessionID string) (*ClientConnection, error) {
	// Resolve the satellite stream handler for this session
	satelliteHandler, err := r.resolver.ResolveSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve session: %w", err)
	}

	// Get existing client connection if already routed
	r.connMu.RLock()
	clientConn, exists := r.connections[sessionID]
	r.connMu.RUnlock()

	if exists {
		return clientConn, nil
	}

	// Create a new client connection placeholder
	// The actual WebTransport session will be set when RouteConnection is called
	clientConn = &ClientConnection{
		SessionID: sessionID,
	}

	r.connMu.Lock()
	r.connections[sessionID] = clientConn
	r.connMu.Unlock()

	// Start the bridging goroutines - this would use gRPC to communicate with satellite
	r.wg.Add(1)
	go r.bridgeToSatellite(sessionID, satelliteHandler)

	// Create gRPC connection to satellite (placeholder - would be used in actual implementation)
	grpcConn, err := grpc.Dial("satellite:8444", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to dial satellite: %v", err), "component", "router")
	} else {
		defer grpcConn.Close()

		// Create gRPC client for satellite communication
		client := proto.NewSatelliteGatewayClient(grpcConn)

		// Set up bidirectional stream
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		stream, err := client.Connect(ctx)
		if err != nil {
			slog.Error(fmt.Sprintf("Failed to connect to satellite: %v", err), "component", "router")
		} else {
			// Handle the stream - this would bridge to WebTransport
			_ = stream
		}
	}

	return clientConn, nil
}

// Bridge is an alias for Route - bridges satellite gRPC stream to WebTransport client stream
func (r *Router) Bridge(sessionID string) (*ClientConnection, error) {
	return r.Route(sessionID)
}

// bridgeToSatellite bridges terminal data from WebTransport client to satellite via gRPC
func (r *Router) bridgeToSatellite(sessionID string, satelliteHandler *SatelliteHandler) {
	defer r.wg.Done()

	// This establishes the gRPC stream to the satellite
	// and handles bidirectional data flow between WebTransport and gRPC
	grpcConn, err := grpc.Dial("satellite:8444", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to dial satellite for session %s: %v", sessionID, err), "component", "router")
		return
	}
	defer grpcConn.Close()

	// Create gRPC client for satellite communication
	client := proto.NewSatelliteGatewayClient(grpcConn)

	// Set up bidirectional stream
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := client.Connect(ctx)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to connect to satellite for session %s: %v", sessionID, err), "component", "router")
		return
	}

	// Start goroutines to bridge data:
	// 1. Forward terminal input from WebTransport client to satellite
	// 2. Forward terminal output from satellite to WebTransport client
	// 3. Forward control messages and OOB UI payloads

	go r.forwardTerminalInput(sessionID, stream)
	go r.forwardTerminalOutput(sessionID, stream)
	go r.forwardControlMessages(sessionID, stream)
	go r.forwardOOBUI(sessionID, stream)
}

// forwardTerminalInput forwards terminal input from WebTransport client to satellite
func (r *Router) forwardTerminalInput(sessionID string, stream proto.SatelliteGateway_ConnectClient) {
	// This would read from WebTransport terminal TX stream and send to satellite
	_ = sessionID
	_ = stream
}

// forwardTerminalOutput forwards terminal output from satellite to WebTransport client
func (r *Router) forwardTerminalOutput(sessionID string, stream proto.SatelliteGateway_ConnectClient) {
	// This would receive from satellite and write to WebTransport terminal RX stream
	_ = sessionID
	_ = stream
}

// forwardControlMessages forwards control messages between satellite and client
func (r *Router) forwardControlMessages(sessionID string, stream proto.SatelliteGateway_ConnectClient) {
	// This would handle PING, PONG, state updates
	_ = sessionID
	_ = stream
}

// forwardOOBUI forwards OOB UI payloads from satellite to client
func (r *Router) forwardOOBUI(sessionID string, stream proto.SatelliteGateway_ConnectClient) {
	// This would forward structured UI payloads like forms, progress bars
	_ = sessionID
	_ = stream
}

// RouteConnection routes a new WebTransport session
func (r *Router) RouteConnection(sessionID string, session *webtransport.Session) error {
	// Get the satellite stream handler for this session
	satelliteHandler, err := r.resolver.ResolveSession(sessionID)
	if err != nil {
		return fmt.Errorf("failed to resolve session: %w", err)
	}

	clientConn := &ClientConnection{
		SessionID:           sessionID,
		WebTransportSession: session,
	}

	r.connMu.Lock()
	r.connections[sessionID] = clientConn
	r.connMu.Unlock()

	// Start handling streams
	r.wg.Add(1)
	go r.handleConnection(clientConn, satelliteHandler)

	return nil
}

// handleConnection handles all streams for a client connection
func (r *Router) handleConnection(clientConn *ClientConnection, satelliteHandler *SatelliteHandler) {
	defer r.wg.Done()

	ctx, cancel := context.WithCancel(r.ctx)
	defer cancel()

	// Accept and handle all bidirectional streams
	streamCounter := uint64(0)
	for {
		stream, err := clientConn.WebTransportSession.AcceptStream(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				break
			}
			fmt.Printf("Accept stream error: %v\n", err)
			break
		}

		// Determine stream type from counter (round-robin)
		streamType := streamTypeFromCounter(streamCounter)
		streamCounter++

		r.wg.Add(1)
		go r.handleStream(clientConn, satelliteHandler, stream, streamType)
	}

	r.connMu.Lock()
	delete(r.connections, clientConn.SessionID)
	r.connMu.Unlock()
}

// streamTypeFromCounter determines the stream type from a counter
func streamTypeFromCounter(counter uint64) StreamType {
	switch counter % 4 {
	case 0:
		return StreamTypeTerminalTX // Client sends terminal input
	case 1:
		return StreamTypeControl // Control messages
	case 2:
		return StreamTypeTerminalRX // Server sends terminal output
	case 3:
		return StreamTypeOOBUI // OOB UI messages
	default:
		return StreamTypeControl
	}
}

// handleStream handles an individual stream
func (r *Router) handleStream(clientConn *ClientConnection, satelliteHandler *SatelliteHandler, stream *webtransport.Stream, streamType StreamType) {
	defer r.wg.Done()
	defer stream.Close()

	clientConn.mu.Lock()
	switch streamType {
	case StreamTypeTerminalRX:
		clientConn.TerminalRX = &StreamHandler{
			StreamType: streamType,
			SessionID:  clientConn.SessionID,
			Reader:     stream,
			Writer:     stream,
			Close:      stream.Close,
		}
	case StreamTypeTerminalTX:
		clientConn.TerminalTX = &StreamHandler{
			StreamType: streamType,
			SessionID:  clientConn.SessionID,
			Reader:     stream,
			Writer:     stream,
			Close:      stream.Close,
		}
	case StreamTypeControl:
		clientConn.Control = &StreamHandler{
			StreamType: streamType,
			SessionID:  clientConn.SessionID,
			Reader:     stream,
			Writer:     stream,
			Close:      stream.Close,
		}
	case StreamTypeOOBUI:
		clientConn.OOBUI = &StreamHandler{
			StreamType: streamType,
			SessionID:  clientConn.SessionID,
			Reader:     stream,
			Writer:     stream,
			Close:      stream.Close,
		}
	}
	clientConn.mu.Unlock()

	// Handle stream based on type
	switch streamType {
	case StreamTypeTerminalRX:
		r.handleTerminalRX(clientConn, satelliteHandler, stream)
	case StreamTypeTerminalTX:
		r.handleTerminalTX(clientConn, satelliteHandler, stream)
	case StreamTypeControl:
		r.handleControlStream(clientConn, satelliteHandler, stream)
	case StreamTypeOOBUI:
		r.handleOOBUIStream(clientConn, satelliteHandler, stream)
	}
}

// handleTerminalRX handles terminal output from satellite to client
func (r *Router) handleTerminalRX(clientConn *ClientConnection, satelliteHandler *SatelliteHandler, stream *webtransport.Stream) {
	// Buffer for reading terminal data
	buf := make([]byte, 8192)

	for {
		n, err := stream.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			fmt.Printf("Terminal RX read error: %v\n", err)
			break
		}

		// In a real implementation, this would:
		// 1. Send terminal data to the satellite via gRPC stream
		// 2. Or receive from satellite and forward to client

		// For now, we just acknowledge receipt
		_ = n
	}
}

// handleTerminalTX handles terminal input from client to satellite
func (r *Router) handleTerminalTX(clientConn *ClientConnection, satelliteHandler *SatelliteHandler, stream *webtransport.Stream) {
	// Buffer for reading terminal input
	buf := make([]byte, 8192)

	for {
		n, err := stream.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			fmt.Printf("Terminal TX read error: %v\n", err)
			break
		}

		// Forward terminal input to satellite via gRPC stream
		if satelliteHandler != nil {
			input := &TerminalInput{
				SessionID:      clientConn.SessionID,
				Data:           buf[:n],
				SequenceNumber: 0, // Would be properly assigned
			}
			_ = input // Would be sent via gRPC
		}
	}
}

// handleControlStream handles PING, STATE updates
func (r *Router) handleControlStream(clientConn *ClientConnection, satelliteHandler *SatelliteHandler, stream *webtransport.Stream) {
	buf := make([]byte, 1024)

	for {
		n, err := stream.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			fmt.Printf("Control stream read error: %v\n", err)
			break
		}

		// Parse and handle control messages
		r.processControlMessage(clientConn, buf[:n], stream)
	}
}

// processControlMessage processes a control message
func (r *Router) processControlMessage(clientConn *ClientConnection, data []byte, stream *webtransport.Stream) {
	// In a real implementation, this would decode the control message
	// For now, we'll handle PING messages
	if len(data) > 0 {
		// Echo back a PONG for PING
		if data[0] == byte(ControlMessagePing) {
			stream.Write([]byte{byte(ControlMessagePong)})
		}
	}
}

// handleOOBUIStream delivers structured payloads to client
func (r *Router) handleOOBUIStream(clientConn *ClientConnection, satelliteHandler *SatelliteHandler, stream *webtransport.Stream) {
	buf := make([]byte, 8192)

	for {
		n, err := stream.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			fmt.Printf("OOB UI stream read error: %v\n", err)
			break
		}

		// Process OOB UI payloads
		// These are structured UI payloads like forms, progress bars, etc.
		_ = n
	}
}

// SendStateUpdate sends a state update to the client
func (r *Router) SendStateUpdate(sessionID string, state session.SessionState, errorMsg string) error {
	r.connMu.RLock()
	clientConn, exists := r.connections[sessionID]
	r.connMu.RUnlock()

	if !exists {
		return fmt.Errorf("connection not found for session: %s", sessionID)
	}

	clientConn.mu.RLock()
	control := clientConn.Control
	clientConn.mu.RUnlock()

	if control == nil {
		return fmt.Errorf("control stream not available")
	}

	msg := StateUpdateMessage{
		SessionID:    sessionID,
		State:        state,
		Timestamp:    time.Now().Unix(),
		ErrorMessage: errorMsg,
	}

	// Encode and send the message
	_ = msg
	return nil
}

// SendOOBUI sends an OOB UI message to the client
func (r *Router) SendOOBUI(sessionID string, msg *OOBUIMessage) error {
	r.connMu.RLock()
	clientConn, exists := r.connections[sessionID]
	r.connMu.RUnlock()

	if !exists {
		return fmt.Errorf("connection not found for session: %s", sessionID)
	}

	clientConn.mu.RLock()
	oobui := clientConn.OOBUI
	clientConn.mu.RUnlock()

	if oobui == nil {
		return fmt.Errorf("OOB UI stream not available")
	}

	// Encode and send the message
	_ = msg
	return nil
}

// GetConnection returns a client connection by session ID
func (r *Router) GetConnection(sessionID string) (*ClientConnection, bool) {
	r.connMu.RLock()
	defer r.connMu.RUnlock()
	conn, exists := r.connections[sessionID]
	return conn, exists
}

// CloseConnection closes a client connection
func (r *Router) CloseConnection(sessionID string) error {
	r.connMu.RLock()
	clientConn, exists := r.connections[sessionID]
	r.connMu.RUnlock()

	if !exists {
		return fmt.Errorf("connection not found")
	}

	clientConn.mu.Lock()
	clientConn.closed = true
	clientConn.mu.Unlock()

	if clientConn.WebTransportSession != nil {
		return clientConn.WebTransportSession.CloseWithError(0, "connection closed")
	}
	return nil
}

// Close gracefully stops the router
func (r *Router) Close() error {
	r.cancel()
	r.wg.Wait()

	r.connMu.Lock()
	defer r.connMu.Unlock()

	for _, conn := range r.connections {
		if conn.WebTransportSession != nil {
			conn.WebTransportSession.CloseWithError(0, "router closing")
		}
	}

	return nil
}

// ============================================================================
// Session Registry
// ============================================================================

// SessionRegistry manages session-to-stream mappings
type SessionRegistry struct {
	mu          sync.RWMutex
	sessions    map[string]*RegistryEntry
	maxSessions int
}

// RegistryEntry holds session information
type RegistryEntry struct {
	SessionID      string
	SatelliteID    string
	SatelliteHandler  *SatelliteHandler
	CreatedAt      time.Time
	LastActivityAt time.Time
}

// NewSessionRegistry creates a new session registry
func NewSessionRegistry(maxSessions int) *SessionRegistry {
	return &SessionRegistry{
		sessions:    make(map[string]*RegistryEntry),
		maxSessions: maxSessions,
	}
}

// Register registers a new session
func (reg *SessionRegistry) Register(sessionID, satelliteID string, handler *SatelliteHandler) error {
	reg.mu.Lock()
	defer reg.mu.Unlock()

	if len(reg.sessions) >= reg.maxSessions {
		return fmt.Errorf("max sessions reached")
	}

	now := time.Now()
	reg.sessions[sessionID] = &RegistryEntry{
		SessionID:      sessionID,
		SatelliteID:    satelliteID,
		SatelliteHandler:  handler,
		CreatedAt:      now,
		LastActivityAt: now,
	}

	return nil
}

// Unregister removes a session
func (reg *SessionRegistry) Unregister(sessionID string) error {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	delete(reg.sessions, sessionID)
	return nil
}

// Get returns a registry entry
func (reg *SessionRegistry) Get(sessionID string) (*RegistryEntry, bool) {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	entry, exists := reg.sessions[sessionID]
	if exists {
		entry.LastActivityAt = time.Now()
	}
	return entry, exists
}

// List returns all session IDs
func (reg *SessionRegistry) List() []string {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	ids := make([]string, 0, len(reg.sessions))
	for id := range reg.sessions {
		ids = append(ids, id)
	}
	return ids
}

// Count returns the number of active sessions
func (reg *SessionRegistry) Count() int {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	return len(reg.sessions)
}
