import { useState, useEffect, useCallback, useRef } from 'react';
import { apiRequest } from '../api/client';
import { ToolCall } from '../components/AgentRunView';

// ============================================================================
// Types
// ============================================================================

/**
 * Agent definition from the registry
 */
export interface AgentDefinition {
  id: string;
  name: string;
  display_name: string;
  description: string;
  version: string;
  type?: string;
  category?: string;
  provider?: string;
  model?: string;
  system_prompt?: string;
  tools_config?: Record<string, unknown> | string;
  guardrails?: Record<string, unknown> | string;
  schedule?: string;
  trigger?: string;
  output_config?: Record<string, unknown>;
  routing?: string;
  is_builtin?: boolean;
  is_enterprise?: boolean;
  icon?: string;
  author?: string;
  created_at: string;
  updated_at: string;
}

/**
 * Agent run history entry
 */
export interface AgentRun {
  id: string;
  agent_id: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
  started_at: string;
  ended_at?: string;
  total_tokens?: number;
  estimated_cost?: number;
  tool_call_count?: number;
  input?: Record<string, unknown>;
  output?: Record<string, unknown>;
  error?: string;
}

/**
 * Deploy agent request payload
 */
export interface DeployRequest {
  satellite_id: string;
  session_id?: string;
  config?: Record<string, unknown>;
  secrets?: Record<string, string>;
}

/**
 * Deploy agent response
 */
export interface DeployResponse {
  run_id: string;
  agent_id: string;
  satellite_id: string;
  session_id?: string;
  status: string;
}

/**
 * Create agent request
 */
export interface CreateAgentRequest {
  name: string;
  display_name: string;
  description?: string;
  type: string;
  category: string;
  provider: string;
  model: string;
  system_prompt: string;
  icon?: string;
  tools_config?: Record<string, unknown>;
  guardrails?: Record<string, unknown>;
  schedule?: string;
  trigger?: string;
  routing?: string;
}

/**
 * Update agent request
 */
export interface UpdateAgentRequest {
  display_name?: string;
  description?: string;
  type?: string;
  category?: string;
  provider?: string;
  model?: string;
  system_prompt?: string;
  tools_config?: Record<string, unknown>;
  guardrails?: Record<string, unknown>;
  schedule?: string;
  trigger?: string;
  routing?: string;
}

/**
 * Filters for useAgents hook
 */
export interface AgentFilters {
  category?: string;
  type?: string;
}

/**
 * Agent version summary for version history
 */
export interface AgentVersionSummary {
  id: string;
  version: string;
  change_summary?: string;
  created_by?: string;
  created_at: string;
}

// ============================================================================
// Agent Stream Event Types
// ============================================================================

export interface AgentStreamEventPayload {
  delta?: string;
  tool?: string;
  arguments?: Record<string, unknown>;
  result?: string;
  duration_ms?: number;
  total_tokens?: number;
  error?: string;
  [key: string]: unknown;
}

export interface AgentStreamEvent {
  id: string;
  run_id: string;
  event_type: string;
  payload: AgentStreamEventPayload;
  sequence: number;
  created_at: string;
}

export interface LiveRunState {
  status: 'running' | 'completed' | 'failed' | 'timeout';
  output: string;          // committed text from completed messages
  streamingText: string;   // in-progress text from current message_update stream
  toolCalls: ToolCall[];
  tokenUsage?: { input: number; output: number };
  error?: string;
  result?: string;
  startedAt?: Date;
  endedAt?: Date;
  historyLoaded: boolean;
}

// ============================================================================
// Hooks
// ============================================================================

/**
 * useAgents — Fetch agent definitions with optional category/type filtering
 * 
 * GET /api/v1/agents?category=X&type=Y
 */
