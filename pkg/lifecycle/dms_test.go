package lifecycle

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/daao/nexus/internal/session"
	"github.com/daao/nexus/pkg/proc"
)

// MockProcess is a mock implementation of SuspendableProcess for testing
type MockProcess struct {
	pid       int
	suspended atomic.Bool
	suspendCh chan struct{}
	resumeCh  chan struct{}
}

func NewMockProcess(pid int) *MockProcess {
	return &MockProcess{
		pid:       pid,
		suspendCh: make(chan struct{}, 1),
		resumeCh:  make(chan struct{}, 1),
	}
}

func (m *MockProcess) PID() int {
	return m.pid
}

func (m *MockProcess) Suspend() error {
	m.suspended.Store(true)
	return nil
}

func (m *MockProcess) Resume() error {
	m.suspended.Store(false)
	return nil
}

func (m *MockProcess) IsSuspended() bool {
	return m.suspended.Load()
}

// TestDMSFires tests that the Dead Man's Switch fires the OnSuspend callback
// within 200ms when the idle time exceeds the TTL.
// Acceptance criteria: TTL=100ms → suspend callback within 200ms
func TestDMSFires(t *testing.T) {
	// Track if suspend callback was called
	var suspendCalled atomic.Bool
	var suspendTime time.Time
	var mu sync.Mutex

	sessionID := "test-session-123"
	mockProc := NewMockProcess(12345)

	// Create a DMS with a very small TTL (1 minute)
	// But we'll manually trigger the check by setting lastActivity far in the past
	config := DMSConfig{
		TTL:           1, // 1 minute - but we'll test with idle time
		CheckInterval: 10 * time.Millisecond,
		OnSuspend: func(sid string) error {
			mu.Lock()
			suspendCalled.Store(true)
			suspendTime = time.Now()
			mu.Unlock()
			return nil
		},
	}

	dms := NewDeadManSwitch(sessionID, mockProc, config)

	startTime := time.Now()

	// Override the lastActivity to be in the past (simulating idle)
	// We set it to 2 minutes ago to exceed the 1 minute TTL
	dms.lastActivity = time.Now().Add(-2 * time.Minute)

	dms.Start()

	// Wait for up to 200ms for the callback to fire
	timeout := 200 * time.Millisecond
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		mu.Lock()
		called := suspendCalled.Load()
		mu.Unlock()

		if called {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Stop DMS
	dms.Stop()

	mu.Lock()
	called := suspendCalled.Load()
	elapsed := suspendTime.Sub(startTime)
	mu.Unlock()

	if !called {
		t.Fatal("DMS OnSuspend callback was not called within 200ms")
	}

	// Verify timing - should be within 200ms
	if elapsed > timeout {
		t.Errorf("DMS fired after %v, expected within %v", elapsed, timeout)
	}

	t.Logf("DMS fired after %v", elapsed)
}

// TestDMSWithShortCheckInterval tests DMS with very short check interval
func TestDMSWithShortCheckInterval(t *testing.T) {
	var suspendCalled atomic.Bool

	sessionID := "test-session-short-ttl"
	mockProc := NewMockProcess(54321)

	dms := NewDeadManSwitch(sessionID, mockProc, DMSConfig{
		TTL:           1, // 1 minute minimum
		CheckInterval: 10 * time.Millisecond,
		OnSuspend: func(sid string) error {
			suspendCalled.Store(true)
			return nil
		},
	})

	// Set last activity to 2 minutes ago (exceeds 1 minute TTL)
	dms.lastActivity = time.Now().Add(-2 * time.Minute)

	// Use Start/Stop instead of calling monitorLoop directly
	dms.Start()

	// Wait for up to 200ms
	time.Sleep(200 * time.Millisecond)

	// Stop
	dms.Stop()

	if !suspendCalled.Load() {
		t.Error("DMS should have called OnSuspend within 200ms")
	}
}

// TestDMSDoesNotSuspendWhenRunning tests that DMS doesn't suspend when session is RUNNING
func TestDMSDoesNotSuspendWhenRunning(t *testing.T) {
	var suspendCalled atomic.Bool

	sessionID := "test-session-running"
	mockProc := NewMockProcess(99999)

	// Track current session state - return RUNNING to prevent suspension
	var currentState session.SessionState = "RUNNING"

	dms := NewDeadManSwitch(sessionID, mockProc, DMSConfig{
		TTL:           1,
		CheckInterval: 20 * time.Millisecond,
		OnSuspend: func(sid string) error {
			suspendCalled.Store(true)
			return nil
		},
		GetSessionStateFunc: func(sid string) (session.SessionState, error) {
			return currentState, nil
		},
	})

	// Set last activity way in the past (2 minutes ago, exceeds 1 minute TTL)
	dms.lastActivity = time.Now().Add(-2 * time.Minute)

	dms.Start()
	time.Sleep(100 * time.Millisecond)
	dms.Stop()

	// Should not have called suspend because GetSessionStateFunc returns RUNNING
	if suspendCalled.Load() {
		t.Error("DMS should not call OnSuspend when session is RUNNING")
	}
}

// Ensure proc interface is implemented
var _ proc.SuspendableProcess = (*MockProcess)(nil)
