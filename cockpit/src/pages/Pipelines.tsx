/**
 * Pipelines — List and manage pipelines with run history
 * 
 * Shows all pipelines as cards with:
 * - Name, description, step count, satellite name
 * - Last run status badge, schedule (if configured)
 * - Run Now button, delete button
 * - Expandable to show recent runs via PipelineRunDetail
 * 
 * Enterprise feature gated.
 */

import React, { useState, useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import {
    listPipelines,
    deletePipeline,
    runPipeline,
    listPipelineRuns,
    type Pipeline,
    type PipelineRun
} from '../api/pipelines';
import { getSatellites, type Satellite } from '../api/client';
import { useLicense } from '../hooks/useLicense';
import { useToast } from '../components/Toast';
import PipelineRunDetail from '../components/PipelineRunDetail';
import EnterpriseBadge from '../components/EnterpriseBadge';

// ============================================================================
// Types
// ============================================================================

interface PipelineWithSatellite extends Pipeline {
    satelliteName?: string;
}

interface PipelineWithRuns extends PipelineWithSatellite {
    lastRun?: PipelineRun;
    recentRuns?: PipelineRun[];
}

// ============================================================================
// Helper Functions
// ============================================================================

/** Format a date as relative time */
function timeAgo(dateStr: string): string {
    if (!dateStr) return 'Never';
    const diff = Date.now() - new Date(dateStr).getTime();
    const mins = Math.floor(diff / 60000);
    if (mins < 1) return 'Just now';
    if (mins < 60) return `${mins}m ago`;
    const hrs = Math.floor(mins / 60);
    if (hrs < 24) return `${hrs}h ago`;
    const days = Math.floor(hrs / 24);
    if (days < 7) return `${days}d ago`;
    return new Date(dateStr).toLocaleDateString();
}

/** Truncate text with ellipsis */
function truncate(text: string, maxLength: number): string {
    if (!text || text.length <= maxLength) return text;
    return text.slice(0, maxLength - 3) + '...';
}

// ============================================================================
// Status Badge Component
// ============================================================================

const RunStatusBadge: React.FC<{ status?: PipelineRun['status'] }> = ({ status }) => {
    if (!status) {
        return <span className="badge badge--muted">Never run</span>;
    }

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
// Delete Confirmation Modal
// ============================================================================

interface DeleteModalProps {
    isOpen: boolean;
    pipelineName: string;
    onConfirm: () => void;
    onCancel: () => void;
    isDeleting: boolean;
}

const DeleteModal: React.FC<DeleteModalProps> = ({
    isOpen,
    pipelineName,
    onConfirm,
    onCancel,
    isDeleting
}) => {
    useEffect(() => {
        const handleKeyDown = (e: KeyboardEvent) => {
            if (e.key === 'Escape' && isOpen) {
                onCancel();
            }
        };
        document.addEventListener('keydown', handleKeyDown);
        return () => document.removeEventListener('keydown', handleKeyDown);
    }, [isOpen, onCancel]);

    if (!isOpen) return null;

    return (
        <div className="modal-overlay" onClick={onCancel}>
            <div className="modal" onClick={(e) => e.stopPropagation()}>
                <div className="modal__header">
                    <h3 className="modal__title">Delete Pipeline</h3>
                    <button className="modal__close" onClick={onCancel}>×</button>
                </div>
                <div className="modal__body">
                    <p>Are you sure you want to delete <strong>{pipelineName}</strong>?</p>
                    <p className="text-muted" style={{ fontSize: 13, marginTop: 8 }}>
                        This action cannot be undone. All run history will be lost.
                    </p>
                </div>
                <div className="modal__footer">
                    <button className="btn btn--outline" onClick={onCancel} disabled={isDeleting}>
                        Cancel
                    </button>
                    <button
                        className="btn btn--danger"
                        onClick={onConfirm}
                        disabled={isDeleting}
                    >
                        {isDeleting ? 'Deleting...' : 'Delete'}
                    </button>
                </div>
            </div>
        </div>
    );
};

// ============================================================================
// Pipeline Card Component
// ============================================================================

interface PipelineCardProps {
    pipeline: PipelineWithRuns;
    onRun: () => void;
    onDelete: () => void;
    isRunning: boolean;
    isDeleting: boolean;
}

const PipelineCard: React.FC<PipelineCardProps> = ({
    pipeline,
    onRun,
    onDelete,
    isRunning,
    isDeleting
}) => {
    const navigate = useNavigate();
    const [isExpanded, setIsExpanded] = useState(false);

    return (
        <div className="pipeline-card">
            {/* Card Header */}
            <div className="pipeline-card__header">
                <div
                    className="pipeline-card__name"
                    onClick={() => navigate(`/pipelines/${pipeline.id}/edit`)}
                    title="Edit pipeline"
                >
                    {pipeline.name}
                </div>
                <div className="pipeline-card__actions">
                    <button
                        className="btn btn--primary btn--sm"
                        onClick={onRun}
                        disabled={isRunning}
                        title="Run pipeline now"
                    >
                        {isRunning ? 'Running...' : '▶ Run Now'}
                    </button>
                    <button
                        className="btn btn--outline btn--sm"
                        onClick={onDelete}
                        disabled={isDeleting}
                        title="Delete pipeline"
                    >
                        🗑
                    </button>
                </div>
            </div>

            {/* Card Body */}
            <div className="pipeline-card__body">
                {/* Description */}
                {pipeline.description && (
                    <div className="pipeline-card__description">
                        {truncate(pipeline.description, 120)}
                    </div>
                )}

                {/* Meta Info */}
                <div className="pipeline-card__meta">
                    <div className="pipeline-card__meta-item">
                        <span className="pipeline-card__meta-icon">📡</span>
                        <span>{pipeline.satelliteName || pipeline.satellite_id}</span>
                    </div>
                    <div className="pipeline-card__meta-item">
                        <span className="pipeline-card__meta-icon">⚙️</span>
                        <span>{pipeline.steps?.length || 0} step{pipeline.steps?.length !== 1 ? 's' : ''}</span>
                    </div>
                    {pipeline.schedule && (
                        <div className="pipeline-card__meta-item">
                            <span className="pipeline-card__meta-icon">⏰</span>
                            <span>{pipeline.schedule}</span>
                        </div>
                    )}
                </div>

                {/* Last Run Status */}
                <div className="pipeline-card__status">
                    <span className="pipeline-card__status-label">Last run:</span>
                    <RunStatusBadge status={pipeline.lastRun?.status} />
                    {pipeline.lastRun && (
                        <span className="pipeline-card__status-time">
                            {timeAgo(pipeline.lastRun.started_at)}
                        </span>
                    )}
                </div>
            </div>

            {/* Expand/Collapse Toggle */}
            <div
                className="pipeline-card__expand"
                onClick={() => setIsExpanded(!isExpanded)}
            >
                {isExpanded ? '▲ Hide runs' : '▼ Show recent runs'}
            </div>

            {/* Expanded: Recent Runs */}
            {isExpanded && pipeline.recentRuns && pipeline.recentRuns.length > 0 && (
                <div className="pipeline-card__runs">
                    {pipeline.recentRuns.slice(0, 3).map((run) => (
                        <div key={run.id} className="pipeline-card__run">
                            <PipelineRunDetail run={run} />
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
};

// ============================================================================
// Enterprise Locked State
// ============================================================================

const EnterpriseLocked: React.FC = () => {
    return (
        <div className="pipelines-locked">
            <div className="pipelines-locked__icon">🔒</div>
            <h2 className="pipelines-locked__title">Pipelines — Coming Soon</h2>
            <p className="pipelines-locked__desc">
                Automate complex workflows with multi-step pipelines.
                Run agents sequentially, pass outputs between steps, and schedule automated executions.
            </p>
            <a
                href="https://daao.dev/pricing"
                target="_blank"
                rel="noopener noreferrer"
                className="btn btn--primary"
            >
                Register Interest →
            </a>
        </div>
    );
};

// ============================================================================
// Main Component
// ============================================================================

const Pipelines: React.FC = () => {
    const navigate = useNavigate();
    const { isEnterprise, hasFeature, license, loading: licenseLoading } = useLicense();
    const { showToast } = useToast();

    const [pipelines, setPipelines] = useState<PipelineWithRuns[]>([]);
    const [satellites, setSatellites] = useState<Record<string, Satellite>>({});
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    // For delete modal
    const [deleteTarget, setDeleteTarget] = useState<Pipeline | null>(null);
    const [isDeleting, setIsDeleting] = useState(false);

    // Track running pipelines
    const [runningPipelines, setRunningPipelines] = useState<Set<string>>(new Set());

    // Check if pipelines feature is available
    const pipelinesEnabled = isEnterprise || hasFeature('pipelines');

    // Fetch pipelines and satellites
    useEffect(() => {
        const fetchData = async () => {
            try {
                setLoading(true);

                // Fetch satellites for name lookup
                const satellitesData = await getSatellites();
                const satelliteMap: Record<string, Satellite> = {};
                satellitesData.forEach((sat) => {
                    satelliteMap[sat.id] = sat;
                });
                setSatellites(satelliteMap);

                // Fetch pipelines
                const pipelinesData = await listPipelines();

                // Fetch last run for each pipeline
                const pipelinesWithRuns: PipelineWithRuns[] = await Promise.all(
                    pipelinesData.pipelines.map(async (pipeline) => {
                        try {
                            const runsData = await listPipelineRuns(pipeline.id, { limit: 3 });
                            return {
                                ...pipeline,
                                satelliteName: satelliteMap[pipeline.satellite_id]?.name,
                                recentRuns: runsData.runs,
                                lastRun: runsData.runs[0],
                            };
                        } catch {
                            return {
                                ...pipeline,
                                satelliteName: satelliteMap[pipeline.satellite_id]?.name,
                                recentRuns: [],
                                lastRun: undefined,
                            };
                        }
                    })
                );

                setPipelines(pipelinesWithRuns);
                setError(null);
            } catch (err) {
                const msg = err instanceof Error ? err.message : 'Failed to load pipelines';
                setError(msg);
                showToast(msg, 'error');
            } finally {
                setLoading(false);
            }
        };

        if (pipelinesEnabled) {
            fetchData();
        }
    }, [pipelinesEnabled, showToast]);

    // Handle run pipeline
    const handleRunPipeline = useCallback(async (pipeline: Pipeline) => {
        setRunningPipelines((prev) => new Set(prev).add(pipeline.id));
        try {
            const result = await runPipeline(pipeline.id, pipeline.satellite_id);
            if (result) {
                showToast(`Pipeline "${pipeline.name}" started`, 'success');
                // Refresh to get updated run status
                window.location.reload();
            }
        } catch (err) {
            const msg = err instanceof Error ? err.message : 'Failed to start pipeline';
            showToast(msg, 'error');
        } finally {
            setRunningPipelines((prev) => {
                const next = new Set(prev);
                next.delete(pipeline.id);
                return next;
            });
        }
    }, [showToast]);

    // Handle delete pipeline
    const handleDeletePipeline = useCallback(async () => {
        if (!deleteTarget) return;

        setIsDeleting(true);
        try {
            await deletePipeline(deleteTarget.id);
            showToast(`Pipeline "${deleteTarget.name}" deleted`, 'success');
            setPipelines((prev) => prev.filter((p) => p.id !== deleteTarget.id));
            setDeleteTarget(null);
        } catch (err) {
            const msg = err instanceof Error ? err.message : 'Failed to delete pipeline';
            showToast(msg, 'error');
        } finally {
            setIsDeleting(false);
        }
    }, [deleteTarget, showToast]);

    // Loading state
    if (licenseLoading || (pipelinesEnabled && loading)) {
        return (
            <div>
                <div className="page-header">
                    <h1 className="page-header-title">Pipelines</h1>
                </div>
                <div style={{ textAlign: 'center', padding: '3rem' }}>
                    <div className="spinner" />
                    <p>Loading pipelines...</p>
                </div>
            </div>
        );
    }

    // Enterprise locked state
    if (!pipelinesEnabled) {
        return (
            <div>
                <div className="page-header">
                    <h1 className="page-header-title">
                        Pipelines
                        <EnterpriseBadge />
                    </h1>
                    <div className="page-header-subtitle">Automate complex workflows with multi-step pipelines</div>
                </div>
                <EnterpriseLocked />
            </div>
        );
    }

    return (
        <div>
            {/* Header */}
            <div className="page-header">
                <div>
                    <h1 className="page-header-title">Pipelines</h1>
                    <div className="page-header-subtitle">
                        Automate complex workflows with multi-step pipelines
                    </div>
                </div>
                <button
                    className="btn btn--primary"
                    onClick={() => navigate('/pipelines/new')}
                >
                    + New Pipeline
                </button>
            </div>

            {/* Error State */}
            {error && (
                <div className="alert alert--error">
                    <span>{error}</span>
                    <button onClick={() => setError(null)}>×</button>
                </div>
            )}

            {/* Empty State */}
            {!loading && pipelines.length === 0 && (
                <div className="empty-state">
                    <div className="empty-state__icon">🚀</div>
                    <div className="empty-state__title">No pipelines yet</div>
                    <div className="empty-state__desc">
                        Create your first pipeline to automate multi-step workflows
                    </div>
                    <button
                        className="btn btn--primary"
                        onClick={() => navigate('/pipelines/new')}
                    >
                        Create Pipeline
                    </button>
                </div>
            )}

            {/* Pipeline Cards Grid */}
            {pipelines.length > 0 && (
                <div className="pipelines-grid">
                    {pipelines.map((pipeline) => (
                        <PipelineCard
                            key={pipeline.id}
                            pipeline={pipeline}
                            onRun={() => handleRunPipeline(pipeline)}
                            onDelete={() => setDeleteTarget(pipeline)}
                            isRunning={runningPipelines.has(pipeline.id)}
                            isDeleting={isDeleting && deleteTarget?.id === pipeline.id}
                        />
                    ))}
                </div>
            )}

            {/* Delete Confirmation Modal */}
            <DeleteModal
                isOpen={!!deleteTarget}
                pipelineName={deleteTarget?.name || ''}
                onConfirm={handleDeletePipeline}
                onCancel={() => setDeleteTarget(null)}
                isDeleting={isDeleting}
            />
        </div>
    );
};

export default Pipelines;
