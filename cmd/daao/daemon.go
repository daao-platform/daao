// Package main provides the satellite daemon that runs on remote machines.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/daao/nexus/internal/satellite"
	"github.com/daao/nexus/internal/session"
	"github.com/daao/nexus/pkg/buffer"
	"github.com/daao/nexus/pkg/ipc"
	"github.com/daao/nexus/pkg/lifecycle"
	"github.com/daao/nexus/pkg/pty"
	"github.com/daao/nexus/pkg/sysmetrics"
	"github.com/daao/nexus/proto"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// DefaultNexusAddr is the default address of the Nexus server
const DefaultNexusAddr = "localhost:8444"

// ReconnectBaseDelay is the base delay for exponential backoff
const ReconnectBaseDelay = 1 * time.Second

// ReconnectMaxDelay is the maximum delay for exponential backoff
const ReconnectMaxDelay = 30 * time.Second

// HeartbeatInterval is the interval for sending heartbeats
const HeartbeatInterval = 30 * time.Second

// DefaultDMSTTL is the default DMS TTL in minutes
const DefaultDMSTTL = 60

// DAAO_DMS_TTL is the environment variable for DMS TTL
const DAAO_DMS_TTL = "DAAO_DMS_TTL"

// DAAO_MAX_SESSIONS is the environment variable for per-satellite session cap.
// 0 (default) = unlimited.
const DAAO_MAX_SESSIONS = "DAAO_MAX_SESSIONS"

// getDMSTTL returns the DMS TTL from environment variable or default
func getDMSTTL() int {
	if ttlStr := os.Getenv(DAAO_DMS_TTL); ttlStr != "" {
		var ttl int
		if _, err := fmt.Sscanf(ttlStr, "%d", &ttl); err == nil && ttl > 0 {
			return ttl
		}
	}
	return DefaultDMSTTL
}

// getMaxSessions returns the per-satellite session limit from DAAO_MAX_SESSIONS.
// Returns 0 for unlimited (default).
func getMaxSessions() int {
	if maxStr := os.Getenv(DAAO_MAX_SESSIONS); maxStr != "" {
		var max int
		if _, err := fmt.Sscanf(maxStr, "%d", &max); err == nil && max > 0 {
			return max
		}
	}
	return 0
}

// DaemonConfig holds configuration for the daemon
type DaemonConfig struct {
	// NexusAddr is the address of the Nexus server
	NexusAddr string

	// SatelliteID is the unique identifier for this satellite
	SatelliteID string

	// Fingerprint is the hardware fingerprint of this satellite
	Fingerprint string

	// PrivateKey is the private key for authentication
	PrivateKey string
}

// Session represents an active terminal session
type Session struct {
	ID          string
	AgentBinary string
	Pty         pty.Pty
	IPCServer   *ipc.Server
	RingBuffer  *buffer.RingBuffer
	DMS         *lifecycle.DeadManSwitch
	Process     *os.Process
	State       session.SessionState
	CloseOnce   sync.Once
	Wg          sync.WaitGroup

	// localClients holds writers for locally attached terminals.
	// forwardPtyOutput fans out PTY data to all registered writers.
	localClients   []io.Writer
	localClientsMu sync.RWMutex
}

// addLocalClient registers a writer to receive live PTY output.
func (s *Session) addLocalClient(w io.Writer) {
	s.localClientsMu.Lock()
	defer s.localClientsMu.Unlock()
	s.localClients = append(s.localClients, w)
}

// removeLocalClient unregisters a writer.
func (s *Session) removeLocalClient(w io.Writer) {
	s.localClientsMu.Lock()
	defer s.localClientsMu.Unlock()
	for i, c := range s.localClients {
		if c == w {
			s.localClients = append(s.localClients[:i], s.localClients[i+1:]...)
			return
		}
	}
}

// Daemon is the main satellite daemon that manages PTY sessions
type Daemon struct {
	config        DaemonConfig
	sessions      map[string]*Session
	sessionsMu    sync.RWMutex
	bridges       map[string]*satellite.PiBridge
	bridgesMu     sync.RWMutex
	grpcConn      *grpc.ClientConn
	grpcClient    proto.SatelliteGatewayClient
	controlServer *ControlServer
	startOnce     sync.Once
	stopOnce      sync.Once
	running       atomic.Bool
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup

	// sendCh carries bulk data messages (PTY output, buffer replays).
	// sendPriorityCh carries control messages (heartbeats, state updates,
	// telemetry) that must not be starved by high-throughput PTY output.
	// agentEventCh carries Pi RPC agent events. Isolated from PTY output
	// so a burst of message_update events cannot delay terminal keystrokes.
	// Drops gracefully when full — DB is the source of truth.
	// Both are drained by a single streamWriter goroutine per connection,
	// eliminating concurrent stream.Send() calls.
	sendCh         chan *proto.SatelliteMessage
	sendPriorityCh chan *proto.SatelliteMessage
	agentEventCh   chan *proto.SatelliteMessage
}

// NewDaemon creates a new Daemon instance
func NewDaemon(config DaemonConfig) *Daemon {
	if config.NexusAddr == "" {
		config.NexusAddr = DefaultNexusAddr
	}

	ctx, cancel := context.WithCancel(context.Background())

	d := &Daemon{
		config:         config,
		sessions:       make(map[string]*Session),
		bridges:        make(map[string]*satellite.PiBridge),
		ctx:            ctx,
		cancel:         cancel,
		sendCh:         make(chan *proto.SatelliteMessage, 256),
		sendPriorityCh: make(chan *proto.SatelliteMessage, 64),
		agentEventCh:   make(chan *proto.SatelliteMessage, 512),
	}

	return d
}

// Start starts the daemon and connects to Nexus
func (d *Daemon) Start() error {
	d.startOnce.Do(func() {
		d.running.Store(true)

		// Start control server for local CLI commands (daao sessions/attach)
		d.controlServer = NewControlServer(d)
		if err := d.controlServer.Start(); err != nil {
			log.Printf("Warning: failed to start control server: %v", err)
		}

		// Seed standard context files and start watching for local edits.
		contextDir := satellite.ContextDir()
		if err := satellite.SeedStandardFiles(contextDir); err != nil {
			log.Printf("Warning: failed to seed context files: %v", err)
		}
		d.wg.Add(1)
		go d.runContextWatcher(contextDir)

		// Start gRPC connection in background
		d.wg.Add(1)
		go d.runGrpcLoop()

		// Start signal handler
		d.wg.Add(1)
		go d.signalHandler()
	})

	return nil
}

