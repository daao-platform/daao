package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/daao/nexus/internal/agentstream"
	"github.com/daao/nexus/internal/database"
	"github.com/daao/nexus/internal/session"
	"github.com/daao/nexus/pkg/buffer"
	"github.com/daao/nexus/proto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionStoreInterface defines the interface for session storage operations
type SessionStoreInterface interface {
	GetSession(ctx context.Context, id uuid.UUID) (*session.Session, error)
	UpdateState(ctx context.Context, id uuid.UUID, state session.SessionState) error
	TransitionSession(ctx context.Context, id uuid.UUID, targetState session.SessionState) (*session.Session, error)
	WriteEventLog(ctx context.Context, event *session.EventLog) error
}

// StreamRegistryInterface defines the interface for managing gRPC streams
type StreamRegistryInterface interface {
	RegisterStream(sessionID string, ch chan<- *proto.NexusMessage)
	UnregisterStream(sessionID string)
	RegisterSatelliteStream(satelliteID string, ch chan<- *proto.NexusMessage)
	UnregisterSatelliteStream(satelliteID string)
	SendToSession(sessionID string, msg *proto.NexusMessage) bool
	SendToSatellite(satelliteID string, msg *proto.NexusMessage) bool
}

// RingBufferPoolInterface defines the interface for ring buffer pool operations
type RingBufferPoolInterface interface {
	GetOrCreateBuffer(sessionID string) *buffer.RingBuffer
	RemoveBuffer(sessionID string)
}

// RecordingPoolInterface defines the interface for session recording operations.
// Nil-safe: when nil, recording is disabled.
type RecordingPoolInterface interface {
	WriteIfRecording(sessionID string, data []byte)
}

// DBPoolInterface defines the interface for database pool operations.
// This is compatible with *pgxpool.Pool directly.
type DBPoolInterface interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// AgentRunHubInterface defines the interface for publishing agent run events.
// Nil-safe: when nil, event publishing is disabled.
type AgentRunHubInterface interface {
	Publish(runID uuid.UUID, event agentstream.AgentStreamEvent)
	Close(runID uuid.UUID)
}

// TelemetryHandlerFunc is called when a telemetry report is received from a satellite.
// satelliteID is the canonical UUID string, metrics maps metric names to float64 values.
type TelemetryHandlerFunc func(ctx context.Context, satelliteID string, metrics map[string]float64)

// SatelliteGatewayServerImpl implements the SatelliteGateway gRPC service
type SatelliteGatewayServerImpl struct {
	proto.UnimplementedSatelliteGatewayServer
	sessionStore     SessionStoreInterface
	streamRegistry   StreamRegistryInterface
	dbPool           DBPoolInterface
	ringBufferPool   RingBufferPoolInterface
	recordingPool    RecordingPoolInterface
	runEventHub      AgentRunHubInterface // nil-safe
	telemetryHandler TelemetryHandlerFunc   // nil-safe — forwards telemetry to Forge Scheduler
	batchWriter      *database.BatchEventWriter // nil-safe
}

// NewSatelliteGatewayServerImpl creates a new SatelliteGateway server implementation
func NewSatelliteGatewayServerImpl(
	sessionStore SessionStoreInterface,
	streamRegistry StreamRegistryInterface,
	dbPool DBPoolInterface,
	ringBufferPool RingBufferPoolInterface,
	recordingPool RecordingPoolInterface,
	runEventHub AgentRunHubInterface,
	telemetryHandler TelemetryHandlerFunc,
	batchWriter *database.BatchEventWriter, // new — nil in community mode
) *SatelliteGatewayServerImpl {
	return &SatelliteGatewayServerImpl{
		sessionStore:     sessionStore,
		streamRegistry:   streamRegistry,
		dbPool:           dbPool,
		ringBufferPool:   ringBufferPool,
		recordingPool:    recordingPool,
		runEventHub:      runEventHub,
		telemetryHandler: telemetryHandler,
		batchWriter:      batchWriter,
	}
}

