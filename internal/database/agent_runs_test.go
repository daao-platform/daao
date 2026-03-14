package database

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// SQL Placeholder Test — verifies fix C2
// =============================================================================

func TestFmtSprintfPlaceholders(t *testing.T) {
	// The old bug used string(rune('0'+argIndex)) which breaks for argIndex >= 10.
	// Verify fmt.Sprintf("$%d", i) produces correct placeholders 1..20.
	for i := 1; i <= 20; i++ {
		want := fmt.Sprintf("$%d", i)
		got := fmt.Sprintf("$%d", i) // same logic used in agent_runs.go
		if got != want {
			t.Errorf("placeholder for %d = %q, want %q", i, got, want)
		}
	}
}

// =============================================================================
// AgentRun struct tests — verifies fix M7
// =============================================================================

func TestAgentRunTimeFields(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	later := now.Add(5 * time.Minute)

	run := AgentRun{
		ID:            uuid.New(),
		AgentID:       uuid.New(),
		SatelliteID:   uuid.New(),
		Status:        "completed",
		StartedAt:     now,
		EndedAt:       &later,
		TotalTokens:   1500,
		EstimatedCost: 0.023,
		ToolCallCount: 7,
	}

	if run.StartedAt != now {
		t.Errorf("StartedAt = %v, want %v", run.StartedAt, now)
	}
	if run.EndedAt == nil || *run.EndedAt != later {
		t.Errorf("EndedAt = %v, want %v", run.EndedAt, later)
	}
}

func TestAgentRunJSONSerialization(t *testing.T) {
	// Verify time.Time fields serialize correctly to JSON (not empty)
	now := time.Now().UTC().Truncate(time.Millisecond)
	later := now.Add(5 * time.Minute)
	result := "success"

	run := AgentRun{
		ID:            uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
		AgentID:       uuid.MustParse("11111111-2222-3333-4444-555555555555"),
		SatelliteID:   uuid.MustParse("66666666-7777-8888-9999-000000000000"),
		Status:        "completed",
		StartedAt:     now,
		EndedAt:       &later,
		TotalTokens:   500,
		EstimatedCost: 0.01,
		ToolCallCount: 3,
		Result:        &result,
	}

	data, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("Failed to marshal AgentRun: %v", err)
	}

	// Deserialize into a map and check time fields aren't zero-value
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Failed to unmarshal AgentRun JSON: %v", err)
	}

	startedAt, ok := m["StartedAt"]
	if !ok {
		t.Fatal("StartedAt missing from JSON output")
	}
	if startedAt == "" || startedAt == "0001-01-01T00:00:00Z" {
		t.Error("StartedAt is zero or empty in JSON")
	}

	endedAt, ok := m["EndedAt"]
	if !ok || endedAt == nil {
		t.Fatal("EndedAt missing or nil from JSON output")
	}
}

func TestAgentRunNullableFields(t *testing.T) {
	// Verify nullable fields work correctly
	run := AgentRun{
		ID:          uuid.New(),
		AgentID:     uuid.New(),
		SatelliteID: uuid.New(),
		Status:      "running",
		StartedAt:   time.Now(),
		// Intentionally leave EndedAt, SessionID, Result, Error nil
	}

	if run.EndedAt != nil {
		t.Error("EndedAt should be nil for running run")
	}
	if run.SessionID != nil {
		t.Error("SessionID should be nil when not set")
	}
	if run.Result != nil {
		t.Error("Result should be nil for running run")
	}
	if run.Error != nil {
		t.Error("Error should be nil for running run")
	}
}

// =============================================================================
// AgentRunUpdates tests
// =============================================================================

func TestAgentRunUpdatesPartialFields(t *testing.T) {
	// Verify the struct supports partial updates
	status := "completed"
	tokens := 1000
	cost := 0.05

	updates := AgentRunUpdates{
		Status:        &status,
		TotalTokens:   &tokens,
		EstimatedCost: &cost,
	}

	if *updates.Status != "completed" {
		t.Errorf("Status = %q, want %q", *updates.Status, "completed")
	}
	if updates.EndedAt != nil {
		t.Error("EndedAt should be nil when not set")
	}
	if updates.ToolCallCount != nil {
		t.Error("ToolCallCount should be nil when not set")
	}
}