// Stop stops the daemon and all sessions
func (d *Daemon) Stop() error {
	d.stopOnce.Do(func() {
		d.running.Store(false)
		d.cancel()

		// Stop control server
		if d.controlServer != nil {
			d.controlServer.Stop()
		}

		// Snapshot the session list under RLock, then close each session
		// without holding any lock. Holding RLock while calling sess.Close()
		// (which calls Wg.Wait()) deadlocks because goroutines inside
		// forwardPtyOutput call TransitionSessionState, which needs a write lock.
		d.sessionsMu.RLock()
		sessions := make([]*Session, 0, len(d.sessions))
		for _, sess := range d.sessions {
			sessions = append(sessions, sess)
		}
		d.sessionsMu.RUnlock()

		for _, sess := range sessions {
			sess.Close()
		}

		// Stop all active Pi bridges
		d.bridgesMu.RLock()
		bridges := make([]*satellite.PiBridge, 0, len(d.bridges))
		for _, b := range d.bridges {
			bridges = append(bridges, b)
		}
		d.bridgesMu.RUnlock()
		for _, b := range bridges {
			b.Stop()
		}

		// Close gRPC connection
		if d.grpcConn != nil {
			d.grpcConn.Close()
		}

		d.wg.Wait()
	})

	return nil
}

// sendToNexus enqueues a message for delivery to Nexus. Thread-safe: multiple
// goroutines may call this concurrently. The message is sent asynchronously by
// the streamWriter goroutine. Returns false if the send channel is full
// (backpressure) or the daemon context is cancelled.
func (d *Daemon) sendToNexus(msg *proto.SatelliteMessage) bool {
	select {
	case d.sendCh <- msg:
		return true
	case <-d.ctx.Done():
		return false
	default:
		log.Printf("sendToNexus: data channel full, dropping message")
		return false
	}
}

// sendToNexusPriority enqueues a high-priority control message (heartbeat,
// state update, telemetry) for delivery to Nexus. These are drained before
// bulk data messages by the streamWriter.
func (d *Daemon) sendToNexusPriority(msg *proto.SatelliteMessage) bool {
	select {
	case d.sendPriorityCh <- msg:
		return true
	case <-d.ctx.Done():
		return false
	default:
		log.Printf("sendToNexusPriority: priority channel full, dropping message")
		return false
	}
}

// sendToNexusAgent enqueues a Pi RPC agent event message for delivery to
// Nexus. These are round-robin'd with bulk data (PTY output) by the streamWriter.
// Drops gracefully when full — DB is the source of truth.
func (d *Daemon) sendToNexusAgent(msg *proto.SatelliteMessage) bool {
	select {
	case d.agentEventCh <- msg:
		return true
	case <-d.ctx.Done():
		return false
	default:
		log.Printf("sendToNexusAgent: agent event channel full, dropping event")
		return false
	}
}

// streamWriter is the sole goroutine that calls stream.Send(). It drains
// priority messages first, then round-robins between agent events and bulk
// data messages. Runs until ctx is cancelled.
func (d *Daemon) streamWriter(ctx context.Context, stream proto.SatelliteGateway_ConnectClient) {
	for {
		// Priority-first: drain all priority messages before data.
		select {
		case msg := <-d.sendPriorityCh:
			if err := stream.Send(msg); err != nil {
				log.Printf("streamWriter: priority send failed: %v", err)
				return
			}
			continue
		default:
		}

		// No priority pending — round-robin agent events and bulk data
		select {
		case <-ctx.Done():
			return
		case msg := <-d.sendPriorityCh:
			if err := stream.Send(msg); err != nil {
				log.Printf("streamWriter: priority send failed: %v", err)
				return
			}
		case msg := <-d.agentEventCh:
			if err := stream.Send(msg); err != nil {
				log.Printf("streamWriter: agent send failed: %v", err)
				return
			}
		case msg := <-d.sendCh:
			if err := stream.Send(msg); err != nil {
				log.Printf("streamWriter: data send failed: %v", err)
				return
			}
		}
	}
}

// CreateSession creates a new PTY session
func (d *Daemon) CreateSession(sessionID string, agentBinary string, agentArgs []string, cols, rows int32, workingDir string) (*Session, error) {
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	// Enforce per-satellite session limit (DAAO_MAX_SESSIONS)
	maxSessions := getMaxSessions()
	if maxSessions > 0 {
		d.sessionsMu.RLock()
		current := len(d.sessions)
		d.sessionsMu.RUnlock()
		if current >= maxSessions {
			return nil, fmt.Errorf("satellite session limit reached (max: %d, active: %d)", maxSessions, current)
		}
	}

	// Create PTY using pkg/pty
	ptyMaster, process, err := d.spawnProcessWithPTY(agentBinary, agentArgs, uint16(cols), uint16(rows), workingDir)
	if err != nil {
		return nil, fmt.Errorf("failed to spawn process with PTY: %w", err)
	}

	// Create IPC server for this session
	ipcServer, err := ipc.NewServer(ipc.ServerConfig{
		SessionID: sessionID,
		OnConnect: func(conn *ipc.Conn) {
			log.Printf("IPC client connected to session %s", sessionID)
		},
		OnDisconnect: func(conn *ipc.Conn, err error) {
			log.Printf("IPC client disconnected from session %s: %v", sessionID, err)
		},
	})
	if err != nil {
		ptyMaster.Close()
		process.Kill()
		return nil, fmt.Errorf("failed to create IPC server: %w", err)
	}

	// Start IPC server
	if err := ipcServer.Start(); err != nil {
		ptyMaster.Close()
		process.Kill()
		ipcServer.Close()
		return nil, fmt.Errorf("failed to start IPC server: %w", err)
	}

	// Create ring buffer for terminal output
	ringBuffer := buffer.NewRingBuffer(0)

	// Create session
	sess := &Session{
		ID:          sessionID,
		AgentBinary: agentBinary,
		Pty:         ptyMaster,
		IPCServer:   ipcServer,
		RingBuffer:  ringBuffer,
		Process:     process,
		State:       session.SessionState(session.StateRunning),
	}

	// Store session
	d.sessionsMu.Lock()
	d.sessions[sessionID] = sess
	d.sessionsMu.Unlock()

	// Set up DMS for this session (using pkg/lifecycle)
	// DMS is started when session transitions to DETACHED state
	// Create a suspendable process wrapper based on platform
	suspendableProc := newSuspendableProcess(process.Pid)
	dmsConfig := lifecycle.DMSConfig{
		TTL: getDMSTTL(), // Use configurable TTL from DAAO_DMS_TTL env var
		OnSuspend: func(sid string) error {
			log.Printf("DMS suspending session %s", sid)
			// Transition to SUSPENDED state when DMS fires
			d.sessionsMu.RLock()
			s, ok := d.sessions[sid]
			d.sessionsMu.RUnlock()
			if ok {
				s.State = session.SessionState(session.StateSuspended)
				log.Printf("Session %s transitioned to SUSPENDED state", sid)
			}
			return nil
		},
		OnResume: func(sid string) error {
			log.Printf("DMS resuming session %s", sid)
			// Transition to RUNNING state when DMS resumes
			d.sessionsMu.RLock()
			s, ok := d.sessions[sid]
			d.sessionsMu.RUnlock()
			if ok {
				s.State = session.SessionState(session.StateRunning)
				log.Printf("Session %s transitioned to RUNNING state", sid)
			}
			return nil
		},
		GetSessionStateFunc: func(sid string) (session.SessionState, error) {
			d.sessionsMu.RLock()
			s, ok := d.sessions[sid]
			d.sessionsMu.RUnlock()
			if !ok {
				return "", fmt.Errorf("session not found")
			}
			return s.State, nil
		},
	}
	sess.DMS = lifecycle.NewDeadManSwitch(sessionID, suspendableProc, dmsConfig)

	// Start forwarding PTY output to ring buffer and gRPC
	sess.Wg.Add(1)
	go d.forwardPtyOutput(sess)

	// Update Nexus with initial state (priority: control message)
	d.sendToNexusPriority(&proto.SatelliteMessage{
		Payload: &proto.SatelliteMessage_SessionStateUpdate{
			SessionStateUpdate: &proto.SessionStateUpdate{
				SessionId: sessionID,
				State:     proto.SessionState_SESSION_STATE_RUNNING,
				Timestamp: time.Now().Unix(),
			},
		},
	})

	return sess, nil
}

