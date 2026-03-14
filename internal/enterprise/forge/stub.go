// Package forge provides enterprise Agent Forge analytics and scheduling.
//
// This is a stub implementation for the public repository. Enterprise
// customers receive the full implementation with their license.
package forge

import (
	"context"
	"errors"
	"time"

	"github.com/daao/nexus/internal/license"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"
)

// ErrEnterpriseRequired is returned when Forge features are used without license.
var ErrEnterpriseRequired = errors.New("enterprise license required: Agent Forge — upgrade at https://daao.io")

// ---------------------------------------------------------------------------
// Domain types (needed for compilation — no business logic)
// ---------------------------------------------------------------------------

// TimeInterval represents the bucketing interval for time series data.
type TimeInterval string

const (
	IntervalHourly TimeInterval = "hourly"
	IntervalDaily  TimeInterval = "daily"
)

// AggregateStats represents overall agent run statistics.
type AggregateStats struct {
	TotalRuns     int     `json:"total_runs"`
	SuccessRate   float64 `json:"success_rate"`
	AvgDurationMs int64   `json:"avg_duration_ms"`
	TotalTokens   int     `json:"total_tokens"`
	TotalCost     float64 `json:"total_cost"`
}

// AgentStats represents statistics for a specific agent.
type AgentStats struct {
	AgentID       uuid.UUID `json:"agent_id"`
	TotalRuns     int       `json:"total_runs"`
	SuccessRate   float64   `json:"success_rate"`
	AvgDurationMs int64     `json:"avg_duration_ms"`
	TotalTokens   int       `json:"total_tokens"`
	TotalCost     float64   `json:"total_cost"`
}

// SatelliteStats represents statistics for a specific satellite.
type SatelliteStats struct {
	SatelliteID      uuid.UUID    `json:"satellite_id"`
	TotalRuns        int          `json:"total_runs"`
	SuccessRate      float64      `json:"success_rate"`
	AvgDurationMs    int64        `json:"avg_duration_ms"`
	TotalTokens      int          `json:"total_tokens"`
	TotalCost        float64      `json:"total_cost"`
	MostActiveAgents []AgentStats `json:"most_active_agents"`
}

// TimeSeriesPoint represents a single data point in a time series.
type TimeSeriesPoint struct {
	Timestamp time.Time `json:"timestamp"`
	TotalRuns int       `json:"total_runs"`
	Successes int       `json:"successes"`
	Failures  int       `json:"failures"`
	Tokens    int       `json:"tokens"`
	Cost      float64   `json:"cost"`
}

// TriggerCondition defines the condition for event-triggered agents.
type TriggerCondition struct {
	Metric    string  `json:"metric"`
	Threshold float64 `json:"threshold"`
	Operator  string  `json:"operator"`
}

// TriggerConfig holds the configuration for an event trigger.
type TriggerConfig struct {
	AgentID     uuid.UUID        `json:"agent_id"`
	Condition   TriggerCondition `json:"condition"`
	SatelliteID uuid.UUID        `json:"satellite_id"`
	Cooldown    time.Duration    `json:"cooldown"`
	MaxRetries  int              `json:"max_retries"`
}

// ScheduleConfig holds the configuration for a scheduled agent.
type ScheduleConfig struct {
	AgentID     uuid.UUID `json:"agent_id"`
	CronExpr    string    `json:"cron_expr"`
	SatelliteID uuid.UUID `json:"satellite_id"`
	MaxRetries  int       `json:"max_retries"`
}

// TelemetryReport represents a telemetry report from a satellite.
type TelemetryReport struct {
	SatelliteID uuid.UUID          `json:"satellite_id"`
	Timestamp   time.Time          `json:"timestamp"`
	Metrics     map[string]float64 `json:"metrics"`
}

// AgentRunner is an interface for triggering agent runs.
type AgentRunner interface {
	RunAgent(ctx context.Context, agentID, satelliteID uuid.UUID, triggerSource string) error
}

// FailureHandler handles failures with notification, retry, and escalation.
type FailureHandler interface {
	Notify(ctx context.Context, title, body string) error
	Escalate(ctx context.Context, agentID, satelliteID uuid.UUID, reason string) error
}

