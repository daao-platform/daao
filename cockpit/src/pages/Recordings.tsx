import { useState, useEffect, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { listAllRecordings, type RecordingWithSession, getSessionPreview } from '../api/client';
import { useLicense } from '../hooks/useLicense';

/**
 * Recordings — browse and play back recorded terminal sessions.
 * Card grid layout with terminal preview thumbnails matching Stitch mockup.
 */
export default function Recordings() {
    const navigate = useNavigate();
    const [recordings, setRecordings] = useState<RecordingWithSession[]>([]);
    const [loading, setLoading] = useState(true);
    const [search, setSearch] = useState('');
    const [previews, setPreviews] = useState<Record<string, string>>({});
    const { license, isCommunity } = useLicense();

    useEffect(() => {
        (async () => {
            try {
                const data = await listAllRecordings();
                setRecordings(data);

                // Fetch terminal previews for each recording's session
                const previewMap: Record<string, string> = {};
                for (const rec of data.slice(0, 10)) {
                    try {
                        const p = await getSessionPreview(rec.session_id);
                        if (p.has_content) {
                            previewMap[rec.id] = p.text;
                        }
                    } catch { /* preview is non-critical */ }
                }
                setPreviews(previewMap);
            } catch (err) {
                console.error('Failed to load recordings:', err);
            } finally {
                setLoading(false);
            }
        })();
    }, []);

    const filtered = useMemo(() => {
        if (!search.trim()) return recordings;
        const q = search.toLowerCase();
        return recordings.filter(r =>
            (r.session_name || '').toLowerCase().includes(q) ||
            (r.filename || '').toLowerCase().includes(q) ||
            (r.id || '').toLowerCase().includes(q)
        );
    }, [recordings, search]);

    const formatDuration = (ms: number) => {
        if (ms <= 0) return '—';
        const s = Math.floor(ms / 1000);
        const m = Math.floor(s / 60);
        const rem = s % 60;
        return `${m}:${rem.toString().padStart(2, '0')}`;
    };

    const formatSize = (bytes: number) => {
        if (bytes <= 0) return '—';
        if (bytes < 1024) return `${bytes}B`;
        if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`;
        return `${(bytes / (1024 * 1024)).toFixed(1)}MB`;
    };

    const formatDate = (dateStr: string) => {
        const d = new Date(dateStr);
        return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
    };

    const maxRecordings = license?.max_recordings || 50;
    const storagePct = Math.min(100, (recordings.length / maxRecordings) * 100);

    return (
        <div>
            {/* Header */}
            <div className="page-header" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                <div>
                    <h1 className="page-header-title" style={{ textTransform: 'uppercase', letterSpacing: '0.02em' }}>Recordings</h1>
                    <div className="page-header-subtitle">Browse and play back recorded terminal sessions</div>
                </div>
                <button className="btn btn--primary btn--sm" style={{ gap: 6, flexShrink: 0 }}>
                    <span style={{ fontSize: 14 }}>●</span> New Recording
                </button>
            </div>

            {/* Search */}
            <div style={{ marginBottom: '1.5rem' }}>
                <input
                    type="text"
                    placeholder="🔍  Search recordings..."
                    value={search}
                    onChange={e => setSearch(e.target.value)}
                    className="search-input"
                    style={{ maxWidth: '600px' }}
                />
            </div>

            {/* Recording limit nudge */}
            {license && license.max_recordings > 0 && (() => {
                const count = recordings.length;
                const limit = license.max_recordings;
                const pct = Math.round((count / limit) * 100);
                const atLimit = count >= limit;
                const nearLimit = count >= limit * 0.8;
                if (!nearLimit) return null;
                return (
                    <div className={`limit-bar ${atLimit ? 'limit-bar--warning' : 'limit-bar--info'}`}>
                        <div className="limit-bar__content">
                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                                {atLimit ? (
                                    <><circle cx="12" cy="12" r="10" /><line x1="12" y1="8" x2="12" y2="12" /><line x1="12" y1="16" x2="12.01" y2="16" /></>
                                ) : (
                                    <><circle cx="12" cy="12" r="10" /><line x1="12" y1="16" x2="12" y2="12" /><line x1="12" y1="8" x2="12.01" y2="8" /></>
                                )}
                            </svg>
                            <span>
                                {atLimit
                                    ? `Recording limit reached (${count}/${limit})`
                                    : `${count} of ${limit} recordings used (${pct}%)`
                                }
                            </span>
                            {isCommunity && (
                                <a href="https://daao.dev/pricing" target="_blank" rel="noopener noreferrer" className="limit-bar__upgrade">
                                    Upgrade for unlimited →
                                </a>
                            )}
                        </div>
                        <div className="limit-bar__progress">
                            <div className="limit-bar__fill" style={{ width: `${Math.min(pct, 100)}%` }} />
                        </div>
                    </div>
                );
            })()}

            {/* Recordings Grid */}
            {loading ? (
                <div style={{ textAlign: 'center', padding: '3rem', color: 'var(--text-muted)' }}>
                    <div className="spinner" />
                    <p>Loading recordings...</p>
                </div>
            ) : (
                <div className="recordings-grid">
                    {filtered.map(rec => (
                        <div
                            key={rec.id}
                            className="recording-card"
                            onClick={() => navigate(`/recording/${rec.id}`)}
                        >
                            {/* Terminal Preview */}
                            <div className="recording-card__preview">
                                <div className="recording-card__preview-dots">
                                    <span /><span /><span />
                                </div>
                                <pre>{previews[rec.id] || `$ ${rec.session_name || 'Session'}\n> Recording started...`}</pre>
                                <div className="recording-card__duration">
                                    {rec.stopped_at ? `⏱ ${formatDuration(rec.duration_ms)}` : '● LIVE'}
                                </div>
                            </div>

                            {/* Info */}
                            <div className="recording-card__info">
                                <div className="recording-card__name">
                                    {rec.session_name || 'Session Recording'}
                                </div>
                                <div className="recording-card__meta">
                                    <span>📅 {formatDate(rec.started_at)}</span>
                                    <span>·</span>
                                    <span>📡 {rec.session_id?.slice(0, 8) || 'local'}</span>
                                    <span className="recording-card__size">{formatSize(rec.size_bytes)}</span>
                                </div>
                            </div>
                        </div>
                    ))}

                    {/* Empty slot CTA */}
                    {filtered.length === 0 && !search && (
                        <div className="recording-card recording-card--empty">
                            <div style={{ fontSize: '2.5rem', marginBottom: 12, opacity: 0.3 }}>👻</div>
                            <div style={{ fontWeight: 600, fontSize: 13, color: 'var(--text-muted)', marginBottom: 4 }}>
                                No recordings yet
                            </div>
                            <button className="btn btn--primary btn--sm" style={{ marginTop: 8 }}>
                                START A RECORDING
                            </button>
                        </div>
                    )}

                    {filtered.length > 0 && filtered.length < 4 && (
                        <div className="recording-card recording-card--empty">
                            <div style={{ fontSize: '2rem', marginBottom: 8, opacity: 0.3 }}>👻</div>
                            <div style={{ fontWeight: 600, fontSize: 12, color: 'var(--text-muted)' }}>
                                START A RECORDING
                            </div>
                        </div>
                    )}

                    {search && filtered.length === 0 && (
                        <div className="recording-card recording-card--empty" style={{ gridColumn: '1 / -1' }}>
                            <div style={{ fontSize: '2.5rem', marginBottom: 12, opacity: 0.3 }}>🔍</div>
                            <div style={{ fontWeight: 600, fontSize: 13, color: 'var(--text-muted)' }}>
                                No matching recordings
                            </div>
                            <div style={{ fontSize: 12, color: 'var(--text-muted)', marginTop: 4 }}>
                                Try a different search term
                            </div>
                        </div>
                    )}
                </div>
            )}

            {/* Storage Limit Footer */}
            {!loading && (
                <div className="recordings-storage">
                    <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                        <span style={{ fontWeight: 600, color: 'var(--text)' }}>Storage Limit</span>
                        <span style={{ fontWeight: 700, fontSize: 14, color: 'var(--text)' }}>
                            {recordings.length} / {maxRecordings}
                        </span>
                    </div>
                    <div>
                        <div style={{ fontSize: 11, color: 'var(--text-muted)', marginBottom: 4 }}>
                            {recordings.length} recording{recordings.length !== 1 ? 's' : ''} stored out of {maxRecordings} total allowed
                        </div>
                        <div className="recordings-storage__bar" style={{ width: '100%' }}>
                            <div className="recordings-storage__fill" style={{ width: `${storagePct}%` }} />
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}