// AttachSession attaches to an existing session
func (d *Daemon) AttachSession(sessionID string) (*Session, error) {
	d.sessionsMu.RLock()
	sess, ok := d.sessions[sessionID]
	d.sessionsMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	return sess, nil
}

// TransitionSessionState transitions a session to a new state and handles DMS
func (d *Daemon) TransitionSessionState(sessionID string, newState session.SessionState) error {
	d.sessionsMu.Lock()
	defer d.sessionsMu.Unlock()

	sess, ok := d.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}

	oldState := sess.State
	sess.State = newState

	log.Printf("Session %s transitioned from %s to %s", sessionID, oldState, newState)

	// Handle DMS based on state transitions
	switch newState {
	case session.SessionState(session.StateDetached):
		// DETACHED: Start DMS monitoring
		if sess.DMS != nil {
			sess.DMS.Start()
			log.Printf("DMS started for session %s (DETACHED)", sessionID)
		}
	case session.SessionState(session.StateReAttaching):
		// RE_ATTACHING: Stop DMS monitoring
		if sess.DMS != nil {
			sess.DMS.Stop()
			log.Printf("DMS stopped for session %s (RE_ATTACHING)", sessionID)
		}
	case session.SessionState(session.StateRunning):
		// RUNNING: If coming from SUSPENDED, resume process via DMS
		if sess.DMS != nil && sess.DMS.IsSuspended() {
			sess.DMS.RecordActivity()
			log.Printf("Session %s resumed via DMS (RUNNING)", sessionID)
		}
	}

	return nil
}

