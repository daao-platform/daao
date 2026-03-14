/**
 * PipelineRunDetail — Shows detailed information about a pipeline run
 * including status, duration, trigger source, and step-by-step progress.
 */

import React, { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { getPipelineRun, type PipelineRun, type PipelineStepRun } from '../api/pipelines';
import { useToast } from './Toast';

// ============================================================================
// Types
// ============================================================================

interface PipelineRunDetailProps {
    runId?: string;
    run?: PipelineRun;
}

// ============================================================================
// Helper Functions
// ============================================================================

/** Format duration in ms to human readable string */
function formatDuration(ms: number): string {
    if (ms <= 0) return '—';
    const seconds = Math.floor(ms / 1000);
    const minutes = Math.floor(seconds / 60);
    const hours = Math.floor(minutes / 60);

    if (hours > 0) {
        return `${hours}h ${minutes % 60}m ${seconds % 60}s`;
    }
    if (minutes > 0) {
        return `${minutes}m ${seconds % 60}s`;
    }
    return `${seconds}s`;
}

/** Calculate duration between two ISO dates */
function calculateDuration(startedAt: string, endedAt?: string): number {
    if (!endedAt) return Date.now() - new Date(startedAt).getTime();
    return new Date(endedAt).getTime() - new Date(startedAt).getTime();
}

/** Format date to local string */
function formatDate(dateStr: string): string {
    return new Date(dateStr).toLocaleString(undefined, {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
    });
}

// ============================================================================
// Status Badge Component
// ============================================================================

const StatusBadge: React.FC<{ status: PipelineRun['status'] }> = ({ status }) => {
    const statusConfig: Record<PipelineRun['status'], { label: string; className: string }> = {
        pending: { label: 'Pending', className: 'badge--pending' },
        running: { label: 'Running', className: 'badge--running' },
        completed: { label: 'Completed', className: 'badge--success' },
        failed: { label: 'Failed', className: 'badge--error' },
        cancelled: { label: 'Cancelled', className: 'badge--muted' },
    };

    const config = statusConfig[status] || statusConfig.pending;

    return (
        <span className={`badge ${config.className}`}>
            {status === 'running' && <span className="badge__dot badge__dot--pulse" />}
            {config.label}
        </span>
    );
};

// ============================================================================
// Step Status Badge Component
// ============================================================================

const StepStatusBadge: React.FC<{ status: PipelineStepRun['status'] }> = ({ status }) => {
    const statusConfig: Record<PipelineStepRun['status'], { label: string; className: string }> = {
        pending: { label: 'Pending', className: 'badge--pending' },
        running: { label: 'Running', className: 'badge--running' },
        completed: { label: 'Done', className: 'badge--success' },
        failed: { label: 'Failed', className: 'badge--error' },
        skipped: { label: 'Skipped', className: 'badge--muted' },
    };

    const config = statusConfig[status] || statusConfig.pending;

    return (
        <span className={`badge badge--sm ${config.className}`}>
            {status === 'running' && <span className="badge__dot badge__dot--pulse" />}
            {config.label}
        </span>
    );
};

// ============================================================================
// Trigger Source Badge Component
// ============================================================================

const TriggerSourceBadge: React.FC<{ source: PipelineRun['trigger_source'] }> = ({ source }) => {
    const sourceLabels: Record<PipelineRun['trigger_source'], string> = {
        manual: 'Manual',
        schedule: 'Schedule',
        webhook: 'Webhook',
    };

    return (
        <span className="trigger-badge">
            {source === 'schedule' && <span className="trigger-badge__icon">⏰</span>}
            {source === 'manual' && <span className="trigger-badge__icon">👤</span>}
            {source === 'webhook' && <span className="trigger-badge__icon">🔗</span>}
            {sourceLabels[source] || source}
        </span>
    );
};

// ============================================================================
// Step Detail Component
// ============================================================================

interface StepDetailProps {
    stepRun: PipelineStepRun;
    stepNumber: number;
}

const StepDetail: React.FC<StepDetailProps> = ({ stepRun, stepNumber }) => {
    const [isExpanded, setIsExpanded] = useState(false);
    const duration = calculateDuration(stepRun.started_at || '', stepRun.ended_at);

    return (
        <div className="pipeline-step-detail">
            <div
                className="pipeline-step-detail__header"
                onClick={() => setIsExpanded(!isExpanded)}
            >
                <div className="pipeline-step-detail__number">
                    {stepNumber}
                </div>
                <div className="pipeline-step-detail__info">
                    <div className="pipeline-step-detail__title">
                        <span className="pipeline-step-detail__label">Step {stepNumber}</span>
                        <StepStatusBadge status={stepRun.status} />
                    </div>
                    <div className="pipeline-step-detail__duration">
                        {stepRun.started_at ? formatDuration(duration) : '—'}
                    </div>
                </div>
                <div className="pipeline-step-detail__toggle">
                    {isExpanded ? '▼' : '▶'}
                </div>
            </div>

            {isExpanded && (
                <div className="pipeline-step-detail__content">
                    {/* Input/Output Preview */}
                    {(stepRun.output || stepRun.error) && (
                        <div className="pipeline-step-detail__section">
                            <div className="pipeline-step-detail__section-title">Output</div>
                            <pre className="pipeline-step-detail__output">
                                {stepRun.output || stepRun.error}
                            </pre>
                        </div>
                    )}

                    {/* Error Message */}
                    {stepRun.status === 'failed' && stepRun.error && (
                        <div className="pipeline-step-detail__error">
                            <div className="pipeline-step-detail__section-title">Error</div>
                            <div className="pipeline-step-detail__error-message">
                                {stepRun.error}
                            </div>
                        </div>
                    )}

                    {/* View Run Link */}
                    {stepRun.agent_run_id && (
                        <Link
                            to={`/forge/run/${stepRun.agent_run_id}`}
                            className="pipeline-step-detail__view-run"
                        >
                            View Run →
                        </Link>
                    )}
                </div>
            )}
        </div>
    );
};

// ============================================================================
// Main Component
// ============================================================================

const PipelineRunDetail: React.FC<PipelineRunDetailProps> = ({ runId, run: initialRun }) => {
    const { showToast } = useToast();
    const [run, setRun] = useState<PipelineRun | null>(initialRun || null);
    const [loading, setLoading] = useState(!initialRun && !!runId);
    const [error, setError] = useState<string | null>(null);

    // Fetch run if only runId is provided
    useEffect(() => {
        if (!runId || initialRun) return;

        const fetchRun = async () => {
            try {
                setLoading(true);
                const data = await getPipelineRun(runId);
                setRun(data);
                setError(null);
            } catch (err) {
                const msg = err instanceof Error ? err.message : 'Failed to load pipeline run';
                setError(msg);
                showToast(msg, 'error');
            } finally {
                setLoading(false);
            }
        };

        fetchRun();
    }, [runId, initialRun, showToast]);

    // Loading state
    if (loading) {
        return (
            <div className="pipeline-run-detail">
                <div className="pipeline-run-detail__loading">
                    <div className="spinner" />
                    <span>Loading pipeline run...</span>
                </div>
            </div>
        );
    }

    // Error state
    if (error && !run) {
        return (
            <div className="pipeline-run-detail">
                <div className="pipeline-run-detail__error">
                    <span className="pipeline-run-detail__error-icon">⚠️</span>
                    <span>{error}</span>
                </div>
            </div>
        );
    }

    // No run data
    if (!run) {
        return (
            <div className="pipeline-run-detail">
                <div className="pipeline-run-detail__empty">
                    No run data available
                </div>
            </div>
        );
    }

    const duration = calculateDuration(run.started_at, run.ended_at);

    return (
        <div className="pipeline-run-detail">
            {/* Header */}
            <div className="pipeline-run-detail__header">
                <div className="pipeline-run-detail__status">
                    <StatusBadge status={run.status} />
                </div>
                <div className="pipeline-run-detail__meta">
                    <div className="pipeline-run-detail__meta-item">
                        <span className="pipeline-run-detail__meta-label">Duration</span>
                        <span className="pipeline-run-detail__meta-value">
                            {formatDuration(duration)}
                        </span>
                    </div>
                    <div className="pipeline-run-detail__meta-item">
                        <span className="pipeline-run-detail__meta-label">Trigger</span>
                        <span className="pipeline-run-detail__meta-value">
                            <TriggerSourceBadge source={run.trigger_source} />
                        </span>
                    </div>
                    <div className="pipeline-run-detail__meta-item">
                        <span className="pipeline-run-detail__meta-label">Started</span>
                        <span className="pipeline-run-detail__meta-value">
                            {formatDate(run.started_at)}
                        </span>
                    </div>
                </div>
            </div>

            {/* Error Message */}
            {run.status === 'failed' && run.error && (
                <div className="pipeline-run-detail__error-banner">
                    <div className="pipeline-run-detail__error-banner-icon">⚠️</div>
                    <div className="pipeline-run-detail__error-banner-content">
                        <div className="pipeline-run-detail__error-banner-title">Pipeline Failed</div>
                        <div className="pipeline-run-detail__error-banner-message">{run.error}</div>
                    </div>
                </div>
            )}

            {/* Step Progress */}
            <div className="pipeline-run-detail__steps">
                <div className="pipeline-run-detail__steps-title">
                    Steps ({run.step_runs?.length || 0})
                </div>

                {run.step_runs && run.step_runs.length > 0 ? (
                    <div className="pipeline-run-detail__steps-list">
                        {run.step_runs.map((stepRun, index) => (
                            <StepDetail
                                key={stepRun.step_id || index}
                                stepRun={stepRun}
                                stepNumber={stepRun.step_order || index + 1}
                            />
                        ))}
                    </div>
                ) : (
                    <div className="pipeline-run-detail__steps-empty">
                        No step information available
                    </div>
                )}
            </div>
        </div>
    );
};

export default PipelineRunDetail;
