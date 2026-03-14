/**
 * AgentRunPage — Page wrapper for viewing a specific agent run
 * 
 * Extracts runId from URL params, fetches run data,
 * and mounts the existing AgentRunView component.
 */

import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import AgentRunView, { type RunEvent } from '../components/AgentRunView';
import { ArrowLeftIcon } from '../components/Icons';
import { apiRequest } from '../api/client';

// ============================================================================
// Component
// ============================================================================

const AgentRunPage: React.FC = () => {
    const { runId } = useParams<{ runId: string }>();
    const navigate = useNavigate();
    const [run, setRun] = useState<RunEvent | undefined>(undefined);
    const [isLoading, setIsLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        if (!runId) {
            setError('No run ID provided');
            setIsLoading(false);
            return;
        }

        const fetchRun = async () => {
            setIsLoading(true);
            setError(null);
            try {
                const response = await apiRequest<{
                    id: string;
                    status: string;
                    started_at: string;
                    ended_at?: string;
                    total_tokens?: number;
                    tool_call_count?: number;
                    error?: string;
                    output?: Record<string, unknown>;
                }>(`/runs/${runId}`);

                // Transform API response to RunEvent format
                const runEvent: RunEvent = {
                    id: response.id,
                    status: response.status as RunEvent['status'],
                    startedAt: new Date(response.started_at),
                    endedAt: response.ended_at ? new Date(response.ended_at) : undefined,
                    toolCalls: [],
                    output: response.output ? JSON.stringify(response.output, null, 2) : '',
                    tokenUsage: response.total_tokens
                        ? { input: Math.floor(response.total_tokens * 0.3), output: Math.floor(response.total_tokens * 0.7) }
                        : undefined,
                    error: response.error,
                };
                setRun(runEvent);
            } catch (err) {
                if (err instanceof Error && err.message.includes('404')) {
                    setError('Run not found');
                } else {
                    setError(err instanceof Error ? err.message : 'Failed to load run');
                }
            } finally {
                setIsLoading(false);
            }
        };

        fetchRun();
    }, [runId]);

    return (
        <div>
            {/* Back navigation */}
            <div style={{ marginBottom: 16 }}>
                <button
                    className="btn btn--ghost"
                    onClick={() => navigate('/forge')}
                    style={{ display: 'flex', alignItems: 'center', gap: 6 }}
                >
                    <ArrowLeftIcon size={16} />
                    Back to Forge
                </button>
            </div>

            {/* Page Header */}
            <div className="page-header">
                <h1 className="page-header-title">Agent Run</h1>
                <div className="page-header-subtitle">
                    {runId ? `Run: ${runId.substring(0, 12)}...` : 'Unknown Run'}
                </div>
            </div>

            {/* Content */}
            {isLoading ? (
                <div style={{ textAlign: 'center', padding: 48 }}>
                    <div className="spinner" />
                    <p style={{ color: 'var(--text-muted)', marginTop: 12 }}>Loading run data...</p>
                </div>
            ) : error ? (
                <div className="forge-empty">
                    <div className="forge-empty__title">{error === 'Run not found' ? '404 — Run Not Found' : 'Error'}</div>
                    <div className="forge-empty__desc">{error}</div>
                    <button className="btn btn--primary btn--sm" onClick={() => navigate('/forge')}>
                        Return to Forge
                    </button>
                </div>
            ) : (
                <AgentRunView run={run} runId={runId} />
            )}
        </div>
    );
};

export default AgentRunPage;
