/**
 * Pipeline React Hooks
 * 
 * Custom React hooks for managing pipeline state and API interactions.
 * Provides loading states, error handling, and refetch capabilities.
 */

import { useState, useEffect, useCallback } from 'react';
import { apiRequest } from '../api/client';
import type {
  Pipeline,
  PipelineRun,
  PipelineStep,
  CreatePipelineRequest,
  UpdatePipelineRequest,
} from '../api/pipelines';

/**
 * usePipelines — Fetch pipelines list with optional satellite filtering
 * 
 * GET /api/v1/pipelines?satellite_id=X
 */
export function usePipelines(satelliteId?: string) {
  const [pipelines, setPipelines] = useState<Pipeline[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [total, setTotal] = useState(0);

  const fetchPipelines = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const params = new URLSearchParams();
      if (satelliteId) {
        params.append('satellite_id', satelliteId);
      }
      const query = params.toString();
      const endpoint = `/pipelines${query ? `?${query}` : ''}`;
      const response = await apiRequest<{ pipelines: Pipeline[], total: number }>(endpoint);
      setPipelines(response.pipelines || []);
      setTotal(response.total || 0);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setPipelines([]);
      setTotal(0);
    } finally {
      setIsLoading(false);
    }
  }, [satelliteId]);

  useEffect(() => {
    fetchPipelines();
  }, [fetchPipelines]);

  return {
    pipelines,
    total,
    isLoading,
    error,
    refetch: fetchPipelines,
  };
}

/**
 * usePipeline — Fetch single pipeline with its steps
 * 
 * GET /api/v1/pipelines/:id
 */
export function usePipeline(id: string) {
  const [pipeline, setPipeline] = useState<Pipeline | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchPipeline = useCallback(async () => {
    if (!id) return;

    setIsLoading(true);
    setError(null);
    try {
      const response = await apiRequest<Pipeline>(`/pipelines/${id}`);
      setPipeline(response);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setPipeline(null);
    } finally {
      setIsLoading(false);
    }
  }, [id]);

  useEffect(() => {
    fetchPipeline();
  }, [fetchPipeline]);

  return {
    pipeline,
    isLoading,
    error,
    refetch: fetchPipeline,
  };
}

/**
 * usePipelineRuns — Fetch run history for a pipeline with pagination
 * 
 * GET /api/v1/pipelines/:pipelineId/runs
 */
