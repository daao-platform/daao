package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/daao/nexus/internal/enterprise/forge"
	"github.com/daao/nexus/internal/license"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"
)

// ScheduleHandler handles schedule and trigger API endpoints.
// It requires an enterprise license to function.
type ScheduleHandler struct {
	dbPool     *pgxpool.Pool
	scheduler  *forge.Scheduler
	licenseMgr *license.Manager
}

// NewScheduleHandler creates a new ScheduleHandler.
func NewScheduleHandler(dbPool *pgxpool.Pool, scheduler *forge.Scheduler, licenseMgr *license.Manager) *ScheduleHandler {
	return &ScheduleHandler{
		dbPool:     dbPool,
		scheduler:  scheduler,
		licenseMgr: licenseMgr,
	}
}

// RegisterScheduleRoutes registers the schedule and trigger routes.
func RegisterScheduleRoutes(mux *http.ServeMux, handler *ScheduleHandler) {
	// Schedule endpoints
	mux.HandleFunc("PUT /api/v1/agents/{id}/schedule", handler.HandleSetSchedule)
	mux.HandleFunc("DELETE /api/v1/agents/{id}/schedule", handler.HandleDeleteSchedule)

	// Trigger endpoints
	mux.HandleFunc("PUT /api/v1/agents/{id}/trigger", handler.HandleSetTrigger)
	mux.HandleFunc("DELETE /api/v1/agents/{id}/trigger", handler.HandleDeleteTrigger)

	// List endpoints
	mux.HandleFunc("GET /api/v1/schedules", handler.HandleListSchedules)
	mux.HandleFunc("GET /api/v1/triggers", handler.HandleListTriggers)
}

// ---------------------------------------------------------------------------
// Request/Response Types
// ---------------------------------------------------------------------------

// SetScheduleRequest represents a request to set a schedule for an agent.
type SetScheduleRequest struct {
	CronExpr    string    `json:"cron_expr"`
	SatelliteID uuid.UUID `json:"satellite_id"`
	MaxRetries  int       `json:"max_retries"`
}

// SetTriggerRequest represents a request to set a trigger for an agent.
type SetTriggerRequest struct {
	Condition   forge.TriggerCondition `json:"condition"`
	SatelliteID uuid.UUID              `json:"satellite_id"`
	Cooldown    time.Duration          `json:"cooldown"`
	MaxRetries  int                    `json:"max_retries"`
}

// ScheduleResponse represents a schedule in API responses.
type ScheduleResponse struct {
	AgentID     uuid.UUID  `json:"agent_id"`
	CronExpr    string     `json:"cron_expr"`
	SatelliteID uuid.UUID  `json:"satellite_id"`
	MaxRetries  int        `json:"max_retries"`
	NextRunAt   *time.Time `json:"next_run_at,omitempty"`
}

// TriggerResponse represents a trigger in API responses.
type TriggerResponse struct {
	AgentID          uuid.UUID              `json:"agent_id"`
	Condition        forge.TriggerCondition `json:"condition"`
	SatelliteID      uuid.UUID              `json:"satellite_id"`
	Cooldown         time.Duration          `json:"cooldown"`
	MaxRetries       int                    `json:"max_retries"`
	LastTriggeredAt  *time.Time             `json:"last_triggered_at,omitempty"`
}

// ---------------------------------------------------------------------------
// Enterprise License Check
// ---------------------------------------------------------------------------

