// Package lifecycle provides session lifecycle management including
// the Dead Man's Switch (DMS) for automatic process suspension.
package lifecycle

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/daao/nexus/internal/session"
	"github.com/daao/nexus/pkg/proc"
	"github.com/google/uuid"
)

// DefaultTTL is the default idle timeout in minutes
const DefaultTTL = 60

// DefaultCheckInterval is the default interval for checking idle time
const DefaultCheckInterval = 10 * time.Second

// EventLogger defines the interface for logging events to the event_logs table
type EventLogger interface {
	// LogEvent logs an event to the event_logs table
	LogEvent(ctx context.Context, event *session.EventLog) error
}

// DMSConfig holds the configuration for the Dead Man's Switch
type DMSConfig struct {
	// TTL is the idle timeout in minutes after which the process is suspended
	// Default: 60 minutes
	TTL int

	// CheckInterval is how often to check if the session has been idle
	// Default: 10 seconds
	CheckInterval time.Duration

	// OnSuspend is called when the process is suspended
	OnSuspend func(sessionID string) error

	// OnResume is called when the process is resumed
	OnResume func(sessionID string) error

	// GetSessionStateFunc returns the current state of a session
	// If not provided, DMS will not automatically disable when RUNNING
	GetSessionStateFunc func(sessionID string) (session.SessionState, error)

	// EventLogger is used to log DMS events to the event_logs table
	// If provided, DMS_TRIGGERED and DMS_RESUMED events will be persisted
	EventLogger EventLogger

	// SatelliteID is the satellite ID for event logging
	SatelliteID *uuid.UUID

	// UserID is the user ID for event logging
	UserID *uuid.UUID
}

// DeadManSwitch monitors session idle time and automatically suspends
// the process when the idle TTL is exceeded. It respects the session state
// and only suspends when the session is DETACHED.
type DeadManSwitch struct {
	config         DMSConfig
	sessionID      string
	process        proc.SuspendableProcess
	lastActivity   time.Time
	mu             sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	timer          *time.Timer
	suspended      bool
	suspendCh      chan struct{}
	resumeCh       chan struct{}
}

// NewDeadManSwitch creates a new Dead Man's Switch instance
func NewDeadManSwitch(sessionID string, process proc.SuspendableProcess, config DMSConfig) *DeadManSwitch {
	// Apply defaults
	if config.TTL <= 0 {
		config.TTL = DefaultTTL
	}
	if config.CheckInterval <= 0 {
		config.CheckInterval = DefaultCheckInterval
	}

	ctx, cancel := context.WithCancel(context.Background())

	dms := &DeadManSwitch{
		config:       config,
		sessionID:    sessionID,
		process:      process,
		lastActivity: time.Now(),
		ctx:          ctx,
		cancel:       cancel,
		suspendCh:    make(chan struct{}, 1),
		resumeCh:     make(chan struct{}, 1),
	}

	return dms
}

// Start starts the Dead Man's Switch monitoring loop
func (d *DeadManSwitch) Start() {
	d.wg.Add(1)
	go d.monitorLoop()
}

// Stop stops the Dead Man's Switch monitoring
func (d *DeadManSwitch) Stop() {
	d.cancel()
	d.wg.Wait()
}

// RecordActivity records user activity and resets the idle timer
func (d *DeadManSwitch) RecordActivity() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastActivity = time.Now()

	// If suspended, signal resume
	if d.suspended {
		select {
		case d.resumeCh <- struct{}{}:
		default:
		}
	}
}

// GetLastActivity returns the last activity timestamp
func (d *DeadManSwitch) GetLastActivity() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.lastActivity
}

// IsSuspended returns whether the process is currently suspended
func (d *DeadManSwitch) IsSuspended() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.suspended
}

// monitorLoop is the main monitoring loop that checks idle time
func (d *DeadManSwitch) monitorLoop() {
	defer d.wg.Done()

	// Create a ticker for periodic checks
	ticker := time.NewTicker(d.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-d.suspendCh:
			d.doSuspend()
		case <-d.resumeCh:
			d.doResume()
		case <-ticker.C:
			d.checkAndSuspend()
		}
	}
}