export function usePipelineRuns(pipelineId: string) {
  const [runs, setRuns] = useState<PipelineRun[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [total, setTotal] = useState(0);

  const fetchRuns = useCallback(async () => {
    if (!pipelineId) return;

    setIsLoading(true);
    setError(null);
    try {
      const response = await apiRequest<{ runs: PipelineRun[], total: number }>(`/pipelines/${pipelineId}/runs`);
      setRuns(response.runs || []);
      setTotal(response.total || 0);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setRuns([]);
      setTotal(0);
    } finally {
      setIsLoading(false);
    }
  }, [pipelineId]);

  useEffect(() => {
    fetchRuns();
  }, [fetchRuns]);

  return {
    runs,
    total,
    isLoading,
    error,
    refetch: fetchRuns,
  };
}

// ============================================================================
// Action Hooks
// ============================================================================

/**
 * useCreatePipeline — Create a new pipeline
 * 
 * POST /api/v1/pipelines
 */
export function useCreatePipeline() {
  const [isCreating, setIsCreating] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const createPipeline = useCallback(async (request: CreatePipelineRequest): Promise<Pipeline | null> => {
    setIsCreating(true);
    setError(null);
    try {
      const response = await apiRequest<Pipeline>('/pipelines', {
        method: 'POST',
        body: JSON.stringify(request),
      });
      return response;
    } catch (err) {
      const e = err instanceof Error ? err : new Error(String(err));
      setError(e);
      return null;
    } finally {
      setIsCreating(false);
    }
  }, []);

  return {
    createPipeline,
    isCreating,
    error,
  };
}

/**
 * useUpdatePipeline — Update an existing pipeline
 * 
 * PUT /api/v1/pipelines/:id
 */
export function useUpdatePipeline() {
  const [isUpdating, setIsUpdating] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const updatePipeline = useCallback(async (id: string, request: UpdatePipelineRequest): Promise<Pipeline | null> => {
    setIsUpdating(true);
    setError(null);
    try {
      const response = await apiRequest<Pipeline>(`/pipelines/${id}`, {
        method: 'PUT',
        body: JSON.stringify(request),
      });
      return response;
    } catch (err) {
      const e = err instanceof Error ? err : new Error(String(err));
      setError(e);
      return null;
    } finally {
      setIsUpdating(false);
    }
  }, []);

  return {
    updatePipeline,
    isUpdating,
    error,
  };
}

/**
 * useDeletePipeline — Delete a pipeline
 * 
 * DELETE /api/v1/pipelines/:id
 */
export function useDeletePipeline() {
  const [isDeleting, setIsDeleting] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const deletePipeline = useCallback(async (id: string): Promise<boolean> => {
    setIsDeleting(true);
    setError(null);
    try {
      await apiRequest<{ status: string }>(`/pipelines/${id}`, {
        method: 'DELETE',
      });
      return true;
    } catch (err) {
      const e = err instanceof Error ? err : new Error(String(err));
      setError(e);
      return false;
    } finally {
      setIsDeleting(false);
    }
  }, []);

  return {
    deletePipeline,
    isDeleting,
    error,
  };
}

/**
 * useRunPipeline — Trigger a pipeline run
 * 
 * POST /api/v1/pipelines/:id/run
 */
export function useRunPipeline() {
  const [isRunning, setIsRunning] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const runPipeline = useCallback(async (id: string, satelliteId?: string): Promise<string | null> => {
    setIsRunning(true);
    setError(null);
    try {
      const body = satelliteId ? { satellite_id: satelliteId } : {};
      const response = await apiRequest<{ run_id: string }>(`/pipelines/${id}/run`, {
        method: 'POST',
        body: JSON.stringify(body),
      });
      return response.run_id;
    } catch (err) {
      const e = err instanceof Error ? err : new Error(String(err));
      setError(e);
      return null;
    } finally {
      setIsRunning(false);
    }
  }, []);

  return {
    runPipeline,
    isRunning,
    error,
  };
}

/**
 * usePipelineSchedule — Manage pipeline schedule
 * 
 * POST /api/v1/pipelines/:id/schedule
 * DELETE /api/v1/pipelines/:id/schedule
 */
export function usePipelineSchedule() {
  const [isScheduling, setIsScheduling] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const setSchedule = useCallback(async (id: string, cronExpr: string): Promise<boolean> => {
    setIsScheduling(true);
    setError(null);
    try {
      await apiRequest<{ status: string }>(`/pipelines/${id}/schedule`, {
        method: 'POST',
        body: JSON.stringify({ cron: cronExpr }),
      });
      return true;
    } catch (err) {
      const e = err instanceof Error ? err : new Error(String(err));
      setError(e);
      return false;
    } finally {
      setIsScheduling(false);
    }
  }, []);

  const removeSchedule = useCallback(async (id: string): Promise<boolean> => {
    setIsScheduling(true);
    setError(null);
    try {
      await apiRequest<{ status: string }>(`/pipelines/${id}/schedule`, {
        method: 'DELETE',
      });
      return true;
    } catch (err) {
      const e = err instanceof Error ? err : new Error(String(err));
      setError(e);
      return false;
    } finally {
      setIsScheduling(false);
    }
  }, []);

  return {
    setSchedule,
    removeSchedule,
    isScheduling,
    error,
  };
}

// ============================================================================
// Types Export
// ============================================================================

export type { Pipeline, PipelineStep, PipelineRun, CreatePipelineRequest, UpdatePipelineRequest };
