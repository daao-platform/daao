/**
 * AgentDetailPanel — Enhanced slide-in panel for viewing agent details
 * 
 * Displays agent information with tabbed navigation:
 * - Overview: Description, provider/model, category/type, guardrails, tools, system prompt, author
 * - Versions: Version history with rollback capability
 * - Runs: Run history with status, duration, tokens
 */

import React, { useState, useEffect, useCallback } from 'react';
import { AgentDefinition, AgentRun } from '../hooks/useAgents';
import { apiRequest } from '../api/client';
import { XIcon } from './Icons';

// ============================================================================
// Types
// ============================================================================

export interface AgentDetailPanelProps {
    agent: AgentDefinition;
    isOpen: boolean;
    onClose: () => void;
    onDeploy: (agent: AgentDefinition) => void;
    onEdit: (agent: AgentDefinition) => void;
    onDelete: (agent: AgentDefinition) => void;
}

/** Agent version summary from API */
interface AgentVersionSummary {
    id: string;
    version: string;
    change_summary: string;
    created_by: string;
    created_at: string;
}

/** Tab IDs */
type TabId = 'overview' | 'versions' | 'runs';

// ============================================================================
// Helper Functions
// ============================================================================

/** Get icon letter from agent */
function getIconLetter(agent: AgentDefinition): string {
    if (agent.icon) return agent.icon;
    const name = agent.display_name || agent.name;
    return name.charAt(0).toUpperCase();
}

/** Get CSS class for category-tinted icon */
function getIconClass(category?: string): string {
    switch (category) {
        case 'infrastructure': return 'agent-panel-icon--infrastructure';
        case 'development': return 'agent-panel-icon--development';
        case 'security': return 'agent-panel-icon--security';
        case 'operations': return 'agent-panel-icon--operations';
        default: return 'agent-panel-icon--default';
    }
}

/** Format duration from start and end timestamps */
const formatDuration = (startedAt: string, endedAt?: string): string => {
    if (!startedAt) return 'N/A';
    const start = new Date(startedAt).getTime();
    const end = endedAt ? new Date(endedAt).getTime() : Date.now();
    const ms = end - start;

    if (ms < 1000) return `${ms}ms`;
    if (ms < 60000) return `${(ms / 1000).toFixed(2)}s`;
    const minutes = Math.floor(ms / 60000);
    const seconds = ((ms % 60000) / 1000).toFixed(0);
    return `${minutes}m ${seconds}s`;
};

/** Format date as relative time */
const formatRelativeTime = (dateStr: string): string => {
    if (!dateStr) return 'N/A';
    const diff = Date.now() - new Date(dateStr).getTime();
    const mins = Math.floor(diff / 60000);
    if (mins < 1) return 'Just now';
    if (mins < 60) return `${mins}m ago`;
    const hrs = Math.floor(mins / 60);
    if (hrs < 24) return `${hrs}h ago`;
    const days = Math.floor(hrs / 24);
    if (days < 7) return `${days}d ago`;
    return new Date(dateStr).toLocaleDateString();
};

/** Format token count */
const formatTokens = (tokens?: number): string => {
    if (tokens === undefined || tokens === null) return '—';
    return tokens.toLocaleString();
};

/** Parse JSONB field — handles string or object */
const parseJsonField = (field: unknown): Record<string, unknown> | null => {
    if (!field) return null;
    if (typeof field === 'object' && field !== null) return field as Record<string, unknown>;
    if (typeof field === 'string') {
        try {
            return JSON.parse(field);
        } catch {
            return null;
        }
    }
    return null;
};

// ============================================================================
// Inline Hooks (to be replaced by FR-7 hooks)
// ============================================================================