// Connect establishes a bidirectional stream for session communication
// It handles TerminalData, SessionStateUpdate from satellite and sends
// NexusMessage (TerminalInput, ResizeCommand, etc.) to satellite
func (s *SatelliteGatewayServerImpl) Connect(stream proto.SatelliteGateway_ConnectServer) error {
	slog.Info("SatelliteGateway: New connection established", "component", "grpc")

	// Channel for outbound messages to send to satellite
	outboundCh := make(chan *proto.NexusMessage, 100)

	// sessionID -> active run_id string (populated on DeployAgentCommand or lookup)
	sessionRunMap := make(map[string]string)
	// per-session sequence counters for AgentEvents
	agentSeq := make(map[string]int)

	// Tracks session IDs associated with this connection for cleanup
	sessionIDs := make(map[string]struct{})
	var sessionIDsMu sync.Mutex

	// Helper to register a session with this connection
	registerSession := func(sid string) {
		if sid == "" {
			return
		}
		sessionIDsMu.Lock()
		defer sessionIDsMu.Unlock()
		if _, ok := sessionIDs[sid]; !ok {
			sessionIDs[sid] = struct{}{}
			if s.streamRegistry != nil {
				s.streamRegistry.RegisterStream(sid, outboundCh)
				slog.Info(fmt.Sprintf("SatelliteGateway: Registered stream for session %s", sid), "component", "grpc")
			}
		}
	}

	// Helper to unregister all sessions
	unregisterAll := func() {
		sessionIDsMu.Lock()
		defer sessionIDsMu.Unlock()
		if s.streamRegistry != nil {
			for sid := range sessionIDs {
				s.streamRegistry.UnregisterStream(sid)
				slog.Info(fmt.Sprintf("SatelliteGateway: Unregistered stream for session %s", sid), "component", "grpc")
			}
		}
	}

	// Track the satellite ID for this connection — set on RegisterRequest
	var satelliteID string

	// Context for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// On disconnect, set satellite status to offline and cleanup
	defer func() {
		unregisterAll()
		if satelliteID != "" {
			if s.streamRegistry != nil {
				s.streamRegistry.UnregisterSatelliteStream(satelliteID)
				slog.Info(fmt.Sprintf("SatelliteGateway: Unregistered stream for satellite %s", satelliteID), "component", "grpc")
			}
			if s.dbPool != nil {
				_, err := s.dbPool.Exec(context.Background(),
					`UPDATE satellites SET status = 'offline', updated_at = NOW() 
					 WHERE id::text = $1 OR name = $1 OR fingerprint = $1`,
					satelliteID,
				)
				if err != nil {
					slog.Error(fmt.Sprintf("SatelliteGateway: Failed to set satellite %s offline: %v", satelliteID, err), "component", "grpc")
				} else {
					slog.Info(fmt.Sprintf("SatelliteGateway: Satellite %s set to offline", satelliteID), "component", "grpc")
				}
			}
		}
	}()

	// Goroutine to send messages to satellite.
	// It drains outboundCh even after ctx is cancelled so that any messages
	// queued just before disconnect (e.g. SessionReconciliation) are delivered.
	var senderWg sync.WaitGroup
	senderWg.Add(1)
	go func() {
		defer senderWg.Done()
		for {
			select {
			case msg, ok := <-outboundCh:
				if !ok {
					return
				}
				if err := stream.Send(msg); err != nil {
					slog.Error(fmt.Sprintf("SatelliteGateway: Error sending message: %v", err), "component", "grpc")
					// Drain remaining without sending to unblock writers
					for len(outboundCh) > 0 {
						<-outboundCh
					}
					return
				}
			case <-ctx.Done():
				// Flush any messages already queued before exiting
				for len(outboundCh) > 0 {
					msg := <-outboundCh
					if err := stream.Send(msg); err != nil {
						return
					}
				}
				return
			}
		}
	}()

	for {
		// Receive messages from satellite
		satMsg, err := stream.Recv()
		if err != nil {
			if err != io.EOF {
				slog.Error(fmt.Sprintf("SatelliteGateway: Error receiving message: %v", err), "component", "grpc")
			}
			cancel()
			senderWg.Wait()
			return err
		}

		// Handle the message based on its type
		switch payload := satMsg.Payload.(type) {
		case *proto.SatelliteMessage_RegisterRequest:
			// Handle satellite registration — update status to active
			rawID := payload.RegisterRequest.SatelliteId
			fingerprint := payload.RegisterRequest.Fingerprint
			version := payload.RegisterRequest.Version
			satOS := payload.RegisterRequest.Os
			satArch := payload.RegisterRequest.Arch
			slog.Info(fmt.Sprintf("SatelliteGateway: RegisterRequest received for satellite %s (fingerprint: %s, version: %s, os: %s, arch: %s, agents: %v)", rawID, fingerprint, version, satOS, satArch, payload.RegisterRequest.AvailableAgents), "component", "grpc")

			// Build available_agents JSON
			agentsJSON := "[]"
			if len(payload.RegisterRequest.AvailableAgents) > 0 {
				if b, err := json.Marshal(payload.RegisterRequest.AvailableAgents); err == nil {
					agentsJSON = string(b)
				}
			}

			// Resolve canonical UUID from DB — also store version/os/arch/available_agents
			var resolvedID string
			if s.dbPool != nil {
				err := s.dbPool.QueryRow(context.Background(),
					`UPDATE satellites SET status = 'active', fingerprint = $2, 
					 version = $3, os = $4, arch = $5, available_agents = $6::jsonb, updated_at = NOW() 
					 WHERE id::text = $1 OR name = $1 OR fingerprint = $2
					 RETURNING id::text`,
					rawID, fingerprint, version, satOS, satArch, agentsJSON,
				).Scan(&resolvedID)

				if err != nil {
					// Fallback: associate with most recent pending satellite
					err = s.dbPool.QueryRow(context.Background(),
						`UPDATE satellites SET status = 'active', fingerprint = $1,
						 version = $2, os = $3, arch = $4, available_agents = $5::jsonb, updated_at = NOW() 
						 WHERE id = (SELECT id FROM satellites WHERE status = 'pending' ORDER BY created_at DESC LIMIT 1)
						 RETURNING id::text`,
						fingerprint, version, satOS, satArch, agentsJSON,
					).Scan(&resolvedID)
				}

				if err != nil {
					// Auto-register: satellite not found in DB (e.g. after volume wipe).
					// Create a new record so the satellite self-heals without manual re-registration.
					// Use the hostname sent as rawID (or "satellite" fallback) for the name.
					satName := rawID
					if len(satName) == 36 && satName[8] == '-' {
						// rawID looks like a UUID, use a friendlier default name
						satName = fmt.Sprintf("satellite-%s", satOS)
					}
					err = s.dbPool.QueryRow(context.Background(),
						`INSERT INTO satellites (id, name, owner_id, fingerprint, status, version, os, arch, available_agents)
						 VALUES (gen_random_uuid(), $1, '00000000-0000-0000-0000-000000000000', $2, 'active', $3, $4, $5, $6::jsonb)
						 ON CONFLICT (fingerprint) WHERE fingerprint IS NOT NULL DO UPDATE
						   SET status = 'active', version = EXCLUDED.version, os = EXCLUDED.os,
						       arch = EXCLUDED.arch, available_agents = EXCLUDED.available_agents, updated_at = NOW()
						 RETURNING id::text`,
						satName, fingerprint, version, satOS, satArch, agentsJSON,
					).Scan(&resolvedID)
					if err == nil {
						slog.Info(fmt.Sprintf("SatelliteGateway: Auto-registered satellite %s (fingerprint: %s) → %s", satName, fingerprint, resolvedID), "component", "grpc")
					} else {
						slog.Error(fmt.Sprintf("SatelliteGateway: Auto-register INSERT failed for %s: %v", satName, err), "component", "grpc")
					}
				}

				if err == nil {
					satelliteID = resolvedID
					slog.Info(fmt.Sprintf("SatelliteGateway: Resolved canonical ID for satellite %s: %s", rawID, satelliteID), "component", "grpc")

					// Auto-populate OS/arch tags on satellite connect
					if satOS != "" || satArch != "" {
						autoTags := []string{}
						if satOS != "" {
							autoTags = append(autoTags, "os:"+satOS)
						}
						if satArch != "" {
							autoTags = append(autoTags, "arch:"+satArch)
						}
						if len(autoTags) > 0 {
							_, err := s.dbPool.Exec(context.Background(),
								`UPDATE satellites SET tags = (
								SELECT ARRAY(SELECT DISTINCT unnest(COALESCE(tags,'{}') || $2::text[]))
							) WHERE id = $1`,
								satelliteID, autoTags,
							)
							if err != nil {
								slog.Error(fmt.Sprintf("SatelliteGateway: Failed to update auto-tags for satellite %s: %v", satelliteID, err), "component", "grpc")
							} else {
								slog.Info(fmt.Sprintf("SatelliteGateway: Auto-populated tags %v for satellite %s", autoTags, satelliteID), "component", "grpc")
							}
						}
					}
				} else {
					slog.Info(fmt.Sprintf("SatelliteGateway: Could not resolve canonical ID for satellite %s: %v", rawID, err), "component", "grpc")
					satelliteID = rawID // Fallback to raw ID
				}
			} else {
				satelliteID = rawID
			}

			// Register satellite stream with the resolved canonical ID
			if s.streamRegistry != nil {
				s.streamRegistry.RegisterSatelliteStream(satelliteID, outboundCh)
				slog.Info(fmt.Sprintf("SatelliteGateway: Registered stream for satellite %s", satelliteID), "component", "grpc")
			}

			// Check if an update is available and notify the satellite
			if version != "" {
				latestVersion := os.Getenv("DAAO_LATEST_VERSION")
				if latestVersion != "" && version != latestVersion {
					nexusURL := os.Getenv("NEXUS_URL")
					if nexusURL == "" {
						nexusURL = "http://localhost:8081"
					}
					binaryName := fmt.Sprintf("daao-%s-%s", satOS, satArch)
					if satOS == "windows" {
						binaryName += ".exe"
					}
					updateMsg := &proto.NexusMessage{
						Payload: &proto.NexusMessage_UpdateAvailable{
							UpdateAvailable: &proto.UpdateAvailable{
								LatestVersion: latestVersion,
								DownloadUrl:   fmt.Sprintf("%s/releases/%s", nexusURL, binaryName),
								Force:         false,
							},
						},
					}
					select {
					case outboundCh <- updateMsg:
						slog.Info(fmt.Sprintf("SatelliteGateway: Sent UpdateAvailable (v%s -> v%s) to satellite %s", version, latestVersion, satelliteID), "component", "grpc")
					default:
						slog.Info(fmt.Sprintf("SatelliteGateway: Could not send UpdateAvailable — channel full"), "component", "grpc")
					}
				}
			}

			// Send session reconciliation so satellite can prune orphan sessions
			if s.dbPool != nil && satelliteID != "" {
				rows, err := s.dbPool.Query(context.Background(),
					`SELECT id::text FROM sessions
					 WHERE satellite_id::text = $1
					   AND state NOT IN ('TERMINATED')`,
					satelliteID,
				)
				if err == nil {
					var activeIDs []string
					for rows.Next() {
						var sid string
						if err := rows.Scan(&sid); err == nil {
							activeIDs = append(activeIDs, sid)
						}
					}
					rows.Close()
					reconcileMsg := &proto.NexusMessage{
						Payload: &proto.NexusMessage_SessionReconciliation{
							SessionReconciliation: &proto.SessionReconciliation{
								ActiveSessionIds: activeIDs,
							},
						},
					}
					select {
					case outboundCh <- reconcileMsg:
						slog.Info(fmt.Sprintf("SatelliteGateway: Sent SessionReconciliation to satellite %s (%d active sessions)", satelliteID, len(activeIDs)), "component", "grpc")
					default:
						slog.Info(fmt.Sprintf("SatelliteGateway: Could not send SessionReconciliation — channel full"), "component", "grpc")
					}
				} else {
					slog.Error(fmt.Sprintf("SatelliteGateway: Failed to query active sessions for reconciliation: %v", err), "component", "grpc")
				}
			}

		case *proto.SatelliteMessage_TerminalData:
			// Handle terminal data - write to per-session RingBuffer
			newSessionID := payload.TerminalData.SessionId
			data := payload.TerminalData.Data

			// Register session if not already tracked
			registerSession(newSessionID)

			if newSessionID != "" && len(data) > 0 {
				if s.ringBufferPool != nil {
					rb := s.ringBufferPool.GetOrCreateBuffer(newSessionID)
					if rb != nil {
						rb.Write(data)
						slog.Info(fmt.Sprintf("SatelliteGateway: Wrote %d bytes to ring buffer for session %s", len(data), newSessionID), "component", "grpc")
					}
				}
				// Tap for session recording — no-op if session is not being recorded
				if s.recordingPool != nil {
					s.recordingPool.WriteIfRecording(newSessionID, data)
				}
			}

		case *proto.SatelliteMessage_SessionStateUpdate:
			// Handle session state update - persist to SessionStore
			newSessionID := payload.SessionStateUpdate.SessionId
			state := payload.SessionStateUpdate.State
			timestamp := payload.SessionStateUpdate.Timestamp
			errorMsg := payload.SessionStateUpdate.ErrorMessage

			// Register session if not already tracked
			registerSession(newSessionID)

			if newSessionID != "" && s.sessionStore != nil {
				ctx := context.Background()

				// Convert proto state to session state
				sessionState := convertProtoToSessionState(state)

				// Try to get existing session and transition
				parsedUUID, err := parseSessionID(newSessionID)
				if err == nil {
					existingSession, err := s.sessionStore.GetSession(ctx, parsedUUID)
					if err == nil {
						// Perform state transition
						updatedSession, err := session.TransitionSession(existingSession, sessionState)
						if err == nil {
							// Update in database
							err = s.sessionStore.UpdateState(ctx, parsedUUID, updatedSession.State)
							if err == nil {
								slog.Info(fmt.Sprintf("SatelliteGateway: Session %s transitioned to state %s", newSessionID, state), "component", "grpc")

								// Write event log with SatelliteID
								var eventSatelliteID uuid.UUID
								if satelliteID != "" {
									eventSatelliteID, err = uuid.Parse(satelliteID)
									if err != nil {
										slog.Warn(fmt.Sprintf("SatelliteGateway: Warning: could not parse satelliteID %q: %v", satelliteID, err), "component", "grpc")
										eventSatelliteID = uuid.Nil
									}
								} else {
									slog.Warn(fmt.Sprintf("SatelliteGateway: Warning: satelliteID is empty for session %s", newSessionID), "component", "grpc")
									eventSatelliteID = uuid.Nil
								}

								err = s.sessionStore.WriteEventLog(ctx, &session.EventLog{
									SessionID:   parsedUUID,
									SatelliteID: &eventSatelliteID,
									EventType:   session.EventStateChange,
									Payload: map[string]interface{}{
										"state": state.String(),
									},
								})
								if err != nil {
									slog.Error(fmt.Sprintf("SatelliteGateway: Failed to write event log for session %s: %v", newSessionID, err), "component", "grpc")
								}
							}
						}
					} else {
						// Session doesn't exist, just log
						slog.Info(fmt.Sprintf("SatelliteGateway: Session %s state update: %s (session not found in store)", newSessionID, state), "component", "grpc")
					}
				}

				// Clean up RingBuffer and unregister stream when session is terminated
				if state == proto.SessionState_SESSION_STATE_TERMINATED {
					if s.ringBufferPool != nil {
						s.ringBufferPool.RemoveBuffer(newSessionID)
					}
					// Helper handles unregistering from streamRegistry
					sessionIDsMu.Lock()
					if s.streamRegistry != nil {
						s.streamRegistry.UnregisterStream(newSessionID)
					}
					delete(sessionIDs, newSessionID)
					sessionIDsMu.Unlock()
					slog.Info(fmt.Sprintf("SatelliteGateway: Removed ring buffer for terminated session %s", newSessionID), "component", "grpc")

					// Mark any still-running agent_run for this session as failed.
					// This covers the case where Pi exits without emitting an agent_end event.
					if pool, ok := s.dbPool.(*pgxpool.Pool); ok && newSessionID != "" {
						status := "failed"
						errText := "agent process terminated without output"
						if errorMsg != "" {
							errText = errorMsg
						}
						runIDStr, hasMapped := sessionRunMap[newSessionID]
						if !hasMapped {
							row := s.dbPool.QueryRow(context.Background(),
								`SELECT id::text FROM agent_runs WHERE session_id::text = $1 AND status = 'running' LIMIT 1`,
								newSessionID,
							)
							var rid string
							if row.Scan(&rid) == nil {
								runIDStr = rid
								hasMapped = true
							}
						}
						if hasMapped {
							if rid, err := uuid.Parse(runIDStr); err == nil {
								now := time.Now().UTC()
								updates := database.AgentRunUpdates{Status: &status, Error: &errText, EndedAt: &now}
								if _, err := database.UpdateAgentRun(context.Background(), pool, rid, updates); err != nil {
									slog.Error(fmt.Sprintf("SatelliteGateway: failed to mark agent_run %s as failed: %v", runIDStr, err), "component", "grpc")
								} else {
									slog.Error(fmt.Sprintf("SatelliteGateway: agent_run %s marked failed (session terminated)", runIDStr), "component", "grpc")
									if s.runEventHub != nil {
										s.runEventHub.Close(rid)
									}
								}
							}
						}
					}
				}

				_ = timestamp
				if errorMsg != "" {
					slog.Error(fmt.Sprintf("SatelliteGateway: Session %s terminated with error: %s", newSessionID, errorMsg), "component", "grpc")
				}
			}

		case *proto.SatelliteMessage_IpcEvent:
			// Handle IPC events — route daao.* methods to their handlers
			evt := payload.IpcEvent
			slog.Info(fmt.Sprintf("SatelliteGateway: IPC event for session %s: %s", evt.SessionId, evt.EventType), "component", "grpc")

			switch evt.EventType {
			case "daao.proposeAction":
				// HITL: AI agent is proposing a command for human approval.
				// Insert into the hitl_proposals table so the Cockpit can display it.
				if s.dbPool != nil && satelliteID != "" {
					var reqPayload struct {
						ProposalID    string `json:"proposal_id"`
						Command       string `json:"command"`
						Justification string `json:"justification"`
						RiskLevel     string `json:"risk_level"`
					}
					if err := json.Unmarshal(evt.Payload, &reqPayload); err != nil {
						slog.Error(fmt.Sprintf("HITL: failed to parse proposeAction payload: %v", err), "component", "grpc")
						break
					}
					if reqPayload.Command == "" {
						slog.Info(fmt.Sprintf("HITL: proposeAction missing command field"), "component", "grpc")
						break
					}
					if reqPayload.ProposalID == "" {
						reqPayload.ProposalID = uuid.New().String()
					}
					if reqPayload.RiskLevel == "" {
						reqPayload.RiskLevel = "medium"
					}

					proposalUUID := uuid.New()
					_, err := s.dbPool.Exec(context.Background(),
						`INSERT INTO hitl_proposals
						 (id, session_id, satellite_id, proposal_id, command, justification, risk_level, status, created_at, expires_at)
						 VALUES ($1, $2::uuid, $3::uuid, $4, $5, $6, $7::risk_level, 'pending', NOW(), NOW() + INTERVAL '15 minutes')`,
						proposalUUID, evt.SessionId, satelliteID,
						reqPayload.ProposalID, reqPayload.Command, reqPayload.Justification, reqPayload.RiskLevel,
					)
					if err != nil {
						slog.Error(fmt.Sprintf("HITL: failed to insert proposal: %v", err), "component", "grpc")
					} else {
						slog.Info(fmt.Sprintf("HITL: Proposal %s queued — command=%q risk=%s session=%s",
							proposalUUID, reqPayload.Command, reqPayload.RiskLevel, evt.SessionId), "component", "grpc")
					}
				}

			default:
				// Unknown IPC event type — log and ignore
				slog.Info(fmt.Sprintf("SatelliteGateway: Unknown IPC event type: %s", evt.EventType), "component", "grpc")
			}

		case *proto.SatelliteMessage_HeartbeatPing:
			// Heartbeat received — update satellite status silently.
			// The SatelliteLiveness checker logs when heartbeats are *missed*.
			// Keep satellite marked as active in the database
			if satelliteID != "" && s.dbPool != nil {
				_, err := s.dbPool.Exec(context.Background(),
					`UPDATE satellites SET status = 'active', updated_at = NOW() WHERE id::text = $1`,
					satelliteID,
				)
				if err != nil {
					slog.Error(fmt.Sprintf("SatelliteGateway: Failed to update satellite status on heartbeat: %v", err), "component", "grpc")
				}
			}

		case *proto.SatelliteMessage_TelemetryReport:
			// Handle telemetry report — store system metrics in database
			report := payload.TelemetryReport
			if satelliteID != "" && s.dbPool != nil {
				gpuJSON := "[]"
				if len(report.Gpus) > 0 {
					gpuList := make([]map[string]interface{}, len(report.Gpus))
					for i, g := range report.Gpus {
						gpuList[i] = map[string]interface{}{
							"index":               g.Index,
							"name":                g.Name,
							"utilization_percent": g.UtilizationPercent,
							"memory_used_bytes":   g.MemoryUsedBytes,
							"memory_total_bytes":  g.MemoryTotalBytes,
							"temperature_celsius": g.TemperatureCelsius,
						}
					}
					if b, err := json.Marshal(gpuList); err == nil {
						gpuJSON = string(b)
					}
				}

				_, err := s.dbPool.Exec(context.Background(),
					`INSERT INTO satellite_telemetry
					 (satellite_id, cpu_percent, memory_percent, memory_used_bytes, memory_total_bytes,
					  disk_percent, disk_used_bytes, disk_total_bytes, gpu_data, active_sessions)
					 VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10)`,
					satelliteID,
					report.CpuPercent, report.MemoryPercent,
					report.MemoryUsedBytes, report.MemoryTotalBytes,
					report.DiskPercent, report.DiskUsedBytes, report.DiskTotalBytes,
					gpuJSON, report.ActiveSessions,
				)
				if err != nil {
					slog.Error(fmt.Sprintf("SatelliteGateway: Failed to store telemetry for satellite %s: %v", satelliteID, err), "component", "grpc")
				}

				// Update available_agents on satellite if provided
				if len(report.AvailableAgents) > 0 {
					agentsJSON := "[]"
					if b, err := json.Marshal(report.AvailableAgents); err == nil {
						agentsJSON = string(b)
					}
					_, err = s.dbPool.Exec(context.Background(),
						`UPDATE satellites SET available_agents = $2::jsonb WHERE id::text = $1`,
						satelliteID, agentsJSON,
					)
					if err != nil {
						slog.Error(fmt.Sprintf("SatelliteGateway: Failed to update available_agents for satellite %s: %v", satelliteID, err), "component", "grpc")
					}
				}
			}

			// Forward telemetry to Forge Scheduler for threshold trigger evaluation
			if s.telemetryHandler != nil && satelliteID != "" {
				metrics := map[string]float64{
					"cpu_usage":    report.CpuPercent,
					"memory_usage": report.MemoryPercent,
					"disk_usage":   report.DiskPercent,
				}
				s.telemetryHandler(context.Background(), satelliteID, metrics)
			}

		case *proto.SatelliteMessage_BufferReplay:
			// Handle buffer replay — satellite reconnected and is replaying its
			// local ring buffer so Nexus can rehydrate the in-memory pool.
			sid := payload.BufferReplay.SessionId
			data := payload.BufferReplay.Data

			// Register session if not already tracked
			registerSession(sid)

			if sid != "" && len(data) > 0 && s.ringBufferPool != nil {
				rb := s.ringBufferPool.GetOrCreateBuffer(sid)
				if rb != nil {
					rb.Write(data)
					slog.Info(fmt.Sprintf("SatelliteGateway: Replayed %d bytes into ring buffer for session %s", len(data), sid), "component", "grpc")
				}
			}

		case *proto.SatelliteMessage_ContextFileUpdate:
			// Satellite is reporting a local context file change — upsert into DB.
			file := payload.ContextFileUpdate.File
			if file == nil || s.dbPool == nil {
				break
			}
			_, err := s.dbPool.Exec(stream.Context(),
				`INSERT INTO satellite_context_files
				    (satellite_id, file_path, content, last_modified_by)
				 VALUES ($1::uuid, $2, $3, $4)
				 ON CONFLICT (satellite_id, file_path) DO UPDATE
				    SET content          = EXCLUDED.content,
				        version          = satellite_context_files.version + 1,
				        last_modified_by = EXCLUDED.last_modified_by,
				        updated_at       = NOW()`,
				file.SatelliteId, file.FilePath, string(file.Content), file.ModifiedBy,
			)
			if err != nil {
				slog.Error(fmt.Sprintf("SatelliteGateway: failed to upsert context file %s for satellite %s: %v",
					file.FilePath, file.SatelliteId, err), "component", "grpc")
			} else {
				slog.Info(fmt.Sprintf("SatelliteGateway: upserted context file %s for satellite %s", file.FilePath, file.SatelliteId), "component", "grpc")
			}

		case *proto.SatelliteMessage_AgentEvent:
			ae := payload.AgentEvent
			if ae == nil {
				break
			}
			slog.Info(fmt.Sprintf("SatelliteGateway: AgentEvent session=%s type=%s", ae.SessionId, ae.EventType), "component", "grpc")

			agentSeq[ae.SessionId]++
			seq := agentSeq[ae.SessionId]

			runIDStr, hasRun := sessionRunMap[ae.SessionId]
			if !hasRun {
				// Try to look up run_id synchronously from DB
				if s.dbPool != nil && ae.SessionId != "" {
					rows, err := s.dbPool.Query(context.Background(),
						`SELECT id::text FROM agent_runs WHERE session_id::text = $1 AND status = 'running' ORDER BY started_at DESC LIMIT 1`,
						ae.SessionId,
					)
					if err == nil {
						if rows.Next() {
							var rid string
							if rows.Scan(&rid) == nil {
								sessionRunMap[ae.SessionId] = rid
								runIDStr = rid
								hasRun = true
							}
						}
						rows.Close()
					}
				}
			}
			if !hasRun {
				slog.Info(fmt.Sprintf("SatelliteGateway: AgentEvent for session %s has no run mapping, skipping", ae.SessionId), "component", "grpc")
				break
			}
			runID, err := uuid.Parse(runIDStr)
			if err != nil {
				break
			}

			event := agentstream.AgentStreamEvent{
				ID:        uuid.New().String(),
				RunID:     runID.String(),
				EventType: ae.EventType,
				Payload:   json.RawMessage(ae.Payload),
				Sequence:  seq,
				CreatedAt: time.Now(),
			}

			// Fast path: publish to hub for live SSE
			if s.runEventHub != nil {
				s.runEventHub.Publish(runID, event)
			}

			// Async path: persist to DB
			// message_update, tool_execution_* are batched (high frequency).
			// agent_start and agent_end are written immediately (must be queryable right away).
			if s.dbPool != nil {
				isBatchable := ae.EventType == "message_update" ||
					ae.EventType == "tool_execution_start" ||
					ae.EventType == "tool_execution_end"

				if isBatchable && s.batchWriter != nil {
					s.batchWriter.Append(runID, ae.EventType, []byte(ae.Payload), seq)
				} else {
					go func(e agentstream.AgentStreamEvent, rid uuid.UUID, payload []byte) {
						pool, ok := s.dbPool.(*pgxpool.Pool)
						if !ok {
							return
						}
						if _, err := database.InsertAgentRunEvent(context.Background(), pool, rid, e.EventType, payload, e.Sequence); err != nil {
							slog.Error(fmt.Sprintf("SatelliteGateway: failed to persist AgentRunEvent: %v", err), "component", "grpc")
						}
					}(event, runID, []byte(ae.Payload))
				}
			}

			// On agent_end: update run summary
			if ae.EventType == "agent_end" && s.dbPool != nil {
				if s.batchWriter != nil {
					s.batchWriter.Close(runID) // flush remaining events before updating run summary
				}
				go func(rid uuid.UUID, rawPayload []byte) {
					pool, ok := s.dbPool.(*pgxpool.Pool)
					if !ok {
						return
					}
					var p map[string]interface{}
					status := "completed"
					if err := json.Unmarshal(rawPayload, &p); err == nil {
						if errMsg, _ := p["error"].(string); errMsg != "" {
							status = "failed"
						}
					}
					now := time.Now()
					updates := database.AgentRunUpdates{Status: &status, EndedAt: &now}
					if err := json.Unmarshal(rawPayload, &p); err == nil {
						if r, _ := p["result"].(string); r != "" {
							updates.Result = &r
						}
						if t, _ := p["total_tokens"].(float64); t > 0 {
							ti := int(t)
							updates.TotalTokens = &ti
						}
					}
					if _, err := database.UpdateAgentRun(context.Background(), pool, rid, updates); err != nil {
						slog.Error(fmt.Sprintf("SatelliteGateway: failed to update agent_run on agent_end: %v", err), "component", "grpc")
					}
					if s.runEventHub != nil {
						s.runEventHub.Close(rid)
					}
				}(runID, ae.Payload)
			}

		default:
			slog.Info(fmt.Sprintf("SatelliteGateway: Unknown message type received"), "component", "grpc")
		}
	}
}