export function useAgents(filters?: AgentFilters) {
  const [agents, setAgents] = useState<AgentDefinition[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchAgents = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const params = new URLSearchParams();
      if (filters?.category && filters.category !== 'all') {
        params.append('category', filters.category);
      }
      if (filters?.type) {
        params.append('type', filters.type);
      }
      const query = params.toString();
      const endpoint = query ? `/agents?${query}` : '/agents';
      const response = await apiRequest<{ items: AgentDefinition[], total: number }>(endpoint);
      setAgents(response.items || []);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setAgents([]);
    } finally {
      setIsLoading(false);
    }
  }, [filters?.category, filters?.type]);

  useEffect(() => {
    fetchAgents();
  }, [fetchAgents]);

  return {
    agents,
    isLoading,
    error,
    refetch: fetchAgents,
  };
}

/**
 * useAgentDetail — Fetch agent details and run history
 * 
 * GET /api/v1/agents/:id
 * GET /api/v1/agents/:id/runs
 */
export function useAgentDetail(id: string) {
  const [agent, setAgent] = useState<AgentDefinition | null>(null);
  const [runs, setRuns] = useState<AgentRun[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchAgentDetail = useCallback(async () => {
    if (!id) return;

    setIsLoading(true);
    setError(null);
    try {
      const [agentResponse, runsResponse] = await Promise.all([
        apiRequest<{ agent: AgentDefinition }>(`/agents/${id}`),
        apiRequest<{ runs: AgentRun[] }>(`/agents/${id}/runs`),
      ]);
      setAgent(agentResponse.agent);
      setRuns(runsResponse.runs || []);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setAgent(null);
      setRuns([]);
    } finally {
      setIsLoading(false);
    }
  }, [id]);

  useEffect(() => {
    fetchAgentDetail();
  }, [fetchAgentDetail]);

  return {
    agent,
    runs,
    isLoading,
    error,
    refetch: fetchAgentDetail,
  };
}

/**
 * useAgentRuns — Fetch run history for a specific agent
 * 
 * GET /api/v1/agents/:id/runs
 */
export function useAgentRuns(agentId: string) {
  const [runs, setRuns] = useState<AgentRun[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchRuns = useCallback(async () => {
    if (!agentId) return;

    setIsLoading(true);
    setError(null);
    try {
      const response = await apiRequest<{ runs: AgentRun[] }>(`/agents/${agentId}/runs`);
      setRuns(response.runs || []);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setRuns([]);
    } finally {
      setIsLoading(false);
    }
  }, [agentId]);

  useEffect(() => {
    fetchRuns();
  }, [fetchRuns]);

  return {
    runs,
    isLoading,
    error,
    refetch: fetchRuns,
  };
}

/**
 * Agent run with context — includes agent/satellite/pipeline display names
 */
export interface AgentRunWithContext {
  id: string;
  agent_id: string;
  agent_name: string;
  satellite_id: string;
  satellite_name: string;
  status: string;
  trigger_source: string;
  started_at: string;
  ended_at?: string;
  total_tokens: number;
  estimated_cost: number;
  tool_call_count: number;
  pipeline_run_id?: string;
  pipeline_name?: string;
  step_order?: number;
}

/**
 * useAllAgentRuns — Fetch all agent runs across all agents with context
 *
 * GET /api/v1/runs?status=X&agent_id=Y
 */
export function useAllAgentRuns(filters?: { status?: string; agent_id?: string }) {
  const [runs, setRuns] = useState<AgentRunWithContext[]>([]);
  const [total, setTotal] = useState(0);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchRuns = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const params = new URLSearchParams();
      params.append('limit', '100');
      if (filters?.status && filters.status !== 'all') {
        params.append('status', filters.status);
      }
      if (filters?.agent_id) {
        params.append('agent_id', filters.agent_id);
      }
      const query = params.toString();
      const response = await apiRequest<{ runs: AgentRunWithContext[]; total: number }>(`/runs?${query}`);
      setRuns(response.runs || []);
      setTotal(response.total || 0);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setRuns([]);
    } finally {
      setIsLoading(false);
    }
  }, [filters?.status, filters?.agent_id]);

  useEffect(() => {
    fetchRuns();
  }, [fetchRuns]);

  return { runs, total, isLoading, error, refetch: fetchRuns };
}

/**
 * useDeployAgent — Deploy an agent to a satellite
 * 
 * POST /api/v1/agents/:id/deploy
 */
