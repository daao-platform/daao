// Package database provides PostgreSQL connection pool management and query functions.
package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pipeline represents a pipeline stored in the database.
type Pipeline struct {
	ID          uuid.UUID
	Name        string
	Description *string
	SatelliteID *uuid.UUID
	CreatedBy   uuid.UUID
	OnFailure   string
	MaxRetries  int
	Schedule    *string
	IsEnabled   bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Steps       []PipelineStep
}

// PipelineStep represents a step within a pipeline.
type PipelineStep struct {
	ID         uuid.UUID
	PipelineID uuid.UUID
	StepOrder  int
	AgentID    uuid.UUID
	InputMode  string
	OutputMode string
	Config     string
	CreatedAt  time.Time
}

// PipelineRun represents a pipeline execution run.
type PipelineRun struct {
	ID            uuid.UUID
	PipelineID    uuid.UUID
	SatelliteID   *uuid.UUID
	Status        string
	CurrentStep   *int
	TriggerSource string
	StartedAt     time.Time
	EndedAt       *time.Time
	Error         *string
	CreatedAt     time.Time
	StepRuns      []PipelineStepRun
}

// PipelineStepRun represents a step execution within a pipeline run.
type PipelineStepRun struct {
	ID           uuid.UUID
	PipelineRunID uuid.UUID
	StepOrder    int
	AgentRunID   *uuid.UUID
	Status       string
	InputText    *string
	OutputText   *string
	StartedAt    *time.Time
	EndedAt      *time.Time
	Error        *string
	CreatedAt    time.Time
}

// CreatePipeline inserts a new pipeline into the database.
// Returns the created Pipeline with generated ID or an error.
func CreatePipeline(ctx context.Context, dbPool *pgxpool.Pool, pipeline *Pipeline) error {
	query := `
		INSERT INTO pipelines (
			name, description, satellite_id, created_by, on_failure, max_retries, schedule, is_enabled
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`

	err := dbPool.QueryRow(ctx, query,
		pipeline.Name,
		pipeline.Description,
		pipeline.SatelliteID,
		pipeline.CreatedBy,
		pipeline.OnFailure,
		pipeline.MaxRetries,
		pipeline.Schedule,
		pipeline.IsEnabled,
	).Scan(
		&pipeline.ID,
		&pipeline.CreatedAt,
		&pipeline.UpdatedAt,
	)
	if err != nil {
		return err
	}
	return nil
}

