package session

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestSessionStateTransitions is an exhaustive table-driven test for the session
// state machine. It covers all 36 state×state combinations (6 origins × 6 targets)
// and asserts which transitions are valid and which are rejected.
func TestSessionStateTransitions(t *testing.T) {
	allStates := []SessionState{
		StateProvisioning,
		StateRunning,
		StateDetached,
		StateReAttaching,
		StateSuspended,
		StateTerminated,
	}

	// Build expected validity from ValidTransitions map
	isValid := func(from, to SessionState) bool {
		for _, target := range ValidTransitions[from] {
			if target == to {
				return true
			}
		}
		return false
	}

	for _, from := range allStates {
		for _, to := range allStates {
			name := string(from) + "->" + string(to)
			expected := isValid(from, to)

			t.Run(name, func(t *testing.T) {
				s := &Session{
					ID:             uuid.New(),
					SatelliteID:    uuid.New(),
					UserID:         uuid.New(),
					Name:           "test-session",
					AgentBinary:    "test-agent",
					AgentArgs:      []string{},
					State:          from,
					Cols:           80,
					Rows:           24,
					LastActivityAt: time.Now(),
					CreatedAt:      time.Now(),
				}

				result, err := TransitionSession(s, to)

				if expected {
					if err != nil {
						t.Errorf("Expected valid transition %s->%s to succeed, got: %v", from, to, err)
					}
					if result == nil {
						t.Fatal("Expected non-nil session on valid transition")
					}
					if result.State != to {
						t.Errorf("Expected state %s, got: %s", to, result.State)
					}
				} else {
					if err == nil {
						t.Errorf("Expected invalid transition %s->%s to fail, got nil error", from, to)
					}
					if result != nil {
						t.Errorf("Expected nil session on invalid transition %s->%s", from, to)
					}
				}
			})
		}
	}
}

// TestTransitionSession_TimestampSideEffects verifies that TransitionSession
// correctly manages timestamp fields as sessions move through states.
func TestTransitionSession_TimestampSideEffects(t *testing.T) {
	t.Run("StartedAt set only once", func(t *testing.T) {
		s := &Session{
			ID: uuid.New(), SatelliteID: uuid.New(), UserID: uuid.New(),
			Name: "test", AgentBinary: "agent", AgentArgs: []string{},
			State: StateProvisioning, Cols: 80, Rows: 24,
			LastActivityAt: time.Now(), CreatedAt: time.Now(),
		}

		// First transition to RUNNING sets StartedAt
		result, err := TransitionSession(s, StateRunning)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.StartedAt == nil {
			t.Fatal("StartedAt should be set on first transition to RUNNING")
		}
		firstStartedAt := *result.StartedAt

		// Transition to DETACHED and back to RUNNING — StartedAt should NOT change
		result, _ = TransitionSession(result, StateDetached)
		result, _ = TransitionSession(result, StateRunning)
		if result.StartedAt == nil || !result.StartedAt.Equal(firstStartedAt) {
			t.Error("StartedAt should not change on subsequent transitions to RUNNING")
		}
	})

	t.Run("DetachedAt cleared on resume", func(t *testing.T) {
		now := time.Now()
		s := &Session{
			ID: uuid.New(), SatelliteID: uuid.New(), UserID: uuid.New(),
			Name: "test", AgentBinary: "agent", AgentArgs: []string{},
			State: StateDetached, Cols: 80, Rows: 24,
			DetachedAt: &now, LastActivityAt: now, CreatedAt: now,
		}

		result, err := TransitionSession(s, StateRunning)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.DetachedAt != nil {
			t.Error("DetachedAt should be cleared on transition to RUNNING")
		}
	})

	t.Run("SuspendedAt cleared on resume", func(t *testing.T) {
		now := time.Now()
		s := &Session{
			ID: uuid.New(), SatelliteID: uuid.New(), UserID: uuid.New(),
			Name: "test", AgentBinary: "agent", AgentArgs: []string{},
			State: StateSuspended, Cols: 80, Rows: 24,
			SuspendedAt: &now, LastActivityAt: now, CreatedAt: now,
		}

		result, err := TransitionSession(s, StateRunning)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.SuspendedAt != nil {
			t.Error("SuspendedAt should be cleared on transition to RUNNING")
		}
	})

	t.Run("TerminatedAt set on termination", func(t *testing.T) {
		s := &Session{
			ID: uuid.New(), SatelliteID: uuid.New(), UserID: uuid.New(),
			Name: "test", AgentBinary: "agent", AgentArgs: []string{},
			State: StateRunning, Cols: 80, Rows: 24,
			LastActivityAt: time.Now(), CreatedAt: time.Now(),
		}

		result, err := TransitionSession(s, StateTerminated)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.TerminatedAt == nil {
			t.Error("TerminatedAt should be set on TERMINATED transition")
		}
	})

	t.Run("invalid state string rejected", func(t *testing.T) {
		s := &Session{
			ID: uuid.New(), SatelliteID: uuid.New(), UserID: uuid.New(),
			Name: "test", AgentBinary: "agent", AgentArgs: []string{},
			State: StateRunning, Cols: 80, Rows: 24,
			LastActivityAt: time.Now(), CreatedAt: time.Now(),
		}

		result, err := TransitionSession(s, "INVALID_STATE")
		if err == nil {
			t.Error("Expected invalid state to return error")
		}
		if result != nil {
			t.Error("Expected nil session on invalid state")
		}
	})
}

