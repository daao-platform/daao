/**
 * Pipeline API Client
 * 
 * TypeScript types and API functions for pipeline operations.
 * Provides functions for creating, managing, and running pipelines.
 */

import { apiRequest } from './client';

// ============================================================================
// Types
// ============================================================================

/**
 * Pipeline step configuration
 */
export interface PipelineStep {
  id: string;
  pipeline_id: string;
  step_order: number;
  agent_id: string;
  input_mode: 'none' | 'previous' | 'manual';
  output_mode: 'none' | 'pass' | 'collect';
  config: Record<string, unknown>;
}

/**
 * Pipeline step run status
 */
export interface PipelineStepRun {
  step_id: string;
  step_order: number;
  agent_run_id?: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'skipped';
  started_at?: string;
  ended_at?: string;
  output?: string;
  error?: string;
}

/**
 * Pipeline run status
 */
export type PipelineRunStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';

/**
 * Pipeline trigger source
 */
export type TriggerSource = 'manual' | 'schedule' | 'webhook';

/**
 * Pipeline run
 */
export interface PipelineRun {
  id: string;
  pipeline_id: string;
  satellite_id: string;
  status: PipelineRunStatus;
  current_step: number;
  trigger_source: TriggerSource;
  started_at: string;
  ended_at?: string;
  error?: string;
  step_runs: PipelineStepRun[];
}

/**
 * Pipeline model
 */
export interface Pipeline {
  id: string;
  name: string;
  description: string;
  satellite_id: string;
  on_failure: 'abort' | 'continue' | 'retry';
  max_retries: number;
  schedule?: string;
  is_enabled: boolean;
  steps: PipelineStep[];
  created_at: string;
  updated_at: string;
}

// ============================================================================
// Request Types
// ============================================================================

/**
 * Create pipeline request payload
 */
export interface CreatePipelineRequest {
  name: string;
  description?: string;
  satellite_id: string;
  on_failure?: 'abort' | 'continue' | 'retry';
  max_retries?: number;
  schedule?: string;
  is_enabled?: boolean;
  steps?: Omit<PipelineStep, 'id' | 'pipeline_id'>[];
}

/**
 * Update pipeline request payload
 */
export interface UpdatePipelineRequest {
  name?: string;
  description?: string;
  satellite_id?: string;
  on_failure?: 'abort' | 'continue' | 'retry';
  max_retries?: number;
  schedule?: string;
  is_enabled?: boolean;
  steps?: Omit<PipelineStep, 'id' | 'pipeline_id'>[];
}

// ============================================================================
// API Functions
// ============================================================================

/**
 * Create a new pipeline
 * 
 * POST /api/v1/pipelines
 */
export async function createPipeline(req: CreatePipelineRequest): Promise<Pipeline> {
  return apiRequest<Pipeline>('/pipelines', {
    method: 'POST',
    body: JSON.stringify(req),
  });
}

/**
 * Get a pipeline by ID
 * 
 * GET /api/v1/pipelines/:id
 */
export async function getPipeline(id: string): Promise<Pipeline> {
  return apiRequest<Pipeline>(`/pipelines/${id}`);
}

/**
 * List pipelines with optional filtering and pagination
 * 
 * GET /api/v1/pipelines
 */
export interface ListPipelinesParams {
  limit?: number;
  offset?: number;
  satellite_id?: string;
}

export async function listPipelines(params?: ListPipelinesParams): Promise<{ pipelines: Pipeline[], total: number }> {
  const queryParams = new URLSearchParams();

  if (params?.limit !== undefined) {
    queryParams.append('limit', params.limit.toString());
  }
  if (params?.offset !== undefined) {
    queryParams.append('offset', params.offset.toString());
  }
  if (params?.satellite_id) {
    queryParams.append('satellite_id', params.satellite_id);
  }

  const queryString = queryParams.toString();
  const endpoint = `/pipelines${queryString ? `?${queryString}` : ''}`;

  return apiRequest<{ pipelines: Pipeline[], total: number }>(endpoint);
}

/**
 * Update a pipeline
 * 
 * PUT /api/v1/pipelines/:id
 */
export async function updatePipeline(id: string, req: UpdatePipelineRequest): Promise<Pipeline> {
  return apiRequest<Pipeline>(`/pipelines/${id}`, {
    method: 'PUT',
    body: JSON.stringify(req),
  });
}

/**
 * Delete a pipeline
 * 
 * DELETE /api/v1/pipelines/:id
 */
export async function deletePipeline(id: string): Promise<void> {
  await apiRequest<{ status: string }>(`/pipelines/${id}`, {
    method: 'DELETE',
  });
}

/**
 * Trigger a pipeline run
 * 
 * POST /api/v1/pipelines/:id/run
 */
export async function runPipeline(id: string, satelliteId?: string): Promise<{ run_id: string }> {
  const body = satelliteId ? { satellite_id: satelliteId } : {};
  return apiRequest<{ run_id: string }>(`/pipelines/${id}/run`, {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

/**
 * List pipeline runs with pagination
 * 
 * GET /api/v1/pipelines/:pipelineId/runs
 */
export interface ListPipelineRunsParams {
  limit?: number;
  offset?: number;
}

export async function listPipelineRuns(pipelineId: string, params?: ListPipelineRunsParams): Promise<{ runs: PipelineRun[], total: number }> {
  const queryParams = new URLSearchParams();

  if (params?.limit !== undefined) {
    queryParams.append('limit', params.limit.toString());
  }
  if (params?.offset !== undefined) {
    queryParams.append('offset', params.offset.toString());
  }

  const queryString = queryParams.toString();
  const endpoint = `/pipelines/${pipelineId}/runs${queryString ? `?${queryString}` : ''}`;

  return apiRequest<{ runs: PipelineRun[], total: number }>(endpoint);
}

/**
 * Get a specific pipeline run
 * 
 * GET /api/v1/pipelines/runs/:runId
 */
export async function getPipelineRun(runId: string): Promise<PipelineRun> {
  return apiRequest<PipelineRun>(`/pipelines/runs/${runId}`);
}

/**
 * Set pipeline schedule (cron expression)
 * 
 * POST /api/v1/pipelines/:id/schedule
 */
export async function setPipelineSchedule(id: string, cronExpr: string): Promise<void> {
  await apiRequest<{ status: string }>(`/pipelines/${id}/schedule`, {
    method: 'POST',
    body: JSON.stringify({ cron: cronExpr }),
  });
}

/**
 * Delete pipeline schedule
 * 
 * DELETE /api/v1/pipelines/:id/schedule
 */
export async function deletePipelineSchedule(id: string): Promise<void> {
  await apiRequest<{ status: string }>(`/pipelines/${id}/schedule`, {
    method: 'DELETE',
  });
}

// ============================================================================
// Exports
// ============================================================================

export default {
  createPipeline,
  getPipeline,
  listPipelines,
  updatePipeline,
  deletePipeline,
  runPipeline,
  listPipelineRuns,
  getPipelineRun,
  setPipelineSchedule,
  deletePipelineSchedule,
};