export function useDeployAgent() {
  const [isDeploying, setIsDeploying] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const deploy = useCallback(async (agentId: string, request?: DeployRequest): Promise<DeployResponse | null> => {
    setIsDeploying(true);
    setError(null);
    try {
      const response = await apiRequest<DeployResponse>(`/agents/${agentId}/deploy`, {
        method: 'POST',
        body: JSON.stringify(request || {}),
      });
      return response;
    } catch (err) {
      const error = err instanceof Error ? err : new Error(String(err));
      setError(error);
      return null;
    } finally {
      setIsDeploying(false);
    }
  }, []);

  return {
    deploy,
    isDeploying,
    error,
  };
}

/**
 * useCreateAgent — Create a new agent definition
 * 
 * POST /api/v1/agents
 */
export function useCreateAgent() {
  const [isCreating, setIsCreating] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const createAgent = useCallback(async (request: CreateAgentRequest): Promise<AgentDefinition | null> => {
    setIsCreating(true);
    setError(null);
    try {
      const response = await apiRequest<{ agent: AgentDefinition }>('/agents', {
        method: 'POST',
        body: JSON.stringify(request),
      });
      return response.agent;
    } catch (err) {
      const e = err instanceof Error ? err : new Error(String(err));
      setError(e);
      return null;
    } finally {
      setIsCreating(false);
    }
  }, []);

  return {
    createAgent,
    isCreating,
    error,
  };
}

/**
 * useUpdateAgent — Update an existing agent definition
 * 
 * PUT /api/v1/agents/:id
 */
export function useUpdateAgent() {
  const [isUpdating, setIsUpdating] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const updateAgent = useCallback(async (agentId: string, request: UpdateAgentRequest): Promise<AgentDefinition | null> => {
    setIsUpdating(true);
    setError(null);
    try {
      const response = await apiRequest<{ agent: AgentDefinition }>(`/agents/${agentId}`, {
        method: 'PUT',
        body: JSON.stringify(request),
      });
      return response.agent;
    } catch (err) {
      const e = err instanceof Error ? err : new Error(String(err));
      setError(e);
      return null;
    } finally {
      setIsUpdating(false);
    }
  }, []);

  return {
    updateAgent,
    isUpdating,
    error,
  };
}

/**
 * useDeleteAgent — Delete an agent definition
 * 
 * DELETE /api/v1/agents/:id
 */