// TestValidateTransition tests the ValidateTransition function
func TestValidateTransition(t *testing.T) {
	t.Run("valid transition returns nil", func(t *testing.T) {
		err := ValidateTransition(StateRunning, StateDetached)
		if err != nil {
			t.Errorf("Expected nil, got: %v", err)
		}
	})

	t.Run("invalid transition returns error", func(t *testing.T) {
		err := ValidateTransition(StateProvisioning, StateSuspended)
		if err == nil {
			t.Error("Expected error for invalid transition")
		}
	})

	t.Run("invalid target state returns error", func(t *testing.T) {
		err := ValidateTransition(StateRunning, "NOT_A_STATE")
		if err == nil {
			t.Error("Expected error for invalid state")
		}
	})
}

// TestIsTransitionValid tests the IsTransitionValid helper
func TestIsTransitionValid(t *testing.T) {
	tests := []struct {
		from SessionState
		to   SessionState
		want bool
	}{
		{StateProvisioning, StateRunning, true},
		{StateProvisioning, StateTerminated, true},
		{StateProvisioning, StateDetached, false},
		{StateRunning, StateDetached, true},
		{StateRunning, StateSuspended, true},
		{StateRunning, StateTerminated, true},
		{StateRunning, StateProvisioning, false},
		{StateDetached, StateRunning, true},
		{StateDetached, StateReAttaching, true},
		{StateDetached, StateTerminated, true},
		{StateDetached, StateSuspended, false},
		{StateTerminated, StateRunning, false},
		{StateTerminated, StateDetached, false},
		{StateSuspended, StateRunning, true},
		{StateSuspended, StateTerminated, true},
		{StateSuspended, StateDetached, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			got := IsTransitionValid(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("IsTransitionValid(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

// TestGetValidTransitions tests the GetValidTransitions helper
func TestGetValidTransitions(t *testing.T) {
	transitions := GetValidTransitions(StateRunning)
	if len(transitions) == 0 {
		t.Error("Expected transitions from RUNNING state")
	}

	// Check specific transitions
	foundDetached := false
	foundSuspended := false
	foundTerminated := false

	for _, t := range transitions {
		if t == StateDetached {
			foundDetached = true
		}
		if t == StateSuspended {
			foundSuspended = true
		}
		if t == StateTerminated {
			foundTerminated = true
		}
	}

	if !foundDetached {
		t.Error("Expected RUNNING -> DETACHED transition")
	}
	if !foundSuspended {
		t.Error("Expected RUNNING -> SUSPENDED transition")
	}
	if !foundTerminated {
		t.Error("Expected RUNNING -> TERMINATED transition")
	}

	// TERMINATED should have no transitions
	terminatedTransitions := GetValidTransitions(StateTerminated)
	if len(terminatedTransitions) != 0 {
		t.Error("Expected no transitions from TERMINATED state")
	}
}

// TestSessionStateIsValid tests the SessionState.IsValid method
func TestSessionStateIsValid(t *testing.T) {
	validStates := []SessionState{
		StateProvisioning,
		StateRunning,
		StateDetached,
		StateReAttaching,
		StateSuspended,
		StateTerminated,
	}

	for _, state := range validStates {
		if !state.IsValid() {
			t.Errorf("Expected %s to be valid", state)
		}
	}

	invalidState := SessionState("INVALID")
	if invalidState.IsValid() {
		t.Error("Expected invalid state to return false")
	}
}
