// Package api provides HTTP handlers for the Nexus API Gateway.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/daao/nexus/internal/audit"
	"github.com/daao/nexus/internal/auth"
	"github.com/daao/nexus/internal/database"
	"github.com/daao/nexus/internal/enterprise/forge"
	"github.com/daao/nexus/internal/license"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PipelineHandler handles pipeline CRUD and execution API endpoints.
// It requires an enterprise license with session_chaining feature to function.
type PipelineHandler struct {
	dbPool           *pgxpool.Pool
	pipelineExecutor forge.PipelineRunner
	licenseMgr       *license.Manager
	auditLogger      *audit.AuditLogger
}

// NewPipelineHandler creates a new PipelineHandler.
func NewPipelineHandler(dbPool *pgxpool.Pool, pipelineExecutor forge.PipelineRunner, licenseMgr *license.Manager, auditLogger ...*audit.AuditLogger) *PipelineHandler {
	h := &PipelineHandler{
		dbPool:           dbPool,
		pipelineExecutor: pipelineExecutor,
		licenseMgr:       licenseMgr,
	}
	if len(auditLogger) > 0 {
		h.auditLogger = auditLogger[0]
	}
	return h
}

// RegisterPipelineRoutes registers the pipeline routes.
func RegisterPipelineRoutes(mux *http.ServeMux, handler *PipelineHandler) {
	// Pipeline CRUD
	mux.HandleFunc("POST /api/v1/pipelines", handler.HandleCreatePipeline)
	mux.HandleFunc("GET /api/v1/pipelines", handler.HandleListPipelines)
	mux.HandleFunc("GET /api/v1/pipelines/{id}", handler.HandleGetPipeline)
	mux.HandleFunc("PUT /api/v1/pipelines/{id}", handler.HandleUpdatePipeline)
	mux.HandleFunc("DELETE /api/v1/pipelines/{id}", handler.HandleDeletePipeline)

	// Pipeline execution
	mux.HandleFunc("POST /api/v1/pipelines/{id}/run", handler.HandleRunPipeline)
	mux.HandleFunc("GET /api/v1/pipelines/{id}/runs", handler.HandleListPipelineRuns)

	// Pipeline run detail
	mux.HandleFunc("GET /api/v1/pipeline-runs/{run_id}", handler.HandleGetPipelineRun)

	// Pipeline scheduling
	mux.HandleFunc("PUT /api/v1/pipelines/{id}/schedule", handler.HandleSetPipelineSchedule)
	mux.HandleFunc("DELETE /api/v1/pipelines/{id}/schedule", handler.HandleDeletePipelineSchedule)
}

// ---------------------------------------------------------------------------
// Enterprise License Check
// ---------------------------------------------------------------------------

