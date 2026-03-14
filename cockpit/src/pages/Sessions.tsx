import React, { useState, useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { Session, getSessions, attachSession, detachSession, suspendSession, resumeSession, killSession, deleteSession, renameSession, SessionsPaginatedResponse } from '../api/client';
import { useApi } from '../hooks';
import { useWebSocket } from '../hooks';
import { useToast } from '../components/Toast';
import { TerminalIcon, PlusIcon } from '../components/Icons';
import NewSessionModal from '../components/NewSessionModal';
import TerminalPreview from '../components/TerminalPreview';

/**
 * State badge component
 */
const StateBadge: React.FC<{ state: string }> = ({ state }) => {
    const s = state?.toUpperCase() || 'UNKNOWN';
    let className = 'badge ';
    switch (s) {
        case 'RUNNING': className += 'badge--running'; break;
        case 'DETACHED': className += 'badge--detached'; break;
        case 'SUSPENDED': className += 'badge--suspended'; break;
        case 'PROVISIONING': className += 'badge--provisioning'; break;
        case 'TERMINATED': className += 'badge--terminated'; break;
        default: className += 'badge--terminated';
    }
    return (
        <span className={className}>
            <span className="badge__dot" />
            {s}
        </span>
    );
};

const timeAgo = (dateStr: string): string => {
    if (!dateStr) return '';
    const diff = Date.now() - new Date(dateStr).getTime();
    const mins = Math.floor(diff / 60000);
    if (mins < 1) return 'just now';
    if (mins < 60) return `${mins}m ago`;
    const hrs = Math.floor(mins / 60);
    if (hrs < 24) return `${hrs}h ago`;
    return `${Math.floor(hrs / 24)}d ago`;
};

type FilterState = 'all' | 'running' | 'detached' | 'suspended' | 'terminated';

/**
 * Inline session name with rename-on-click support
 */
const SessionName: React.FC<{ session: Session; onRenamed: (newName: string) => void }> = ({ session, onRenamed }) => {
    const [isRenaming, setIsRenaming] = useState(false);
    const [value, setValue] = useState('');
    const { showToast } = useToast();

    const startRename = (e: React.MouseEvent) => {
        e.stopPropagation();
        setValue(session.name || '');
        setIsRenaming(true);
    };

    const commitRename = async () => {
        const trimmed = value.trim();
        setIsRenaming(false);
        if (!trimmed || trimmed === session.name) return;
        try {
            await renameSession(session.id, trimmed);
            onRenamed(trimmed);
            showToast('Session renamed', 'success');
        } catch (err) {
            const msg = err instanceof Error ? err.message : 'Unknown error';
            showToast(`Failed to rename: ${msg}`, 'error');
        }
    };

    if (isRenaming) {
        return (
            <input
                className="session-card__rename-input"
                value={value}
                autoFocus
                onClick={e => e.stopPropagation()}
                onChange={e => setValue(e.target.value)}
                onKeyDown={e => {
                    if (e.key === 'Enter') commitRename();
                    else if (e.key === 'Escape') setIsRenaming(false);
                }}
                onBlur={commitRename}
            />
        );
    }

    return (
        <span className="session-card__name-wrapper">
            <span className="session-card__name">{session.name || session.id.slice(0, 12)}</span>
            <button
                className="session-card__rename-btn"
                onClick={startRename}
                title="Rename session"
                aria-label="Rename session"
            >
                ✎
            </button>
        </span>
    );
};

/**
 * Sessions page — Full session management with filters, search, actions, and deletion
 */
const Sessions: React.FC = () => {
    const [filter, setFilter] = useState<FilterState>('all');
    const [search, setSearch] = useState('');
    const [showModal, setShowModal] = useState(false);
    const [deletingIds, setDeletingIds] = useState<Set<string>>(new Set());
    const [clearingTerminated, setClearingTerminated] = useState(false);
    const [extraSessions, setExtraSessions] = useState<Session[]>([]);
    const [extraCursor, setExtraCursor] = useState<string | undefined>(undefined);
    const [loadingMore, setLoadingMore] = useState(false);
    const navigate = useNavigate();
    const { showToast } = useToast();

    // Use useApi hook for initial data fetch
    const { data: paginatedData, loading, error, refetch } = useApi<SessionsPaginatedResponse>(() => getSessions());

    // Reset extra (load-more) sessions when fresh data arrives from refetch
    useEffect(() => {
        if (paginatedData) {
            setExtraSessions([]);
            setExtraCursor(undefined);
        }
    }, [paginatedData]);

    // Derive full sessions list directly from paginatedData (no extra render cycle)
    const sessionsList = [...(paginatedData?.items ?? []), ...extraSessions];
    const nextCursor = extraCursor ?? paginatedData?.next_cursor;

    // Use useWebSocket hook for real-time updates
    const wsProtocol = typeof window !== 'undefined' && window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = typeof window !== 'undefined' ? `${wsProtocol}//${window.location.host}/api/v1/sessions/stream` : '';
    const { lastMessage } = useWebSocket(wsUrl);

    // Update sessions when WebSocket message received
    useEffect(() => {
        if (lastMessage && typeof lastMessage === 'object' && 'type' in lastMessage) {
            const msg = lastMessage as { type: string; sessions?: Session[] };
            if (msg.type === 'session_update' && Array.isArray(msg.sessions)) {
                refetch();
            }
        }
    }, [lastMessage, refetch]);

    // Load more sessions
    const loadMore = useCallback(async () => {
        if (!nextCursor || loadingMore) return;

        setLoadingMore(true);
        try {
            const response = await getSessions({ cursor: nextCursor });
            setExtraSessions(prev => [...prev, ...response.items]);
            setExtraCursor(response.next_cursor);
        } catch (err) {
            const errorMessage = err instanceof Error ? err.message : 'Unknown error';
            showToast(`Failed to load more sessions: ${errorMessage}`, 'error');
        } finally {
            setLoadingMore(false);
        }
    }, [nextCursor, loadingMore, showToast]);

    // Filter + search
    const filtered = (sessionsList || []).filter(s => {
        const state = s.state?.toUpperCase() || '';
        if (filter !== 'all' && state !== filter.toUpperCase()) return false;
        if (search) {
            const q = search.toLowerCase();
            return (
                (s.name || '').toLowerCase().includes(q) ||
                s.id.toLowerCase().includes(q) ||
                (s.agent_binary || '').toLowerCase().includes(q) ||
                (s.satellite_id || '').toLowerCase().includes(q)
            );
        }
        return true;
    });

    // Counts
    const counts = {
        all: (sessionsList || []).length,
        running: (sessionsList || []).filter(s => s.state?.toUpperCase() === 'RUNNING').length,
        detached: (sessionsList || []).filter(s => s.state?.toUpperCase() === 'DETACHED').length,
        suspended: (sessionsList || []).filter(s => s.state?.toUpperCase() === 'SUSPENDED').length,
        terminated: (sessionsList || []).filter(s => s.state?.toUpperCase() === 'TERMINATED').length,
    };

    // Actions
    const handleAction = async (sessionId: string, action: string) => {
        try {
            switch (action) {
                case 'detach':
                    await detachSession(sessionId);
                    showToast('Session detached successfully', 'success');
                    break;
                case 'suspend':
                    await suspendSession(sessionId);
                    showToast('Session suspended successfully', 'success');
                    break;
                case 'resume':
                    await resumeSession(sessionId);
                    showToast('Session resumed successfully', 'success');
                    break;
                case 'kill':
                    await killSession(sessionId);
                    showToast('Session terminated successfully', 'success');
                    break;
            }
            refetch();
        } catch (err) {
            const errorMessage = err instanceof Error ? err.message : 'Unknown error';
            showToast(`Failed to ${action} session: ${errorMessage}`, 'error');
            console.error(`Failed to ${action} session:`, err);
        }
    };

    // Delete a single terminated session
    const handleDelete = async (sessionId: string) => {
        setDeletingIds(prev => new Set(prev).add(sessionId));
        try {
            await deleteSession(sessionId);
            showToast('Session deleted', 'success');
            refetch();
        } catch (err) {
            const errorMessage = err instanceof Error ? err.message : 'Unknown error';
            showToast(`Failed to delete session: ${errorMessage}`, 'error');
        } finally {
            setDeletingIds(prev => {
                const next = new Set(prev);
                next.delete(sessionId);
                return next;
            });
        }
    };

    // Clear all terminated sessions
    const handleClearTerminated = async () => {
        const terminated = (sessionsList || []).filter(s => s.state?.toUpperCase() === 'TERMINATED');
        if (terminated.length === 0) return;

        setClearingTerminated(true);
        let deleted = 0;
        let failed = 0;

        for (const s of terminated) {
            try {
                await deleteSession(s.id);
                deleted++;
            } catch {
                failed++;
            }
        }

        setClearingTerminated(false);

        if (failed > 0) {
            showToast(`Deleted ${deleted} sessions, ${failed} failed`, 'error');
        } else {
            showToast(`Deleted ${deleted} terminated session${deleted !== 1 ? 's' : ''}`, 'success');
        }
        refetch();
    };

    const filters: { key: FilterState; label: string }[] = [
        { key: 'all', label: 'All' },
        { key: 'running', label: 'Running' },
        { key: 'detached', label: 'Detached' },
        { key: 'suspended', label: 'Suspended' },
        { key: 'terminated', label: 'Terminated' },
    ];

    return (
        <>
            <div>
                {/* Page Header */}
                <div className="page-header">
                    <h1 className="page-header-title">Sessions</h1>
                    <div className="page-header-subtitle">Manage active and historical agent sessions</div>
                </div>

                {/* Search + New Session + Clear Terminated */}
                <div className="sessions-toolbar">
                    <input
                        type="text"
                        className="search-input"
                        placeholder="Search sessions…"
                        value={search}
                        onChange={e => setSearch(e.target.value)}
                    />
                    <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                        {counts.terminated > 0 && (
                            <button
                                className="btn btn--outline btn--sm"
                                onClick={handleClearTerminated}
                                disabled={clearingTerminated}
                                title={`Delete all ${counts.terminated} terminated sessions`}
                            >
                                {clearingTerminated ? 'Clearing...' : `Clear ${counts.terminated} Terminated`}
                            </button>
                        )}
                        <button className="btn btn--primary btn--sm" onClick={() => setShowModal(true)}>
                            <PlusIcon size={16} />
                            New Session
                        </button>
                    </div>
                </div>

                {/* Filter Tabs */}
                <div className="filter-tabs">
                    {filters.map(f => (
                        <button
                            key={f.key}
                            className={`filter-tab${filter === f.key ? ' active' : ''}`}
                            onClick={() => setFilter(f.key)}
                        >
                            {f.label}
                            <span className="filter-tab__count">{counts[f.key]}</span>
                        </button>
                    ))}
                </div>

                {/* Session List */}
                {loading ? (
                    <div className="empty-state">
                        <div className="empty-state__desc">Loading sessions…</div>
                    </div>
                ) : filtered.length === 0 ? (
                    <div className="empty-state">
                        <div className="empty-state__icon">
                            <TerminalIcon size={28} />
                        </div>
                        <div className="empty-state__title">
                            {search ? 'No matching sessions' : filter !== 'all' ? `No ${filter} sessions` : 'No sessions yet'}
                        </div>
                        <div className="empty-state__desc">
                            {search
                                ? 'Try a different search term.'
                                : 'Start a new session to deploy an AI agent on your satellites.'}
                        </div>
                    </div>
                ) : (
                    <>
                        <div className="session-list" style={{ marginTop: 24 }}>
                            {filtered.map(session => {
                                const state = session.state?.toUpperCase() || 'UNKNOWN';
                                const isDeleting = deletingIds.has(session.id);
                                const stateColor = state === 'RUNNING' ? 'var(--accent)' : state === 'DETACHED' ? 'var(--warning)' : state === 'SUSPENDED' ? 'var(--info)' : 'var(--text-muted)';
                                return (
                                    <div key={session.id} className={`session-card animate-fadeIn session-card--${state.toLowerCase()}${isDeleting ? ' session-card--deleting' : ''}`}
                                        style={isDeleting ? { opacity: 0.5, pointerEvents: 'none' } : undefined}
                                    >
                                        <div className="session-card__grid">
                                            {/* Column 1: Name + Status */}
                                            <div className="session-card__info">
                                                <SessionName
                                                    session={session}
                                                    onRenamed={() => refetch()}
                                                />
                                                <div className="session-card__status-line" style={{ color: stateColor }}>
                                                    <div className={`session-card__status-dot session-card__status-dot--${state.toLowerCase()}`} />
                                                    {state}
                                                </div>
                                            </div>

                                            {/* Column 2: Details with icons */}
                                            <div className="session-card__details">
                                                <div className="session-card__detail-row">
                                                    <span className="material-symbols-outlined">terminal</span>
                                                    <span>{session.agent_binary || 'bash'}{session.cols && session.rows ? ` (${session.cols}×${session.rows})` : ''}</span>
                                                </div>
                                                <div className="session-card__detail-row">
                                                    <span className="material-symbols-outlined">satellite_alt</span>
                                                    <span>{session.satellite_id?.slice(0, 12) || 'local'}</span>
                                                </div>
                                                {session.last_activity_at && (
                                                    <div className="session-card__detail-row">
                                                        <span className="material-symbols-outlined">timer</span>
                                                        <span>{timeAgo(session.last_activity_at)}</span>
                                                    </div>
                                                )}
                                            </div>

                                            {/* Column 3: Terminal preview */}
                                            <div className="session-card__terminal-preview">
                                                <TerminalPreview sessionId={session.id} />
                                            </div>

                                            {/* Column 4: Actions */}
                                            <div className="session-card__actions" style={{ justifyContent: 'flex-end' }}>
                                                {(state === 'RUNNING' || state === 'DETACHED') && (
                                                    <button className="btn btn--primary btn--sm" onClick={() => navigate(`/session/${session.id}`)}>
                                                        Attach
                                                    </button>
                                                )}
                                                {state === 'RUNNING' && (
                                                    <>
                                                        <button className="btn btn--outline btn--sm" onClick={() => handleAction(session.id, 'detach')}>
                                                            Detach
                                                        </button>
                                                        <button className="btn btn--outline btn--sm" onClick={() => handleAction(session.id, 'suspend')}>
                                                            Suspend
                                                        </button>
                                                    </>
                                                )}
                                                {state === 'SUSPENDED' && (
                                                    <button className="btn btn--outline btn--sm" onClick={() => handleAction(session.id, 'resume')}>
                                                        Resume
                                                    </button>
                                                )}
                                                {state !== 'TERMINATED' && (
                                                    <button className="btn btn--danger btn--sm" onClick={() => handleAction(session.id, 'kill')}>
                                                        Kill
                                                    </button>
                                                )}
                                                {state === 'TERMINATED' && (
                                                    <button
                                                        className="btn btn--danger btn--sm"
                                                        onClick={() => handleDelete(session.id)}
                                                        disabled={isDeleting}
                                                    >
                                                        {isDeleting ? 'Deleting...' : 'Delete'}
                                                    </button>
                                                )}
                                            </div>
                                        </div>
                                    </div>
                                );
                            })}
                        </div>

                        {/* Session Footer */}
                        <div className="sessions-footer">
                            <span>Showing {filtered.length} of {sessionsList.length} sessions</span>
                            <div className="sessions-footer__counts">
                                <span className="sessions-footer__count">
                                    <span className="sessions-footer__dot" style={{ background: 'var(--accent)' }} />
                                    {counts.running} Running
                                </span>
                                <span className="sessions-footer__count">
                                    <span className="sessions-footer__dot" style={{ background: 'var(--warning)' }} />
                                    {counts.detached} Detached
                                </span>
                                <span className="sessions-footer__count">
                                    <span className="sessions-footer__dot" style={{ background: 'var(--text-muted)' }} />
                                    {counts.suspended + counts.terminated} Idle
                                </span>
                            </div>
                        </div>

                        {/* Load More button */}
                        {nextCursor && (
                            <div style={{ display: 'flex', justifyContent: 'center', marginTop: 16, marginBottom: 24 }}>
                                <button
                                    className="btn btn--outline"
                                    onClick={loadMore}
                                    disabled={loadingMore}
                                >
                                    {loadingMore ? 'Loading...' : 'Load More'}
                                </button>
                            </div>
                        )}
                    </>
                )}
            </div>
            <NewSessionModal
                isOpen={showModal}
                onClose={() => setShowModal(false)}
                onCreated={() => {
                    setShowModal(false);
                    refetch(); // Refetch sessions after creation
                }}
            />
        </>
    );
};

export default Sessions;
