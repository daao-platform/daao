import React, { useState, useEffect, useCallback, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { getSatellites, getSessions, createSatellite, deleteSatellite, renameSatellite, getSatelliteTelemetry, getSatelliteTelemetryHistory, Satellite, Session, SessionsPaginatedResponse, TelemetryData, TelemetryPoint } from '../api/client';
import { useApi } from '../hooks';
import { useLicense } from '../hooks/useLicense';
import { SatellitesIcon, PlusIcon, MoreIcon, ServerIcon, XIcon } from '../components/Icons';
import EnterpriseBadge from '../components/EnterpriseBadge';
import Sparkline from '../components/Sparkline';
import ContextEditor from '../components/ContextEditor';
import SatelliteTagEditor from '../components/SatelliteTagEditor';

/**
 * Format a date string as relative time (e.g., "2h ago", "3d ago")
 */
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

/**
 * Status badge component
 */
const StatusBadge: React.FC<{ status: string }> = ({ status }) => {
    const isActive = status?.toLowerCase() === 'active' || status?.toLowerCase() === 'online';
    return (
        <span className={`badge ${isActive ? 'badge--running' : 'badge--suspended'}`}>
            <span className="badge__dot" />
            {isActive ? 'Active' : status?.charAt(0).toUpperCase() + status?.slice(1) || 'Pending'}
        </span>
    );
};

/**
 * Copy button component with visual feedback
 */
const CopyButton: React.FC<{ text: string; label?: string }> = ({ text, label = 'Copy' }) => {
    const [copied, setCopied] = useState(false);

    const handleCopy = async () => {
        try {
            await navigator.clipboard.writeText(text);
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
        } catch {
            // Fallback for non-HTTPS contexts
            const textarea = document.createElement('textarea');
            textarea.value = text;
            document.body.appendChild(textarea);
            textarea.select();
            document.execCommand('copy');
            document.body.removeChild(textarea);
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
        }
    };

    return (
        <button
            className={`btn btn--outline btn--sm ${copied ? 'btn--success' : ''}`}
            onClick={handleCopy}
            type="button"
            style={{ fontSize: 12, minWidth: 56 }}
        >
            {copied ? '✓ Copied' : label}
        </button>
    );
};

/**
 * Setup instructions component — shows install commands for the satellite
 */
const SetupInstructions: React.FC<{ nexusUrl?: string; compact?: boolean }> = ({ nexusUrl, compact = false }) => {
    const baseUrl = nexusUrl || (typeof window !== 'undefined' ? `${window.location.protocol}//${window.location.host}` : 'https://your-nexus');
    const [activeTab, setActiveTab] = useState<'linux' | 'windows'>('linux');

    const linuxCmd = `curl -fsSL ${baseUrl}/install | NEXUS_URL=${baseUrl} bash`;
    const windowsCmd = `$env:NEXUS_URL = "${baseUrl}"; irm ${baseUrl}/install.ps1 | iex`;

    return (
        <div className="setup-instructions" style={{
            background: 'var(--bg-elevated)',
            border: '1px solid var(--border)',
            borderRadius: 'var(--radius-md)',
            padding: compact ? 12 : 16,
            marginTop: compact ? 8 : 0,
        }}>
            {!compact && (
                <div style={{ fontWeight: 600, fontSize: 14, marginBottom: 12, color: 'var(--text-primary)' }}>
                    Install the DAAO daemon on your satellite machine:
                </div>
            )}

            {/* Tab selector */}
            <div style={{ display: 'flex', gap: 0, marginBottom: 10, borderBottom: '1px solid var(--border)' }}>
                <button
                    onClick={() => setActiveTab('linux')}
                    style={{
                        padding: '6px 14px',
                        fontSize: 12,
                        fontWeight: 500,
                        background: 'none',
                        border: 'none',
                        borderBottom: activeTab === 'linux' ? '2px solid var(--accent)' : '2px solid transparent',
                        color: activeTab === 'linux' ? 'var(--text-primary)' : 'var(--text-muted)',
                        cursor: 'pointer',
                    }}
                >
                    Linux / macOS
                </button>
                <button
                    onClick={() => setActiveTab('windows')}
                    style={{
                        padding: '6px 14px',
                        fontSize: 12,
                        fontWeight: 500,
                        background: 'none',
                        border: 'none',
                        borderBottom: activeTab === 'windows' ? '2px solid var(--accent)' : '2px solid transparent',
                        color: activeTab === 'windows' ? 'var(--text-primary)' : 'var(--text-muted)',
                        cursor: 'pointer',
                    }}
                >
                    Windows
                </button>
            </div>

            {/* Command display */}
            <div style={{
                background: 'var(--bg-primary)',
                borderRadius: 'var(--radius-sm)',
                padding: '10px 12px',
                fontFamily: 'var(--font-mono, monospace)',
                fontSize: 12,
                lineHeight: 1.5,
                color: 'var(--text-secondary)',
                wordBreak: 'break-all',
                display: 'flex',
                alignItems: 'flex-start',
                justifyContent: 'space-between',
                gap: 8,
            }}>
                <code style={{ flex: 1, userSelect: 'all' }}>
                    {activeTab === 'linux' ? linuxCmd : windowsCmd}
                </code>
                <CopyButton text={activeTab === 'linux' ? linuxCmd : windowsCmd} />
            </div>

            {!compact && (
                <div style={{ fontSize: 12, color: 'var(--text-muted)', marginTop: 10 }}>
                    This will download the daemon, register with Nexus, and set up auto-start.
                </div>
            )}
        </div>
    );
};

/**
 * Add Satellite Modal — with post-registration setup instructions
 */
const AddSatelliteModal: React.FC<{
    isOpen: boolean;
    onClose: () => void;
    onCreated: () => void;
}> = ({ isOpen, onClose, onCreated }) => {
    const [name, setName] = useState('');
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [showSetup, setShowSetup] = useState(false);

    if (!isOpen) return null;

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        if (!name.trim()) return;

        setLoading(true);
        setError(null);

        try {
            await createSatellite({ name: name.trim() });
            setShowSetup(true);
            onCreated();
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Failed to create satellite');
        } finally {
            setLoading(false);
        }
    };

    const handleClose = () => {
        setName('');
        setError(null);
        setShowSetup(false);
        onClose();
    };

    const handleOverlayClick = (e: React.MouseEvent) => {
        if (e.target === e.currentTarget) handleClose();
    };

    return (
        <div className="modal-overlay" onClick={handleOverlayClick}>
            <div className="modal" role="dialog" aria-modal="true" aria-labelledby="add-sat-title" style={{ maxWidth: showSetup ? 560 : 440 }}>
                <div className="modal__header">
                    <h2 id="add-sat-title" className="modal__title">
                        {showSetup ? '✓ Satellite Registered' : 'Add Satellite'}
                    </h2>
                    <button className="modal__close" onClick={handleClose} type="button" aria-label="Close">
                        <XIcon size={20} />
                    </button>
                </div>

                {showSetup ? (
                    /* Post-registration setup instructions */
                    <div>
                        <div className="modal__body">
                            <div style={{
                                background: 'var(--bg-success, rgba(34, 197, 94, 0.1))',
                                border: '1px solid var(--border-success, rgba(34, 197, 94, 0.3))',
                                borderRadius: 'var(--radius-md)',
                                padding: '10px 14px',
                                fontSize: 13,
                                color: 'var(--text-secondary)',
                                marginBottom: 16,
                            }}>
                                <strong>{name}</strong> has been registered. Now install the daemon on that machine to complete setup.
                            </div>
                            <SetupInstructions />
                        </div>
                        <div className="modal__footer">
                            <button type="button" className="btn btn--primary" onClick={handleClose}>
                                Done
                            </button>
                        </div>
                    </div>
                ) : (
                    /* Registration form */
                    <form onSubmit={handleSubmit}>
                        <div className="modal__body">
                            {error && <div className="form-error">{error}</div>}
                            <div className="form-group">
                                <label htmlFor="sat-name" className="form-label">
                                    Machine Name
                                    <span className="form-label--required"> *</span>
                                </label>
                                <input
                                    id="sat-name"
                                    type="text"
                                    className="form-input"
                                    placeholder="e.g., dev-workstation, gpu-server-1"
                                    value={name}
                                    onChange={(e) => { setName(e.target.value); setError(null); }}
                                    required
                                    autoFocus
                                />
                            </div>
                        </div>
                        <div className="modal__footer">
                            <button type="button" className="btn btn--secondary" onClick={handleClose} disabled={loading}>
                                Cancel
                            </button>
                            <button type="submit" className="btn btn--primary" disabled={loading || !name.trim()}>
                                {loading ? 'Registering...' : 'Register Satellite'}
                            </button>
                        </div>
                    </form>
                )}
            </div>
        </div>
    );
};