// checkEnterpriseLicense checks if the license has the session_chaining feature and returns a 403 response if not.
func (h *PipelineHandler) checkEnterpriseLicense(w http.ResponseWriter) bool {
	if !h.licenseMgr.HasFeature(license.FeatureSessionChaining) {
		writeJSONError(w, http.StatusForbidden, "pipelines require an enterprise license with session_chaining feature")
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Request/Response Types
// ---------------------------------------------------------------------------

// CreatePipelineRequest represents a request to create a new pipeline with steps.
type CreatePipelineRequest struct {
	Name        string            `json:"name"`
	Description *string           `json:"description"`
	SatelliteID *uuid.UUID        `json:"satellite_id"`
	OnFailure   string            `json:"on_failure"`
	MaxRetries  int               `json:"max_retries"`
	Steps       []CreateStepInput `json:"steps"`
}

// CreateStepInput represents a step in the create pipeline request.
type CreateStepInput struct {
	AgentID    uuid.UUID `json:"agent_id"`
	InputMode  string    `json:"input_mode"`
	OutputMode string    `json:"output_mode"`
	Config     string    `json:"config"`
}

// UpdatePipelineRequest represents a request to update a pipeline.
type UpdatePipelineRequest struct {
	Name        *string    `json:"name"`
	Description *string    `json:"description"`
	SatelliteID *uuid.UUID `json:"satellite_id"`
	OnFailure   *string    `json:"on_failure"`
	MaxRetries  *int       `json:"max_retries"`
	IsEnabled   *bool      `json:"is_enabled"`
}

// UpdatePipelineStepsRequest represents a request to update pipeline steps.
type UpdatePipelineStepsRequest struct {
	Steps []CreateStepInput `json:"steps"`
}

// RunPipelineRequest represents a request to run a pipeline.
type RunPipelineRequest struct {
	SatelliteID *uuid.UUID `json:"satellite_id"`
}

// SetPipelineScheduleRequest represents a request to set a pipeline schedule.
type SetPipelineScheduleRequest struct {
	CronExpr string `json:"cron_expr"`
}

// PipelineResponse represents a pipeline in API responses.
type PipelineResponse struct {
	ID          uuid.UUID              `json:"id"`
	Name        string                 `json:"name"`
	Description *string                `json:"description"`
	SatelliteID *uuid.UUID             `json:"satellite_id"`
	OnFailure   string                 `json:"on_failure"`
	MaxRetries  int                    `json:"max_retries"`
	Schedule    *string                `json:"schedule"`
	IsEnabled   bool                   `json:"is_enabled"`
	Steps       []PipelineStepResponse `json:"steps"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// PipelineStepResponse represents a pipeline step in API responses.
type PipelineStepResponse struct {
	ID         uuid.UUID `json:"id"`
	StepOrder  int       `json:"step_order"`
	AgentID    uuid.UUID `json:"agent_id"`
	InputMode  string    `json:"input_mode"`
	OutputMode string    `json:"output_mode"`
	Config     string    `json:"config"`
}

// PipelineRunResponse represents a pipeline run in API responses.
type PipelineRunResponse struct {
	ID            uuid.UUID  `json:"id"`
	PipelineID    uuid.UUID  `json:"pipeline_id"`
	SatelliteID   *uuid.UUID `json:"satellite_id"`
	Status        string     `json:"status"`
	CurrentStep   *int       `json:"current_step"`
	TriggerSource string     `json:"trigger_source"`
	StartedAt     time.Time  `json:"started_at"`
	EndedAt       *time.Time `json:"ended_at,omitempty"`
	Error         *string    `json:"error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// PipelineRunDetailResponse represents a pipeline run with step runs in API responses.
type PipelineRunDetailResponse struct {
	ID            uuid.UUID                 `json:"id"`
	PipelineID    uuid.UUID                 `json:"pipeline_id"`
	SatelliteID   *uuid.UUID                `json:"satellite_id"`
	Status        string                    `json:"status"`
	CurrentStep   *int                      `json:"current_step"`
	TriggerSource string                    `json:"trigger_source"`
	StartedAt     time.Time                 `json:"started_at"`
	EndedAt       *time.Time                `json:"ended_at,omitempty"`
	Error         *string                   `json:"error,omitempty"`
	CreatedAt     time.Time                 `json:"created_at"`
	StepRuns      []PipelineStepRunResponse `json:"step_runs"`
}

// PipelineStepRunResponse represents a step run in API responses.
type PipelineStepRunResponse struct {
	ID         uuid.UUID  `json:"id"`
	StepOrder  int        `json:"step_order"`
	AgentRunID *uuid.UUID `json:"agent_run_id,omitempty"`
	Status     string     `json:"status"`
	InputText  *string    `json:"input_text,omitempty"`
	OutputText *string    `json:"output_text,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	EndedAt    *time.Time `json:"ended_at,omitempty"`
	Error      *string    `json:"error,omitempty"`
}

// PipelineListResponse represents a paginated list of pipelines.
type PipelineListResponse struct {
	Pipelines []PipelineResponse `json:"pipelines"`
	Total     int                `json:"total"`
}

// PipelineRunListResponse represents a paginated list of pipeline runs.
type PipelineRunListResponse struct {
	Runs  []PipelineRunResponse `json:"runs"`
	Total int                   `json:"total"`
}

// RunPipelineResponse represents the response after triggering a pipeline run.
type RunPipelineResponse struct {
	RunID      uuid.UUID `json:"run_id"`
	PipelineID uuid.UUID `json:"pipeline_id"`
	Status     string    `json:"status"`
}

// ---------------------------------------------------------------------------
// Helper Functions
// ---------------------------------------------------------------------------

// databasePipelineToResponse converts a database.Pipeline to a PipelineResponse.
func databasePipelineToResponse(p *database.Pipeline) *PipelineResponse {
	resp := &PipelineResponse{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		SatelliteID: p.SatelliteID,
		OnFailure:   p.OnFailure,
		MaxRetries:  p.MaxRetries,
		Schedule:    p.Schedule,
		IsEnabled:   p.IsEnabled,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}

	steps := make([]PipelineStepResponse, len(p.Steps))
	for i, step := range p.Steps {
		steps[i] = PipelineStepResponse{
			ID:         step.ID,
			StepOrder:  step.StepOrder,
			AgentID:    step.AgentID,
			InputMode:  step.InputMode,
			OutputMode: step.OutputMode,
			Config:     step.Config,
		}
	}
	resp.Steps = steps

	return resp
}

// databasePipelineRunToResponse converts a database.PipelineRun to a PipelineRunResponse.
func databasePipelineRunToResponse(run *database.PipelineRun) *PipelineRunResponse {
	return &PipelineRunResponse{
		ID:            run.ID,
		PipelineID:    run.PipelineID,
		SatelliteID:   run.SatelliteID,
		Status:        run.Status,
		CurrentStep:   run.CurrentStep,
		TriggerSource: run.TriggerSource,
		StartedAt:     run.StartedAt,
		EndedAt:       run.EndedAt,
		Error:         run.Error,
		CreatedAt:     run.CreatedAt,
	}
}

// databasePipelineRunToDetailResponse converts a database.PipelineRun to a detailed PipelineRunDetailResponse.
func databasePipelineRunToDetailResponse(run *database.PipelineRun) *PipelineRunDetailResponse {
	resp := &PipelineRunDetailResponse{
		ID:            run.ID,
		PipelineID:    run.PipelineID,
		SatelliteID:   run.SatelliteID,
		Status:        run.Status,
		CurrentStep:   run.CurrentStep,
		TriggerSource: run.TriggerSource,
		StartedAt:     run.StartedAt,
		EndedAt:       run.EndedAt,
		Error:         run.Error,
		CreatedAt:     run.CreatedAt,
	}

	stepRuns := make([]PipelineStepRunResponse, len(run.StepRuns))
	for i, sr := range run.StepRuns {
		stepRuns[i] = PipelineStepRunResponse{
			ID:         sr.ID,
			StepOrder:  sr.StepOrder,
			AgentRunID: sr.AgentRunID,
			Status:     sr.Status,
			InputText:  sr.InputText,
			OutputText: sr.OutputText,
			StartedAt:  sr.StartedAt,
			EndedAt:    sr.EndedAt,
			Error:      sr.Error,
		}
	}
	resp.StepRuns = stepRuns

	return resp
}

// parseLimitOffset parses pagination parameters.
func parseLimitOffset(r *http.Request, defaultLimit, maxLimit int) (int, int) {
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := defaultLimit
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			if l > maxLimit {
				limit = maxLimit
			} else {
				limit = l
			}
		}
	}

	offset := 0
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o > 0 {
			offset = o
		}
	}

	return limit, offset
}

// parseSatelliteID parses an optional satellite_id query parameter.
func parseSatelliteID(r *http.Request) *uuid.UUID {
	satIDStr := r.URL.Query().Get("satellite_id")
	if satIDStr == "" {
		return nil
	}
	satID, err := uuid.Parse(satIDStr)
	if err != nil {
		return nil
	}
	return &satID
}

// agentExists checks if an agent exists in the database.
func agentExists(ctx context.Context, dbPool *pgxpool.Pool, agentID uuid.UUID) (bool, error) {
	var exists bool
	err := dbPool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM agent_definitions WHERE id = $1)", agentID).Scan(&exists)
	return exists, err
}

// satelliteActive checks if a satellite is active in the database.
func satelliteActive(ctx context.Context, dbPool *pgxpool.Pool, satelliteID uuid.UUID) (bool, error) {
	var status string
	err := dbPool.QueryRow(ctx, "SELECT status FROM satellites WHERE id = $1", satelliteID).Scan(&status)
	if err != nil {
		return false, err
	}
	return status == "active", nil
}

// ---------------------------------------------------------------------------
// Pipeline Handlers
// ---------------------------------------------------------------------------

// HandleCreatePipeline handles POST /api/v1/pipelines
// Creates a pipeline with steps in a single request.
func (h *PipelineHandler) HandleCreatePipeline(w http.ResponseWriter, r *http.Request) {
	if !h.checkEnterpriseLicense(w) {
		return
	}

	// Parse request body
	var req CreatePipelineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate required fields
	if req.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Validate at least 2 steps
	if len(req.Steps) < 2 {
		writeJSONError(w, http.StatusBadRequest, "pipeline must have at least 2 steps")
		return
	}

	// Validate all agent_ids exist
	ctx := r.Context()
	for _, step := range req.Steps {
		exists, err := agentExists(ctx, h.dbPool, step.AgentID)
		if err != nil {
			slog.Error(fmt.Sprintf("HandleCreatePipeline: failed to check agent existence: %v", err), "component", "api")
			writeJSONError(w, http.StatusInternalServerError, "failed to validate agents")
			return
		}
		if !exists {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("agent %s not found", step.AgentID))
			return
		}
	}

	// Validate satellite is active if specified
	if req.SatelliteID != nil {
		active, err := satelliteActive(ctx, h.dbPool, *req.SatelliteID)
		if err != nil {
			slog.Error(fmt.Sprintf("HandleCreatePipeline: failed to check satellite status: %v", err), "component", "api")
			writeJSONError(w, http.StatusInternalServerError, "failed to validate satellite")
			return
		}
		if !active {
			writeJSONError(w, http.StatusBadRequest, "satellite is not active")
			return
		}
	}

	// Set defaults
	if req.OnFailure == "" {
		req.OnFailure = "stop"
	}
	if req.MaxRetries == 0 {
		req.MaxRetries = 3
	}

	// Get user ID from context (default to system if not available)
	var createdBy uuid.UUID
	user, _ := auth.UserFromContext(r.Context())
	if user != nil {
		createdBy = uuid.MustParse(user.ID)
	} else {
		createdBy = uuid.Nil
	}

	// Create pipeline
	pipeline := &database.Pipeline{
		Name:        req.Name,
		Description: req.Description,
		SatelliteID: req.SatelliteID,
		CreatedBy:   createdBy,
		OnFailure:   req.OnFailure,
		MaxRetries:  req.MaxRetries,
		IsEnabled:   true,
	}

	if err := database.CreatePipeline(ctx, h.dbPool, pipeline); err != nil {
		slog.Error(fmt.Sprintf("HandleCreatePipeline: failed to create pipeline: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to create pipeline")
		return
	}

	// Create steps
	steps := make([]database.PipelineStep, len(req.Steps))
	for i, stepReq := range req.Steps {
		steps[i] = database.PipelineStep{
			PipelineID: pipeline.ID,
			StepOrder:  i + 1,
			AgentID:    stepReq.AgentID,
			InputMode:  stepReq.InputMode,
			OutputMode: stepReq.OutputMode,
			Config:     stepReq.Config,
		}
	}

	if err := database.CreatePipelineSteps(ctx, h.dbPool, pipeline.ID, steps); err != nil {
		slog.Error(fmt.Sprintf("HandleCreatePipeline: failed to create pipeline steps: %v", err), "component", "api")
		// Rollback: delete the pipeline
		_ = database.DeletePipeline(ctx, h.dbPool, pipeline.ID)
		writeJSONError(w, http.StatusInternalServerError, "failed to create pipeline steps")
		return
	}

	// Load the created pipeline with steps
	pipeline, err := database.GetPipeline(ctx, h.dbPool, pipeline.ID)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleCreatePipeline: failed to get created pipeline: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve pipeline")
		return
	}

	// Audit log
	if h.auditLogger != nil {
		h.auditLogger.Log(r.Context(), "create", "pipeline", pipeline.ID.String(), map[string]interface{}{
			"name":  pipeline.Name,
			"steps": len(req.Steps),
		})
	}

	writeJSON(w, http.StatusCreated, databasePipelineToResponse(pipeline))
}

// HandleListPipelines handles GET /api/v1/pipelines
// Lists pipelines with pagination and optional satellite filter.
func (h *PipelineHandler) HandleListPipelines(w http.ResponseWriter, r *http.Request) {
	if !h.checkEnterpriseLicense(w) {
		return
	}

	limit, offset := parseLimitOffset(r, 20, 100)
	satelliteID := parseSatelliteID(r)

	ctx := r.Context()
	pipelines, total, err := database.ListPipelines(ctx, h.dbPool, limit, offset, satelliteID)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleListPipelines: failed to list pipelines: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to list pipelines")
		return
	}

	if pipelines == nil {
		pipelines = []*database.Pipeline{}
	}

	response := make([]PipelineResponse, len(pipelines))
	for i, p := range pipelines {
		// Load steps for each pipeline
		fullPipeline, err := database.GetPipeline(ctx, h.dbPool, p.ID)
		if err != nil {
			slog.Error(fmt.Sprintf("HandleListPipelines: failed to get pipeline steps: %v", err), "component", "api")
			writeJSONError(w, http.StatusInternalServerError, "failed to load pipeline details")
			return
		}
		resp := databasePipelineToResponse(fullPipeline)
		response[i] = *resp
	}

	writeJSON(w, http.StatusOK, PipelineListResponse{
		Pipelines: response,
		Total:     total,
	})
}

// HandleGetPipeline handles GET /api/v1/pipelines/:id
// Gets a pipeline with steps by ID.
func (h *PipelineHandler) HandleGetPipeline(w http.ResponseWriter, r *http.Request) {
	if !h.checkEnterpriseLicense(w) {
		return
	}

	// Extract pipeline ID from path
	idStr := r.PathValue("id")
	if idStr == "" {
		writeJSONError(w, http.StatusBadRequest, "pipeline_id is required")
		return
	}

	pipelineID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid pipeline_id")
		return
	}

	ctx := r.Context()
	pipeline, err := database.GetPipeline(ctx, h.dbPool, pipelineID)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleGetPipeline: failed to get pipeline: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to get pipeline")
		return
	}

	if pipeline == nil {
		writeJSONError(w, http.StatusNotFound, "pipeline not found")
		return
	}

	writeJSON(w, http.StatusOK, databasePipelineToResponse(pipeline))
}

// HandleUpdatePipeline handles PUT /api/v1/pipelines/:id
// Updates a pipeline definition and replaces steps atomically.
func (h *PipelineHandler) HandleUpdatePipeline(w http.ResponseWriter, r *http.Request) {
	if !h.checkEnterpriseLicense(w) {
		return
	}

	// Extract pipeline ID from path
	idStr := r.PathValue("id")
	if idStr == "" {
		writeJSONError(w, http.StatusBadRequest, "pipeline_id is required")
		return
	}

	pipelineID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid pipeline_id")
		return
	}

	ctx := r.Context()

	// Check if pipeline exists
	existing, err := database.GetPipeline(ctx, h.dbPool, pipelineID)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleUpdatePipeline: failed to get pipeline: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to get pipeline")
		return
	}

	if existing == nil {
		writeJSONError(w, http.StatusNotFound, "pipeline not found")
		return
	}

	// Parse request body
	var req CreatePipelineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate at least 2 steps if steps are provided
	if len(req.Steps) > 0 && len(req.Steps) < 2 {
		writeJSONError(w, http.StatusBadRequest, "pipeline must have at least 2 steps")
		return
	}

	// Validate all agent_ids exist if steps are provided
	for _, step := range req.Steps {
		exists, err := agentExists(ctx, h.dbPool, step.AgentID)
		if err != nil {
			slog.Error(fmt.Sprintf("HandleUpdatePipeline: failed to check agent existence: %v", err), "component", "api")
			writeJSONError(w, http.StatusInternalServerError, "failed to validate agents")
			return
		}
		if !exists {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("agent %s not found", step.AgentID))
			return
		}
	}

	// Validate satellite is active if specified
	if req.SatelliteID != nil {
		active, err := satelliteActive(ctx, h.dbPool, *req.SatelliteID)
		if err != nil {
			slog.Error(fmt.Sprintf("HandleUpdatePipeline: failed to check satellite status: %v", err), "component", "api")
			writeJSONError(w, http.StatusInternalServerError, "failed to validate satellite")
			return
		}
		if !active {
			writeJSONError(w, http.StatusBadRequest, "satellite is not active")
			return
		}
	}

	// Update pipeline fields
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Description != nil {
		existing.Description = req.Description
	}
	if req.SatelliteID != nil {
		existing.SatelliteID = req.SatelliteID
	}
	if req.OnFailure != "" {
		existing.OnFailure = req.OnFailure
	}
	if req.MaxRetries > 0 {
		existing.MaxRetries = req.MaxRetries
	}

	// Update pipeline in database
	if err := database.UpdatePipeline(ctx, h.dbPool, existing); err != nil {
		slog.Error(fmt.Sprintf("HandleUpdatePipeline: failed to update pipeline: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to update pipeline")
		return
	}

	// Replace steps atomically if steps are provided
	if len(req.Steps) > 0 {
		// Delete old steps
		if err := database.DeletePipelineSteps(ctx, h.dbPool, pipelineID); err != nil {
			slog.Error(fmt.Sprintf("HandleUpdatePipeline: failed to delete old steps: %v", err), "component", "api")
			writeJSONError(w, http.StatusInternalServerError, "failed to update pipeline steps")
			return
		}

		// Insert new steps
		steps := make([]database.PipelineStep, len(req.Steps))
		for i, stepReq := range req.Steps {
			steps[i] = database.PipelineStep{
				PipelineID: pipelineID,
				StepOrder:  i + 1,
				AgentID:    stepReq.AgentID,
				InputMode:  stepReq.InputMode,
				OutputMode: stepReq.OutputMode,
				Config:     stepReq.Config,
			}
		}

		if err := database.CreatePipelineSteps(ctx, h.dbPool, pipelineID, steps); err != nil {
			slog.Error(fmt.Sprintf("HandleUpdatePipeline: failed to create new steps: %v", err), "component", "api")
			writeJSONError(w, http.StatusInternalServerError, "failed to update pipeline steps")
			return
		}
	}

	// Load the updated pipeline with steps
	updated, err := database.GetPipeline(ctx, h.dbPool, pipelineID)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleUpdatePipeline: failed to get updated pipeline: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve pipeline")
		return
	}

	// Audit log
	if h.auditLogger != nil {
		h.auditLogger.Log(r.Context(), "update", "pipeline", pipelineID.String(), map[string]interface{}{
			"name": updated.Name,
		})
	}

	writeJSON(w, http.StatusOK, databasePipelineToResponse(updated))
}

// HandleDeletePipeline handles DELETE /api/v1/pipelines/:id
// Deletes a pipeline (cascade deletes steps and runs).
func (h *PipelineHandler) HandleDeletePipeline(w http.ResponseWriter, r *http.Request) {
	if !h.checkEnterpriseLicense(w) {
		return
	}

	// Extract pipeline ID from path
	idStr := r.PathValue("id")
	if idStr == "" {
		writeJSONError(w, http.StatusBadRequest, "pipeline_id is required")
		return
	}

	pipelineID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid pipeline_id")
		return
	}

	ctx := r.Context()

	// Check if pipeline exists
	existing, err := database.GetPipeline(ctx, h.dbPool, pipelineID)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleDeletePipeline: failed to get pipeline: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to get pipeline")
		return
	}

	if existing == nil {
		writeJSONError(w, http.StatusNotFound, "pipeline not found")
		return
	}

	// Audit log before deletion
	if h.auditLogger != nil {
		h.auditLogger.Log(r.Context(), "delete", "pipeline", pipelineID.String(), map[string]interface{}{
			"name": existing.Name,
		})
	}

	// Delete pipeline (cascade will handle steps and runs)
	if err := database.DeletePipeline(ctx, h.dbPool, pipelineID); err != nil {
		slog.Error(fmt.Sprintf("HandleDeletePipeline: failed to delete pipeline: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to delete pipeline")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Pipeline Execution Handlers
// ---------------------------------------------------------------------------

// HandleRunPipeline handles POST /api/v1/pipelines/:id/run
// Triggers pipeline execution in a goroutine.
func (h *PipelineHandler) HandleRunPipeline(w http.ResponseWriter, r *http.Request) {
	if !h.checkEnterpriseLicense(w) {
		return
	}

	// Extract pipeline ID from path
	idStr := r.PathValue("id")
	if idStr == "" {
		writeJSONError(w, http.StatusBadRequest, "pipeline_id is required")
		return
	}

	pipelineID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid pipeline_id")
		return
	}

	ctx := r.Context()

	// Check if pipeline exists
	existing, err := database.GetPipeline(ctx, h.dbPool, pipelineID)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleRunPipeline: failed to get pipeline: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to get pipeline")
		return
	}

	if existing == nil {
		writeJSONError(w, http.StatusNotFound, "pipeline not found")
		return
	}

	// Parse optional request body
	var req RunPipelineRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Ignore error - satellite_id is optional
		}
	}

	// Use request satellite_id if provided, otherwise use pipeline's satellite
	satelliteID := uuid.Nil
	if req.SatelliteID != nil {
		satelliteID = *req.SatelliteID
	} else if existing.SatelliteID != nil {
		satelliteID = *existing.SatelliteID
	}

	// Validate satellite is active if specified
	if satelliteID != uuid.Nil {
		active, err := satelliteActive(ctx, h.dbPool, satelliteID)
		if err != nil {
			slog.Error(fmt.Sprintf("HandleRunPipeline: failed to check satellite status: %v", err), "component", "api")
			writeJSONError(w, http.StatusInternalServerError, "failed to validate satellite")
			return
		}
		if !active {
			writeJSONError(w, http.StatusBadRequest, "satellite is not active")
			return
		}
	}

	// Create a pipeline run record to get the run_id
	pipelineRun := &database.PipelineRun{
		PipelineID:    pipelineID,
		SatelliteID:   &satelliteID,
		Status:        "pending",
		TriggerSource: "manual",
	}

	if err := database.CreatePipelineRun(ctx, h.dbPool, pipelineRun); err != nil {
		slog.Error(fmt.Sprintf("HandleRunPipeline: failed to create pipeline run: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to create pipeline run")
		return
	}

	// Launch executor in goroutine
	if h.pipelineExecutor != nil {
		go func() {
			execCtx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
			defer cancel()
			_ = h.pipelineExecutor.RunPipeline(execCtx, pipelineID, satelliteID, "manual")
		}()
	}

	// Audit log
	if h.auditLogger != nil {
		h.auditLogger.Log(r.Context(), "run", "pipeline", pipelineID.String(), map[string]interface{}{
			"run_id": pipelineRun.ID.String(),
		})
	}

	writeJSON(w, http.StatusAccepted, RunPipelineResponse{
		RunID:      pipelineRun.ID,
		PipelineID: pipelineID,
		Status:     "started",
	})
}

// HandleListPipelineRuns handles GET /api/v1/pipelines/:id/runs
// Lists pipeline runs with pagination.
func (h *PipelineHandler) HandleListPipelineRuns(w http.ResponseWriter, r *http.Request) {
	if !h.checkEnterpriseLicense(w) {
		return
	}

	// Extract pipeline ID from path
	idStr := r.PathValue("id")
	if idStr == "" {
		writeJSONError(w, http.StatusBadRequest, "pipeline_id is required")
		return
	}

	pipelineID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid pipeline_id")
		return
	}

	ctx := r.Context()

	// Check if pipeline exists
	existing, err := database.GetPipeline(ctx, h.dbPool, pipelineID)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleListPipelineRuns: failed to get pipeline: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to get pipeline")
		return
	}

	if existing == nil {
		writeJSONError(w, http.StatusNotFound, "pipeline not found")
		return
	}

	limit, offset := parseLimitOffset(r, 20, 100)

	runs, total, err := database.ListPipelineRuns(ctx, h.dbPool, pipelineID, limit, offset)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleListPipelineRuns: failed to list pipeline runs: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to list pipeline runs")
		return
	}

	if runs == nil {
		runs = []*database.PipelineRun{}
	}

	response := make([]PipelineRunResponse, len(runs))
	for i, run := range runs {
		response[i] = *databasePipelineRunToResponse(run)
	}

	writeJSON(w, http.StatusOK, PipelineRunListResponse{
		Runs:  response,
		Total: total,
	})
}

// HandleGetPipelineRun handles GET /api/v1/pipeline-runs/:run_id
// Gets a pipeline run detail with step runs.
func (h *PipelineHandler) HandleGetPipelineRun(w http.ResponseWriter, r *http.Request) {
	if !h.checkEnterpriseLicense(w) {
		return
	}

	// Extract run ID from path
	runIDStr := r.PathValue("run_id")
	if runIDStr == "" {
		writeJSONError(w, http.StatusBadRequest, "run_id is required")
		return
	}

	runID, err := uuid.Parse(runIDStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid run_id")
		return
	}

	ctx := r.Context()
	run, err := database.GetPipelineRun(ctx, h.dbPool, runID)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleGetPipelineRun: failed to get pipeline run: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to get pipeline run")
		return
	}

	if run == nil {
		writeJSONError(w, http.StatusNotFound, "pipeline run not found")
		return
	}

	writeJSON(w, http.StatusOK, databasePipelineRunToDetailResponse(run))
}

// ---------------------------------------------------------------------------
// Pipeline Schedule Handlers
// ---------------------------------------------------------------------------

// HandleSetPipelineSchedule handles PUT /api/v1/pipelines/:id/schedule
// Sets a cron schedule for a pipeline.
func (h *PipelineHandler) HandleSetPipelineSchedule(w http.ResponseWriter, r *http.Request) {
	if !h.checkEnterpriseLicense(w) {
		return
	}

	// Extract pipeline ID from path
	idStr := r.PathValue("id")
	if idStr == "" {
		writeJSONError(w, http.StatusBadRequest, "pipeline_id is required")
		return
	}

	pipelineID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid pipeline_id")
		return
	}

	ctx := r.Context()

	// Check if pipeline exists
	existing, err := database.GetPipeline(ctx, h.dbPool, pipelineID)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleSetPipelineSchedule: failed to get pipeline: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to get pipeline")
		return
	}

	if existing == nil {
		writeJSONError(w, http.StatusNotFound, "pipeline not found")
		return
	}

	// Parse request body
	var req SetPipelineScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate cron expression
	if err := validateCronExpression(req.CronExpr); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid cron expression: %v", err))
		return
	}

	// Update pipeline with schedule
	existing.Schedule = &req.CronExpr
	if err := database.UpdatePipeline(ctx, h.dbPool, existing); err != nil {
		slog.Error(fmt.Sprintf("HandleSetPipelineSchedule: failed to update pipeline: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to set schedule")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "ok",
		"pipeline_id": pipelineID,
		"cron_expr":   req.CronExpr,
	})
}

// HandleDeletePipelineSchedule handles DELETE /api/v1/pipelines/:id/schedule
// Removes a schedule from a pipeline.
func (h *PipelineHandler) HandleDeletePipelineSchedule(w http.ResponseWriter, r *http.Request) {
	if !h.checkEnterpriseLicense(w) {
		return
	}

	// Extract pipeline ID from path
	idStr := r.PathValue("id")
	if idStr == "" {
		writeJSONError(w, http.StatusBadRequest, "pipeline_id is required")
		return
	}

	pipelineID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid pipeline_id")
		return
	}

	ctx := r.Context()

	// Check if pipeline exists
	existing, err := database.GetPipeline(ctx, h.dbPool, pipelineID)
	if err != nil {
		slog.Error(fmt.Sprintf("HandleDeletePipelineSchedule: failed to get pipeline: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to get pipeline")
		return
	}

	if existing == nil {
		writeJSONError(w, http.StatusNotFound, "pipeline not found")
		return
	}

	// Remove schedule
	existing.Schedule = nil
	if err := database.UpdatePipeline(ctx, h.dbPool, existing); err != nil {
		slog.Error(fmt.Sprintf("HandleDeletePipelineSchedule: failed to update pipeline: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to remove schedule")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "ok",
		"pipeline_id": pipelineID,
	})
}
