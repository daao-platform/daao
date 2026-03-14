import { useState, useEffect, useRef, useCallback } from 'react';
import type { NotificationItem } from '../api/client';
import { getUnreadCount, markNotificationRead, markAllNotificationsRead, getNotifications } from '../api/client';

/**
 * useNotifications — SSE-based real-time notification hook.
 *
 * Opens an EventSource to /api/v1/notifications/stream for real-time push.
 * Auto-reconnects with exponential backoff. Maintains unread count and
 * recent notifications list. Triggers browser Notification for CRITICAL events.
 */
export function useNotifications() {
    const [notifications, setNotifications] = useState<NotificationItem[]>([]);
    const [unreadCount, setUnreadCount] = useState(0);
    const [connected, setConnected] = useState(false);
    const eventSourceRef = useRef<EventSource | null>(null);
    const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const reconnectAttemptsRef = useRef(0);
    const isMountedRef = useRef(true);
    const maxReconnectAttempts = 20;
    const baseDelay = 1000;

    // Fetch initial unread count and recent notifications
    const fetchInitial = useCallback(async () => {
        try {
            const [countResult, listResult] = await Promise.all([
                getUnreadCount(),
                getNotifications(20),
            ]);
            if (isMountedRef.current) {
                setUnreadCount(countResult.count);
                setNotifications(listResult.notifications || []);
            }
        } catch {
            // Silently fail — SSE will keep the state up to date
        }
    }, []);

    // Connect to SSE stream
    const connect = useCallback(() => {
        if (!isMountedRef.current) return;
        if (eventSourceRef.current?.readyState === EventSource.OPEN) return;

        const es = new EventSource('/api/v1/notifications/stream');
        eventSourceRef.current = es;

        es.addEventListener('connected', () => {
            if (isMountedRef.current) {
                setConnected(true);
                reconnectAttemptsRef.current = 0;
            }
        });

        es.addEventListener('notification', (event) => {
            if (!isMountedRef.current) return;
            try {
                const notif: NotificationItem = JSON.parse(event.data);
                setNotifications(prev => [notif, ...prev].slice(0, 50));
                setUnreadCount(prev => prev + 1);

                // Browser notification for CRITICAL priority
                if (notif.priority === 'CRITICAL' && Notification.permission === 'granted') {
                    new Notification(notif.title, {
                        body: notif.body,
                        icon: '/favicon.ico',
                        tag: `daao-${notif.id}`,
                    });
                }
            } catch {
                // Invalid JSON — skip
            }
        });

        es.onerror = () => {
            if (isMountedRef.current) {
                setConnected(false);
                es.close();
                eventSourceRef.current = null;

                // Reconnect with exponential backoff
                if (reconnectAttemptsRef.current < maxReconnectAttempts) {
                    const delay = baseDelay * Math.pow(2, Math.min(reconnectAttemptsRef.current, 6));
                    reconnectAttemptsRef.current++;
                    reconnectTimeoutRef.current = setTimeout(connect, delay);
                }
            }
        };
    }, []);

    // Mark a single notification as read
    const markRead = useCallback(async (id: string) => {
        try {
            await markNotificationRead(id);
            setNotifications(prev => prev.map(n => n.id === id ? { ...n, read: true } : n));
            setUnreadCount(prev => Math.max(0, prev - 1));
        } catch {
            // Silently fail
        }
    }, []);

    // Mark all notifications as read
    const markAllRead = useCallback(async () => {
        try {
            await markAllNotificationsRead();
            setNotifications(prev => prev.map(n => ({ ...n, read: true })));
            setUnreadCount(0);
        } catch {
            // Silently fail
        }
    }, []);

    useEffect(() => {
        isMountedRef.current = true;
        fetchInitial();
        connect();

        return () => {
            isMountedRef.current = false;
            if (reconnectTimeoutRef.current) clearTimeout(reconnectTimeoutRef.current);
            if (eventSourceRef.current) {
                eventSourceRef.current.close();
                eventSourceRef.current = null;
            }
        };
    }, [connect, fetchInitial]);

    return {
        notifications,
        unreadCount,
        connected,
        markRead,
        markAllRead,
    };
}
