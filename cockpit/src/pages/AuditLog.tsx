import React, { useState, useEffect, useCallback } from 'react';
import { getAuditLog, exportAuditLogCsv, type AuditLogEntry } from '../api/client';
import { useApi } from '../hooks';
import { useToast } from '../components/Toast';

// Action options for filter dropdown
const ACTION_OPTIONS = [
    { value: '', label: 'All Actions' },
    { value: 'session.create', label: 'session.create' },
    { value: 'session.kill', label: 'session.kill' },
    { value: 'session.suspend', label: 'session.suspend' },
    { value: 'session.resume', label: 'session.resume' },
    { value: 'session.detach', label: 'session.detach' },
    { value: 'session.attach', label: 'session.attach' },
    { value: 'agent.deploy', label: 'agent.deploy' },
    { value: 'agent.delete', label: 'agent.delete' },
    { value: 'satellite.create', label: 'satellite.create' },
    { value: 'satellite.delete', label: 'satellite.delete' },
    { value: 'user.login', label: 'user.login' },
    { value: 'user.logout', label: 'user.logout' },
    { value: 'settings.update', label: 'settings.update' },
];

// Resource type options for filter dropdown
const RESOURCE_TYPE_OPTIONS = [
    { value: '', label: 'All Resources' },
    { value: 'session', label: 'session' },
    { value: 'agent', label: 'agent' },
    { value: 'satellite', label: 'satellite' },
    { value: 'user', label: 'user' },
    { value: 'settings', label: 'settings' },
];

function formatDate(dateStr: string): string {
    const d = new Date(dateStr);
    return d.toLocaleString(undefined, {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
    });
}

function formatDateForInput(dateStr: string): string {
    const d = new Date(dateStr);
    // Format for datetime-local input: YYYY-MM-DDTHH:mm
    const year = d.getFullYear();
    const month = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    const hours = String(d.getHours()).padStart(2, '0');
    const minutes = String(d.getMinutes()).padStart(2, '0');
    return `${year}-${month}-${day}T${hours}:${minutes}`;
}

/**
 * Expandable JSON viewer for audit log details
 */
const JsonViewer: React.FC<{ details: Record<string, unknown> | null }> = ({ details }) => {
    const [isExpanded, setIsExpanded] = useState(false);

    if (!details || Object.keys(details).length === 0) {
        return <span className="text-muted">—</span>;
    }

    const jsonString = JSON.stringify(details, null, 2);

    return (
        <div className="json-viewer">
            <button
                className="json-viewer__toggle"
                onClick={() => setIsExpanded(!isExpanded)}
                aria-expanded={isExpanded}
            >
                {isExpanded ? '▼' : '▶'} {isExpanded ? 'Hide' : 'View'} Details
            </button>
            {isExpanded && (
                <pre className="json-viewer__content">
                    <code>{jsonString}</code>
                </pre>
            )}
        </div>
    );
};

/**
 * Loading skeleton for table rows
 */
const TableSkeleton: React.FC<{ rows?: number }> = ({ rows = 5 }) => {
    return (
        <>
            {Array.from({ length: rows }).map((_, i) => (
                <tr key={i} className="loading-skeleton-row">
                    <td><div className="skeleton skeleton--text" /></td>
                    <td><div className="skeleton skeleton--text" /></td>
                    <td><div className="skeleton skeleton--text" /></td>
                    <td><div className="skeleton skeleton--text" /></td>
                    <td><div className="skeleton skeleton--text" /></td>
                    <td><div className="skeleton skeleton--text" /></td>
                    <td><div className="skeleton skeleton--text" /></td>
                </tr>
            ))}
        </>
    );
};

/**
 * Audit Log Page — View and filter audit log entries
 */