// GetPipeline retrieves a pipeline by its ID, including all steps sorted by step_order.
// Returns the Pipeline or nil if not found, or an error.
func GetPipeline(ctx context.Context, dbPool *pgxpool.Pool, id uuid.UUID) (*Pipeline, error) {
	// First get the pipeline
	pipelineQuery := `
		SELECT id, name, description, satellite_id, created_by, on_failure, max_retries, schedule, is_enabled, created_at, updated_at
		FROM pipelines
		WHERE id = $1
	`

	var p Pipeline
	var description *string
	var satelliteID *uuid.UUID

	err := dbPool.QueryRow(ctx, pipelineQuery, id).Scan(
		&p.ID,
		&p.Name,
		&description,
		&satelliteID,
		&p.CreatedBy,
		&p.OnFailure,
		&p.MaxRetries,
		&p.Schedule,
		&p.IsEnabled,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	p.Description = description
	p.SatelliteID = satelliteID

	// Now get all steps sorted by step_order
	stepsQuery := `
		SELECT id, pipeline_id, step_order, agent_id, input_mode, output_mode, config, created_at
		FROM pipeline_steps
		WHERE pipeline_id = $1
		ORDER BY step_order ASC
	`

	rows, err := dbPool.Query(ctx, stepsQuery, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []PipelineStep
	for rows.Next() {
		var step PipelineStep
		err := rows.Scan(
			&step.ID,
			&step.PipelineID,
			&step.StepOrder,
			&step.AgentID,
			&step.InputMode,
			&step.OutputMode,
			&step.Config,
			&step.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}
	p.Steps = steps

	return &p, rows.Err()
}

// ListPipelines retrieves pipelines with pagination and optional satellite filter.
// Returns a slice of Pipelines, total count, or an error.
func ListPipelines(ctx context.Context, dbPool *pgxpool.Pool, limit, offset int, satelliteID *uuid.UUID) ([]*Pipeline, int, error) {
	// Build dynamic query
	baseQuery := `
		SELECT id, name, description, satellite_id, created_by, on_failure, max_retries, schedule, is_enabled, created_at, updated_at
		FROM pipelines
		WHERE 1=1
	`

	countQuery := `SELECT COUNT(*) FROM pipelines WHERE 1=1`

	args := []interface{}{}
	argIndex := 1

	if satelliteID != nil {
		baseQuery += fmt.Sprintf(" AND satellite_id = $%d", argIndex)
		countQuery += fmt.Sprintf(" AND satellite_id = $%d", argIndex)
		args = append(args, *satelliteID)
		argIndex++
	}

	// Get total count
	var total int
	err := dbPool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Add pagination
	baseQuery += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, limit, offset)

	rows, err := dbPool.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var pipelines []*Pipeline
	for rows.Next() {
		var p Pipeline
		var description *string
		var satID *uuid.UUID

		err := rows.Scan(
			&p.ID,
			&p.Name,
			&description,
			&satID,
			&p.CreatedBy,
			&p.OnFailure,
			&p.MaxRetries,
			&p.Schedule,
			&p.IsEnabled,
			&p.CreatedAt,
			&p.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		p.Description = description
		p.SatelliteID = satID
		pipelines = append(pipelines, &p)
	}

	return pipelines, total, rows.Err()
}

// UpdatePipeline updates a pipeline's mutable fields.
// Returns an error if the update fails.
func UpdatePipeline(ctx context.Context, dbPool *pgxpool.Pool, pipeline *Pipeline) error {
	query := `
		UPDATE pipelines
		SET name = $1, description = $2, satellite_id = $3, on_failure = $4, max_retries = $5, schedule = $6, is_enabled = $7, updated_at = NOW()
		WHERE id = $8
		RETURNING created_at, updated_at
	`

	err := dbPool.QueryRow(ctx, query,
		pipeline.Name,
		pipeline.Description,
		pipeline.SatelliteID,
		pipeline.OnFailure,
		pipeline.MaxRetries,
		pipeline.Schedule,
		pipeline.IsEnabled,
		pipeline.ID,
	).Scan(
		&pipeline.CreatedAt,
		&pipeline.UpdatedAt,
	)
	if err != nil {
		return err
	}
	return nil
}

// DeletePipeline deletes a pipeline by its ID.
// Cascade handles children (pipeline_steps, pipeline_runs, pipeline_step_runs).
// Returns an error if the deletion fails.
func DeletePipeline(ctx context.Context, dbPool *pgxpool.Pool, id uuid.UUID) error {
	_, err := dbPool.Exec(ctx, "DELETE FROM pipelines WHERE id = $1", id)
	return err
}

// CreatePipelineSteps bulk inserts steps for a pipeline.
// Returns an error if the insert fails.
func CreatePipelineSteps(ctx context.Context, dbPool *pgxpool.Pool, pipelineID uuid.UUID, steps []PipelineStep) error {
	if len(steps) == 0 {
		return nil
	}

	query := `
		INSERT INTO pipeline_steps (pipeline_id, step_order, agent_id, input_mode, output_mode, config)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	for i := range steps {
		_, err := dbPool.Exec(ctx, query,
			pipelineID,
			steps[i].StepOrder,
			steps[i].AgentID,
			steps[i].InputMode,
			steps[i].OutputMode,
			steps[i].Config,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// DeletePipelineSteps deletes all steps for a pipeline.
// Returns an error if the deletion fails.
func DeletePipelineSteps(ctx context.Context, dbPool *pgxpool.Pool, pipelineID uuid.UUID) error {
	_, err := dbPool.Exec(ctx, "DELETE FROM pipeline_steps WHERE pipeline_id = $1", pipelineID)
	return err
}

// CreatePipelineRun inserts a new pipeline run into the database.
// Returns the created PipelineRun with generated ID or an error.
func CreatePipelineRun(ctx context.Context, dbPool *pgxpool.Pool, run *PipelineRun) error {
	query := `
		INSERT INTO pipeline_runs (pipeline_id, satellite_id, status, current_step, trigger_source, started_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		RETURNING id, started_at, created_at
	`

	err := dbPool.QueryRow(ctx, query,
		run.PipelineID,
		run.SatelliteID,
		run.Status,
		run.CurrentStep,
		run.TriggerSource,
	).Scan(
		&run.ID,
		&run.StartedAt,
		&run.CreatedAt,
	)
	if err != nil {
		return err
	}
	return nil
}

// GetPipelineRun retrieves a pipeline run by its ID, including all step runs sorted by step_order.
// Returns the PipelineRun or nil if not found, or an error.
func GetPipelineRun(ctx context.Context, dbPool *pgxpool.Pool, id uuid.UUID) (*PipelineRun, error) {
	// First get the pipeline run
	runQuery := `
		SELECT id, pipeline_id, satellite_id, status, current_step, trigger_source, started_at, ended_at, error, created_at
		FROM pipeline_runs
		WHERE id = $1
	`

	var run PipelineRun
	var satelliteID *uuid.UUID
	var currentStep *int
	var endedAt *time.Time
	var error *string

	err := dbPool.QueryRow(ctx, runQuery, id).Scan(
		&run.ID,
		&run.PipelineID,
		&satelliteID,
		&run.Status,
		&currentStep,
		&run.TriggerSource,
		&run.StartedAt,
		&endedAt,
		&error,
		&run.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	run.SatelliteID = satelliteID
	run.CurrentStep = currentStep
	run.EndedAt = endedAt
	run.Error = error

	// Now get all step runs sorted by step_order
	stepRunsQuery := `
		SELECT id, pipeline_run_id, step_order, agent_run_id, status, input_text, output_text, started_at, ended_at, error, created_at
		FROM pipeline_step_runs
		WHERE pipeline_run_id = $1
		ORDER BY step_order ASC
	`

	rows, err := dbPool.Query(ctx, stepRunsQuery, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stepRuns []PipelineStepRun
	for rows.Next() {
		var stepRun PipelineStepRun
		var agentRunID *uuid.UUID
		var inputText *string
		var outputText *string
		var startedAt *time.Time
		var endedAtStep *time.Time
		var errorStep *string

		err := rows.Scan(
			&stepRun.ID,
			&stepRun.PipelineRunID,
			&stepRun.StepOrder,
			&agentRunID,
			&stepRun.Status,
			&inputText,
			&outputText,
			&startedAt,
			&endedAtStep,
			&errorStep,
			&stepRun.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		stepRun.AgentRunID = agentRunID
		stepRun.InputText = inputText
		stepRun.OutputText = outputText
		stepRun.StartedAt = startedAt
		stepRun.EndedAt = endedAtStep
		stepRun.Error = errorStep
		stepRuns = append(stepRuns, stepRun)
	}
	run.StepRuns = stepRuns

	return &run, rows.Err()
}

// UpdatePipelineRun updates a pipeline run's fields (status, current_step, ended_at, error).
// Returns an error if the update fails.
func UpdatePipelineRun(ctx context.Context, dbPool *pgxpool.Pool, run *PipelineRun) error {
	query := `
		UPDATE pipeline_runs
		SET status = $1, current_step = $2, ended_at = $3, error = $4
		WHERE id = $5
		RETURNING started_at, created_at
	`

	err := dbPool.QueryRow(ctx, query,
		run.Status,
		run.CurrentStep,
		run.EndedAt,
		run.Error,
		run.ID,
	).Scan(
		&run.StartedAt,
		&run.CreatedAt,
	)
	if err != nil {
		return err
	}
	return nil
}

// ListPipelineRuns retrieves pipeline runs for a specific pipeline with pagination.
// Returns a slice of PipelineRuns, total count, or an error.
func ListPipelineRuns(ctx context.Context, dbPool *pgxpool.Pool, pipelineID uuid.UUID, limit, offset int) ([]*PipelineRun, int, error) {
	// Get total count
	var total int
	countQuery := `SELECT COUNT(*) FROM pipeline_runs WHERE pipeline_id = $1`
	err := dbPool.QueryRow(ctx, countQuery, pipelineID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Get paginated results
	query := `
		SELECT id, pipeline_id, satellite_id, status, current_step, trigger_source, started_at, ended_at, error, created_at
		FROM pipeline_runs
		WHERE pipeline_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := dbPool.Query(ctx, query, pipelineID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var runs []*PipelineRun
	for rows.Next() {
		var run PipelineRun
		var satelliteID *uuid.UUID
		var currentStep *int
		var endedAt *time.Time
		var error *string

		err := rows.Scan(
			&run.ID,
			&run.PipelineID,
			&satelliteID,
			&run.Status,
			&currentStep,
			&run.TriggerSource,
			&run.StartedAt,
			&endedAt,
			&error,
			&run.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		run.SatelliteID = satelliteID
		run.CurrentStep = currentStep
		run.EndedAt = endedAt
		run.Error = error
		runs = append(runs, &run)
	}

	return runs, total, rows.Err()
}

// CreatePipelineStepRun inserts a new pipeline step run into the database.
// Returns the created PipelineStepRun with generated ID or an error.
func CreatePipelineStepRun(ctx context.Context, dbPool *pgxpool.Pool, stepRun *PipelineStepRun) error {
	query := `
		INSERT INTO pipeline_step_runs (pipeline_run_id, step_order, status)
		VALUES ($1, $2, $3)
		RETURNING id, created_at
	`

	err := dbPool.QueryRow(ctx, query,
		stepRun.PipelineRunID,
		stepRun.StepOrder,
		stepRun.Status,
	).Scan(
		&stepRun.ID,
		&stepRun.CreatedAt,
	)
	if err != nil {
		return err
	}
	return nil
}

// UpdatePipelineStepRun updates a pipeline step run's fields (status, agent_run_id, input_text, output_text, started_at, ended_at, error).
// Returns an error if the update fails.
func UpdatePipelineStepRun(ctx context.Context, dbPool *pgxpool.Pool, stepRun *PipelineStepRun) error {
	query := `
		UPDATE pipeline_step_runs
		SET status = $1, agent_run_id = $2, input_text = $3, output_text = $4, started_at = $5, ended_at = $6, error = $7
		WHERE id = $8
		RETURNING created_at
	`

	err := dbPool.QueryRow(ctx, query,
		stepRun.Status,
		stepRun.AgentRunID,
		stepRun.InputText,
		stepRun.OutputText,
		stepRun.StartedAt,
		stepRun.EndedAt,
		stepRun.Error,
		stepRun.ID,
	).Scan(
		&stepRun.CreatedAt,
	)
	if err != nil {
		return err
	}
	return nil
}
