// Package main provides a mock satellite simulator for testing.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/daao/nexus/proto"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	// Default values
	defaultNexusAddr  = "localhost:8444"
	defaultSatName    = "test-sat"
	defaultOutput     = "Hello!\r\n"
	defaultSessionTTL = 5 * time.Second

	// Heartbeat interval
	heartbeatInterval = 30 * time.Second

	// Terminal data chunk size
	terminalDataChunkSize = 32

	// Small delay between chunks
	chunkDelay = 50 * time.Millisecond
)

// MockSatellite represents a mock satellite simulator
type MockSatellite struct {
	// Configuration
	nexusAddr   string
	satName     string
	fingerprint string
	output      string
	sessionTTL  time.Duration

	// gRPC connection
	ctx        context.Context
	cancel     context.CancelFunc
	grpcConn   *grpc.ClientConn
	grpcClient proto.SatelliteGatewayClient
	stream     proto.SatelliteGateway_ConnectClient

	// State
	mu         sync.Mutex
	running    bool
	sessionIDs map[string]bool
}

func main() {
	// Parse flags
	nexusAddr := flag.String("nexus-grpc", defaultNexusAddr, "Nexus gRPC address")
	satName := flag.String("name", defaultSatName, "Satellite name")
	output := flag.String("output", defaultOutput, "Output to stream during session")
	sessionTTL := flag.Duration("session-ttl", defaultSessionTTL, "Session TTL duration")

	flag.Parse()

	// Generate UUID and fingerprint for this satellite run.
	// The fingerprint is randomized so that stale fingerprint records from
	// previous test runs don't match this satellite in the gRPC gateway query.
	satID := uuid.New().String()
	satFingerprint := uuid.New().String()
	fmt.Printf("MOCK_SATELLITE_UUID: %s\n", satID)

	// Create satellite instance
	satellite := &MockSatellite{
		nexusAddr:   *nexusAddr,
		satName:     *satName,
		fingerprint: satFingerprint,
		output:      *output,
		sessionTTL:  *sessionTTL,
		sessionIDs:  make(map[string]bool),
	}

	// Create context with cancel
	satellite.ctx, satellite.cancel = context.WithCancel(context.Background())

	// Log startup
	log.Printf("[MOCK] Satellite starting: name=%s, nexus=%s, session-ttl=%v", satellite.satName, satellite.nexusAddr, satellite.sessionTTL)

	// Start signal handler
	go satellite.signalHandler()

	// Run gRPC loop
	satellite.runGrpcLoop()
}

// signalHandler handles OS signals for clean shutdown
func (s *MockSatellite) signalHandler() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	select {
	case <-s.ctx.Done():
	case sig := <-sigCh:
		log.Printf("[MOCK] Received signal %v, shutting down...", sig)
		s.cleanup()
		os.Exit(0)
	}
}

// cleanup performs cleanup on shutdown
func (s *MockSatellite) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.running = false

	if s.cancel != nil {
		s.cancel()
	}

	if s.grpcConn != nil {
		log.Printf("[MOCK] Closing gRPC connection")
		s.grpcConn.Close()
	}
}

// runGrpcLoop maintains the gRPC connection to Nexus with exponential backoff
func (s *MockSatellite) runGrpcLoop() {
	delay := time.Second

	for s.ctx.Err() == nil {
		done := make(chan struct{})
		err := s.connectToNexus(done)
		if err == nil {
			// Connected — wait until the connection drops before reconnecting
			delay = time.Second
			select {
			case <-done:
			case <-s.ctx.Done():
				return
			}
			log.Printf("[MOCK] Connection to Nexus lost, reconnecting in %s...", delay)
			time.Sleep(delay)
		} else {
			log.Printf("[MOCK] Failed to connect to Nexus: %v", err)
			// Exponential backoff
			time.Sleep(delay)
			delay = delay * 2
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
		}
	}
}

// connectToNexus establishes a gRPC connection to Nexus
func (s *MockSatellite) connectToNexus(done chan struct{}) error {
	// Use TLS — Nexus gRPC server requires TLS
	// InsecureSkipVerify for self-signed certs in dev
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec
	}
	creds := credentials.NewTLS(tlsConfig)

	conn, err := grpc.NewClient(
		s.nexusAddr,
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return fmt.Errorf("failed to dial Nexus: %w", err)
	}

	s.grpcConn = conn
	s.grpcClient = proto.NewSatelliteGatewayClient(conn)

	// Create bidirectional stream
	stream, err := s.grpcClient.Connect(s.ctx)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create stream: %w", err)
	}

	s.stream = stream

	// Send registration
	registerReq := &proto.RegisterRequest{
		SatelliteId: s.satName,
		Fingerprint: s.fingerprint,
		PublicKey:   "mock-public-key",
		Timestamp:   time.Now().Unix(),
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

	log.Printf("[MOCK] Registered with Nexus (satellite: %s)", s.satName)

	// Mark as running
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	// Start heartbeat loop
	go s.heartbeatLoop()

	// Start receiving messages from Nexus
	go s.receiveMessages(done)

	return nil
}