// resolveBinary attempts to find the full path for a binary name.
// It searches the system PATH, DAAO runtime directories, and common
// OS-specific locations for user-installed tools. Works on Windows,
// Linux, and macOS (including minimal Docker containers).
func resolveBinary(name string) string {
	// If it's already an absolute path or has path separators, use as-is
	if strings.ContainsAny(name, `/\`) {
		return name
	}

	// Build search path from system PATH using the platform separator
	searchDirs := []string{}
	pathSep := string(os.PathListSeparator) // ":" on Unix, ";" on Windows
	for _, p := range strings.Split(os.Getenv("PATH"), pathSep) {
		if p != "" {
			searchDirs = append(searchDirs, p)
		}
	}

	// Platform-specific extensions and extra search directories
	var extensions []string

	switch runtime.GOOS {
	case "windows":
		// Windows: prefer native extensions. npm installs both a bash script
		// (extensionless) and a .cmd wrapper. We need the .cmd for CreateProcess.
		extensions = []string{".exe", ".cmd", ".bat", ""}
		profile := os.Getenv("USERPROFILE")
		if profile != "" {
			searchDirs = append(searchDirs,
				filepath.Join(profile, "AppData", "Local", "Microsoft", "WinGet", "Links"),
				filepath.Join(profile, "AppData", "Local", "Programs"),
				filepath.Join(profile, "go", "bin"),
				filepath.Join(profile, ".cargo", "bin"),
				filepath.Join(profile, ".local", "bin"),
				filepath.Join(profile, "AppData", "Roaming", "npm"),
			)
		}
		programFiles := os.Getenv("ProgramFiles")
		if programFiles == "" {
			programFiles = `C:\Program Files`
		}
		searchDirs = append(searchDirs,
			`C:\Windows\System32`,
			`C:\Windows`,
			filepath.Join(programFiles, "nodejs"),
			filepath.Join(programFiles, "daao", "runtime", "node"),
			filepath.Join(programFiles, "daao", "runtime", "node", "node_modules", ".bin"),
			`C:\tools\bin`,
		)
	default:
		// Linux / macOS: extensionless binaries
		extensions = []string{""}
		home := os.Getenv("HOME")
		if home != "" {
			searchDirs = append(searchDirs,
				filepath.Join(home, ".local", "bin"),
				filepath.Join(home, "go", "bin"),
				filepath.Join(home, ".cargo", "bin"),
				filepath.Join(home, ".npm-global", "bin"),
			)
		}
		// DAAO private runtime (bootstrapped Node.js + Pi)
		searchDirs = append(searchDirs,
			"/opt/daao/runtime/node/bin",
			"/opt/daao/runtime/node/lib/node_modules/.bin",
			"/usr/local/bin",
			"/usr/bin",
			"/bin",
			"/usr/sbin",
			"/sbin",
		)
	}

	log.Printf("resolveBinary(%q): GOOS=%s, PATH has %d entries, searching %d dirs total",
		name, runtime.GOOS, len(strings.Split(os.Getenv("PATH"), pathSep)), len(searchDirs))

	for _, dir := range searchDirs {
		for _, ext := range extensions {
			candidate := filepath.Join(dir, name+ext)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				log.Printf("resolveBinary(%q): FOUND at %q", name, candidate)
				return candidate
			}
		}
	}
	log.Printf("resolveBinary(%q): NOT FOUND in %d directories", name, len(searchDirs))
	return name // Return original if not found; let exec handle the error
}

// wellKnownAgents is the list of agent binaries to probe for availability.
// Includes AI coding agents, DAAO runtime binaries, and system shells.
var wellKnownAgents = []string{
	"claude", "gemini", "gpt", "codex", "aider",
	"pi", "node",
	"powershell", "pwsh", "bash", "zsh", "fish", "cmd",
}

// probeAvailableAgents checks which well-known agent binaries can be resolved
// on this machine. Returns the names of agents that were found.
func probeAvailableAgents() []string {
	var found []string
	for _, name := range wellKnownAgents {
		resolved := resolveBinary(name)
		if resolved != name {
			// resolveBinary returns the original name if not found
			found = append(found, name)
		}
	}
	log.Printf("probeAvailableAgents: found %d agents: %v", len(found), found)
	return found
}

// spawnProcessWithPTY spawns a process with PTY using pkg/pty and pkg/proc detach flags
func (d *Daemon) spawnProcessWithPTY(agentBinary string, agentArgs []string, cols, rows uint16, workingDir string) (pty.Pty, *os.Process, error) {
	// If agentBinary is empty, use default shell
	if agentBinary == "" {
		agentBinary = os.Getenv("SHELL")
		if agentBinary == "" {
			if runtime.GOOS == "windows" {
				agentBinary = "cmd.exe"
			} else {
				agentBinary = "/bin/bash"
			}
		}
	}

	// Resolve to full path so the daemon finds binaries even with a restricted PATH
	agentBinary = resolveBinary(agentBinary)
	log.Printf("spawnProcessWithPTY: resolved binary to %q", agentBinary)

	// Create PTY
	ptyMaster, err := pty.NewPty(cols, rows)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create PTY: %w", err)
	}

	// For ConPTY, do NOT pass CREATE_NEW_PROCESS_GROUP or other console-related
	// flags. ConPTY + EXTENDED_STARTUPINFO_PRESENT is sufficient — the pseudo
	// console IS the console. Extra flags (especially CREATE_NEW_PROCESS_GROUP)
	// cause the child to allocate its own visible console, bypassing ConPTY's
	// output pipe so forwardPtyOutput never reads any data.
	var detachFlags uint32 = 0

	// Determine working directory: use provided value, fall back to user home
	cwd := workingDir
	if cwd == "" {
		cwd = os.Getenv("USERPROFILE")
		if cwd == "" {
			cwd, _ = os.UserHomeDir()
		}
	}

	// Prepare process arguments
	args := append([]string{agentBinary}, agentArgs...)

	// Start the process using the PTY's Start method (wires up ConPTY)
	process, err := ptyMaster.Start(
		agentBinary,
		args,
		cwd,
		os.Environ(),
		detachFlags,
	)

	if err != nil {
		ptyMaster.Close()
		return nil, nil, fmt.Errorf("failed to start process: %w", err)
	}

	return ptyMaster, process, nil
}

// runGrpcLoop maintains the gRPC connection to Nexus with exponential backoff
func (d *Daemon) runGrpcLoop() {
	defer d.wg.Done()

	delay := ReconnectBaseDelay

	for d.ctx.Err() == nil {
		done := make(chan struct{})
		err := d.connectToNexus(done)
		if err == nil {
			// Connected — wait until the connection drops before reconnecting
			delay = ReconnectBaseDelay
			select {
			case <-done:
			case <-d.ctx.Done():
				return
			}
			log.Printf("Connection to Nexus lost, reconnecting in %s...", delay)
			time.Sleep(delay)
		} else {
			log.Printf("Failed to connect to Nexus: %v", err)
			// Exponential backoff
			time.Sleep(delay)
			delay = delay * 2
			if delay > ReconnectMaxDelay {
				delay = ReconnectMaxDelay
			}
		}
	}
}

// connectToNexus establishes a gRPC connection to Nexus.
// done is closed when the connection drops (receive loop exits).
func (d *Daemon) connectToNexus(done chan struct{}) error {
	// Use TLS with the pinned Nexus CA certificate
	tlsConfig, err := nexusTLSConfig()
	if err != nil {
		return fmt.Errorf("TLS setup failed (run 'daao login' first): %w", err)
	}
	creds := credentials.NewTLS(tlsConfig)

	conn, err := grpc.NewClient(
		d.config.NexusAddr,
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(8*1024*1024)), // 8MB — match server-side limit
	)
	if err != nil {
		return fmt.Errorf("failed to dial Nexus: %w", err)
	}

	d.grpcConn = conn
	d.grpcClient = proto.NewSatelliteGatewayClient(conn)

	// Create bidirectional stream
	stream, err := d.grpcClient.Connect(d.ctx)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create stream: %w", err)
	}

	// Send registration directly on the stream (before the writer starts).
	availableAgents := probeAvailableAgents()
	registerReq := &proto.RegisterRequest{
		SatelliteId:     d.config.SatelliteID,
		Fingerprint:     d.config.Fingerprint,
		PublicKey:       d.config.PrivateKey,
		Timestamp:       time.Now().Unix(),
		Version:         Version,
		Os:              runtime.GOOS,
		Arch:            runtime.GOARCH,
		AvailableAgents: availableAgents,
	}

	err = stream.Send(&proto.SatelliteMessage{
		Payload: &proto.SatelliteMessage_RegisterRequest{
			RegisterRequest: registerReq,
		},
	})
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to send registration: %w", err)
	}

	log.Printf("Registered with Nexus (satellite: %s, fingerprint: %s)", d.config.SatelliteID, d.config.Fingerprint)

	// Per-connection context: cancelled when this connection's receive loop
	// exits (on disconnect). All connection-scoped goroutines use this.
	connCtx, connCancel := context.WithCancel(d.ctx)

	// Start the single stream writer — the only goroutine allowed to call
	// stream.Send(). All other goroutines enqueue via sendToNexus/sendToNexusPriority.
	go d.streamWriter(connCtx, stream)

	// Replay active sessions AFTER the writer starts so the channels are
	// being drained. This notifies Nexus about sessions that survived a
	// Nexus restart.
	d.replayActiveSessions()

	// Heartbeat and telemetry are connection-scoped.
	go d.heartbeatLoop(connCtx)
	go d.telemetryLoop(connCtx)

	// Start receiving messages from Nexus; close done when loop exits
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer close(done)
		defer connCancel() // stop streamWriter/heartbeat/telemetry
		d.receiveMessages(stream)
	}()

	return nil
}

// heartbeatLoop sends periodic heartbeats to Nexus via the priority channel.
// ctx is a per-connection context that is cancelled when the connection drops,
// ensuring this goroutine exits instead of accumulating across reconnects.
func (d *Daemon) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()

	seq := int64(0)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			seq++
			if !d.sendToNexusPriority(&proto.SatelliteMessage{
				Payload: &proto.SatelliteMessage_HeartbeatPing{
					HeartbeatPing: &proto.HeartbeatPing{
						Timestamp:      time.Now().Unix(),
						SequenceNumber: seq,
					},
				},
			}) {
				log.Printf("Failed to enqueue heartbeat")
			}
		}
	}
}

// telemetryLoop sends periodic system metrics to Nexus via the priority channel.
// ctx is a per-connection context that is cancelled when the connection drops,
// ensuring this goroutine exits instead of accumulating across reconnects.
func (d *Daemon) telemetryLoop(ctx context.Context) {
	// Telemetry every 60 seconds
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Warmup: collect once immediately (first CPU reading is baseline)
	sysmetrics.Collect()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metrics, err := sysmetrics.Collect()
			if err != nil {
				log.Printf("Telemetry: failed to collect metrics: %v", err)
				continue
			}

			// Count active sessions
			d.sessionsMu.RLock()
			activeCount := len(d.sessions)
			d.sessionsMu.RUnlock()

			// Build GPU proto messages
			var gpuProtos []*proto.GpuMetrics
			for _, g := range metrics.GPUs {
				gpuProtos = append(gpuProtos, &proto.GpuMetrics{
					Index:              int32(g.Index),
					Name:               g.Name,
					UtilizationPercent: g.UtilizationPercent,
					MemoryUsedBytes:    g.MemoryUsedBytes,
					MemoryTotalBytes:   g.MemoryTotalBytes,
					TemperatureCelsius: g.TemperatureCelsius,
				})
			}

			d.sendToNexusPriority(&proto.SatelliteMessage{
				Payload: &proto.SatelliteMessage_TelemetryReport{
					TelemetryReport: &proto.TelemetryReport{
						SatelliteId:      d.config.SatelliteID,
						CpuPercent:       metrics.CPUPercent,
						MemoryPercent:    metrics.MemoryPercent,
						MemoryUsedBytes:  metrics.MemoryUsedBytes,
						MemoryTotalBytes: metrics.MemoryTotalBytes,
						DiskPercent:      metrics.DiskPercent,
						DiskUsedBytes:    metrics.DiskUsedBytes,
						DiskTotalBytes:   metrics.DiskTotalBytes,
						Gpus:             gpuProtos,
						Timestamp:        metrics.Timestamp,
						ActiveSessions:   int32(activeCount),
						AvailableAgents:  probeAvailableAgents(),
					},
				},
			})
			log.Printf("Telemetry: sent CPU=%.1f%% MEM=%.1f%% DISK=%.1f%% GPUs=%d sessions=%d",
				metrics.CPUPercent, metrics.MemoryPercent, metrics.DiskPercent,
				len(metrics.GPUs), activeCount)
		}
	}
}

// receiveMessages receives messages from Nexus on the given stream.
// Returns when the stream closes or the daemon context is cancelled.
// NOTE: wg.Done is handled by the caller (connectToNexus wrapper goroutine).
func (d *Daemon) receiveMessages(stream proto.SatelliteGateway_ConnectClient) {
	for {
		msg, err := stream.Recv()
		if err != nil {
			if d.ctx.Err() == nil {
				if err == io.EOF {
					log.Printf("Stream closed by Nexus")
				} else {
					log.Printf("Error receiving message: %v", err)
				}
			}
			return
		}
		d.handleNexusMessage(msg)
	}
}

// handleNexusMessage handles an incoming message from Nexus
func (d *Daemon) handleNexusMessage(msg *proto.NexusMessage) {
	switch m := msg.Payload.(type) {
	case *proto.NexusMessage_TerminalInput:
		d.handleTerminalInput(m.TerminalInput)
	case *proto.NexusMessage_ResizeCommand:
		d.handleResizeCommand(m.ResizeCommand)
	case *proto.NexusMessage_SuspendCommand:
		d.handleSuspendCommand(m.SuspendCommand)
	case *proto.NexusMessage_ResumeCommand:
		d.handleResumeCommand(m.ResumeCommand)
	case *proto.NexusMessage_KillCommand:
		d.handleKillCommand(m.KillCommand)
	case *proto.NexusMessage_StartSessionCommand:
		d.handleStartSessionCommand(m.StartSessionCommand)
	case *proto.NexusMessage_UpdateAvailable:
		d.handleUpdateAvailable(m.UpdateAvailable)
	case *proto.NexusMessage_SessionReconciliation:
		d.handleSessionReconciliation(m.SessionReconciliation)
	case *proto.NexusMessage_DeployAgentCommand:
		d.handleDeployAgentCommand(m.DeployAgentCommand)
	case *proto.NexusMessage_ContextFilePush:
		d.handleContextFilePush(m.ContextFilePush)
	case *proto.NexusMessage_ProvisionRuntimeCommand:
		d.handleProvisionRuntimeCommand(m.ProvisionRuntimeCommand)
	}
}

// handleProvisionRuntimeCommand handles a Nexus-initiated runtime provisioning
// request. Runs BootstrapRuntime in a goroutine and streams progress back.
func (d *Daemon) handleProvisionRuntimeCommand(cmd *proto.ProvisionRuntimeCommand) {
	log.Printf("Received ProvisionRuntimeCommand (force=%v)", cmd.Force)

	go func() {
		// If force isn't set and runtime is already installed, skip
		if !cmd.Force && satellite.IsRuntimeInstalled() {
			log.Printf("Runtime already installed and force=false, skipping")
			d.sendToNexusPriority(&proto.SatelliteMessage{
				Payload: &proto.SatelliteMessage_ProvisionProgress{
					ProvisionProgress: &proto.ProvisionProgress{
						Step:     "complete",
						Message:  "Runtime already installed",
						Complete: true,
					},
				},
			})
			return
		}

		err := satellite.BootstrapRuntime(d.ctx, func(step, msg string) {
			log.Printf("Bootstrap [%s]: %s", step, msg)
			d.sendToNexusPriority(&proto.SatelliteMessage{
				Payload: &proto.SatelliteMessage_ProvisionProgress{
					ProvisionProgress: &proto.ProvisionProgress{
						Step:     step,
						Message:  msg,
						Complete: step == "complete",
					},
				},
			})
		})

		if err != nil {
			log.Printf("Bootstrap failed: %v", err)
			d.sendToNexusPriority(&proto.SatelliteMessage{
				Payload: &proto.SatelliteMessage_ProvisionProgress{
					ProvisionProgress: &proto.ProvisionProgress{
						Step:     "error",
						Message:  fmt.Sprintf("Bootstrap failed: %v", err),
						Complete: true,
						Error:    err.Error(),
					},
				},
			})
		}
	}()
}

// handleDeployAgentCommand spawns a Pi RPC process for the given agent and
// forwards its events back to Nexus until the agent finishes or the daemon stops.
func (d *Daemon) handleDeployAgentCommand(cmd *proto.DeployAgentCommand) {
	sessionID := cmd.SessionId
	agentName := cmd.AgentDefinition.GetName()
	log.Printf("Received DeployAgentCommand for session %s (agent: %s)", sessionID, agentName)

	// Lazy provisioning: if runtime isn't installed, bootstrap it now.
	// This is the "Ansible-over-gRPC" self-provisioning — the satellite
	// downloads Node.js + Pi on first deploy, no SSH required.
	//
	// resolveBinary("pi") is the authoritative check — it searches DAAO runtime
	// dirs, user npm paths, and system PATH. If it finds Pi, PiBridge.Start()
	// will also find it, so bootstrap is unnecessary even if IsRuntimeInstalled()
	// returns false (e.g. node.exe missing but pi.cmd resolvable).
	piResolved := resolveBinary("pi")
	piAvailable := piResolved != "pi"
	if !satellite.IsRuntimeInstalled() && !piAvailable {
		log.Printf("handleDeployAgentCommand: runtime not installed, bootstrapping for session %s", sessionID)
		if err := satellite.BootstrapRuntime(d.ctx, func(step, msg string) {
			log.Printf("Bootstrap [%s]: %s", step, msg)
		}); err != nil {
			log.Printf("handleDeployAgentCommand: bootstrap failed for session %s: %v", sessionID, err)
			d.sendToNexusPriority(&proto.SatelliteMessage{
				Payload: &proto.SatelliteMessage_SessionStateUpdate{
					SessionStateUpdate: &proto.SessionStateUpdate{
						SessionId:    sessionID,
						State:        proto.SessionState_SESSION_STATE_TERMINATED,
						Timestamp:    time.Now().Unix(),
						ErrorMessage: fmt.Sprintf("runtime bootstrap failed: %v", err),
					},
				},
			})
			return
		}
		log.Printf("handleDeployAgentCommand: bootstrap complete for session %s", sessionID)
	}

	bridge := satellite.NewPiBridge()
	if err := bridge.Start(cmd.AgentDefinition, cmd.Secrets); err != nil {
		log.Printf("handleDeployAgentCommand: failed to start Pi bridge for session %s: %v", sessionID, err)
		// Notify Nexus that this session failed → TERMINATED
		d.sendToNexusPriority(&proto.SatelliteMessage{
			Payload: &proto.SatelliteMessage_SessionStateUpdate{
				SessionStateUpdate: &proto.SessionStateUpdate{
					SessionId:    sessionID,
					State:        proto.SessionState_SESSION_STATE_TERMINATED,
					Timestamp:    time.Now().Unix(),
					ErrorMessage: fmt.Sprintf("failed to start agent: %v", err),
				},
			},
		})
		return
	}

	// Successfully started → transition to RUNNING
	d.sendToNexusPriority(&proto.SatelliteMessage{
		Payload: &proto.SatelliteMessage_SessionStateUpdate{
			SessionStateUpdate: &proto.SessionStateUpdate{
				SessionId: sessionID,
				State:     proto.SessionState_SESSION_STATE_RUNNING,
				Timestamp: time.Now().Unix(),
			},
		},
	})
	log.Printf("handleDeployAgentCommand: Pi bridge started for session %s, state → RUNNING", sessionID)

	d.bridgesMu.Lock()
	// Stop any existing bridge for this session before replacing it
	if old, ok := d.bridges[sessionID]; ok {
		old.Stop()
	}
	d.bridges[sessionID] = bridge
	d.bridgesMu.Unlock()

	// Build a contextual initial prompt so the agent knows where it's running.
	hostname, _ := os.Hostname()
	contextDir := satellite.ContextDir()
	initialPrompt := fmt.Sprintf(
		"You are running on satellite %q (hostname: %s, OS: %s/%s).\n"+
			"DAAO context directory: %s\n"+
			"Begin executing your task immediately. Run commands, gather data, and produce your output now.",
		agentName, hostname, runtime.GOOS, runtime.GOARCH, contextDir,
	)
	if err := bridge.SendPrompt(initialPrompt); err != nil {
		log.Printf("handleDeployAgentCommand: failed to send initial prompt for session %s: %v", sessionID, err)
	}

	// Forward Pi events to Nexus in a background goroutine.
	// The events channel is closed by PiBridge after the process exits and
	// all stdout/stderr is drained, so ranging over it is safe and complete.
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer func() {
			d.bridgesMu.Lock()
			delete(d.bridges, sessionID)
			d.bridgesMu.Unlock()

			// Agent finished → TERMINATED
			exitCode := bridge.ExitCode()
			d.sendToNexusPriority(&proto.SatelliteMessage{
				Payload: &proto.SatelliteMessage_SessionStateUpdate{
					SessionStateUpdate: &proto.SessionStateUpdate{
						SessionId: sessionID,
						State:     proto.SessionState_SESSION_STATE_TERMINATED,
						Timestamp: time.Now().Unix(),
					},
				},
			})
			log.Printf("handleDeployAgentCommand: Pi bridge finished for session %s (exit code: %d), state → TERMINATED", sessionID, exitCode)
		}()

		for {
			select {
			case event, ok := <-bridge.Events():
				if !ok {
					// Events channel closed — process exited and all events drained
					return
				}
				d.sendToNexusAgent(&proto.SatelliteMessage{
					Payload: &proto.SatelliteMessage_AgentEvent{
						AgentEvent: satellite.GetAgentEventProto(sessionID, event),
					},
				})
			case <-d.ctx.Done():
				bridge.Stop()
				return
			}
		}
	}()
}

// handleSessionReconciliation compares local sessions against the authoritative
// list from Nexus and prunes any orphans (sessions Nexus considers terminated
// or doesn't know about). This handles lost KillCommands during reconnects.
func (d *Daemon) handleSessionReconciliation(msg *proto.SessionReconciliation) {
	activeSet := make(map[string]struct{}, len(msg.ActiveSessionIds))
	for _, id := range msg.ActiveSessionIds {
		activeSet[id] = struct{}{}
	}

	log.Printf("Reconciliation: Nexus reports %d active session(s)", len(activeSet))

	// Snapshot sessions to avoid holding lock during cleanup
	d.sessionsMu.RLock()
	var orphans []string
	for sid, sess := range d.sessions {
		if sess.State == session.SessionState(session.StateTerminated) {
			continue // already terminated locally, will be cleaned up
		}
		if _, ok := activeSet[sid]; !ok {
			orphans = append(orphans, sid)
		}
	}
	d.sessionsMu.RUnlock()

	for _, sid := range orphans {
		log.Printf("Reconciliation: pruning orphan session %s (not in Nexus active list)", sid)
		d.sessionsMu.RLock()
		sess, ok := d.sessions[sid]
		d.sessionsMu.RUnlock()
		if ok {
			sess.Close()
		}
		d.sessionsMu.Lock()
		delete(d.sessions, sid)
		d.sessionsMu.Unlock()
	}

	if len(orphans) > 0 {
		log.Printf("Reconciliation: pruned %d orphan session(s)", len(orphans))
	}
}

// handleUpdateAvailable processes an update notification from Nexus
func (d *Daemon) handleUpdateAvailable(msg *proto.UpdateAvailable) {
	log.Printf("Update available: %s -> %s (download: %s, force: %v)",
		Version, msg.LatestVersion, msg.DownloadUrl, msg.Force)

	// Save notification for `daao update` to find
	info := &updateInfo{
		LatestVersion: msg.LatestVersion,
		DownloadURL:   msg.DownloadUrl,
		Force:         msg.Force,
	}
	if err := saveUpdateNotification(info); err != nil {
		log.Printf("Failed to save update notification: %v", err)
	}
}

// handleStartSessionCommand creates and starts a new session
func (d *Daemon) handleStartSessionCommand(cmd *proto.StartSessionCommand) {
	log.Printf("Received StartSessionCommand for session %s (binary: %s)", cmd.SessionId, cmd.AgentBinary)

	sess, err := d.CreateSession(cmd.SessionId, cmd.AgentBinary, cmd.AgentArgs, cmd.Cols, cmd.Rows, cmd.WorkingDir)
	if err != nil {
		log.Printf("Failed to create session %s: %v", cmd.SessionId, err)
		// Send error update back to Nexus
		d.sendToNexusPriority(&proto.SatelliteMessage{
			Payload: &proto.SatelliteMessage_SessionStateUpdate{
				SessionStateUpdate: &proto.SessionStateUpdate{
					SessionId:    cmd.SessionId,
					State:        proto.SessionState_SESSION_STATE_TERMINATED,
					Timestamp:    time.Now().Unix(),
					ErrorMessage: err.Error(),
				},
			},
		})
		return
	}

	log.Printf("Successfully started session %s", sess.ID)
}

// handleTerminalInput forwards terminal input to the PTY
func (d *Daemon) handleTerminalInput(input *proto.TerminalInput) {
	d.sessionsMu.RLock()
	sess, ok := d.sessions[input.SessionId]
	d.sessionsMu.RUnlock()

	if !ok || sess == nil {
		return
	}

	if sess.Pty != nil {
		_, err := sess.Pty.Write(input.Data)
		if err != nil {
			log.Printf("Error writing to PTY: %v", err)
		}
	}

	// Record activity for DMS
	if sess.DMS != nil {
		sess.DMS.RecordActivity()
	}
}

// handleResizeCommand resizes the PTY
func (d *Daemon) handleResizeCommand(cmd *proto.ResizeCommand) {
	d.sessionsMu.RLock()
	sess, ok := d.sessions[cmd.SessionId]
	d.sessionsMu.RUnlock()

	if !ok || sess == nil {
		return
	}

	if sess.Pty != nil {
		err := sess.Pty.Resize(uint16(cmd.Width), uint16(cmd.Height))
		if err != nil {
			log.Printf("Error resizing PTY for session %s: %v", sess.ID, err)
		}
	}
}

// handleSuspendCommand suspends a session
func (d *Daemon) handleSuspendCommand(cmd *proto.SuspendCommand) {
	d.sessionsMu.RLock()
	sess, ok := d.sessions[cmd.SessionId]
	d.sessionsMu.RUnlock()

	if !ok || sess == nil {
		log.Printf("SuspendCommand: session %s not found", cmd.SessionId)
		return
	}

	log.Printf("Suspending session %s", cmd.SessionId)
	sess.State = session.SessionState(session.StateSuspended)

	// Send state update back to Nexus
	d.sendToNexusPriority(&proto.SatelliteMessage{
		Payload: &proto.SatelliteMessage_SessionStateUpdate{
			SessionStateUpdate: &proto.SessionStateUpdate{
				SessionId: cmd.SessionId,
				State:     proto.SessionState_SESSION_STATE_SUSPENDED,
				Timestamp: time.Now().Unix(),
			},
		},
	})
}

// handleResumeCommand resumes a session
func (d *Daemon) handleResumeCommand(cmd *proto.ResumeCommand) {
	d.sessionsMu.RLock()
	sess, ok := d.sessions[cmd.SessionId]
	d.sessionsMu.RUnlock()

	if !ok || sess == nil {
		log.Printf("ResumeCommand: session %s not found", cmd.SessionId)
		return
	}

	log.Printf("Resuming session %s", cmd.SessionId)
	sess.State = session.SessionState(session.StateRunning)

	// Reset DMS timer on resume
	if sess.DMS != nil {
		sess.DMS.RecordActivity()
	}

	// Send state update back to Nexus
	d.sendToNexusPriority(&proto.SatelliteMessage{
		Payload: &proto.SatelliteMessage_SessionStateUpdate{
			SessionStateUpdate: &proto.SessionStateUpdate{
				SessionId: cmd.SessionId,
				State:     proto.SessionState_SESSION_STATE_RUNNING,
				Timestamp: time.Now().Unix(),
			},
		},
	})
}

// handleKillCommand terminates a session
func (d *Daemon) handleKillCommand(cmd *proto.KillCommand) {
	d.sessionsMu.RLock()
	sess, ok := d.sessions[cmd.SessionId]
	d.sessionsMu.RUnlock()

	if !ok || sess == nil {
		log.Printf("KillCommand: session %s not found", cmd.SessionId)
		return
	}

	log.Printf("Killing session %s", cmd.SessionId)
	sess.Close()

	d.sessionsMu.Lock()
	delete(d.sessions, cmd.SessionId)
	d.sessionsMu.Unlock()

	// Send state update back to Nexus
	d.sendToNexusPriority(&proto.SatelliteMessage{
		Payload: &proto.SatelliteMessage_SessionStateUpdate{
			SessionStateUpdate: &proto.SessionStateUpdate{
				SessionId: cmd.SessionId,
				State:     proto.SessionState_SESSION_STATE_TERMINATED,
				Timestamp: time.Now().Unix(),
			},
		},
	})
}

// forwardPtyOutput forwards PTY output to ring buffer and gRPC
func (d *Daemon) forwardPtyOutput(sess *Session) {
	defer sess.Wg.Done()

	// Goroutine to wait for process exit, then clean up.
	sess.Wg.Add(1)
	go func() {
		defer sess.Wg.Done()
		if sess.Process != nil {
			state, err := sess.Process.Wait()
			if err != nil {
				log.Printf("Process for session %s: Wait error: %v", sess.ID, err)
			} else {
				log.Printf("Process for session %s exited: success=%v exitCode=%v", sess.ID, state.Success(), state.ExitCode())
			}
		}

		// Close the PTY. This releases ConPTY's internal write handle to the output
		// pipe, making outputPipe.Read() in the outer loop return an error so it exits.
		// This is the reliable way to unblock the outer read goroutine.
		sess.Pty.Close()

		// Transition to terminated
		d.TransitionSessionState(sess.ID, session.SessionState(session.StateTerminated))

		// Update Nexus
		d.sendToNexusPriority(&proto.SatelliteMessage{
			Payload: &proto.SatelliteMessage_SessionStateUpdate{
				SessionStateUpdate: &proto.SessionStateUpdate{
					SessionId: sess.ID,
					State:     proto.SessionState_SESSION_STATE_TERMINATED,
					Timestamp: time.Now().Unix(),
				},
			},
		})
	}()

	buf := make([]byte, 4096)

	for {
		select {
		case <-d.ctx.Done():
			log.Printf("ForwardPtyOutput: Context done for session %s", sess.ID)
			return
		default:
		}

		n, err := sess.Pty.Read(buf)
		if err != nil {
			if err == io.EOF {
				log.Printf("PTY closed for session %s (EOF)", sess.ID)
			} else {
				// Access is denied (ERROR_ACCESS_DENIED) can happen if ConPTY is closing
				if strings.Contains(err.Error(), "Access is denied") {
					log.Printf("PTY access denied for session %s (likely closing)", sess.ID)
				} else {
					log.Printf("Error reading PTY for session %s: %v", sess.ID, err)
				}
			}
			return
		}

		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			log.Printf("ForwardPtyOutput: Read %d bytes from PTY for session %s", n, sess.ID)

			// Write to ring buffer
			if sess.RingBuffer != nil {
				_, _ = sess.RingBuffer.Write(data)
			}

			// Fan-out to locally attached terminals
			sess.localClientsMu.RLock()
			for _, w := range sess.localClients {
				w.Write(data) //nolint:errcheck
			}
			sess.localClientsMu.RUnlock()

			// Send to Nexus via gRPC
			seq := time.Now().UnixNano()
			d.sendToNexus(&proto.SatelliteMessage{
				Payload: &proto.SatelliteMessage_TerminalData{
					TerminalData: &proto.TerminalData{
						SessionId:      sess.ID,
						Data:           data,
						SequenceNumber: seq,
						Timestamp:      time.Now().Unix(),
						IsStdout:       true,
					},
				},
			})
		}
	}
}

// replayActiveSessions re-announces all live sessions and replays their ring
// buffers to Nexus. Called after reconnecting so Nexus can rehydrate its
// in-memory state after a restart.
func (d *Daemon) replayActiveSessions() {
	d.sessionsMu.RLock()
	defer d.sessionsMu.RUnlock()

	if len(d.sessions) == 0 {
		return
	}

	log.Printf("Replaying %d active session(s) to Nexus", len(d.sessions))

	for _, sess := range d.sessions {
		if sess.State == session.SessionState(session.StateTerminated) {
			continue
		}

		// Re-announce session state (priority: control message)
		d.sendToNexusPriority(&proto.SatelliteMessage{
			Payload: &proto.SatelliteMessage_SessionStateUpdate{
				SessionStateUpdate: &proto.SessionStateUpdate{
					SessionId: sess.ID,
					State:     sessionStateToProto(sess.State),
					Timestamp: time.Now().Unix(),
				},
			},
		})

		// Replay ring buffer snapshot
		if sess.RingBuffer != nil && sess.RingBuffer.Len() > 0 {
			snapshot := sess.RingBuffer.Snapshot()
			d.sendToNexus(&proto.SatelliteMessage{
				Payload: &proto.SatelliteMessage_BufferReplay{
					BufferReplay: &proto.BufferReplay{
						SessionId: sess.ID,
						Data:      snapshot,
					},
				},
			})
			log.Printf("Replayed %d bytes of ring buffer for session %s", len(snapshot), sess.ID)
		}
	}
}

// sessionStateToProto converts a daemon-side session state to proto enum.
func sessionStateToProto(state session.SessionState) proto.SessionState {
	switch state {
	case session.SessionState(session.StateRunning):
		return proto.SessionState_SESSION_STATE_RUNNING
	case session.SessionState(session.StateSuspended):
		return proto.SessionState_SESSION_STATE_SUSPENDED
	case session.SessionState(session.StateDetached):
		return proto.SessionState_SESSION_STATE_DETACHED
	case session.SessionState(session.StateReAttaching):
		return proto.SessionState_SESSION_STATE_RE_ATTACHING
	case session.SessionState(session.StateTerminated):
		return proto.SessionState_SESSION_STATE_TERMINATED
	case session.SessionState(session.StateProvisioning):
		return proto.SessionState_SESSION_STATE_STARTING
	default:
		return proto.SessionState_SESSION_STATE_RUNNING
	}
}

// signalHandler handles OS signals
func (d *Daemon) signalHandler() {
	defer d.wg.Done()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	select {
	case <-d.ctx.Done():
	case sig := <-sigCh:
		log.Printf("Received signal %v, shutting down...", sig)
		d.Stop()
	}
}

// Close closes a session
func (s *Session) Close() {
	s.CloseOnce.Do(func() {
		// Stop DMS
		if s.DMS != nil {
			s.DMS.Stop()
		}

		// Close IPC server
		if s.IPCServer != nil {
			s.IPCServer.Close()
		}

		// Close PTY
		if s.Pty != nil {
			s.Pty.Close()
		}

		// Kill process
		if s.Process != nil {
			s.Process.Kill()
		}

		s.Wg.Wait()
	})
}

// runContextWatcher watches the context directory for local file changes and
// forwards them to Nexus as ContextFileUpdate messages.
func (d *Daemon) runContextWatcher(dir string) {
	defer d.wg.Done()

	watcher := satellite.NewContextWatcher(dir)
	if err := watcher.Start(); err != nil {
		log.Printf("ContextWatcher: failed to start: %v", err)
		return
	}
	defer watcher.Stop()

	// On connect, send all existing context files to Nexus so it has the
	// current state without waiting for edits.
	for name, content := range satellite.ReadAllContextFiles(dir) {
		d.sendContextUpdate(name, content, false)
	}

	for {
		select {
		case <-d.ctx.Done():
			return
		case event, ok := <-watcher.Events():
			if !ok {
				return
			}
			d.sendContextUpdate(event.FilePath, event.Content, event.Deleted)
		}
	}
}

func (d *Daemon) sendContextUpdate(filePath, content string, deleted bool) {
	c := content
	if deleted {
		c = ""
	}
	d.sendToNexus(&proto.SatelliteMessage{
		Payload: &proto.SatelliteMessage_ContextFileUpdate{
			ContextFileUpdate: &proto.ContextFileUpdate{
				File: &proto.ContextFileSync{
					SatelliteId: d.config.SatelliteID,
					FilePath:    filePath,
					Content:     []byte(c),
					Version:     0, // Nexus assigns the canonical version
					ModifiedBy:  "local:" + d.config.SatelliteID,
				},
			},
		},
	})
}

// handleContextFilePush receives a file pushed from Nexus and writes it to disk.
func (d *Daemon) handleContextFilePush(push *proto.ContextFilePush) {
	if push.File == nil {
		return
	}
	dir := satellite.ContextDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("ContextFilePush: failed to create context dir: %v", err)
		return
	}
	// Reject path traversal attempts
	base := filepath.Base(push.File.FilePath)
	if base != push.File.FilePath || base == "." || base == ".." {
		log.Printf("ContextFilePush: rejected suspicious file path: %q", push.File.FilePath)
		return
	}
	path := filepath.Join(dir, base)
	if err := os.WriteFile(path, push.File.Content, 0644); err != nil {
		log.Printf("ContextFilePush: failed to write %s: %v", base, err)
		return
	}
	log.Printf("ContextFilePush: wrote %s (%d bytes)", base, len(push.File.Content))
}