// convertProtoToSessionState converts proto session state to session package state
func convertProtoToSessionState(protoState proto.SessionState) session.SessionState {
	switch protoState {
	case proto.SessionState_SESSION_STATE_STARTING:
		return session.StateProvisioning
	case proto.SessionState_SESSION_STATE_RUNNING:
		return session.StateRunning
	case proto.SessionState_SESSION_STATE_SUSPENDED:
		return session.StateSuspended
	case proto.SessionState_SESSION_STATE_TERMINATED:
		return session.StateTerminated
	case proto.SessionState_SESSION_STATE_DETACHED:
		return session.StateDetached
	case proto.SessionState_SESSION_STATE_RE_ATTACHING:
		return session.StateReAttaching
	default:
		return session.SessionState(protoState.String())
	}
}

// parseSessionID parses a session ID string to UUID
func parseSessionID(sessionID string) (uuid.UUID, error) {
	return uuid.Parse(sessionID)
}

// SendTerminalInput sends terminal input to a session
func (s *SatelliteGatewayServerImpl) SendTerminalInput(sessionID string, data []byte, seqNum int64) *proto.NexusMessage {
	return &proto.NexusMessage{
		Payload: &proto.NexusMessage_TerminalInput{
			TerminalInput: &proto.TerminalInput{
				SessionId:      sessionID,
				Data:           data,
				SequenceNumber: seqNum,
			},
		},
	}
}