export function useDeleteAgent() {
  const [isDeleting, setIsDeleting] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const deleteAgent = useCallback(async (agentId: string): Promise<boolean> => {
    setIsDeleting(true);
    setError(null);
    try {
      await apiRequest<{ status: string }>(`/agents/${agentId}`, {
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
    deleteAgent,
    isDeleting,
    error,
  };
}

/**
 * useAgentRunStream — Subscribe to agent run event stream
 * 
 * SSE endpoint: /api/v1/runs/:runId/stream
 */
export function useAgentRunStream(runId: string | null): {
  state: LiveRunState | null;
  connected: boolean;
  error: string | null;
} {
  const [state, setState] = useState<LiveRunState | null>(null);
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pendingToolCalls = useRef<Map<string, ToolCall>>(new Map());

  useEffect(() => {
    if (!runId) return;
    setState({ status: 'running', output: '', streamingText: '', toolCalls: [], historyLoaded: false });
    const es = new EventSource(`/api/v1/runs/${runId}/stream`);
    // Track seen sequence numbers to deduplicate events that arrive both
    // via history replay AND the live stream (the server subscribes to the
    // hub before replaying history to avoid gaps, causing overlap).
    const seenSeqs = new Set<number>();
    es.addEventListener('connected', () => setConnected(true));
    es.addEventListener('live_start', () => setState(prev => prev ? { ...prev, historyLoaded: true } : prev));
    const handleEvent = (raw: string) => {
      try {
        const event: AgentStreamEvent = JSON.parse(raw);
        // Deduplicate by sequence number — skip events we've already processed
        if (event.sequence > 0 && seenSeqs.has(event.sequence)) return;
        if (event.sequence > 0) seenSeqs.add(event.sequence);
        setState(prev => prev ? applyAgentEvent(prev, event, pendingToolCalls.current) : prev);
        // Close the stream when the agent finishes — no more events are coming
        if (event.event_type === 'agent_end') {
          es.close();
          setConnected(false);
        }
      } catch (e) { console.error('useAgentRunStream parse error', e); }
    };
    es.addEventListener('history', (e: MessageEvent) => handleEvent(e.data));
    es.addEventListener('agent_event', (e: MessageEvent) => handleEvent(e.data));
    es.onerror = () => { setConnected(false); setError('Stream connection lost'); };
    return () => { es.close(); setConnected(false); };
  }, [runId]);

  return { state, connected, error };
}

/**
 * useAgentVersions — Fetch version history for an agent
 * 
 * GET /api/v1/agents/:id/versions
 */
export function useAgentVersions(agentId: string) {
  const [versions, setVersions] = useState<AgentVersionSummary[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!agentId) return;
    setIsLoading(true);
    apiRequest<{ versions: AgentVersionSummary[] }>(`/agents/${agentId}/versions`)
      .then(data => setVersions(data.versions || []))
      .catch(err => setError(err instanceof Error ? err.message : String(err)))
      .finally(() => setIsLoading(false));
  }, [agentId]);

  return { versions, isLoading, error };
}

/**
 * useRollbackAgent — Rollback an agent to a previous version
 * 
 * POST /api/v1/agents/:id/rollback
 */
export function useRollbackAgent() {
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const rollback = useCallback(async (agentId: string, version: string) => {
    setIsLoading(true);
    setError(null);
    try {
      const result = await apiRequest<{ agent: AgentDefinition }>(`/agents/${agentId}/rollback`, {
        method: 'POST',
        body: JSON.stringify({ version }),
      });
      return result;
    } catch (err: any) {
      setError(err.message);
      throw err;
    } finally {
      setIsLoading(false);
    }
  }, []);

  return { rollback, isLoading, error };
}

/**
 * useExportAgent — Export an agent definition as YAML
 * 
 * GET /api/v1/agents/:id/export
 */
export function useExportAgent() {
  const exportAgent = useCallback(async (agentId: string, agentName: string) => {
    const response = await fetch(`/api/v1/agents/${agentId}/export`, {
      headers: { 'Authorization': `Bearer ${localStorage.getItem('token') || ''}` },
    });
    if (!response.ok) throw new Error('Export failed');
    const blob = await response.blob();
    const url = window.URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${agentName}.agent.yaml`;
    a.click();
    window.URL.revokeObjectURL(url);
  }, []);

  return { exportAgent };
}

/**
 * useImportAgent — Import an agent definition from YAML
 * 
 * POST /api/v1/agents/import
 */
export function useImportAgent() {
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const importAgent = useCallback(async (file: File) => {
    setIsLoading(true);
    setError(null);
    try {
      const formData = new FormData();
      formData.append('file', file);
      const response = await fetch('/api/v1/agents/import', {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${localStorage.getItem('token') || ''}` },
        body: formData,
      });
      if (!response.ok) {
        const errData = await response.json();
        throw new Error(errData.error || 'Import failed');
      }
      return await response.json();
    } catch (err: any) {
      setError(err.message);
      throw err;
    } finally {
      setIsLoading(false);
    }
  }, []);

  return { importAgent, isLoading, error };
}

/** Extract plain text from a pi message content (handles multiple formats). */
function extractText(content: unknown): string {
  if (typeof content === 'string') return content;
  if (Array.isArray(content)) {
    return content
      .filter((c: { type: string }) => c.type === 'text')
      .map((c: { text?: string }) => c.text ?? '')
      .join('');
  }
  // Fallback: if it's an object (shouldn't happen, but prevents React error #31)
  if (content && typeof content === 'object') {
    const obj = content as Record<string, unknown>;
    // Some providers nest text under .content or .text
    if (typeof obj.text === 'string') return obj.text;
    if (typeof obj.content === 'string') return obj.content;
    return JSON.stringify(content);
  }
  return '';
}

