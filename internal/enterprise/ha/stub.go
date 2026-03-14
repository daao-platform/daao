// Package ha provides enterprise High Availability implementations.
//
// This is a stub implementation for the public repository. Enterprise
// customers receive the full implementation with their license.
//
// In community mode, all factory functions return in-memory
// implementations with no external dependencies (no NATS, S3, or Redis).
package ha

import (
	"context"
	"log/slog"
	"sync"

	"github.com/daao/nexus/internal/agentstream"
	"github.com/daao/nexus/internal/auth"
	"github.com/daao/nexus/internal/enterprise/forge"
	"github.com/daao/nexus/internal/license"
	"github.com/daao/nexus/internal/recording"
	"github.com/daao/nexus/internal/stream"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

// NewStreamRegistry returns an in-memory stream registry (community mode).
// The NATS connection is always nil in the public build.
func NewStreamRegistry(_ *license.Manager) (stream.StreamRegistryInterface, *nats.Conn) {
	slog.Info("ha: using in-memory stream registry (community mode)", "component", "ha")
	return stream.NewStreamRegistry(), nil
}

// NewRunEventHub returns an in-memory run event hub (community mode).
func NewRunEventHub(_ *license.Manager, _ *nats.Conn) agentstream.RunEventHubInterface {
	slog.Info("ha: using in-memory run event hub (community mode)", "component", "ha")
	return agentstream.NewRunEventHub()
}

// NewRecordingPool returns a local filesystem recording pool (community mode).
func NewRecordingPool(_ *license.Manager, dataDir string) (recording.RecordingPoolInterface, error) {
	slog.Info("ha: using local recording pool", "data_dir", dataDir, "component", "ha")
	return recording.NewRecordingPool(dataDir), nil
}

// NewRateLimiter returns an in-memory rate limiter (community mode).
func NewRateLimiter(_ *license.Manager) auth.RateLimiterInterface {
	slog.Info("ha: using in-memory rate limiter (community mode)", "component", "ha")
	return auth.NewRateLimiter()
}

// LeaderSchedulerGuard wraps a *forge.Scheduler with leader election.
// In community mode, this always returns nil — the caller creates the
// scheduler directly without HA coordination.
type LeaderSchedulerGuard struct {
	mu    sync.Mutex
	sched *forge.Scheduler
}

// NewLeaderSchedulerGuard returns nil in community mode.
// The caller should create a scheduler directly when nil is returned.
func NewLeaderSchedulerGuard(
	_ *license.Manager,
	_ *pgxpool.Pool,
	_ func() (*forge.Scheduler, error),
	_ func(*forge.Scheduler),
) *LeaderSchedulerGuard {
	return nil // community: caller creates scheduler directly
}

// Start is a no-op for the community stub.
func (g *LeaderSchedulerGuard) Start(_ context.Context) {}

// Stop is a no-op for the community stub.
func (g *LeaderSchedulerGuard) Stop() {}

// Scheduler returns nil (community mode — no HA scheduler).
func (g *LeaderSchedulerGuard) Scheduler() *forge.Scheduler {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.sched
}

// IsLeader always returns false in community mode.
func (g *LeaderSchedulerGuard) IsLeader() bool {
	return false
}
