import React, { useState, useEffect, useRef, ReactNode } from 'react';
import { useAgentRunStream } from '../hooks/useAgents';

// ============================================================================
// Types
// ============================================================================

export type RunStatus = 'running' | 'completed' | 'failed' | 'timeout';

export interface ToolCall {
  id: string;
  name: string;
  arguments: Record<string, unknown> | string;
  result?: string;
  duration?: number; // in milliseconds
  startedAt: Date;
  endedAt?: Date;
}

export interface TokenUsage {
  input: number;
  output: number;
}

export interface RunEvent {
  id: string;
  status: RunStatus;
  startedAt: Date;
  endedAt?: Date;
  toolCalls: ToolCall[];
  output: string;
  tokenUsage?: TokenUsage;
  error?: string;
  result?: string;
}

export interface AgentRunViewProps {
  run?: RunEvent;
  runId?: string; // when provided, enables live SSE streaming
  className?: string;
  onEvent?: (event: Partial<RunEvent>) => void;
}

// ============================================================================
// Status Badge Component
// ============================================================================

const StatusBadge: React.FC<{ status: RunStatus }> = ({ status }) => {
  const getStatusConfig = () => {
    switch (status) {
      case 'running':
        return { label: 'Running', className: 'status-badge--running' };
      case 'completed':
        return { label: 'Completed', className: 'status-badge--completed' };
      case 'failed':
        return { label: 'Failed', className: 'status-badge--failed' };
      case 'timeout':
        return { label: 'Timeout', className: 'status-badge--timeout' };
      default:
        return { label: 'Unknown', className: '' };
    }
  };

  const config = getStatusConfig();

  return React.createElement('span', {
    className: `status-badge ${config.className}`,
  }, config.label);
};

// ============================================================================
// Tool Call Card Component
// ============================================================================

const ToolCallCard: React.FC<{ toolCall: ToolCall }> = ({ toolCall }) => {
  const [isExpanded, setIsExpanded] = useState(false);

  const formatDuration = (ms?: number): string => {
    if (ms === undefined) return '';
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  };

  const formatArguments = (args: Record<string, unknown> | string): string => {
    if (typeof args === 'string') {
      try {
        const parsed = JSON.parse(args);
        return JSON.stringify(parsed, null, 2);
      } catch {
        return args;
      }
    }
    return JSON.stringify(args, null, 2);
  };

  const duration = toolCall.endedAt && toolCall.startedAt
    ? toolCall.endedAt.getTime() - toolCall.startedAt.getTime()
    : toolCall.duration;

  return React.createElement('div', { className: 'tool-call-card' },
    // Header with tool name and duration badge
    React.createElement('div', { className: 'tool-call-header' },
      React.createElement('span', { className: 'tool-call-name' }, toolCall.name),
      duration !== undefined && React.createElement('span', {
        className: 'tool-call-duration'
      }, formatDuration(duration))
    ),
    // Collapsible arguments section
    React.createElement('div', { className: 'tool-call-args-wrapper' },
      React.createElement('button', {
        className: 'tool-call-expand-btn',
        onClick: () => setIsExpanded(!isExpanded),
        'aria-expanded': isExpanded,
      },
        React.createElement('span', null, isExpanded ? '▼ Arguments' : '▶ Arguments')
      ),
      isExpanded && React.createElement('pre', {
        className: 'tool-call-args'
      }, formatArguments(toolCall.arguments))
    ),
    // Result section
    toolCall.result && React.createElement('div', { className: 'tool-call-result' },
      React.createElement('div', { className: 'tool-call-result-label' }, 'Result:'),
      React.createElement('pre', { className: 'tool-call-result-content' },
        typeof toolCall.result === 'string'
          ? (toolCall.result.length > 500
            ? toolCall.result.substring(0, 500) + '...'
            : toolCall.result)
          : JSON.stringify(toolCall.result, null, 2)
      )
    )
  );
};

// ============================================================================
// Token Usage Counter Component
// ============================================================================

