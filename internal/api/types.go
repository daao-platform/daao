// Package api provides HTTP handlers for the Nexus API Gateway.
package api

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ============================================================================
// Request Types
// ============================================================================

// CreateSessionRequest represents a request to create a new session
type CreateSessionRequest struct {
	Name        string   `json:"name"`
	SatelliteID string   `json:"satellite_id"`
	AgentBinary string   `json:"agent_binary"`
	AgentArgs   []string `json:"agent_args"`
	WorkingDir  string   `json:"working_dir,omitempty"`
	Cols        int16    `json:"cols"`
	Rows        int16    `json:"rows"`
}

// CreateSatelliteRequest represents a request to register a new satellite
type CreateSatelliteRequest struct {
	Name     string `json:"name"`
	Region   string `json:"region"`
	Endpoint string `json:"endpoint"`
}

// ============================================================================
// Response Types
// ============================================================================

// SessionResponse represents a session in API responses
type SessionResponse struct {
	ID             uuid.UUID  `json:"id"`
	SatelliteID    uuid.UUID  `json:"satellite_id"`
	UserID         uuid.UUID  `json:"user_id"`
	Name           string     `json:"name"`
	AgentBinary    string     `json:"agent_binary"`
	AgentArgs      string     `json:"agent_args"`
	State          string     `json:"state"`
	OSPID          int        `json:"os_pid"`
	PTSName        string     `json:"pts_name"`
	Cols           int        `json:"cols"`
	Rows           int        `json:"rows"`
	LastActivityAt time.Time  `json:"last_activity_at"`
	StartedAt      time.Time  `json:"started_at"`
	DetachedAt     *time.Time `json:"detached_at"`
	SuspendedAt    *time.Time `json:"suspended_at"`
	TerminatedAt   *time.Time `json:"terminated_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

// SatelliteResponse represents a satellite in API responses
type SatelliteResponse struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	Region          string    `json:"region"`
	Endpoint        string    `json:"endpoint"`
	Status          string    `json:"status"`
	Os              string    `json:"os,omitempty"`
	Arch            string    `json:"arch,omitempty"`
	Version         string    `json:"version,omitempty"`
	Tags            []string  `json:"tags"`
	AvailableAgents []string  `json:"available_agents"`
	CreatedAt       time.Time `json:"created_at"`
}

// Satellite represents a satellite in the internal API
type Satellite struct {
	ID        uuid.UUID
	Name      string
	Region    string
	Endpoint  string
	Status    string
	CreatedAt time.Time
}

// ConfigResponse represents the DMS/heartbeat configuration in API responses
type ConfigResponse struct {
	DMSTTLMinutes            int `json:"dms_ttl_minutes"`
	HeartbeatIntervalSeconds int `json:"heartbeat_interval_seconds"`
}

// HeartbeatRequest represents a satellite heartbeat request
type HeartbeatRequest struct {
	Fingerprint string `json:"fingerprint"`
	SatelliteID string `json:"satellite_id"`
}

// RenameSessionRequest is the body for PATCH /api/v1/sessions/{id}/name
type RenameSessionRequest struct {
	Name string `json:"name"`
}

// HeartbeatResponse represents a satellite heartbeat response
type HeartbeatResponse struct {
	OK        bool   `json:"ok"`
	Timestamp int64  `json:"timestamp"`
	Message   string `json:"message,omitempty"`
}

// ErrorResponse represents an error in API responses
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

// HealthCheck represents the status of a single health check component
type HealthCheck struct {
	Status   string `json:"status"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Listener string `json:"listener,omitempty"`
}

// HealthResponse represents a health check response
type HealthResponse struct {
	Status         string                 `json:"status"`
	Version        string                 `json:"version"`
	UptimeSeconds  int64                  `json:"uptime_seconds"`
	Checks         map[string]interface{} `json:"checks"`
}

// HealthDeps holds the dependencies required for health checks
type HealthDeps struct {
	Pool             *pgxpool.Pool
	GRPCServer       interface{ GetAddr() string }
	StartTime        time.Time
	Version          string
	GetSatelliteCount func() int
	GetSessionCount  func() int
}

// ListResponse represents a list response
type ListResponse struct {
	Items interface{} `json:"items"`
	Count int         `json:"count"`
	Total int         `json:"total"`
}

// TelemetryResponse represents satellite telemetry data
type TelemetryResponse struct {
	SatelliteID    string          `json:"satellite_id"`
	CPUPercent     float64         `json:"cpu_percent"`
	MemoryPercent  float64         `json:"memory_percent"`
	MemoryUsed     int64           `json:"memory_used_bytes"`
	MemoryTotal    int64           `json:"memory_total_bytes"`
	DiskPercent    float64         `json:"disk_percent"`
	DiskUsed       int64           `json:"disk_used_bytes"`
	DiskTotal      int64           `json:"disk_total_bytes"`
	GPUs           json.RawMessage `json:"gpus"`
	ActiveSessions int             `json:"active_sessions"`
	CollectedAt    time.Time       `json:"collected_at"`
}