/**
 * Satellites page — Real data from API with Add/Delete functionality
 */
const Satellites: React.FC = () => {
    const navigate = useNavigate();
    const [showAddModal, setShowAddModal] = useState(false);
    const [deletingId, setDeletingId] = useState<string | null>(null);
    const [menuOpenId, setMenuOpenId] = useState<string | null>(null);
    const [expandedSetup, setExpandedSetup] = useState<string | null>(null);
    const [renamingId, setRenamingId] = useState<string | null>(null);
    const [renameName, setRenameName] = useState('');
    const [renameLoading, setRenameLoading] = useState(false);
    const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
    const [expandedContext, setExpandedContext] = useState<string | null>(null);
    const menuRef = useRef<HTMLDivElement>(null);
    const { license, isCommunity } = useLicense();

    // Telemetry state for active satellites
    const [telemetryMap, setTelemetryMap] = useState<Record<string, TelemetryData>>({});
    const [historyMap, setHistoryMap] = useState<Record<string, TelemetryPoint[]>>({});

    // Use useApi hook to fetch satellites
    const { data: satellites, loading: satellitesLoading, error: satellitesError, refetch: refetchSatellites } = useApi<Satellite[]>(() => getSatellites());

    // Use useApi hook to fetch sessions for counting active sessions per satellite
    const { data: sessionsData, loading: sessionsLoading, error: sessionsError, refetch: refetchSessions } = useApi<SessionsPaginatedResponse>(() => getSessions());
    const sessions = sessionsData?.items ?? [];

    const loading = satellitesLoading || sessionsLoading;
    const error = satellitesError || sessionsError;

    // Fetch telemetry for active satellites
    const fetchTelemetry = useCallback(async () => {
        if (!satellites) return;
        const active = satellites.filter(s => s.status?.toLowerCase() === 'active' || s.status?.toLowerCase() === 'online');
        for (const sat of active) {
            try {
                const [telem, hist] = await Promise.all([
                    getSatelliteTelemetry(sat.id).catch(() => null),
                    getSatelliteTelemetryHistory(sat.id).catch(() => []),
                ]);
                if (telem) setTelemetryMap(prev => ({ ...prev, [sat.id]: telem }));
                if (hist) setHistoryMap(prev => ({ ...prev, [sat.id]: hist }));
            } catch { /* ignore */ }
        }
    }, [satellites]);

    useEffect(() => {
        fetchTelemetry();
        const interval = setInterval(fetchTelemetry, 15000);
        return () => clearInterval(interval);
    }, [fetchTelemetry]);

    // Compute active sessions count per satellite
    const getActiveSessionCount = (satelliteId: string): number => {
        if (!sessions) return 0;
        return sessions.filter(s =>
            s.satellite_id === satelliteId &&
            s.state?.toUpperCase() === 'RUNNING'
        ).length;
    };

    // Refetch function that refreshes both data sources
    const refetch = () => {
        refetchSatellites();
        refetchSessions();
    };

    // Handle delete satellite
    const handleDelete = async (id: string) => {
        setDeletingId(id);
        setMenuOpenId(null);
        setConfirmDeleteId(null);
        try {
            await deleteSatellite(id);
            refetch();
        } catch (err) {
            console.error('Failed to delete satellite:', err);
        } finally {
            setDeletingId(null);
        }
    };

    // Handle rename satellite
    const handleRename = async (id: string) => {
        if (!renameName.trim()) return;
        setRenameLoading(true);
        try {
            await renameSatellite(id, renameName.trim());
            setRenamingId(null);
            setRenameName('');
            refetch();
        } catch (err) {
            console.error('Failed to rename satellite:', err);
        } finally {
            setRenameLoading(false);
        }
    };

    // Close dropdown on click outside
    useEffect(() => {
        const handleClickOutside = (e: MouseEvent) => {
            if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
                setMenuOpenId(null);
            }
        };
        if (menuOpenId) {
            document.addEventListener('mousedown', handleClickOutside);
            return () => document.removeEventListener('mousedown', handleClickOutside);
        }
    }, [menuOpenId]);

    const isPending = (status: string) => {
        const s = status?.toLowerCase();
        return !s || s === 'pending';
    };

    const isOffline = (status: string) => {
        const s = status?.toLowerCase();
        return s === 'offline' || s === 'inactive';
    };

    // Color thresholds matching Dashboard's SystemHealth gauges
    const getBarColor = (value: number): string => {
        if (value < 70) return 'var(--accent)';
        if (value < 90) return 'var(--warning)';
        return 'var(--danger)';
    };

    return (
        <div>
            <div className="page-header">
                <h1 className="page-header-title">Satellites</h1>
                <div className="page-header-subtitle">Manage your remote machines</div>
            </div>

            {/* Loading State */}
            {loading && (
                <div className="empty-state">
                    <div className="empty-state__desc">Loading satellites...</div>
                </div>
            )}

            {/* Error State */}
            {error && !loading && (
                <div className="empty-state">
                    <div className="empty-state__icon">
                        <ServerIcon size={28} />
                    </div>
                    <div className="empty-state__title">Failed to load satellites</div>
                    <div className="empty-state__desc">
                        {error instanceof Error ? error.message : 'An unexpected error occurred'}
                    </div>
                    <button className="btn btn--primary btn--sm" onClick={refetch} style={{ marginTop: 16 }}>
                        Retry
                    </button>
                </div>
            )}

            {/* Empty State — with setup instructions */}
            {!loading && !error && (!satellites || satellites.length === 0) && (
                <div className="empty-state">
                    <div className="empty-state__icon">
                        <SatellitesIcon size={28} />
                    </div>
                    <div className="empty-state__title">No satellites registered</div>
                    <div className="empty-state__desc" style={{ marginBottom: 20 }}>
                        Register a machine and install the DAAO daemon to get started.
                    </div>
                    <button className="btn btn--primary btn--sm" style={{ marginBottom: 24 }} onClick={() => setShowAddModal(true)}>
                        <PlusIcon size={16} />
                        Add Satellite
                    </button>
                    <div style={{ maxWidth: 520, width: '100%', textAlign: 'left' }}>
                        <SetupInstructions />
                    </div>
                </div>
            )}

            {/* Satellite Cards */}
            {!loading && !error && satellites && satellites.length > 0 && (
                <>
                    <div className="section-header">
                        <h2 className="section-title">
                            {satellites.length} Machine{satellites.length !== 1 ? 's' : ''}
                            {isCommunity && license && license.max_satellites > 0 && (
                                <span className="section-title__limit"> / {license.max_satellites} max</span>
                            )}
                        </h2>
                        <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
                            {isCommunity && license && satellites.length >= (license.max_satellites * 0.8) && (
                                <span className="limit-badge">
                                    {satellites.length >= license.max_satellites ? 'Limit reached' : `${satellites.length}/${license.max_satellites}`}
                                </span>
                            )}
                            <button className="btn btn--primary btn--sm" onClick={() => setShowAddModal(true)}>
                                <PlusIcon size={16} />
                                Add Satellite
                            </button>
                        </div>
                    </div>

                    {/* Telemetry retention enterprise nudge */}
                    {isCommunity && (
                        <div className="enterprise-nudge">
                            <EnterpriseBadge size="small" />
                            <span className="enterprise-nudge__text">
                                Telemetry history limited to 1 hour — Enterprise unlocks 30-day retention
                            </span>
                        </div>
                    )}

                    {/* Active Satellites — expanded cards with telemetry */}
                    {satellites.filter(s => !isPending(s.status)).map(sat => {
                        const activeSessions = getActiveSessionCount(sat.id);
                        const isDeleting = deletingId === sat.id;
                        const telemetry = telemetryMap[sat.id];
                        return (
                            <div key={sat.id} className="card animate-fadeIn" style={{ marginBottom: 24, padding: 24 }}>
                                {/* Header: icon + name + status + heartbeat + manage */}
                                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 24 }}>
                                    <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
                                        <div style={{
                                            width: 48, height: 48, borderRadius: 12,
                                            background: 'rgba(45, 212, 191, 0.1)',
                                            border: '1px solid rgba(45, 212, 191, 0.2)',
                                            display: 'flex', alignItems: 'center', justifyContent: 'center',
                                        }}>
                                            <span className="material-symbols-outlined" style={{ color: 'var(--accent)', fontSize: 28 }}>terminal</span>
                                        </div>
                                        <div>
                                            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                                                <h2 style={{ fontSize: 20, fontWeight: 700, letterSpacing: '-0.02em' }}>{sat.name}</h2>
                                                <StatusBadge status={sat.status} />
                                            </div>
                                            <p style={{ fontSize: 13, color: 'var(--text-muted)', marginTop: 2 }}>
                                                {(sat as any).os || 'macOS arm64'} • DAAO v0.1.0 • <span style={{ color: 'var(--text-secondary)' }}>Uptime: {timeAgo(sat.created_at)}</span>
                                            </p>
                                            <SatelliteTagEditor
                                                satelliteId={sat.id}
                                                tags={(sat as any).tags || []}
                                                onSave={async () => refetchSatellites()}
                                            />
                                        </div>
                                    </div>
                                    <div style={{ display: 'flex', alignItems: 'center', gap: 24 }}>
                                        {sat.updated_at && (
                                            <div style={{ textAlign: 'right' }}>
                                                <p style={{ fontSize: 10, textTransform: 'uppercase', letterSpacing: '0.1em', color: 'var(--text-muted)', fontWeight: 700 }}>Last Heartbeat</p>
                                                <p style={{ fontSize: 14, color: 'var(--text-secondary)', fontWeight: 500 }}>{timeAgo(sat.updated_at)}</p>
                                            </div>
                                        )}
                                        <div style={{ position: 'relative' }} ref={menuOpenId === sat.id ? menuRef : undefined}>
                                            <button
                                                className="btn btn--outline btn--sm"
                                                onClick={() => setMenuOpenId(menuOpenId === sat.id ? null : sat.id)}
                                                aria-label="Manage node"
                                                style={{ padding: '6px 10px' }}
                                            >
                                                Manage Node
                                            </button>
                                            {menuOpenId === sat.id && (
                                                <div style={{
                                                    position: 'absolute', top: '100%', right: 0, marginTop: 4,
                                                    background: 'var(--bg-elevated)', border: '1px solid var(--border)',
                                                    borderRadius: 'var(--radius-md)', minWidth: 160,
                                                    boxShadow: '0 8px 24px rgba(0,0,0,0.4)', zIndex: 50,
                                                    overflow: 'hidden',
                                                }}>
                                                    <button
                                                        style={{
                                                            width: '100%', padding: '10px 14px', background: 'none',
                                                            border: 'none', color: 'var(--text-secondary)', fontSize: 13,
                                                            textAlign: 'left', cursor: 'pointer', display: 'flex',
                                                            alignItems: 'center', gap: 8,
                                                        }}
                                                        onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(255,255,255,0.05)')}
                                                        onMouseLeave={(e) => (e.currentTarget.style.background = 'none')}
                                                        onClick={() => {
                                                            setRenamingId(sat.id);
                                                            setRenameName(sat.name);
                                                            setMenuOpenId(null);
                                                        }}
                                                    >
                                                        <span className="material-symbols-outlined" style={{ fontSize: 16 }}>edit</span>
                                                        Rename
                                                    </button>
                                                    <div style={{ height: 1, background: 'var(--border)' }} />
                                                    <button
                                                        style={{
                                                            width: '100%', padding: '10px 14px', background: 'none',
                                                            border: 'none', color: 'var(--danger, #ef4444)', fontSize: 13,
                                                            textAlign: 'left', cursor: 'pointer', display: 'flex',
                                                            alignItems: 'center', gap: 8,
                                                        }}
                                                        onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(239,68,68,0.1)')}
                                                        onMouseLeave={(e) => (e.currentTarget.style.background = 'none')}
                                                        onClick={() => {
                                                            setConfirmDeleteId(sat.id);
                                                            setMenuOpenId(null);
                                                        }}
                                                    >
                                                        <span className="material-symbols-outlined" style={{ fontSize: 16 }}>delete</span>
                                                        Delete
                                                    </button>
                                                </div>
                                            )}
                                        </div>
                                    </div>
                                </div>

                                {/* 4-column telemetry grid with bar charts */}
                                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 24 }}>
                                    {/* CPU */}
                                    <div style={{ background: 'rgba(15, 23, 42, 0.4)', borderRadius: 8, padding: 16, border: '1px solid rgba(255,255,255,0.03)' }}>
                                        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                                            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>CPU Usage</span>
                                            <span style={{ fontSize: 12, color: telemetry ? getBarColor(telemetry.cpu_percent) : 'var(--accent)', fontWeight: 700 }}>{telemetry ? `${telemetry.cpu_percent.toFixed(1)}%` : '--'}</span>
                                        </div>
                                        <div style={{ height: 40, display: 'flex', alignItems: 'flex-end', gap: 2 }}>
                                            {[20, 35, 25, 60, 45, telemetry ? telemetry.cpu_percent : 15].map((h, i) => (
                                                <div key={i} style={{
                                                    flex: 1, height: `${h}%`, borderRadius: '2px 2px 0 0',
                                                    background: i === 5 ? (telemetry ? getBarColor(telemetry.cpu_percent) : 'var(--accent)') : 'rgba(45, 212, 191, 0.2)',
                                                }} />
                                            ))}
                                        </div>
                                    </div>
                                    {/* Memory */}
                                    <div style={{ background: 'rgba(15, 23, 42, 0.4)', borderRadius: 8, padding: 16, border: '1px solid rgba(255,255,255,0.03)' }}>
                                        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                                            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>Memory</span>
                                            <span style={{ fontSize: 12, color: telemetry ? getBarColor(telemetry.memory_percent) : 'var(--accent)', fontWeight: 700 }}>{telemetry ? `${telemetry.memory_percent.toFixed(1)}%` : '--'}</span>
                                        </div>
                                        <div style={{ height: 40, display: 'flex', alignItems: 'flex-end', gap: 2 }}>
                                            {[85, 90, 88, 92, 95, telemetry ? telemetry.memory_percent : 50].map((h, i) => (
                                                <div key={i} style={{
                                                    flex: 1, height: `${h}%`, borderRadius: '2px 2px 0 0',
                                                    background: i === 5 ? (telemetry ? getBarColor(telemetry.memory_percent) : 'var(--accent)') : 'rgba(45, 212, 191, 0.8)',
                                                }} />
                                            ))}
                                        </div>
                                    </div>
                                    {/* Disk */}
                                    <div style={{ background: 'rgba(15, 23, 42, 0.4)', borderRadius: 8, padding: 16, border: '1px solid rgba(255,255,255,0.03)' }}>
                                        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                                            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>Disk</span>
                                            <span style={{ fontSize: 12, color: telemetry ? getBarColor(telemetry.disk_percent) : 'var(--accent)', fontWeight: 700 }}>{telemetry ? `${telemetry.disk_percent.toFixed(1)}%` : '--'}</span>
                                        </div>
                                        <div style={{ height: 40, display: 'flex', alignItems: 'flex-end', gap: 2 }}>
                                            {[90, 92, 94, 93, 95, telemetry ? telemetry.disk_percent : 50].map((h, i) => (
                                                <div key={i} style={{
                                                    flex: 1, height: `${h}%`, borderRadius: '2px 2px 0 0',
                                                    background: i === 5 ? (telemetry ? getBarColor(telemetry.disk_percent) : 'var(--accent)') : 'rgba(45, 212, 191, 0.8)',
                                                }} />
                                            ))}
                                        </div>
                                    </div>
                                    {/* Active Sessions */}
                                    <div style={{
                                        background: 'rgba(15, 23, 42, 0.4)', borderRadius: 8, padding: 16,
                                        border: '1px solid rgba(255,255,255,0.03)',
                                        display: 'flex', flexDirection: 'column', justifyContent: 'center', alignItems: 'center',
                                    }}>
                                        <span style={{ fontSize: 10, textTransform: 'uppercase', fontWeight: 700, letterSpacing: '0.05em', color: 'var(--text-muted)', marginBottom: 4 }}>Active Sessions</span>
                                        <span style={{ fontSize: 32, fontWeight: 800, color: 'var(--text)' }}>{activeSessions}</span>
                                    </div>
                                </div>

                                {/* Context Files toggle */}
                                <div style={{ marginTop: 16 }}>
                                    <button
                                        className="btn btn--outline btn--sm"
                                        onClick={() => setExpandedContext(expandedContext === sat.id ? null : sat.id)}
                                        style={{ display: 'flex', alignItems: 'center', gap: 6 }}
                                    >
                                        <span style={{ fontSize: 14 }}>📄</span>
                                        Context Files
                                        <span style={{ fontSize: 10, opacity: 0.6 }}>{expandedContext === sat.id ? '▲' : '▼'}</span>
                                    </button>
                                    {expandedContext === sat.id && (
                                        <ContextEditor satelliteId={sat.id} />
                                    )}
                                </div>

                                {/* Offline warning */}
                                {isOffline(sat.status) && (
                                    <div style={{
                                        background: 'rgba(239, 68, 68, 0.08)',
                                        border: '1px solid rgba(239, 68, 68, 0.25)',
                                        borderRadius: 8, padding: '8px 12px', fontSize: 12,
                                        color: 'var(--text-secondary)', marginTop: 16,
                                    }}>
                                        ⚠ Daemon disconnected — restart the daao service on this machine
                                    </div>
                                )}
                            </div>
                        );
                    })}

                    {/* Pending Satellites — grouped under header */}
                    {satellites.filter(s => isPending(s.status)).length > 0 && (
                        <>
                            <h3 style={{
                                fontSize: 13, fontWeight: 700, color: 'var(--text-muted)',
                                textTransform: 'uppercase', letterSpacing: '0.1em', marginBottom: 16, marginTop: 8,
                            }}>
                                Pending Setup ({satellites.filter(s => isPending(s.status)).length})
                            </h3>
                            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 24 }}>
                                {satellites.filter(s => isPending(s.status)).map(sat => {
                                    const isDeleting = deletingId === sat.id;
                                    return (
                                        <div key={sat.id} className="animate-fadeIn" style={{
                                            background: 'rgba(15, 23, 42, 0.4)',
                                            border: '1px solid rgba(255, 255, 255, 0.05)',
                                            borderRadius: 12, padding: 20,
                                            display: 'flex', flexDirection: 'column', justifyContent: 'space-between',
                                        }}>
                                            <div>
                                                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
                                                    <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                                                        <div style={{
                                                            width: 32, height: 32, borderRadius: 8,
                                                            background: 'rgba(251, 191, 36, 0.1)',
                                                            border: '1px solid rgba(251, 191, 36, 0.2)',
                                                            display: 'flex', alignItems: 'center', justifyContent: 'center',
                                                        }}>
                                                            <span className="material-symbols-outlined" style={{ color: 'var(--warning)', fontSize: 18 }}>pending</span>
                                                        </div>
                                                        <h4 style={{ fontWeight: 700, fontSize: 15 }}>{sat.name}</h4>
                                                    </div>
                                                    <StatusBadge status={sat.status} />
                                                </div>
                                                <p style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 12, display: 'flex', alignItems: 'center', gap: 8 }}>
                                                    <span className="material-symbols-outlined" style={{ fontSize: 14 }}>info</span>
                                                    Waiting for daemon...
                                                </p>
                                                {expandedSetup === sat.id ? (
                                                    <div style={{ marginBottom: 16 }}>
                                                        <SetupInstructions compact />
                                                    </div>
                                                ) : (
                                                    <div style={{
                                                        background: 'rgba(0,0,0,0.5)', borderRadius: 8, padding: 12,
                                                        fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-secondary)',
                                                        border: '1px solid rgba(255,255,255,0.03)', marginBottom: 16,
                                                        overflow: 'hidden', position: 'relative',
                                                    }}>
                                                        <code>curl -sSL {window.location.origin}/install.sh | bash</code>
                                                    </div>
                                                )}
                                            </div>
                                            <button
                                                className="btn btn--outline"
                                                style={{ width: '100%' }}
                                                onClick={() => setExpandedSetup(expandedSetup === sat.id ? null : sat.id)}
                                            >
                                                {expandedSetup === sat.id ? 'Hide Instructions' : 'Setup'}
                                            </button>
                                        </div>
                                    );
                                })}
                            </div>
                        </>
                    )}
                </>
            )}

            <AddSatelliteModal
                isOpen={showAddModal}
                onClose={() => setShowAddModal(false)}
                onCreated={refetch}
            />

            {/* Rename Modal */}
            {renamingId && (
                <div className="modal-overlay" onClick={(e) => { if (e.target === e.currentTarget) { setRenamingId(null); setRenameName(''); } }}>
                    <div className="modal" role="dialog" aria-modal="true" style={{ maxWidth: 420 }}>
                        <div className="modal__header">
                            <h2 className="modal__title">Rename Satellite</h2>
                            <button className="modal__close" onClick={() => { setRenamingId(null); setRenameName(''); }} type="button" aria-label="Close">
                                <XIcon size={20} />
                            </button>
                        </div>
                        <form onSubmit={(e) => { e.preventDefault(); handleRename(renamingId); }}>
                            <div className="modal__body">
                                <div className="form-group">
                                    <label htmlFor="rename-sat" className="form-label">New Name</label>
                                    <input
                                        id="rename-sat"
                                        type="text"
                                        className="form-input"
                                        value={renameName}
                                        onChange={(e) => setRenameName(e.target.value)}
                                        autoFocus
                                        required
                                    />
                                </div>
                            </div>
                            <div className="modal__footer">
                                <button type="button" className="btn btn--secondary" onClick={() => { setRenamingId(null); setRenameName(''); }} disabled={renameLoading}>
                                    Cancel
                                </button>
                                <button type="submit" className="btn btn--primary" disabled={renameLoading || !renameName.trim()}>
                                    {renameLoading ? 'Renaming...' : 'Rename'}
                                </button>
                            </div>
                        </form>
                    </div>
                </div>
            )}

            {/* Delete Confirmation Modal */}
            {confirmDeleteId && (
                <div className="modal-overlay" onClick={(e) => { if (e.target === e.currentTarget) setConfirmDeleteId(null); }}>
                    <div className="modal" role="dialog" aria-modal="true" style={{ maxWidth: 420 }}>
                        <div className="modal__header">
                            <h2 className="modal__title">Delete Satellite</h2>
                            <button className="modal__close" onClick={() => setConfirmDeleteId(null)} type="button" aria-label="Close">
                                <XIcon size={20} />
                            </button>
                        </div>
                        <div className="modal__body">
                            <p style={{ color: 'var(--text-secondary)', fontSize: 14 }}>
                                Are you sure you want to delete <strong>{satellites?.find(s => s.id === confirmDeleteId)?.name}</strong>? This action cannot be undone.
                            </p>
                        </div>
                        <div className="modal__footer">
                            <button type="button" className="btn btn--secondary" onClick={() => setConfirmDeleteId(null)} disabled={deletingId === confirmDeleteId}>
                                Cancel
                            </button>
                            <button
                                type="button"
                                className="btn"
                                style={{ background: 'var(--danger, #ef4444)', color: '#fff', border: 'none' }}
                                disabled={deletingId === confirmDeleteId}
                                onClick={() => handleDelete(confirmDeleteId)}
                            >
                                {deletingId === confirmDeleteId ? 'Deleting...' : 'Delete'}
                            </button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
};

export default Satellites;
