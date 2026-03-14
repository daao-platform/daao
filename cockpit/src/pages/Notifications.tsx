import React, { useState, useEffect, useCallback, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { getNotifications, markNotificationRead, markAllNotificationsRead } from '../api/client';
import type { NotificationItem, NotificationPriority } from '../api/client';

const PRIORITY_COLORS: Record<string, string> = {
    CRITICAL: '#ff4757',
    WARNING: '#ffa502',
    INFO: '#2ed573',
};

const TYPE_LABELS: Record<string, string> = {
    SESSION_TERMINATED: 'Session Terminated',
    SESSION_SUSPENDED: 'Session Suspended',
    SESSION_ERROR: 'Session Error',
    SATELLITE_OFFLINE: 'Satellite Offline',
};

const TYPE_ICONS: Record<string, string> = {
    SESSION_TERMINATED: '⏹',
    SESSION_SUSPENDED: '⏸',
    SESSION_ERROR: '⚠',
    SATELLITE_OFFLINE: '📡',
};

function formatDate(dateStr: string): string {
    const d = new Date(dateStr);
    const now = new Date();
    const diffMs = now.getTime() - d.getTime();
    const diffMins = Math.floor(diffMs / 60000);

    if (diffMins < 1) return 'just now';
    if (diffMins < 60) return `${diffMins} min ago`;
    const diffHrs = Math.floor(diffMins / 60);
    if (diffHrs < 24) return `${diffHrs}h ago`;

    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
}

type FilterType = 'all' | 'session' | 'satellite';
type FilterPriority = 'all' | NotificationPriority;

const Notifications: React.FC = () => {
    const navigate = useNavigate();
    const [items, setItems] = useState<NotificationItem[]>([]);
    const [loading, setLoading] = useState(true);
    const [cursor, setCursor] = useState<string | undefined>();
    const [hasMore, setHasMore] = useState(true);
    const [filterType, setFilterType] = useState<FilterType>('all');
    const [filterPriority, setFilterPriority] = useState<FilterPriority>('all');
    const observerRef = useRef<HTMLDivElement>(null);

    const fetchNotifications = useCallback(async (cursorVal?: string, append = false) => {
        try {
            const result = await getNotifications(50, cursorVal);
            const notifs = result.notifications || [];
            setItems(prev => append ? [...prev, ...notifs] : notifs);
            setCursor(result.next_cursor || undefined);
            setHasMore(!!result.next_cursor);
        } catch {
            // Silent fail
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        fetchNotifications();
    }, [fetchNotifications]);

    // Infinite scroll
    useEffect(() => {
        if (!hasMore || loading) return;
        const observer = new IntersectionObserver(
            entries => {
                if (entries[0]?.isIntersecting && cursor) {
                    fetchNotifications(cursor, true);
                }
            },
            { threshold: 0.5 }
        );
        const current = observerRef.current;
        if (current) observer.observe(current);
        return () => { if (current) observer.unobserve(current); };
    }, [hasMore, loading, cursor, fetchNotifications]);

    const handleMarkRead = async (id: string) => {
        await markNotificationRead(id);
        setItems(prev => prev.map(n => n.id === id ? { ...n, read: true } : n));
    };

    const handleMarkAllRead = async () => {
        await markAllNotificationsRead();
        setItems(prev => prev.map(n => ({ ...n, read: true })));
    };

    const handleClick = (notif: NotificationItem) => {
        if (!notif.read) handleMarkRead(notif.id);
        if (notif.session_id) navigate(`/session/${notif.session_id}`);
        else if (notif.satellite_id) navigate('/satellites');
    };

    // Filter
    const filtered = items.filter(n => {
        if (filterType === 'session' && !n.type.startsWith('SESSION')) return false;
        if (filterType === 'satellite' && n.type !== 'SATELLITE_OFFLINE') return false;
        if (filterPriority !== 'all' && n.priority !== filterPriority) return false;
        return true;
    });

    const unreadCount = items.filter(n => !n.read).length;

    return (
        <div className="notifications-page">
            <div className="notifications-page__header">
                <div>
                    <h1>Notifications</h1>
                    <p className="notifications-page__subtitle">{unreadCount} unread</p>
                </div>
                <div className="notifications-page__controls">
                    <select
                        value={filterType}
                        onChange={e => setFilterType(e.target.value as FilterType)}
                        className="notifications-page__filter"
                    >
                        <option value="all">All types</option>
                        <option value="session">Sessions</option>
                        <option value="satellite">Satellites</option>
                    </select>
                    <select
                        value={filterPriority}
                        onChange={e => setFilterPriority(e.target.value as FilterPriority)}
                        className="notifications-page__filter"
                    >
                        <option value="all">All priorities</option>
                        <option value="CRITICAL">Critical</option>
                        <option value="WARNING">Warning</option>
                        <option value="INFO">Info</option>
                    </select>
                    {unreadCount > 0 && (
                        <button className="notifications-page__mark-all" onClick={handleMarkAllRead}>
                            Mark all read
                        </button>
                    )}
                </div>
            </div>

            {loading ? (
                <div className="notifications-page__loading">
                    <div className="spinner" />
                </div>
            ) : filtered.length === 0 ? (
                <div className="notifications-page__empty">
                    <span style={{ fontSize: 48, opacity: 0.3 }}>🔔</span>
                    <h3>No notifications</h3>
                    <p>When events happen in your sessions and satellites, they'll appear here.</p>
                </div>
            ) : (
                <div className="notifications-page__list">
                    {filtered.map(notif => (
                        <button
                            key={notif.id}
                            className={`notifications-page__item${notif.read ? '' : ' notifications-page__item--unread'}`}
                            onClick={() => handleClick(notif)}
                        >
                            <span
                                className="notifications-page__item-icon"
                                style={{ color: PRIORITY_COLORS[notif.priority] || '#999' }}
                            >
                                {TYPE_ICONS[notif.type] || '🔔'}
                            </span>
                            <div className="notifications-page__item-content">
                                <div className="notifications-page__item-top">
                                    <span className="notifications-page__item-title">{notif.title}</span>
                                    <span className="notifications-page__item-badge" style={{ background: PRIORITY_COLORS[notif.priority] + '22', color: PRIORITY_COLORS[notif.priority] }}>
                                        {notif.priority}
                                    </span>
                                </div>
                                <div className="notifications-page__item-body">{notif.body}</div>
                                <div className="notifications-page__item-meta">
                                    <span>{TYPE_LABELS[notif.type] || notif.type}</span>
                                    <span>·</span>
                                    <span>{formatDate(notif.created_at)}</span>
                                </div>
                            </div>
                            {!notif.read && <span className="notifications-page__item-dot" />}
                        </button>
                    ))}
                    {hasMore && <div ref={observerRef} className="notifications-page__sentinel" />}
                </div>
            )}
        </div>
    );
};

export default Notifications;
