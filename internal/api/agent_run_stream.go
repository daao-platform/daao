package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/daao/nexus/internal/agentstream"
	"github.com/daao/nexus/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HandleAgentRunRoutes returns an HTTP handler that dispatches between:
//   - GET /api/v1/runs               → JSON list of all runs
//   - GET /api/v1/runs/{run_id}        → JSON response with run details
//   - GET /api/v1/runs/{run_id}/stream  → SSE event stream
func HandleAgentRunRoutes(hub agentstream.RunEventHubInterface, pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path

		// Dispatch: exact /api/v1/runs or /api/v1/runs/ → list all runs
		trimmed := strings.TrimRight(strings.TrimPrefix(p, "/api/v1/runs"), "/")
		if trimmed == "" {
			handleListAllRuns(pool, w, r)
			return
		}

		// Dispatch: /stream suffix → SSE, otherwise → JSON run detail
		if strings.HasSuffix(p, "/stream") {
			handleAgentRunStream(hub, pool, w, r)
			return
		}

		// GET /api/v1/runs/{run_id} → return run as JSON
		runIDStr := strings.TrimPrefix(p, "/api/v1/runs/")
		runIDStr = strings.TrimRight(runIDStr, "/")
		runID, err := uuid.Parse(runIDStr)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid run_id")
			return
		}

		run, err := database.GetAgentRun(r.Context(), pool, runID)
		if err != nil {
			slog.Info(fmt.Sprintf("HandleGetAgentRun: %v", err), "component", "api")
			writeJSONError(w, http.StatusInternalServerError, "failed to get run")
			return
		}
		if run == nil {
			writeJSONError(w, http.StatusNotFound, "run not found")
			return
		}

		// Build JSON response matching what the frontend expects
		resp := map[string]interface{}{
			"id":              run.ID.String(),
			"agent_id":        run.AgentID.String(),
			"satellite_id":    run.SatelliteID.String(),
			"status":          run.Status,
			"trigger_source":  run.TriggerSource,
			"started_at":      run.StartedAt.Format(time.RFC3339),
			"total_tokens":    run.TotalTokens,
			"estimated_cost":  run.EstimatedCost,
			"tool_call_count": run.ToolCallCount,
		}
		if run.SessionID != nil {
			resp["session_id"] = run.SessionID.String()
		}
		if run.EndedAt != nil {
			resp["ended_at"] = run.EndedAt.Format(time.RFC3339)
		}
		if run.Result != nil {
			resp["result"] = *run.Result
		}
		if run.Error != nil {
			resp["error"] = *run.Error
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// handleListAllRuns handles GET /api/v1/runs — list all agent runs with context.
func handleListAllRuns(pool *pgxpool.Pool, w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Parse query params
	q := r.URL.Query()
	filters := database.AgentRunFilters{}

	if agentID := q.Get("agent_id"); agentID != "" {
		id, err := uuid.Parse(agentID)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid agent_id")
			return
		}
		filters.AgentID = &id
	}
	if status := q.Get("status"); status != "" {
		filters.Status = &status
	}

	limit := 50
	if l := q.Get("limit"); l != "" {
		if parsed, err := fmt.Sscanf(l, "%d", &limit); parsed != 1 || err != nil {
			limit = 50
		}
	}
	offset := 0
	if o := q.Get("offset"); o != "" {
		if parsed, err := fmt.Sscanf(o, "%d", &offset); parsed != 1 || err != nil {
			offset = 0
		}
	}

	runs, total, err := database.ListAgentRunsWithContext(r.Context(), pool, filters, limit, offset)
	if err != nil {
		slog.Info(fmt.Sprintf("handleListAllRuns: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}

	// Build response with explicit time formatting
	type runJSON struct {
		ID            string  `json:"id"`
		AgentID       string  `json:"agent_id"`
		AgentName     string  `json:"agent_name"`
		SatelliteID   string  `json:"satellite_id"`
		SatelliteName string  `json:"satellite_name"`
		Status        string  `json:"status"`
		TriggerSource string  `json:"trigger_source"`
		StartedAt     string  `json:"started_at"`
		EndedAt       *string `json:"ended_at,omitempty"`
		TotalTokens   int     `json:"total_tokens"`
		EstimatedCost float64 `json:"estimated_cost"`
		ToolCallCount int     `json:"tool_call_count"`
		PipelineRunID *string `json:"pipeline_run_id,omitempty"`
		PipelineName  *string `json:"pipeline_name,omitempty"`
		StepOrder     *int    `json:"step_order,omitempty"`
	}

	items := make([]runJSON, 0, len(runs))
	for _, run := range runs {
		item := runJSON{
			ID:            run.ID.String(),
			AgentID:       run.AgentID.String(),
			AgentName:     run.AgentName,
			SatelliteID:   run.SatelliteID.String(),
			SatelliteName: run.SatelliteName,
			Status:        run.Status,
			TriggerSource: run.TriggerSource,
			StartedAt:     run.StartedAt.Format(time.RFC3339),
			TotalTokens:   run.TotalTokens,
			EstimatedCost: run.EstimatedCost,
			ToolCallCount: run.ToolCallCount,
			StepOrder:     run.StepOrder,
		}
		if run.EndedAt != nil {
			s := run.EndedAt.Format(time.RFC3339)
			item.EndedAt = &s
		}
		if run.PipelineRunID != nil {
			s := run.PipelineRunID.String()
			item.PipelineRunID = &s
		}
		if run.PipelineName != nil {
			item.PipelineName = run.PipelineName
		}
		items = append(items, item)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"runs":  items,
		"total": total,
	})
}

// handleAgentRunStream handles streaming agent run events
// via Server-Sent Events (SSE). The endpoint is GET /api/v1/runs/{run_id}/stream.
func handleAgentRunStream(hub agentstream.RunEventHubInterface, pool *pgxpool.Pool, w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Extract run_id from /api/v1/runs/{run_id}/stream
	p := r.URL.Path
	runIDStr := strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/runs/"), "/stream")
	runID, err := uuid.Parse(runIDStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid run_id")
		return
	}

	// Verify run exists
	run, err := database.GetAgentRun(r.Context(), pool, runID)
	if err != nil {
		slog.Info(fmt.Sprintf("HandleAgentRunStream: GetAgentRun: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to get run")
		return
	}
	if run == nil {
		writeJSONError(w, http.StatusNotFound, "run not found")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Subscribe BEFORE replay to avoid missing live events during replay
	ch := hub.Subscribe(runID)
	defer hub.Unsubscribe(runID, ch)

	fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
	flusher.Flush()

	// Replay history
	history, err := database.ListAgentRunEvents(r.Context(), pool, runID)
	if err != nil {
		slog.Info(fmt.Sprintf("HandleAgentRunStream: ListAgentRunEvents: %v", err), "component", "api")
	} else {
		for _, evt := range history {
			data, _ := json.Marshal(map[string]interface{}{
				"id":         evt.ID.String(),
				"run_id":     evt.RunID.String(),
				"event_type": evt.EventType,
				"payload":    json.RawMessage(evt.Payload),
				"sequence":   evt.Sequence,
				"created_at": evt.CreatedAt.Format(time.RFC3339Nano),
			})
			fmt.Fprintf(w, "event: history\ndata: %s\n\n", data)
		}
	}
	fmt.Fprintf(w, "event: live_start\ndata: {}\n\n")
	flusher.Flush()

	// If already finished, close after replay
	if run.Status == "completed" || run.Status == "failed" ||
		run.Status == "timeout" || run.Status == "killed" {
		return
	}

	// Stream live events
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()
	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return // hub closed (agent_end)
			}
			data, err := json.Marshal(evt)
			if err != nil {
				slog.Info(fmt.Sprintf("HandleAgentRunStream: marshal: %v", err), "component", "api")
				continue
			}
			fmt.Fprintf(w, "event: agent_event\ndata: %s\n\n", data)
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}
