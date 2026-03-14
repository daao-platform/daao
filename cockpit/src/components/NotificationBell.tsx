import React, { useState, useRef, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useNotifications } from '../hooks/useNotifications';
import type { NotificationItem } from '../api/client';

// ============================================================================
// Priority & Type Styling
// ============================================================================

const PRIORITY_COLORS: Record<string, string> = {
    CRITICAL: '#ff4757',
    WARNING: '#ffa502',
    INFO: '#2ed573',
};

const TYPE_ICONS: Record<string, string> = {
    SESSION_TERMINATED: '⏹',
    SESSION_SUSPENDED: '⏸',
    SESSION_ERROR: '⚠',
    SATELLITE_OFFLINE: '📡',
};

function timeAgo(dateStr: string): string {
    const diff = Date.now() - new Date(dateStr).getTime();
    const mins = Math.floor(diff / 60000);
    if (mins < 1) return 'just now';
    if (mins < 60) return `${mins}m ago`;
    const hrs = Math.floor(mins / 60);
    if (hrs < 24) return `${hrs}h ago`;
    const days = Math.floor(hrs / 24);
    return `${days}d ago`;
}

// ============================================================================
// NotificationBell
// ============================================================================

const NotificationBell: React.FC = () => {
    const { notifications, unreadCount, markRead, markAllRead } = useNotifications();
    const [open, setOpen] = useState(false);
    const panelRef = useRef<HTMLDivElement>(null);
    const navigate = useNavigate();

    // Close panel on outside click
    useEffect(() => {
        const handler = (e: MouseEvent) => {
            if (panelRef.current && !panelRef.current.contains(e.target as Node)) {
                setOpen(false);
            }
        };
        if (open) document.addEventListener('mousedown', handler);
        return () => document.removeEventListener('mousedown', handler);
    }, [open]);

    const handleNotificationClick = (notif: NotificationItem) => {
        if (!notif.read) markRead(notif.id);
        if (notif.session_id) {
            navigate(`/session/${notif.session_id}`);
            setOpen(false);
        } else if (notif.satellite_id) {
            navigate('/satellites');
            setOpen(false);
        }
    };

    return (
        <div className="notification-bell-wrapper" ref={panelRef}>
            <button
                className="notification-bell"
                onClick={() => setOpen(!open)}
                aria-label={`Notifications${unreadCount > 0 ? ` (${unreadCount} unread)` : ''}`}
                title="Notifications"
            >
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" />
                    <path d="M13.73 21a2 2 0 0 1-3.46 0" />
                </svg>
                {unreadCount > 0 && (
                    <span className={`notification-badge${unreadCount > 0 ? ' notification-badge--pulse' : ''}`}>
                        {unreadCount > 99 ? '99+' : unreadCount}
                    </span>
                )}
            </button>

            {open && (
                <div className="notification-panel">
                    <div className="notification-panel__header">
                        <h3>Notifications</h3>
                        <div className="notification-panel__actions">
                            {unreadCount > 0 && (
                                <button className="notification-panel__mark-all" onClick={markAllRead}>
                                    Mark all read
                                </button>
                            )}
                            <button
                                className="notification-panel__view-all"
                                onClick={() => { navigate('/notifications'); setOpen(false); }}
                            >
                                View all
                            </button>
                        </div>
                    </div>

                    <div className="notification-panel__list">
                        {notifications.length === 0 ? (
                            <div className="notification-panel__empty">
                                <span style={{ fontSize: 24, opacity: 0.5 }}>🔔</span>
                                <p>No notifications yet</p>
                            </div>
                        ) : (
                            notifications.slice(0, 20).map(notif => (
                                <button
                                    key={notif.id}
                                    className={`notification-item${notif.read ? '' : ' notification-item--unread'}`}
                                    onClick={() => handleNotificationClick(notif)}
                                >
                                    <span
                                        className="notification-item__icon"
                                        style={{ color: PRIORITY_COLORS[notif.priority] || '#999' }}
                                    >
                                        {TYPE_ICONS[notif.type] || '🔔'}
                                    </span>
                                    <div className="notification-item__content">
                                        <div className="notification-item__title">{notif.title}</div>
                                        <div className="notification-item__body">{notif.body}</div>
                                        <div className="notification-item__time">{timeAgo(notif.created_at)}</div>
                                    </div>
                                    {!notif.read && <span className="notification-item__dot" />}
                                </button>
                            ))
                        )}
                    </div>
                </div>
            )}
        </div>
    );
};

export default NotificationBell;