/** Inline fetch for agent versions - will be replaced by useAgentVersions in FR-7 */
function useAgentVersionsInline(agentId: string | undefined) {
    const [versions, setVersions] = useState<AgentVersionSummary[]>([]);
    const [isLoading, setIsLoading] = useState(false);
    const [error, setError] = useState<Error | null>(null);

    const fetchVersions = useCallback(async () => {
        if (!agentId) return;
        setIsLoading(true);
        setError(null);
        try {
            const response = await apiRequest<{ versions: AgentVersionSummary[] }>(
                `/agents/${agentId}/versions`
            );
            setVersions(response.versions || []);
        } catch (err) {
            setError(err instanceof Error ? err : new Error(String(err)));
            setVersions([]);
        } finally {
            setIsLoading(false);
        }
    }, [agentId]);

    useEffect(() => {
        fetchVersions();
    }, [fetchVersions]);

    return { versions, isLoading, error, refetch: fetchVersions };
}

/** Inline fetch for agent runs - will be replaced by FR-7 hook */
function useAgentRunsInline(agentId: string | undefined) {
    const [runs, setRuns] = useState<AgentRun[]>([]);
    const [isLoading, setIsLoading] = useState(false);
    const [error, setError] = useState<Error | null>(null);

    const fetchRuns = useCallback(async () => {
        if (!agentId) return;
        setIsLoading(true);
        setError(null);
        try {
            const response = await apiRequest<{ runs: AgentRun[] }>(
                `/agents/${agentId}/runs`
            );
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

    return { runs, isLoading, error, refetch: fetchRuns };
}

/** Inline export function - will be replaced by useExportAgent in FR-7 */
async function exportAgentInline(agent: AgentDefinition): Promise<void> {
    const yamlContent = `# Agent: ${agent.display_name || agent.name}
# Version: ${agent.version}
# Generated: ${new Date().toISOString()}

name: ${agent.name}
display_name: ${agent.display_name || agent.name}
description: ${agent.description || ''}
type: ${agent.type || 'specialist'}
category: ${agent.category || 'development'}
provider: ${agent.provider || ''}
model: ${agent.model || ''}
version: ${agent.version}

${agent.system_prompt ? `system_prompt: |\n  ${agent.system_prompt.split('\n').join('\n  ')}` : ''}

${agent.tools_config ? `tools_config: ${JSON.stringify(agent.tools_config, null, 2)}` : ''}
${agent.guardrails ? `guardrails: ${JSON.stringify(agent.guardrails, null, 2)}` : ''}
`;

    const blob = new Blob([yamlContent], { type: 'text/yaml' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${agent.name}-${agent.version}.yaml`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
}

// ============================================================================
// Sub-Components
// ============================================================================

/** Status badge component */
const RunStatusBadge: React.FC<{ status: AgentRun['status'] }> = ({ status }) => {
    const getStatusClass = () => {
        switch (status) {
            case 'completed': return 'status-badge--completed';
            case 'running': return 'status-badge--running';
            case 'failed': return 'status-badge--failed';
            case 'pending': return 'status-badge--pending';
            case 'cancelled': return 'status-badge--cancelled';
            default: return '';
        }
    };

    return React.createElement('span', {
        className: `status-badge ${getStatusClass()}`,
    }, status);
};

/** Overview Tab Content */
const OverviewTab: React.FC<{ agent: AgentDefinition }> = ({ agent }) => {
    // Parse tools_config from JSONB
    const toolsConfig = parseJsonField(agent.tools_config);
    const toolAllow = toolsConfig?.allow as string[] | undefined;
    const toolDeny = toolsConfig?.deny as string[] | undefined;
    const toolsList = toolAllow || (toolsConfig ? Object.keys(toolsConfig) : []);

    // Parse guardrails from JSONB
    const guardrails = parseJsonField(agent.guardrails);
    const hitlEnabled = guardrails?.hitl_enabled as boolean | undefined;
    const readOnly = guardrails?.read_only as boolean | undefined;
    const timeout = guardrails?.timeout as number | undefined;
    const maxToolCalls = guardrails?.max_tool_calls as number | undefined;

    // Parse category and type
    const category = agent.category;
    const type = agent.type;

    return React.createElement('div', { className: 'agent-panel-tab-content' },
        // Description
        React.createElement('div', { className: 'agent-panel-section' },
            React.createElement('h4', { className: 'agent-panel-section-title' }, 'Description'),
            React.createElement('p', { className: 'agent-panel-section-text' }, agent.description || 'No description available.')
        ),

        // Provider / Model
        React.createElement('div', { className: 'agent-panel-section' },
            React.createElement('h4', { className: 'agent-panel-section-title' }, 'Provider / Model'),
            React.createElement('div', { className: 'agent-panel-badges' },
                agent.provider && React.createElement('span', { className: 'badge badge--provider' }, agent.provider),
                agent.model && React.createElement('span', { className: 'badge badge--model' }, agent.model)
            )
        ),

        // Category / Type badges
        React.createElement('div', { className: 'agent-panel-section' },
            React.createElement('h4', { className: 'agent-panel-section-title' }, 'Classification'),
            React.createElement('div', { className: 'agent-panel-badges' },
                category && React.createElement('span', { className: 'badge badge--category' }, category),
                type && React.createElement('span', { className: `badge badge--${type}` }, type),
                agent.is_builtin && React.createElement('span', { className: 'badge badge--core' }, 'Built-in'),
                agent.is_enterprise && React.createElement('span', { className: 'badge badge--enterprise' }, 'Coming Soon')
            )
        ),

        // Guardrails summary
        React.createElement('div', { className: 'agent-panel-section' },
            React.createElement('h4', { className: 'agent-panel-section-title' }, 'Guardrails'),
            guardrails
                ? React.createElement('div', { className: 'agent-panel-meta-grid' },
                    hitlEnabled !== undefined && React.createElement('div', { className: 'agent-panel-meta-item' },
                        React.createElement('span', { className: 'agent-panel-meta-label' }, 'Human-in-the-Loop'),
                        React.createElement('span', { className: 'agent-panel-meta-value' }, hitlEnabled ? '✓ Enabled' : '✗ Disabled')
                    ),
                    readOnly !== undefined && React.createElement('div', { className: 'agent-panel-meta-item' },
                        React.createElement('span', { className: 'agent-panel-meta-label' }, 'Read-only'),
                        React.createElement('span', { className: 'agent-panel-meta-value' }, readOnly ? '✓ Yes' : '✗ No')
                    ),
                    timeout !== undefined && React.createElement('div', { className: 'agent-panel-meta-item' },
                        React.createElement('span', { className: 'agent-panel-meta-label' }, 'Timeout'),
                        React.createElement('span', { className: 'agent-panel-meta-value' }, `${timeout}s`)
                    ),
                    maxToolCalls !== undefined && React.createElement('div', { className: 'agent-panel-meta-item' },
                        React.createElement('span', { className: 'agent-panel-meta-label' }, 'Max Tool Calls'),
                        React.createElement('span', { className: 'agent-panel-meta-value' }, String(maxToolCalls))
                    )
                )
                : React.createElement('span', { className: 'text-muted' }, 'No guardrails configured')
        ),

        // Tools config
        React.createElement('div', { className: 'agent-panel-section' },
            React.createElement('h4', { className: 'agent-panel-section-title' }, 'Tools'),
            toolsList.length > 0
                ? React.createElement('div', null,
                    toolAllow && React.createElement('div', { style: { marginBottom: 8 } },
                        React.createElement('span', { style: { fontSize: 12, color: 'var(--text-muted)', marginRight: 8 } }, 'Allow:'),
                        React.createElement('div', { className: 'agent-panel-badges', style: { display: 'inline-flex' } },
                            toolAllow.map((tool, i) =>
                                React.createElement('span', { key: i, className: 'badge badge--tool' }, tool)
                            )
                        )
                    ),
                    toolDeny && React.createElement('div', null,
                        React.createElement('span', { style: { fontSize: 12, color: 'var(--text-muted)', marginRight: 8 } }, 'Deny:'),
                        React.createElement('div', { className: 'agent-panel-badges', style: { display: 'inline-flex' } },
                            toolDeny.map((tool, i) =>
                                React.createElement('span', { key: i, className: 'badge badge--guardrail' }, tool)
                            )
                        )
                    ),
                    !toolAllow && !toolDeny && React.createElement('div', { className: 'agent-panel-badges' },
                        toolsList.map((tool, i) =>
                            React.createElement('span', { key: i, className: 'badge badge--tool' }, String(tool))
                        )
                    )
                )
                : React.createElement('span', { className: 'text-muted' }, 'No tools configured')
        ),

        // System prompt (collapsible)
        React.createElement('div', { className: 'agent-panel-section' },
            React.createElement('h4', { className: 'agent-panel-section-title' }, 'System Prompt'),
            React.createElement('pre', {
                className: 'agent-panel-code-block',
                style: { maxHeight: '200px', overflow: 'auto' },
            }, agent.system_prompt || 'No system prompt defined.')
        ),

        // Author
        agent.author && React.createElement('div', { className: 'agent-panel-section' },
            React.createElement('h4', { className: 'agent-panel-section-title' }, 'Author'),
            React.createElement('span', { className: 'agent-panel-meta-value' }, agent.author)
        )
    );
};

/** Versions Tab Content */
const VersionsTab: React.FC<{ agent: AgentDefinition }> = ({ agent }) => {
    const { versions, isLoading, error, refetch } = useAgentVersionsInline(agent.id);
    const [rollingBack, setRollingBack] = useState<string | null>(null);
    const [showConfirm, setShowConfirm] = useState<string | null>(null);

    const handleRollback = async (versionId: string) => {
        setRollingBack(versionId);
        try {
            // Inline rollback call - will be replaced by FR-7 hook
            await apiRequest(`/agents/${agent.id}/versions/${versionId}/rollback`, {
                method: 'POST',
            });
            refetch();
        } catch (err) {
            console.error('Rollback failed:', err);
        } finally {
            setRollingBack(null);
            setShowConfirm(null);
        }
    };

    if (isLoading) {
        return React.createElement('div', { className: 'agent-panel-tab-content--loading' }, 'Loading versions...');
    }

    if (error) {
        return React.createElement('div', { className: 'agent-panel-tab-content' },
            React.createElement('div', { className: 'agent-panel-error' }, `Failed to load versions: ${error.message}`)
        );
    }

    if (!versions || versions.length === 0) {
        return React.createElement('div', { className: 'agent-panel-tab-content' },
            React.createElement('div', { className: 'agent-panel-empty' }, 'No version history available')
        );
    }

    return React.createElement('div', { className: 'agent-panel-tab-content' },
        React.createElement('table', { className: 'agent-panel-table' },
            React.createElement('thead', null,
                React.createElement('tr', null,
                    React.createElement('th', null, 'Version'),
                    React.createElement('th', null, 'Created'),
                    React.createElement('th', null, 'By'),
                    React.createElement('th', null, 'Changes'),
                    React.createElement('th', null, '')
                )
            ),
            React.createElement('tbody', null,
                versions.map((version) => {
                    const isCurrent = version.version === agent.version;
                    return React.createElement('tr', { 
                        key: version.id, 
                        id: `version-row-${version.version}`,
                        className: isCurrent ? 'agent-panel-version-row--current' : '' 
                    },
                        React.createElement('td', null,
                            React.createElement('span', { className: 'agent-panel-version' }, version.version),
                            isCurrent && React.createElement('span', { className: 'badge badge--current', style: { marginLeft: 8 } }, 'current')
                        ),
                        React.createElement('td', null, formatRelativeTime(version.created_at)),
                        React.createElement('td', null, version.created_by || 'Unknown'),
                        React.createElement('td', { className: 'agent-panel-change-summary' }, version.change_summary || '—'),
                        React.createElement('td', null,
                            !isCurrent && React.createElement('button', {
                                className: 'btn btn--outline btn--sm',
                                onClick: () => setShowConfirm(version.id),
                                disabled: rollingBack !== null,
                            }, rollingBack === version.id ? 'Rolling back...' : 'Rollback')
                        )
                    );
                })
            )
        ),
        // Confirmation dialog
        showConfirm && React.createElement('div', { className: 'agent-panel-modal-overlay' },
            React.createElement('div', { className: 'agent-panel-modal' },
                React.createElement('h3', null, 'Confirm Rollback'),
                React.createElement('p', null, 'Are you sure you want to rollback to this version?'),
                React.createElement('div', { className: 'agent-panel-modal-actions' },
                    React.createElement('button', {
                        className: 'btn btn--outline',
                        onClick: () => setShowConfirm(null),
                    }, 'Cancel'),
                    React.createElement('button', {
                        className: 'btn btn--danger',
                        onClick: () => handleRollback(showConfirm),
                    }, 'Confirm Rollback')
                )
            )
        )
    );
};

/** Runs Tab Content */
const RunsTab: React.FC<{ agent: AgentDefinition }> = ({ agent }) => {
    const { runs, isLoading, error } = useAgentRunsInline(agent.id);

    if (isLoading) {
        return React.createElement('div', { className: 'agent-panel-tab-content--loading' }, 'Loading runs...');
    }

    if (error) {
        return React.createElement('div', { className: 'agent-panel-tab-content' },
            React.createElement('div', { className: 'agent-panel-error' }, `Failed to load runs: ${error.message}`)
        );
    }

    if (!runs || runs.length === 0) {
        return React.createElement('div', { className: 'agent-panel-tab-content' },
            React.createElement('div', { className: 'agent-panel-empty' },
                React.createElement('div', { style: { fontSize: 14, color: 'var(--text-muted)' } }, 'No runs yet'),
                React.createElement('div', { style: { fontSize: 12, color: 'var(--text-muted)', marginTop: 4 } }, 'Deploy this agent to see run history')
            )
        );
    }

    return React.createElement('div', { className: 'agent-panel-tab-content' },
        React.createElement('table', { className: 'agent-panel-table' },
            React.createElement('thead', null,
                React.createElement('tr', null,
                    React.createElement('th', null, 'Status'),
                    React.createElement('th', null, 'Started'),
                    React.createElement('th', null, 'Duration'),
                    React.createElement('th', null, 'Tokens'),
                    React.createElement('th', null, 'Satellite')
                )
            ),
            React.createElement('tbody', null,
                runs.map((run) => 
                    React.createElement('tr', { key: run.id, id: `run-row-${run.id}` },
                        React.createElement('td', null, React.createElement(RunStatusBadge, { status: run.status })),
                        React.createElement('td', null, formatRelativeTime(run.started_at)),
                        React.createElement('td', null, formatDuration(run.started_at, run.ended_at)),
                        React.createElement('td', null, formatTokens(run.total_tokens)),
                        React.createElement('td', null, 
                            // Extract satellite name from run if available, otherwise link to run
                            React.createElement('a', {
                                href: `/runs/${run.id}`,
                                className: 'agent-panel-run-link',
                            }, run.id.substring(0, 8))
                        )
                    )
                )
            )
        )
    );
};

// ============================================================================
// Main Component
// ============================================================================

const AgentDetailPanel: React.FC<AgentDetailPanelProps> = ({
    agent,
    isOpen,
    onClose,
    onDeploy,
    onEdit,
    onDelete,
}) => {
    const [activeTab, setActiveTab] = useState<TabId>('overview');
    const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);

    // Handle Escape key
    useEffect(() => {
        const handleKeyDown = (e: KeyboardEvent) => {
            if (e.key === 'Escape' && isOpen) {
                onClose();
            }
        };

        document.addEventListener('keydown', handleKeyDown);
        return () => document.removeEventListener('keydown', handleKeyDown);
    }, [isOpen, onClose]);

    // Handle backdrop click
    const handleBackdropClick = (e: React.MouseEvent) => {
        if (e.target === e.currentTarget) {
            onClose();
        }
    };

    // Handle export
    const handleExport = useCallback(() => {
        exportAgentInline(agent);
    }, [agent]);

    // Handle delete
    const handleDelete = useCallback(() => {
        onDelete(agent);
        setShowDeleteConfirm(false);
    }, [agent, onDelete]);

    // Don't render if not open
    if (!isOpen) {
        return null;
    }

    const iconLetter = getIconLetter(agent);
    const iconClass = getIconClass(agent.category);

    return React.createElement('div', {
        className: 'agent-panel-overlay',
        onClick: handleBackdropClick
    },
        React.createElement('div', {
            className: 'agent-panel',
            role: 'dialog',
            'aria-modal': 'true',
            'aria-labelledby': 'agent-detail-header'
        },
            // Header
            React.createElement('div', { className: 'agent-panel__header', id: 'agent-detail-header' },
                React.createElement('div', { className: 'agent-panel__header-left' },
                    React.createElement('div', { className: `agent-panel__icon ${iconClass}` },
                        iconLetter
                    ),
                    React.createElement('h2', { className: 'agent-panel__title' },
                        agent.display_name || agent.name
                    ),
                    React.createElement('span', { className: 'badge badge--version' }, `v${agent.version}`)
                ),
                React.createElement('button', {
                    className: 'agent-panel__close',
                    onClick: onClose,
                    'aria-label': 'Close panel',
                    type: 'button',
                }, React.createElement(XIcon, { size: 20 }))
            ),

            // Action Bar
            React.createElement('div', { className: 'agent-panel__actions' },
                React.createElement('button', {
                    id: 'agent-detail-deploy',
                    className: 'btn btn--primary',
                    onClick: () => onDeploy(agent),
                    type: 'button',
                }, 'Deploy'),
                React.createElement('button', {
                    id: 'agent-detail-export',
                    className: 'btn btn--outline',
                    onClick: handleExport,
                    type: 'button',
                }, 'Export YAML'),
                React.createElement('button', {
                    id: 'agent-detail-edit',
                    className: 'btn btn--outline',
                    onClick: () => onEdit(agent),
                    type: 'button',
                }, 'Edit'),
                React.createElement('button', {
                    id: 'agent-detail-delete',
                    className: 'btn btn--danger',
                    onClick: () => setShowDeleteConfirm(true),
                    type: 'button',
                }, 'Delete')
            ),

            // Tab Bar
            React.createElement('div', { className: 'agent-panel__tabs' },
                React.createElement('button', {
                    id: 'tab-overview',
                    className: `agent-panel__tab ${activeTab === 'overview' ? 'agent-panel__tab--active' : ''}`,
                    onClick: () => setActiveTab('overview'),
                    type: 'button',
                }, 'Overview'),
                React.createElement('button', {
                    id: 'tab-versions',
                    className: `agent-panel__tab ${activeTab === 'versions' ? 'agent-panel__tab--active' : ''}`,
                    onClick: () => setActiveTab('versions'),
                    type: 'button',
                }, 'Versions'),
                React.createElement('button', {
                    id: 'tab-runs',
                    className: `agent-panel__tab ${activeTab === 'runs' ? 'agent-panel__tab--active' : ''}`,
                    onClick: () => setActiveTab('runs'),
                    type: 'button',
                }, 'Runs')
            ),

            // Tab Content
            React.createElement('div', { className: 'agent-panel__content' },
                activeTab === 'overview' && React.createElement(OverviewTab, { agent }),
                activeTab === 'versions' && React.createElement(VersionsTab, { agent }),
                activeTab === 'runs' && React.createElement(RunsTab, { agent })
            )
        ),

        // Delete Confirmation Dialog
        showDeleteConfirm && React.createElement('div', { className: 'agent-panel-modal-overlay' },
            React.createElement('div', { className: 'agent-panel-modal' },
                React.createElement('h3', null, 'Confirm Delete'),
                React.createElement('p', null, `Are you sure you want to delete "${agent.display_name || agent.name}"? This action cannot be undone.`),
                React.createElement('div', { className: 'agent-panel-modal-actions' },
                    React.createElement('button', {
                        className: 'btn btn--outline',
                        onClick: () => setShowDeleteConfirm(false),
                    }, 'Cancel'),
                    React.createElement('button', {
                        className: 'btn btn--danger',
                        onClick: handleDelete,
                    }, 'Delete')
                )
            )
        )
    );
};

export default AgentDetailPanel;