const AuditLog: React.FC = () => {
    const { showToast } = useToast();
    
    // Filter state
    const [actionFilter, setActionFilter] = useState('');
    const [resourceTypeFilter, setResourceTypeFilter] = useState('');
    const [sinceFilter, setSinceFilter] = useState('');
    const [untilFilter, setUntilFilter] = useState('');
    
    // Pagination state
    const [offset, setOffset] = useState(0);
    const limit = 50;

    // Build query params for API call
    const getQueryParams = useCallback(() => ({
        action: actionFilter || undefined,
        resource_type: resourceTypeFilter || undefined,
        since: sinceFilter ? new Date(sinceFilter).toISOString() : undefined,
        until: untilFilter ? new Date(untilFilter).toISOString() : undefined,
        limit,
        offset,
    }), [actionFilter, resourceTypeFilter, sinceFilter, untilFilter, offset]);

    // Fetch audit log data
    const { data, loading, error, refetch } = useApi(() => getAuditLog(getQueryParams()));

    // Reset offset when filters change
    useEffect(() => {
        setOffset(0);
    }, [actionFilter, resourceTypeFilter, sinceFilter, untilFilter]);

    const entries = data?.entries ?? [];
    const total = data?.total ?? 0;
    const currentPage = Math.floor(offset / limit) + 1;
    const totalPages = Math.ceil(total / limit);
    const hasNext = offset + limit < total;
    const hasPrev = offset > 0;

    // Handle 403 error
    useEffect(() => {
        if (error) {
            const errMsg = error.message || '';
            if (errMsg.includes('403') || errMsg.toLowerCase().includes('permission denied')) {
                showToast('You do not have permission to view the audit log', 'error');
            }
        }
    }, [error, showToast]);

    // Handle export
    const handleExport = async () => {
        try {
            await exportAuditLogCsv({
                action: actionFilter || undefined,
                resource_type: resourceTypeFilter || undefined,
                since: sinceFilter ? new Date(sinceFilter).toISOString() : undefined,
                until: untilFilter ? new Date(untilFilter).toISOString() : undefined,
            });
            showToast('Audit log exported successfully', 'success');
        } catch (err) {
            const msg = err instanceof Error ? err.message : 'Export failed';
            showToast(msg, 'error');
        }
    };

    // Pagination handlers
    const handlePrev = () => {
        if (hasPrev) {
            setOffset(Math.max(0, offset - limit));
        }
    };

    const handleNext = () => {
        if (hasNext) {
            setOffset(offset + limit);
        }
    };

    return (
        <div>
            {/* Page Header */}
            <div className="page-header">
                <h1 className="page-header-title">Audit Log</h1>
                <div className="page-header-subtitle">View system activity and changes</div>
            </div>

            {/* Filter Bar */}
            <div className="audit-filters">
                <div className="audit-filters__row">
                    <div className="audit-filters__group">
                        <label htmlFor="audit-filter-action" className="audit-filters__label">Action</label>
                        <select
                            id="audit-filter-action"
                            className="audit-filters__select"
                            value={actionFilter}
                            onChange={(e) => setActionFilter(e.target.value)}
                        >
                            {ACTION_OPTIONS.map((opt) => (
                                <option key={opt.value} value={opt.value}>
                                    {opt.label}
                                </option>
                            ))}
                        </select>
                    </div>

                    <div className="audit-filters__group">
                        <label htmlFor="audit-filter-resource" className="audit-filters__label">Resource Type</label>
                        <select
                            id="audit-filter-resource"
                            className="audit-filters__select"
                            value={resourceTypeFilter}
                            onChange={(e) => setResourceTypeFilter(e.target.value)}
                        >
                            {RESOURCE_TYPE_OPTIONS.map((opt) => (
                                <option key={opt.value} value={opt.value}>
                                    {opt.label}
                                </option>
                            ))}
                        </select>
                    </div>

                    <div className="audit-filters__group">
                        <label htmlFor="audit-filter-since" className="audit-filters__label">Since</label>
                        <input
                            id="audit-filter-since"
                            type="datetime-local"
                            className="audit-filters__input"
                            value={sinceFilter}
                            onChange={(e) => setSinceFilter(e.target.value)}
                        />
                    </div>

                    <div className="audit-filters__group">
                        <label htmlFor="audit-filter-until" className="audit-filters__label">Until</label>
                        <input
                            id="audit-filter-until"
                            type="datetime-local"
                            className="audit-filters__input"
                            value={untilFilter}
                            onChange={(e) => setUntilFilter(e.target.value)}
                        />
                    </div>

                    <div className="audit-filters__actions">
                        <button
                            id="audit-export-btn"
                            className="btn btn--outline"
                            onClick={handleExport}
                        >
                            Export CSV
                        </button>
                    </div>
                </div>
            </div>

            {/* Permission Denied State */}
            {error && error.message?.includes('403') && (
                <div className="empty-state">
                    <div className="empty-state__icon">
                        <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                            <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
                            <path d="M7 11V7a5 5 0 0 1 10 0v4" />
                        </svg>
                    </div>
                    <div className="empty-state__title">Permission Denied</div>
                    <div className="empty-state__desc">
                        You do not have permission to view the audit log. Contact your administrator for access.
                    </div>
                </div>
            )}

            {/* Loading State */}
            {loading && (
                <div className="audit-table-wrapper">
                    <table className="audit-table">
                        <thead>
                            <tr>
                                <th>Timestamp</th>
                                <th>Actor</th>
                                <th>Action</th>
                                <th>Resource Type</th>
                                <th>Resource ID</th>
                                <th>IP Address</th>
                                <th>Details</th>
                            </tr>
                        </thead>
                        <tbody>
                            <TableSkeleton rows={5} />
                        </tbody>
                    </table>
                </div>
            )}

            {/* Empty State */}
            {!loading && !error && entries.length === 0 && (
                <div className="empty-state">
                    <div className="empty-state__icon">
                        <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
                            <polyline points="14,2 14,8 20,8" />
                            <line x1="16" y1="13" x2="8" y2="13" />
                            <line x1="16" y1="17" x2="8" y2="17" />
                            <polyline points="10,9 9,9 8,9" />
                        </svg>
                    </div>
                    <div className="empty-state__title">No audit entries found</div>
                    <div className="empty-state__desc">
                        Try adjusting your filters or date range to see more results.
                    </div>
                </div>
            )}

            {/* Data Table */}
            {!loading && !error && entries.length > 0 && (
                <>
                    <div className="audit-table-wrapper">
                        <table className="audit-table">
                            <thead>
                                <tr>
                                    <th>Timestamp</th>
                                    <th>Actor</th>
                                    <th>Action</th>
                                    <th>Resource Type</th>
                                    <th>Resource ID</th>
                                    <th>IP Address</th>
                                    <th>Details</th>
                                </tr>
                            </thead>
                            <tbody>
                                {entries.map((entry) => (
                                    <tr key={entry.id} id={`audit-row-${entry.id}`}>
                                        <td className="audit-table__timestamp">
                                            {formatDate(entry.created_at)}
                                        </td>
                                        <td className="audit-table__actor">
                                            {entry.actor_email || 'System'}
                                        </td>
                                        <td className="audit-table__action">
                                            <span className="audit-table__action-badge">
                                                {entry.action}
                                            </span>
                                        </td>
                                        <td className="audit-table__resource-type">
                                            {entry.resource_type}
                                        </td>
                                        <td className="audit-table__resource-id">
                                            {entry.resource_id || '—'}
                                        </td>
                                        <td className="audit-table__ip">
                                            {entry.ip_address || '—'}
                                        </td>
                                        <td className="audit-table__details">
                                            <JsonViewer details={entry.details as Record<string, unknown> | null} />
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>

                    {/* Pagination */}
                    <div className="audit-pagination">
                        <div className="audit-pagination__info">
                            Showing {offset + 1} - {Math.min(offset + limit, total)} of {total} entries
                        </div>
                        <div className="audit-pagination__controls">
                            <button
                                id="audit-prev-btn"
                                className="btn btn--outline btn--sm"
                                onClick={handlePrev}
                                disabled={!hasPrev}
                            >
                                Previous
                            </button>
                            <span className="audit-pagination__page">
                                Page {currentPage} of {totalPages}
                            </span>
                            <button
                                id="audit-next-btn"
                                className="btn btn--outline btn--sm"
                                onClick={handleNext}
                                disabled={!hasNext}
                            >
                                Next
                            </button>
                        </div>
                    </div>
                </>
            )}
        </div>
    );
};

export default AuditLog;