const TokenUsageCounter: React.FC<{ usage?: TokenUsage }> = ({ usage }) => {
  if (!usage) return null;

  const total = usage.input + usage.output;

  return React.createElement('div', { className: 'token-usage' },
    React.createElement('div', { className: 'token-usage-item' },
      React.createElement('span', { className: 'token-usage-label' }, 'Input:'),
      React.createElement('span', { className: 'token-usage-value' }, usage.input.toLocaleString())
    ),
    React.createElement('div', { className: 'token-usage-item' },
      React.createElement('span', { className: 'token-usage-label' }, 'Output:'),
      React.createElement('span', { className: 'token-usage-value' }, usage.output.toLocaleString())
    ),
    React.createElement('div', { className: 'token-usage-item token-usage-total' },
      React.createElement('span', { className: 'token-usage-label' }, 'Total:'),
      React.createElement('span', { className: 'token-usage-value' }, total.toLocaleString())
    )
  );
};

// ============================================================================
// Streaming Output Panel Component
// ============================================================================

const StreamingOutputPanel: React.FC<{ output: string; autoScroll?: boolean }> = ({
  output,
  autoScroll = true
}) => {
  const outputRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (autoScroll && outputRef.current) {
      outputRef.current.scrollTop = outputRef.current.scrollHeight;
    }
  }, [output, autoScroll]);

  return React.createElement('div', {
    className: 'streaming-output-panel',
    ref: outputRef
  },
    React.createElement('div', { className: 'streaming-output-header' }, 'Output'),
    React.createElement('pre', { className: 'streaming-output-content' },
      typeof output === 'string' ? (output || 'Waiting for output...') : JSON.stringify(output, null, 2)
    )
  );
};

// ============================================================================
// Run Summary Panel Component
// ============================================================================

const RunSummaryPanel: React.FC<{ run: RunEvent }> = ({ run }) => {
  const formatDuration = (start: Date, end?: Date): string => {
    if (!end) return 'N/A';
    const ms = end.getTime() - start.getTime();
    if (ms < 1000) return `${ms}ms`;
    if (ms < 60000) return `${(ms / 1000).toFixed(2)}s`;
    const minutes = Math.floor(ms / 60000);
    const seconds = ((ms % 60000) / 1000).toFixed(0);
    return `${minutes}m ${seconds}s`;
  };

  const totalTokens = run.tokenUsage
    ? run.tokenUsage.input + run.tokenUsage.output
    : 0;

  return React.createElement('div', { className: 'run-summary-panel' },
    React.createElement('h3', { className: 'run-summary-title' }, 'Run Summary'),
    React.createElement('div', { className: 'run-summary-grid' },
      React.createElement('div', { className: 'run-summary-item' },
        React.createElement('span', { className: 'run-summary-label' }, 'Duration'),
        React.createElement('span', { className: 'run-summary-value' },
          formatDuration(run.startedAt, run.endedAt)
        )
      ),
      React.createElement('div', { className: 'run-summary-item' },
        React.createElement('span', { className: 'run-summary-label' }, 'Tokens'),
        React.createElement('span', { className: 'run-summary-value' }, totalTokens.toLocaleString())
      ),
      React.createElement('div', { className: 'run-summary-item' },
        React.createElement('span', { className: 'run-summary-label' }, 'Tool Calls'),
        React.createElement('span', { className: 'run-summary-value' }, run.toolCalls.length)
      ),
      React.createElement('div', { className: 'run-summary-item' },
        React.createElement('span', { className: 'run-summary-label' }, 'Status'),
        React.createElement('span', { className: 'run-summary-value' },
          React.createElement(StatusBadge, { status: run.status })
        )
      )
    ),
    run.result && React.createElement('div', { className: 'run-summary-result' },
      React.createElement('div', { className: 'run-summary-result-label' }, 'Final Result:'),
      React.createElement('pre', { className: 'run-summary-result-content' },
        typeof run.result === 'string' ? run.result : JSON.stringify(run.result, null, 2))
    ),
    run.error && React.createElement('div', { className: 'run-summary-error' },
      React.createElement('div', { className: 'run-summary-error-label' }, 'Error:'),
      React.createElement('pre', { className: 'run-summary-error-content' },
        typeof run.error === 'string' ? run.error : JSON.stringify(run.error, null, 2))
    )
  );
};

// ============================================================================
// Main AgentRunView Component
// ============================================================================