// checkEnterpriseLicense checks if the license is enterprise and returns a 403 response if not.
func (h *ScheduleHandler) checkEnterpriseLicense(w http.ResponseWriter) bool {
	if !h.licenseMgr.IsEnterprise() {
		writeJSONError(w, http.StatusForbidden, "scheduled sessions requires an enterprise license")
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Cron Validation
// ---------------------------------------------------------------------------

// validateCronExpression validates a cron expression and rejects those
// that would fire more than once per minute.
func validateCronExpression(cronExpr string) error {
	// Parse the cron expression
	schedule, err := cron.ParseStandard(cronExpr)
	if err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}

	// Get the next run time
	next := schedule.Next(time.Now())
	if next.IsZero() {
		return fmt.Errorf("cron expression produces no valid run times")
	}

	// Check if it fires more than once per minute
	// A cron expression that fires more than once per minute would have
	// next run time less than 1 minute from now
	if next.Sub(time.Now()) < time.Minute {
		return fmt.Errorf("cron expression fires more than once per minute (minimum interval is 1 minute)")
	}

	return nil
}

// ---------------------------------------------------------------------------
// Schedule Handlers
// ---------------------------------------------------------------------------

// HandleSetSchedule handles PUT /api/v1/agents/:id/schedule
// Validates cron expression, persists to DB, registers with in-memory Scheduler.
func (h *ScheduleHandler) HandleSetSchedule(w http.ResponseWriter, r *http.Request) {
	if !h.checkEnterpriseLicense(w) {
		return
	}

	// Extract agent ID from path
	agentIDStr := r.PathValue("id")
	if agentIDStr == "" {
		writeJSONError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	// Parse request body
	var req SetScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate cron expression
	if err := validateCronExpression(req.CronExpr); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid cron expression: %v", err))
		return
	}

	// Set default max retries
	if req.MaxRetries == 0 {
		req.MaxRetries = 3
	}

	// Persist to database
	scheduleJSON, err := json.Marshal(forge.ScheduleConfig{
		AgentID:     agentID,
		CronExpr:    req.CronExpr,
		SatelliteID: req.SatelliteID,
		MaxRetries:  req.MaxRetries,
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to serialize schedule")
		return
	}

	if h.dbPool != nil {
		_, err = h.dbPool.Exec(r.Context(),
			`UPDATE agent_definitions SET schedule = $1, updated_at = NOW() WHERE id = $2`,
			scheduleJSON, agentID)
		if err != nil {
			slog.Error(fmt.Sprintf("HandleSetSchedule: failed to persist schedule: %v", err), "component", "api")
			writeJSONError(w, http.StatusInternalServerError, "failed to persist schedule")
			return
		}
	}

	// Register with in-memory Scheduler
	if h.scheduler != nil {
		if err := h.scheduler.RegisterSchedule(agentID, req.CronExpr, req.SatelliteID); err != nil {
			slog.Error(fmt.Sprintf("HandleSetSchedule: failed to register schedule: %v", err), "component", "api")
			writeJSONError(w, http.StatusInternalServerError, "failed to register schedule")
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "ok",
		"agent_id": agentID,
	})
}

// HandleDeleteSchedule handles DELETE /api/v1/agents/:id/schedule
// Removes from DB and in-memory Scheduler.
func (h *ScheduleHandler) HandleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	if !h.checkEnterpriseLicense(w) {
		return
	}

	// Extract agent ID from path
	agentIDStr := r.PathValue("id")
	if agentIDStr == "" {
		writeJSONError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	// Remove from database
	if h.dbPool != nil {
		_, err = h.dbPool.Exec(r.Context(),
			`UPDATE agent_definitions SET schedule = NULL, updated_at = NOW() WHERE id = $1`,
			agentID)
		if err != nil {
			slog.Error(fmt.Sprintf("HandleDeleteSchedule: failed to remove schedule from DB: %v", err), "component", "api")
			writeJSONError(w, http.StatusInternalServerError, "failed to remove schedule")
			return
		}
	}

	// Remove from in-memory Scheduler
	if h.scheduler != nil {
		if err := h.scheduler.RemoveSchedule(agentID); err != nil {
			slog.Error(fmt.Sprintf("HandleDeleteSchedule: failed to remove schedule from scheduler: %v", err), "component", "api")
			// Don't fail the request - the DB is already updated
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "ok",
		"agent_id": agentID,
	})
}

// ---------------------------------------------------------------------------
// Trigger Handlers
// ---------------------------------------------------------------------------

// HandleSetTrigger handles PUT /api/v1/agents/:id/trigger
// Validates trigger config, persists to DB, registers with in-memory Scheduler.
func (h *ScheduleHandler) HandleSetTrigger(w http.ResponseWriter, r *http.Request) {
	if !h.checkEnterpriseLicense(w) {
		return
	}

	// Extract agent ID from path
	agentIDStr := r.PathValue("id")
	if agentIDStr == "" {
		writeJSONError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	// Parse request body
	var req SetTriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate trigger condition
	if req.Condition.Metric == "" {
		writeJSONError(w, http.StatusBadRequest, "condition.metric is required")
		return
	}
	if req.Condition.Operator == "" {
		req.Condition.Operator = "gt" // default operator
	}
	validOperators := []string{"gt", "lt", "eq", "gte", "lte"}
	isValidOperator := false
	for _, op := range validOperators {
		if req.Condition.Operator == op {
			isValidOperator = true
			break
		}
	}
	if !isValidOperator {
		writeJSONError(w, http.StatusBadRequest, "condition.operator must be one of: gt, lt, eq, gte, lte")
		return
	}

	// Set default values
	if req.MaxRetries == 0 {
		req.MaxRetries = 3
	}
	if req.Cooldown == 0 {
		req.Cooldown = time.Minute * 5 // default 5 minute cooldown
	}

	// Persist to database - use schedule column for trigger JSON for now
	// Note: In a real implementation, we'd add a trigger column to the schema
	triggerJSON, err := json.Marshal(forge.TriggerConfig{
		AgentID:     agentID,
		Condition:   req.Condition,
		SatelliteID: req.SatelliteID,
		Cooldown:    req.Cooldown,
		MaxRetries:  req.MaxRetries,
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to serialize trigger")
		return
	}

	if h.dbPool != nil {
		// For now, store trigger in schedule column - in production would use separate trigger column
		_, err = h.dbPool.Exec(r.Context(),
			`UPDATE agent_definitions SET schedule = $1, updated_at = NOW() WHERE id = $2`,
			triggerJSON, agentID)
		if err != nil {
			slog.Error(fmt.Sprintf("HandleSetTrigger: failed to persist trigger: %v", err), "component", "api")
			writeJSONError(w, http.StatusInternalServerError, "failed to persist trigger")
			return
		}
	}

	// Register with in-memory Scheduler
	if h.scheduler != nil {
		if err := h.scheduler.RegisterTrigger(agentID, req.Condition, req.SatelliteID, req.Cooldown); err != nil {
			slog.Error(fmt.Sprintf("HandleSetTrigger: failed to register trigger: %v", err), "component", "api")
			writeJSONError(w, http.StatusInternalServerError, "failed to register trigger")
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "ok",
		"agent_id": agentID,
	})
}

// HandleDeleteTrigger handles DELETE /api/v1/agents/:id/trigger
// Removes from DB and in-memory Scheduler.
func (h *ScheduleHandler) HandleDeleteTrigger(w http.ResponseWriter, r *http.Request) {
	if !h.checkEnterpriseLicense(w) {
		return
	}

	// Extract agent ID from path
	agentIDStr := r.PathValue("id")
	if agentIDStr == "" {
		writeJSONError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	// Remove from database
	if h.dbPool != nil {
		_, err = h.dbPool.Exec(r.Context(),
			`UPDATE agent_definitions SET schedule = NULL, updated_at = NOW() WHERE id = $1`,
			agentID)
		if err != nil {
			slog.Error(fmt.Sprintf("HandleDeleteTrigger: failed to remove trigger from DB: %v", err), "component", "api")
			writeJSONError(w, http.StatusInternalServerError, "failed to remove trigger")
			return
		}
	}

	// Remove from in-memory Scheduler
	if h.scheduler != nil {
		if err := h.scheduler.RemoveTrigger(agentID); err != nil {
			slog.Error(fmt.Sprintf("HandleDeleteTrigger: failed to remove trigger from scheduler: %v", err), "component", "api")
			// Don't fail the request - the DB is already updated
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "ok",
		"agent_id": agentID,
	})
}

// ---------------------------------------------------------------------------
// List Handlers
// ---------------------------------------------------------------------------

// HandleListSchedules handles GET /api/v1/schedules
// Lists all active schedules with next run time (from cron.Entry).
func (h *ScheduleHandler) HandleListSchedules(w http.ResponseWriter, r *http.Request) {
	if !h.checkEnterpriseLicense(w) {
		return
	}

	schedules := h.scheduler.ListSchedules()

	// Build response with next run times
	response := make([]ScheduleResponse, 0, len(schedules))
	for _, s := range schedules {
		resp := ScheduleResponse{
			AgentID:     s.AgentID,
			CronExpr:    s.CronExpr,
			SatelliteID: s.SatelliteID,
			MaxRetries:  s.MaxRetries,
		}

		// Get next run time from cron
		if h.scheduler != nil {
			entry := h.scheduler.GetCronEntry(s.AgentID)
			if entry.Next.After(time.Now()) {
				resp.NextRunAt = &entry.Next
			}
		}

		response = append(response, resp)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"schedules": response,
		"count":     len(response),
	})
}

// HandleListTriggers handles GET /api/v1/triggers
// Lists all active triggers with last fired time.
func (h *ScheduleHandler) HandleListTriggers(w http.ResponseWriter, r *http.Request) {
	if !h.checkEnterpriseLicense(w) {
		return
	}

	triggers := h.scheduler.ListTriggers()

	// Build response with last triggered times
	response := make([]TriggerResponse, 0, len(triggers))
	for _, t := range triggers {
		resp := TriggerResponse{
			AgentID:     t.AgentID,
			Condition:   t.Condition,
			SatelliteID: t.SatelliteID,
			Cooldown:    t.Cooldown,
			MaxRetries:  t.MaxRetries,
		}

		// Get last triggered time
		if lastTime := h.scheduler.GetLastTriggered(t.AgentID); !lastTime.IsZero() {
			resp.LastTriggeredAt = &lastTime
		}

		response = append(response, resp)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"triggers": response,
		"count":    len(response),
	})
}

// ---------------------------------------------------------------------------
// Helper Functions
// ---------------------------------------------------------------------------

// GetAgentID extracts the agent ID from a path value.
func GetAgentID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	agentIDStr := r.PathValue("id")
	if agentIDStr == "" {
		writeJSONError(w, http.StatusBadRequest, "agent_id is required")
		return uuid.Nil, false
	}

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid agent_id")
		return uuid.Nil, false
	}

	return agentID, true
}

// ParseLimit parses a limit query parameter with a default.
func ParseLimit(r *http.Request, defaultLimit, maxLimit int) int {
	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		return defaultLimit
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		return defaultLimit
	}

	if limit > maxLimit {
		return maxLimit
	}

	return limit
}

// ValidateTriggerCondition validates a trigger condition.
func ValidateTriggerCondition(condition forge.TriggerCondition) error {
	if condition.Metric == "" {
		return fmt.Errorf("condition.metric is required")
	}

	validOperators := []string{"gt", "lt", "eq", "gte", "lte"}
	isValid := false
	for _, op := range validOperators {
		if condition.Operator == op {
			isValid = true
			break
		}
	}
	if !isValid {
		return fmt.Errorf("condition.operator must be one of: %s", strings.Join(validOperators, ", "))
	}

	return nil
}
