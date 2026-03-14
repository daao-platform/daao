import React from 'react';

/**
 * useApi - Hook for managing async API calls
 * @param fetcher - Function that returns a Promise<T>
 * @returns {data, loading, error, refetch}
 */
export function useApi<T>(fetcher: () => Promise<T>): {
    data: T | undefined;
    loading: boolean;
    error: Error | undefined;
    refetch: () => void;
} {
    const [data, setData] = React.useState<T | undefined>(undefined);
    const [loading, setLoading] = React.useState<boolean>(true);
    const [error, setError] = React.useState<Error | undefined>(undefined);
    const hasLoadedRef = React.useRef(false);

    // Use a ref to hold the latest fetcher so the callback identity stays stable
    const fetcherRef = React.useRef(fetcher);
    fetcherRef.current = fetcher;

    const fetchData = React.useCallback(() => {
        // Only show loading skeleton on the first fetch.
        // Subsequent refetches silently update data to avoid UI flicker.
        if (!hasLoadedRef.current) {
            setLoading(true);
        }
        setError(undefined);

        fetcherRef.current()
            .then((result) => {
                setData(result);
                setLoading(false);
                hasLoadedRef.current = true;
            })
            .catch((err) => {
                setError(err instanceof Error ? err : new Error(String(err)));
                setLoading(false);
                hasLoadedRef.current = true;
            });
    }, []);

    React.useEffect(() => {
        fetchData();
    }, [fetchData]);

    return { data, loading, error, refetch: fetchData };
}

/**
 * useWebSocket - Hook for managing WebSocket connections with auto-reconnect
 * @param url - WebSocket server URL
 * @param authToken - Optional auth token to send as first message after connection
 * @returns {messages, connected, lastMessage}
 */
export function useWebSocket(url: string, authToken?: string): {
    messages: unknown[];
    connected: boolean;
    lastMessage: unknown;
} {
    const MAX_WS_MESSAGES = 100;
    const [messages, setMessages] = React.useState<unknown[]>([]);
    const [connected, setConnected] = React.useState<boolean>(false);
    const [lastMessage, setLastMessage] = React.useState<unknown>(undefined);

    const wsRef = React.useRef<WebSocket | null>(null);
    const reconnectTimeoutRef = React.useRef<ReturnType<typeof setTimeout> | null>(null);
    const reconnectAttemptsRef = React.useRef<number>(0);
    const isMountedRef = React.useRef<boolean>(true);
    const maxReconnectAttempts = 10;
    const baseReconnectDelay = 1000;

    const appendMessage = React.useCallback((msg: unknown) => {
        setMessages((prev) => {
            const next = [...prev, msg];
            return next.length > MAX_WS_MESSAGES ? next.slice(-MAX_WS_MESSAGES) : next;
        });
    }, []);

    const connect = React.useCallback(() => {
        if (!isMountedRef.current) return;
        if (wsRef.current?.readyState === WebSocket.OPEN) {
            return;
        }

        const ws = new WebSocket(url);
        wsRef.current = ws;

        ws.onopen = () => {
            reconnectAttemptsRef.current = 0;
            if (authToken) {
                ws.send(JSON.stringify({ type: 'auth', token: authToken }));
                // Don't set connected until auth_ok received
            } else {
                if (isMountedRef.current) setConnected(true);
            }
        };

        ws.onmessage = (event) => {
            if (!isMountedRef.current) return;
            try {
                const parsed = JSON.parse(event.data);
                if (parsed.type === 'auth_ok') {
                    if (isMountedRef.current) setConnected(true);
                    return;
                }
                if (parsed.type === 'auth_error') {
                    console.error('[WS] Auth failed:', parsed.message);
                    ws.close();
                    return;
                }
                setLastMessage(parsed);
                appendMessage(parsed);
            } catch {
                setLastMessage(event.data);
                appendMessage(event.data);
            }
        };

        ws.onclose = () => {
            if (isMountedRef.current) setConnected(false);

            // Auto-reconnect with exponential backoff (only if still mounted)
            if (isMountedRef.current && reconnectAttemptsRef.current < maxReconnectAttempts) {
                const delay = baseReconnectDelay * Math.pow(2, reconnectAttemptsRef.current);
                reconnectAttemptsRef.current += 1;

                reconnectTimeoutRef.current = setTimeout(() => {
                    connect();
                }, delay);
            }
        };

        ws.onerror = () => {
            // Error will trigger onclose, so we handle reconnection there
        };
    }, [url, authToken]);

    React.useEffect(() => {
        isMountedRef.current = true;
        connect();

        return () => {
            isMountedRef.current = false;
            if (reconnectTimeoutRef.current) {
                clearTimeout(reconnectTimeoutRef.current);
            }
            if (wsRef.current) {
                wsRef.current.close();
                wsRef.current = null;
            }
        };
    }, [connect]);

    return { messages, connected, lastMessage };
}

/**
 * useLocalStorage - Hook for reading/writing to localStorage
 * @param key - localStorage key
 * @param defaultValue - Default value if key doesn't exist
 * @returns [value, setValue]
 */
export function useLocalStorage<T>(key: string, defaultValue: T): [T, (value: T | ((prev: T) => T)) => void] {
    const [value, setValue] = React.useState<T>(() => {
        try {
            const item = localStorage.getItem(key);
            if (item === null) {
                return defaultValue;
            }
            return JSON.parse(item) as T;
        } catch {
            return defaultValue;
        }
    });

    const setStoredValue = React.useCallback((newValue: T | ((prev: T) => T)) => {
        setValue((prev) => {
            const resolvedValue = typeof newValue === 'function'
                ? (newValue as (prev: T) => T)(prev)
                : newValue;

            try {
                localStorage.setItem(key, JSON.stringify(resolvedValue));
            } catch {
                // Ignore localStorage errors (e.g., quota exceeded)
            }

            return resolvedValue;
        });
    }, [key]);

    return [value, setStoredValue];
}