// heartbeatLoop sends periodic heartbeats to Nexus
func (s *MockSatellite) heartbeatLoop() {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	seq := int64(0)

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			stream := s.stream
			running := s.running
			s.mu.Unlock()

			if stream != nil && running {
				seq++
				err := stream.Send(&proto.SatelliteMessage{
					Payload: &proto.SatelliteMessage_HeartbeatPing{
						HeartbeatPing: &proto.HeartbeatPing{
							Timestamp:      time.Now().Unix(),
							SequenceNumber: seq,
						},
					},
				})
				if err != nil {
					log.Printf("[MOCK] Failed to send heartbeat: %v", err)
				} else {
					log.Printf("[MOCK] Sent heartbeat ping (seq=%d)", seq)
				}
			}
		}
	}
}

// receiveMessages receives messages from Nexus
func (s *MockSatellite) receiveMessages(done chan struct{}) {
	defer close(done)

	for s.ctx.Err() == nil {
		s.mu.Lock()
		stream := s.stream
		running := s.running
		s.mu.Unlock()

		if stream == nil || !running {
			time.Sleep(time.Second)
			continue
		}

		msg, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				log.Printf("[MOCK] Stream closed by Nexus")
			} else {
				log.Printf("[MOCK] Error receiving message: %v", err)
			}
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
			return
		}

		s.handleNexusMessage(msg)
	}
}

// handleNexusMessage handles an incoming message from Nexus
func (s *MockSatellite) handleNexusMessage(msg *proto.NexusMessage) {
	switch m := msg.Payload.(type) {
	case *proto.NexusMessage_StartSessionCommand:
		s.handleStartSessionCommand(m.StartSessionCommand)
	default:
		log.Printf("[MOCK] Received unhandled message type: %T", msg.Payload)
	}
}

// handleStartSessionCommand handles a start session command
func (s *MockSatellite) handleStartSessionCommand(cmd *proto.StartSessionCommand) {
	log.Printf("[MOCK] Received StartSessionCommand for session %s", cmd.SessionId)

	// Store session ID
	s.mu.Lock()
	s.sessionIDs[cmd.SessionId] = true
	s.mu.Unlock()

	// Send RUNNING state update
	s.mu.Lock()
	stream := s.stream
	s.mu.Unlock()

	if stream == nil {
		log.Printf("[MOCK] No stream available to send state update")
		return
	}

	err := stream.Send(&proto.SatelliteMessage{
		Payload: &proto.SatelliteMessage_SessionStateUpdate{
			SessionStateUpdate: &proto.SessionStateUpdate{
				SessionId: cmd.SessionId,
				State:     proto.SessionState_SESSION_STATE_RUNNING,
				Timestamp: time.Now().Unix(),
			},
		},
	})
	if err != nil {
		log.Printf("[MOCK] Failed to send RUNNING state: %v", err)
		return
	}

	log.Printf("[MOCK] Sent SESSION_STATE_RUNNING for session %s", cmd.SessionId)

	// Stream output in chunks
	go s.streamOutput(cmd.SessionId, stream)

	// Schedule TERMINATED state after session TTL
	go func() {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(s.sessionTTL):
			s.mu.Lock()
			_, exists := s.sessionIDs[cmd.SessionId]
			s.mu.Unlock()

			if !exists {
				return
			}

			err := stream.Send(&proto.SatelliteMessage{
				Payload: &proto.SatelliteMessage_SessionStateUpdate{
					SessionStateUpdate: &proto.SessionStateUpdate{
						SessionId: cmd.SessionId,
						State:     proto.SessionState_SESSION_STATE_TERMINATED,
						Timestamp: time.Now().Unix(),
					},
				},
			})
			if err != nil {
				log.Printf("[MOCK] Failed to send TERMINATED state: %v", err)
			} else {
				log.Printf("[MOCK] Sent SESSION_STATE_TERMINATED for session %s", cmd.SessionId)
			}

			s.mu.Lock()
			delete(s.sessionIDs, cmd.SessionId)
			s.mu.Unlock()
		}
	}()
}

// streamOutput streams the output in chunks
func (s *MockSatellite) streamOutput(sessionID string, stream proto.SatelliteGateway_ConnectClient) {
	output := []byte(s.output)
	seq := int64(0)

	for i := 0; i < len(output); i += terminalDataChunkSize {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		end := i + terminalDataChunkSize
		if end > len(output) {
			end = len(output)
		}

		chunk := output[i:end]
		seq++

		err := stream.Send(&proto.SatelliteMessage{
			Payload: &proto.SatelliteMessage_TerminalData{
				TerminalData: &proto.TerminalData{
					SessionId:      sessionID,
					Data:           chunk,
					SequenceNumber: seq,
					Timestamp:      time.Now().Unix(),
					IsStdout:       true,
				},
			},
		})
		if err != nil {
			log.Printf("[MOCK] Failed to send terminal data: %v", err)
			return
		}

		log.Printf("[MOCK] Sent terminal data chunk (%d bytes) for session %s", len(chunk), sessionID)

		// Small delay between chunks
		time.Sleep(chunkDelay)
	}

	log.Printf("[MOCK] Finished streaming output for session %s", sessionID)
}
