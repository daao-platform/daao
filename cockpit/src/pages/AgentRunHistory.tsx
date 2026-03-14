/**
 * AgentRunHistory — Browsable list of all agent runs across all agents
 *
 * Shows status, agent name, satellite, pipeline context, duration, tokens, tool calls.
 * Rows click through to /forge/run/:id for the full run replay viewer.
 */

import React, { useState, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAllAgentRuns, type AgentRunWithContext } from '../hooks/useAgents';

// ============================================================================
// Config-driven status badge (A2A-ready: just extend this map)
// ============================================================================

const STATUS_CONFIG: Record<string, { label: string; className: string }> = {
    running: { label: 'Running', className: 'badge--running' },
    completed: { label: 'Completed', className: 'badge--success' },
    failed: { label: 'Failed', className: 'badge--error' },
    timeout: { label: 'Timeout', className: 'badge--warning' },
    killed: { label: 'Killed', className: 'badge--muted' },
    pending: { label: 'Pending', className: 'badge--pending' },
    cancelled: { label: 'Cancelled', className: 'badge--muted' },
    // A2A states (future-compat):
    working: { label: 'Working', className: 'badge--running' },
    'input-required': { label: 'Input Required', className: 'badge--warning' },
    'auth-required': { label: 'Auth Required', className: 'badge--warning' },
    canceled: { label: 'Canceled', className: 'badge--muted' },
    rejected: { label: 'Rejected', className: 'badge--error' },
};

const StatusBadge: React.FC<{ status: string }> = ({ status }) => {
    const config = STATUS_CONFIG[status] || { label: status, className: 'badge--muted' };
    return (
        <span className={`badge ${config.className}`}>
            {(status === 'running' || status === 'working') && (
                <span className="badge__dot badge__dot--pulse" />
            )}
            {config.label}
        </span>
    );
};

// ============================================================================
// Helpers
// ============================================================================

function formatDuration(startedAt: string, endedAt?: string): string {
    const start = new Date(startedAt).getTime();
    const end = endedAt ? new Date(endedAt).getTime() : Date.now();
    const ms = end - start;
    if (ms < 0) return '—';
    const seconds = Math.floor(ms / 1000);
    const minutes = Math.floor(seconds / 60);
    const hours = Math.floor(minutes / 60);
    if (hours > 0) return `${hours}h ${minutes % 60}m`;
    if (minutes > 0) return `${minutes}m ${seconds % 60}s`;
    return `${seconds}s`;
}

function formatTimeAgo(dateStr: string): string {
    const diff = Date.now() - new Date(dateStr).getTime();
    const minutes = Math.floor(diff / 60000);
    const hours = Math.floor(minutes / 60);
    const days = Math.floor(hours / 24);
    if (days > 0) return `${days}d ago`;
    if (hours > 0) return `${hours}h ago`;
    if (minutes > 0) return `${minutes}m ago`;
    return 'Just now';
}

function formatTokens(tokens: number): string {
    if (tokens >= 1000000) return `${(tokens / 1000000).toFixed(1)}M`;
    if (tokens >= 1000) return `${(tokens / 1000).toFixed(1)}k`;
    return String(tokens);
}

// ============================================================================
// Filter Tabs
// ============================================================================

const FILTER_OPTIONS = [
    { value: 'all', label: 'All' },
    { value: 'running', label: 'Running' },
    { value: 'completed', label: 'Completed' },
    { value: 'failed', label: 'Failed' },
    { value: 'timeout', label: 'Timeout' },
];

// ============================================================================
// Main Component
// ============================================================================