// PipelineRunner is an interface for running pipelines.
type PipelineRunner interface {
	RunPipeline(ctx context.Context, pipelineID, satelliteID uuid.UUID, triggerSource string) error
}

// ---------------------------------------------------------------------------
// Analytics (stub)
// ---------------------------------------------------------------------------

// Analytics provides methods for querying agent run analytics.
type Analytics struct {
	pool *pgxpool.Pool
	lm   *license.Manager
}

// NewAnalytics creates a new Analytics instance.
func NewAnalytics(_ *pgxpool.Pool, _ *license.Manager) (*Analytics, error) {
	return nil, ErrEnterpriseRequired
}

func (a *Analytics) GetAggregateStats(_ context.Context) (*AggregateStats, error) {
	return nil, ErrEnterpriseRequired
}
func (a *Analytics) GetAgentStats(_ context.Context, _ uuid.UUID) (*AgentStats, error) {
	return nil, ErrEnterpriseRequired
}
func (a *Analytics) GetSatelliteStats(_ context.Context, _ uuid.UUID) (*SatelliteStats, error) {
	return nil, ErrEnterpriseRequired
}
func (a *Analytics) GetTimeSeries(_ context.Context, _ TimeInterval, _, _ time.Time) ([]TimeSeriesPoint, error) {
	return nil, ErrEnterpriseRequired
}

// ---------------------------------------------------------------------------
// Scheduler (stub)
// ---------------------------------------------------------------------------

// Scheduler manages scheduled and event-triggered agent runs.
type Scheduler struct {
	licenseMgr *license.Manager
}

// NewScheduler creates a new Enterprise Forge Scheduler.
func NewScheduler(_ *license.Manager, _ AgentRunner, _ FailureHandler) (*Scheduler, error) {
	return nil, ErrEnterpriseRequired
}

func (s *Scheduler) RegisterSchedule(_ uuid.UUID, _ string, _ uuid.UUID) error {
	return ErrEnterpriseRequired
}
func (s *Scheduler) RegisterTrigger(_ uuid.UUID, _ TriggerCondition, _ uuid.UUID, _ time.Duration) error {
	return ErrEnterpriseRequired
}
func (s *Scheduler) OnTelemetry(_ context.Context, _ *TelemetryReport) {}
func (s *Scheduler) RemoveSchedule(_ uuid.UUID) error                  { return ErrEnterpriseRequired }
func (s *Scheduler) RemoveTrigger(_ uuid.UUID) error                   { return ErrEnterpriseRequired }
func (s *Scheduler) ListSchedules() map[uuid.UUID]*ScheduleConfig      { return nil }
func (s *Scheduler) ListTriggers() map[uuid.UUID]*TriggerConfig        { return nil }
func (s *Scheduler) LoadFromDB(_ context.Context, _ *pgxpool.Pool) error {
	return ErrEnterpriseRequired
}
func (s *Scheduler) Stop() {}

// GetCronEntry returns the cron entry for a scheduled agent (stub — returns zero value).
func (s *Scheduler) GetCronEntry(_ uuid.UUID) cron.Entry { return cron.Entry{} }

// GetLastTriggered returns the last time a trigger fired (stub — returns zero time).
func (s *Scheduler) GetLastTriggered(_ uuid.UUID) time.Time { return time.Time{} }

// SetPipelineRunner sets the pipeline runner for the scheduler.
func (s *Scheduler) SetPipelineRunner(_ PipelineRunner) {}

// NewDefaultFailureHandler creates a no-op failure handler for community builds.
func NewDefaultFailureHandler(_ interface{}) FailureHandler { return nil }

// PipelineExecutor manages pipeline execution (stub for community builds).
type PipelineExecutor struct{}

// NewPipelineExecutor creates a new PipelineExecutor (stub for community builds).
func NewPipelineExecutor(_ *pgxpool.Pool, _ AgentRunner, _ interface{}, _ FailureHandler, _ *license.Manager) (*PipelineExecutor, error) {
	return nil, ErrEnterpriseRequired
}

func (pe *PipelineExecutor) RunPipeline(_ context.Context, _, _ uuid.UUID, _ string) error {
	return ErrEnterpriseRequired
}