// checkAndSuspend checks if the idle time has exceeded TTL and suspends if appropriate
func (d *DeadManSwitch) checkAndSuspend() {
	d.mu.RLock()
	idleTime := time.Since(d.lastActivity)
	ttlDuration := time.Duration(d.config.TTL) * time.Minute
	isSuspended := d.suspended
	d.mu.RUnlock()

	// Don't suspend if already suspended or DMS is disabled due to RUNNING state
	if isSuspended {
		return
	}

	// Check if DMS should be disabled (session is RUNNING)
	if d.config.GetSessionStateFunc != nil {
		state, err := d.config.GetSessionStateFunc(d.sessionID)
		if err == nil && state == session.StateRunning {
			// DMS is disabled when session is RUNNING
			return
		}
	}

	// Check if idle time exceeds TTL
	if idleTime >= ttlDuration {
		// Signal suspend
		select {
		case d.suspendCh <- struct{}{}:
		default:
		}
	}
}

// doSuspend suspends the process and calls the OnSuspend callback
func (d *DeadManSwitch) doSuspend() {
	d.mu.Lock()
	if d.suspended {
		d.mu.Unlock()
		return
	}

	// Double-check session state before suspending
	if d.config.GetSessionStateFunc != nil {
		state, err := d.config.GetSessionStateFunc(d.sessionID)
		if err == nil && state == session.StateRunning {
			d.mu.Unlock()
			return
		}
	}

	d.suspended = true
	d.mu.Unlock()

	// Suspend the process
	if err := d.process.Suspend(); err != nil {
		// Log error but don't fail
		fmt.Printf("Failed to suspend process: %v\n", err)
		d.mu.Lock()
		d.suspended = false
		d.mu.Unlock()
		return
	}

	// Log DMS_TRIGGERED event to event_logs
	if d.config.EventLogger != nil {
		sessionID, err := uuid.Parse(d.sessionID)
		if err == nil {
			event := &session.EventLog{
				SessionID:   sessionID,
				SatelliteID: d.config.SatelliteID,
				UserID:      d.config.UserID,
				EventType:   session.EventDMSTriggered,
				Payload: map[string]interface{}{
					"idle_ttl_minutes": d.config.TTL,
					"pid":              d.process.PID(),
				},
				CreatedAt: time.Now().UTC(),
			}
			if err := d.config.EventLogger.LogEvent(context.Background(), event); err != nil {
				fmt.Printf("Failed to log DMS_TRIGGERED event: %v\n", err)
			}
		}
	}

	// Call OnSuspend callback
	if d.config.OnSuspend != nil {
		if err := d.config.OnSuspend(d.sessionID); err != nil {
			fmt.Printf("OnSuspend callback failed: %v\n", err)
		}
	}
}

// doResume resumes the process and calls the OnResume callback
func (d *DeadManSwitch) doResume() {
	d.mu.Lock()
	if !d.suspended {
		d.mu.Unlock()
		return
	}

	d.suspended = false
	d.lastActivity = time.Now() // Reset activity on resume
	d.mu.Unlock()

	// Resume the process
	if err := d.process.Resume(); err != nil {
		fmt.Printf("Failed to resume process: %v\n", err)
		return
	}

	// Log DMS_RESUMED event to event_logs
	if d.config.EventLogger != nil {
		sessionID, err := uuid.Parse(d.sessionID)
		if err == nil {
			event := &session.EventLog{
				SessionID:   sessionID,
				SatelliteID: d.config.SatelliteID,
				UserID:      d.config.UserID,
				EventType:   session.EventDMSResumed,
				Payload: map[string]interface{}{
					"pid": d.process.PID(),
				},
				CreatedAt: time.Now().UTC(),
			}
			if err := d.config.EventLogger.LogEvent(context.Background(), event); err != nil {
				fmt.Printf("Failed to log DMS_RESUMED event: %v\n", err)
			}
		}
	}

	// Call OnResume callback
	if d.config.OnResume != nil {
		if err := d.config.OnResume(d.sessionID); err != nil {
			fmt.Printf("OnResume callback failed: %v\n", err)
		}
	}
}

// TimeUntilSuspend returns the time remaining until the process will be suspended
// Returns 0 if already suspended or if DMS is disabled
func (d *DeadManSwitch) TimeUntilSuspend() time.Duration {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.suspended {
		return 0
	}

	ttlDuration := time.Duration(d.config.TTL) * time.Minute
	idleTime := time.Since(d.lastActivity)

	if idleTime >= ttlDuration {
		return 0
	}

	return ttlDuration - idleTime
}
