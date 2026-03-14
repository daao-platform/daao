/**
 * WebSocket Transport
 *
 * Implements TransportClient over a standard WebSocket connection.
 * Extracted from TerminalView.tsx inline WebSocket logic.
 */

import type { TransportClient } from './types';

export class WebSocketTransport implements TransportClient {
    private ws: WebSocket | null = null;
    private _connected = false;
    private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    private reconnectAttempts = 0;
    private maxReconnectAttempts = 10;
    private _url = '';
    private _sessionId = '';
    private _authToken: string | undefined;
    private disposed = false;

    readonly transportType = 'websocket' as const;

    // ---- Callbacks ----
    onConnect?: () => void;
    onDisconnect?: (reason: string) => void;
    onTerminalData?: (data: string | ArrayBuffer) => void;
    onControlMessage?: (msg: any) => void;
    onError?: (error: Error) => void;

    get connected(): boolean {
        return this._connected;
    }

    /**
     * Connect to a WebSocket session stream.
     * Resolves once the connection is open; rejects on first-connect failure.
     * If authToken is provided, waits for auth_ok before resolving.
     */
    async connect(url: string, sessionId: string, authToken?: string): Promise<void> {
        this._url = url;
        this._sessionId = sessionId;
        this._authToken = authToken;
        this.disposed = false;
        this.reconnectAttempts = 0;

        return new Promise<void>((resolve, reject) => {
            this.openSocket(resolve, reject);
        });
    }

    private openSocket(
        firstResolve?: (value: void) => void,
        firstReject?: (reason: Error) => void,
    ): void {
        if (this.disposed) return;

        const ws = new WebSocket(this._url);
        this.ws = ws;

        ws.binaryType = 'arraybuffer';

        ws.onopen = () => {
            this._connected = true;
            this.reconnectAttempts = 0;

            if (this._authToken) {
                // Send auth as first message
                ws.send(JSON.stringify({ type: 'auth', token: this._authToken }));
                // Wait for auth_ok or auth_error in onmessage
            } else {
                // No auth token (dev mode) — resolve immediately
                this.onConnect?.();
                firstResolve?.();
                firstResolve = undefined;
                firstReject = undefined;
            }
        };

        ws.onmessage = (event: MessageEvent) => {
            // Handle auth responses
            if (typeof event.data === 'string' && event.data.startsWith('{')) {
                try {
                    const msg = JSON.parse(event.data);
                    if (msg.type === 'auth_ok') {
                        this.onConnect?.();
                        firstResolve?.();
                        firstResolve = undefined;
                        firstReject = undefined;
                        return;
                    }
                    if (msg.type === 'auth_error') {
                        const err = new Error(`Auth failed: ${msg.message}`);
                        this.onError?.(err);
                        firstReject?.(err);
                        firstResolve = undefined;
                        firstReject = undefined;
                        this.disposed = true; // don't auto-reconnect on auth failure
                        ws.close();
                        return;
                    }
                    // Other control messages
                    if (msg.type) {
                        this.onControlMessage?.(msg);
                        return;
                    }
                } catch {
                    // Not JSON — treat as terminal data
                }
            }

            // Terminal data (string or binary)
            this.onTerminalData?.(event.data);
        };

        ws.onclose = () => {
            this._connected = false;

            if (this.disposed) {
                this.onDisconnect?.('closed');
                return;
            }

            // Exponential backoff reconnection
            if (this.reconnectAttempts >= this.maxReconnectAttempts) {
                this.onDisconnect?.('max reconnect attempts reached');
                return;
            }

            this.reconnectAttempts++;
            const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts - 1), 16000);

            this.onDisconnect?.('reconnecting');

            if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
            this.reconnectTimer = setTimeout(() => {
                this.openSocket();
            }, delay);
        };

        ws.onerror = () => {
            const err = new Error(`WebSocket error connecting to ${this._url}`);
            this.onError?.(err);
            if (firstReject) {
                firstReject(err);
                firstResolve = undefined;
                firstReject = undefined;
            }
        };
    }

    /** Send terminal input data */
    send(data: string | ArrayBuffer): void {
        if (this.ws?.readyState === WebSocket.OPEN) {
            this.ws.send(data);
        }
    }

    /** Send a JSON control message (ping, resize, etc.) */
    sendControl(msg: object): void {
        if (this.ws?.readyState === WebSocket.OPEN) {
            this.ws.send(JSON.stringify(msg));
        }
    }

    /** Close the connection and stop reconnection */
    close(): void {
        this.disposed = true;
        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
            this.reconnectTimer = null;
        }
        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }
        this._connected = false;
    }
}