const AgentRunHistory: React.FC = () => {
    const navigate = useNavigate();
    const [statusFilter, setStatusFilter] = useState('all');
    const [searchQuery, setSearchQuery] = useState('');

    const { runs, total, isLoading, error, refetch } = useAllAgentRuns({
        status: statusFilter !== 'all' ? statusFilter : undefined,
    });

    // Client-side search filter
    const filteredRuns = useMemo(() => {
        if (!searchQuery) return runs;
        const q = searchQuery.toLowerCase();
        return runs.filter(r =>
            r.agent_name.toLowerCase().includes(q) ||
            r.satellite_name.toLowerCase().includes(q) ||
            (r.pipeline_name && r.pipeline_name.toLowerCase().includes(q))
        );
    }, [runs, searchQuery]);

    const handleRowClick = (run: AgentRunWithContext) => {
        navigate(`/forge/run/${run.id}`);
    };

    return (
        <div className="run-history">
            {/* Header */}
            <div className="page-header">
                <div className="page-header__left">
                    <h1 className="page-header__title">Agent Runs</h1>
                    <span className="page-header__count">{total} total</span>
                </div>
                <div className="page-header__right">
                    <button
                        className="btn btn--ghost btn--sm"
                        onClick={refetch}
                        title="Refresh"
                    >
                        ↻ Refresh
                    </button>
                </div>
            </div>

            {/* Filters */}
            <div className="run-history__filters">
                <div className="filter-tabs">
                    {FILTER_OPTIONS.map(opt => (
                        <button
                            key={opt.value}
                            className={`filter-tab${statusFilter === opt.value ? ' filter-tab--active' : ''}`}
                            onClick={() => setStatusFilter(opt.value)}
                        >
                            {opt.label}
                        </button>
                    ))}
                </div>
                <input
                    type="text"
                    className="run-history__search"
                    placeholder="Search by agent, satellite, or pipeline..."
                    value={searchQuery}
                    onChange={e => setSearchQuery(e.target.value)}
                />
            </div>

            {/* Loading */}
            {isLoading && (
                <div className="run-history__loading">
                    <div className="spinner" />
                    <span>Loading runs...</span>
                </div>
            )}

            {/* Error */}
            {error && !isLoading && (
                <div className="run-history__error">
                    <span>⚠️ {error.message}</span>
                    <button className="btn btn--ghost btn--sm" onClick={refetch}>Retry</button>
                </div>
            )}

            {/* Empty state */}
            {!isLoading && !error && filteredRuns.length === 0 && (
                <div className="run-history__empty">
                    <div className="run-history__empty-icon">🚀</div>
                    <div className="run-history__empty-title">No agent runs yet</div>
                    <div className="run-history__empty-desc">
                        Deploy an agent from the Forge to get started.
                    </div>
                </div>
            )}

            {/* Table */}
            {!isLoading && filteredRuns.length > 0 && (
                <div className="run-history__table-wrapper">
                    <table className="run-history__table">
                        <thead>
                            <tr>
                                <th>Status</th>
                                <th>Agent</th>
                                <th>Satellite</th>
                                <th>Context</th>
                                <th>Started</th>
                                <th>Duration</th>
                                <th>Tokens</th>
                                <th>Tools</th>
                            </tr>
                        </thead>
                        <tbody>
                            {filteredRuns.map(run => (
                                <tr
                                    key={run.id}
                                    className="run-history__row"
                                    onClick={() => handleRowClick(run)}
                                    role="button"
                                    tabIndex={0}
                                    onKeyDown={e => e.key === 'Enter' && handleRowClick(run)}
                                >
                                    <td>
                                        <StatusBadge status={run.status} />
                                    </td>
                                    <td className="run-history__cell--agent">
                                        <span className="run-history__agent-name">
                                            {run.agent_name || 'Unknown'}
                                        </span>
                                    </td>
                                    <td className="run-history__cell--satellite">
                                        <span className="run-history__satellite-name">
                                            {run.satellite_name || '—'}
                                        </span>
                                    </td>
                                    <td className="run-history__cell--context">
                                        {run.pipeline_name ? (
                                            <span
                                                className="run-history__pipeline-badge"
                                                onClick={e => {
                                                    e.stopPropagation();
                                                    if (run.pipeline_run_id) {
                                                        navigate('/pipelines');
                                                    }
                                                }}
                                                title={`Pipeline: ${run.pipeline_name}`}
                                            >
                                                🔗 {run.pipeline_name}
                                                {run.step_order != null && (
                                                    <span className="run-history__step-badge">
                                                        Step {run.step_order}
                                                    </span>
                                                )}
                                            </span>
                                        ) : (
                                            <span className="run-history__standalone">—</span>
                                        )}
                                    </td>
                                    <td className="run-history__cell--time">
                                        <span title={new Date(run.started_at).toLocaleString()}>
                                            {formatTimeAgo(run.started_at)}
                                        </span>
                                    </td>
                                    <td className="run-history__cell--duration">
                                        {formatDuration(run.started_at, run.ended_at)}
                                    </td>
                                    <td className="run-history__cell--tokens">
                                        {run.total_tokens > 0 ? formatTokens(run.total_tokens) : '—'}
                                    </td>
                                    <td className="run-history__cell--tools">
                                        {run.tool_call_count > 0 ? run.tool_call_count : '—'}
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
        </div>
    );
};

export default AgentRunHistory;