function applyAgentEvent(state: LiveRunState, event: AgentStreamEvent, pending: Map<string, ToolCall>): LiveRunState {
  const p = event.payload as Record<string, unknown>;
  switch (event.event_type) {

    case 'agent_start':
      return { ...state, startedAt: new Date(event.created_at) };

    // Pi sends the full accumulated message on each message_update — update streaming preview.
    case 'message_update': {
      const msg = p.message as { content?: unknown } | undefined;
      const delta = typeof p.delta === 'string' ? p.delta : '';
      const text = msg ? extractText(msg.content) : delta;
      return { ...state, streamingText: text };
    }

    // message_end: commit the final text from the completed message to output.
    case 'message_end': {
      const msg = p.message as { role?: string; content?: unknown; usage?: { input?: number; output?: number } } | undefined;
      // Extract token usage from Pi's message.usage field
      let newTokenUsage = state.tokenUsage;
      if (msg?.usage) {
        const inp = typeof msg.usage.input === 'number' ? msg.usage.input : 0;
        const out = typeof msg.usage.output === 'number' ? msg.usage.output : 0;
        newTokenUsage = { input: inp, output: out };
      }
      if (msg?.role === 'assistant') {
        const text = extractText(msg.content);
        if (text) return { ...state, output: state.output + (state.output ? '\n\n' : '') + text, streamingText: '', tokenUsage: newTokenUsage };
      }
      return { ...state, streamingText: '', tokenUsage: newTokenUsage };
    }

    // Pi's tool_execution_update: fired when a tool call starts (has toolCallId + args).
    case 'tool_execution_update': {
      const toolCallId = p.toolCallId as string | undefined;
      const toolName = p.toolName as string | undefined;
      const args = p.args as Record<string, unknown> | undefined;
      if (toolCallId && !pending.has(toolCallId)) {
        const tc: ToolCall = { id: toolCallId, name: toolName ?? 'unknown', arguments: args ?? {}, startedAt: new Date(event.created_at) };
        pending.set(toolCallId, tc);
        return { ...state, toolCalls: [...state.toolCalls, tc] };
      }
      return state;
    }

    // Legacy / also emitted by pi for tool lifecycle.
    case 'tool_execution_start': {
      const toolCallId = (p.toolCallId as string | undefined) ?? event.id;
      const name = (p.toolName as string | undefined) ?? (p.tool as string | undefined) ?? 'unknown';
      if (!pending.has(toolCallId)) {
        const tc: ToolCall = { id: toolCallId, name, arguments: (p.args ?? p.arguments ?? {}) as Record<string, unknown>, startedAt: new Date(event.created_at) };
        pending.set(toolCallId, tc);
        return { ...state, toolCalls: [...state.toolCalls, tc] };
      }
      return state;
    }

    case 'tool_execution_end': {
      const toolCallId = (p.toolCallId as string | undefined) ?? event.id;
      // Coerce result to string — raw Pi events may have result as an object
      const result = p.result == null ? undefined
        : typeof p.result === 'string' ? p.result
          : JSON.stringify(p.result);
      return {
        ...state, toolCalls: state.toolCalls.map(tc =>
          tc.id === toolCallId && !tc.endedAt
            ? { ...tc, result, endedAt: new Date(event.created_at), duration: p.duration_ms as number | undefined }
            : tc
        )
      };
    }

    case 'agent_end': {
      // Coerce result/error to strings — raw Pi events may have these as objects
      const endResult = p.result == null ? undefined
        : typeof p.result === 'string' ? p.result
          : JSON.stringify(p.result);
      const endError = p.error == null ? undefined
        : typeof p.error === 'string' ? p.error
          : JSON.stringify(p.error);
      return {
        ...state,
        status: p.error ? 'failed' : 'completed',
        result: endResult,
        error: endError,
        streamingText: '',
        endedAt: new Date(event.created_at),
      };
    }

    default:
      return state;
  }
}