// SendResizeCommand sends a resize command to a session
func (s *SatelliteGatewayServerImpl) SendResizeCommand(sessionID string, width, height, pixelWidth, pixelHeight int32) *proto.NexusMessage {
	return &proto.NexusMessage{
		Payload: &proto.NexusMessage_ResizeCommand{
			ResizeCommand: &proto.ResizeCommand{
				SessionId:   sessionID,
				Width:       width,
				Height:      height,
				PixelWidth:  pixelWidth,
				PixelHeight: pixelHeight,
			},
		},
	}
}

// SendSuspendCommand sends a suspend command to a session
func (s *SatelliteGatewayServerImpl) SendSuspendCommand(sessionID string) *proto.NexusMessage {
	return &proto.NexusMessage{
		Payload: &proto.NexusMessage_SuspendCommand{
			SuspendCommand: &proto.SuspendCommand{
				SessionId: sessionID,
			},
		},
	}
}

// SendResumeCommand sends a resume command to a session
func (s *SatelliteGatewayServerImpl) SendResumeCommand(sessionID string) *proto.NexusMessage {
	return &proto.NexusMessage{
		Payload: &proto.NexusMessage_ResumeCommand{
			ResumeCommand: &proto.ResumeCommand{
				SessionId: sessionID,
			},
		},
	}
}

// SendKillCommand sends a kill command to a session
func (s *SatelliteGatewayServerImpl) SendKillCommand(sessionID string, exitCode int32) *proto.NexusMessage {
	return &proto.NexusMessage{
		Payload: &proto.NexusMessage_KillCommand{
			KillCommand: &proto.KillCommand{
				SessionId: sessionID,
				ExitCode:  exitCode,
			},
		},
	}
}