const AgentRunView: React.FC<AgentRunViewProps> = ({ run, runId, className = '', onEvent }) => {
  const { state: liveState, connected } = useAgentRunStream(runId ?? null);

  // Build effective run: live state takes priority over static prop when runId present
  const effectiveRun: RunEvent | undefined = (runId && liveState) ? {
    id: runId,
    status: liveState.status,
    startedAt: liveState.startedAt ?? run?.startedAt ?? new Date(),
    endedAt: liveState.endedAt,
    toolCalls: liveState.toolCalls,
    output: liveState.output + (liveState.streamingText ? (liveState.output ? '\n\n' : '') + liveState.streamingText : ''),
    tokenUsage: liveState.tokenUsage,
    error: liveState.error,
    result: liveState.result,
  } : run;

  const [localRun, setLocalRun] = useState<RunEvent | undefined>(effectiveRun);
  useEffect(() => { setLocalRun(effectiveRun); }, [effectiveRun]);

  // Sync external run prop
  useEffect(() => {
    setLocalRun(run);
  }, [run]);

  // Handle incoming events (for WebSocket subscription pattern)
  useEffect(() => {
    if (!onEvent) return;

    const handleEvent = (event: Partial<RunEvent>) => {
      setLocalRun(prev => {
        if (!prev && event.id) {
          return {
            id: event.id,
            status: event.status || 'running',
            startedAt: event.startedAt || new Date(),
            toolCalls: event.toolCalls || [],
            output: event.output || '',
            tokenUsage: event.tokenUsage,
            error: event.error,
            result: event.result,
            endedAt: event.endedAt,
          };
        }
        if (prev) {
          return {
            ...prev,
            ...event,
            toolCalls: event.toolCalls ? [...prev.toolCalls, ...event.toolCalls] : prev.toolCalls,
            output: event.output !== undefined ? prev.output + event.output : prev.output,
          };
        }
        return prev;
      });
    };

    // Listen for custom events (WebSocket pattern)
    const handler = (e: CustomEvent) => handleEvent(e.detail);
    window.addEventListener('agent-run-event', handler as EventListener);
    return () => window.removeEventListener('agent-run-event', handler as EventListener);
  }, [onEvent]);

  const currentRun = localRun;

  if (!currentRun) {
    return React.createElement('div', { className: `agent-run-view ${className}` },
      React.createElement('div', { className: 'agent-run-view-empty' },
        'No active agent run'
      )
    );
  }

  return React.createElement('div', { className: `agent-run-view ${className}` },
    // Header with status
    React.createElement('div', { className: 'agent-run-view-header' },
      React.createElement('h2', { className: 'agent-run-view-title' }, 'Agent Run'),
      React.createElement(StatusBadge, { status: currentRun.status }),
      runId && !liveState?.historyLoaded && React.createElement('span', { className: 'live-badge' }, 'Loading...'),
      runId && liveState?.historyLoaded && connected && currentRun?.status === 'running' &&
      React.createElement('span', { className: 'live-badge live-badge--active' }, '● Live')
    ),
    // Token usage
    React.createElement(TokenUsageCounter, { usage: currentRun.tokenUsage }),
    // Main content area
    React.createElement('div', { className: 'agent-run-view-content' },
      // Tool calls section
      React.createElement('div', { className: 'agent-run-view-section' },
        React.createElement('h3', { className: 'section-title' },
          `Tool Calls (${currentRun.toolCalls.length})`
        ),
        currentRun.toolCalls.length > 0
          ? React.createElement('div', { className: 'tool-calls-list' },
            ...currentRun.toolCalls.map((toolCall, index) =>
              React.createElement(ToolCallCard, { key: toolCall.id || index, toolCall })
            )
          )
          : React.createElement('div', { className: 'empty-message' }, 'No tool calls yet')
      ),
      // Streaming output section
      React.createElement('div', { className: 'agent-run-view-section' },
        React.createElement(StreamingOutputPanel, { output: currentRun.output })
      )
    ),
    // Run summary on completion
    (currentRun.status === 'completed' || currentRun.status === 'failed' || currentRun.status === 'timeout') &&
    React.createElement(RunSummaryPanel, { run: currentRun })
  );
};

export default AgentRunView;
