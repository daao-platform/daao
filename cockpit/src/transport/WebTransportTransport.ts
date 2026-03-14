/**
 * WebTransport Transport
 *
 * Implements TransportClient over WebTransport (HTTP/3 QUIC).
 * Thin adapter wrapping the existing WebTransportClient from ./index.ts.
 */

import type { TransportClient } from './types';
import { WebTransportClient } from './index';

export class WebTransportTransport implements TransportClient {
    private client: WebTransportClient | null = null;
    private _connected = false;

    readonly transportType = 'webtransport' as const;

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
     * Connect via WebTransport.
     * The url should be the full WebTransport endpoint, e.g.
     * https://host:8446/webtransport?session_id=<id>
     * @param authToken Optional auth token (currently not implemented - future use)
     */
    async connect(url: string, _sessionId: string, _authToken?: string): Promise<void> {
        // Build WebTransport options — pass cert hash for self-signed cert support
        const options: any = {};
        if ((this as any)._certHash) {
            options.serverCertificateHashes = [{
                algorithm: 'sha-256',
                value: (this as any)._certHash.buffer,
            }];
        }

        this.client = new WebTransportClient({
            url,
            jwt: '', // JWT passed via URL query or server-events in current spec
            onConnect: () => {
                this._connected = true;
                this.onConnect?.();
            },
            onDisconnect: (reason: string) => {
                this._connected = false;
                this.onDisconnect?.(reason);
            },
            onError: (error: Error) => {
                this.onError?.(error);
            },
            onTerminalData: (data: Uint8Array) => {
                // Forward as ArrayBuffer for consistency
                this.onTerminalData?.(data.buffer as ArrayBuffer);
            },
            onControlMessage: (data: Uint8Array) => {
                try {
                    const text = new TextDecoder().decode(data);
                    const msg = JSON.parse(text);
                    this.onControlMessage?.(msg);
                } catch {
                    // Not JSON control data — treat as terminal data
                    this.onTerminalData?.(data.buffer as ArrayBuffer);
                }
            },
        });

        // Pass cert hash options to the underlying WebTransport constructor
        if (options.serverCertificateHashes) {
            (this.client as any)._certHashOptions = options;
        }

        await this.client.connect();
    }

    /** Send terminal input data */
    send(data: string | ArrayBuffer): void {
        if (!this.client?.isConnected()) return;

        const encoded = typeof data === 'string'
            ? new TextEncoder().encode(data)
            : new Uint8Array(data);

        this.client.sendTerminalData(encoded);
    }

    /** Send a JSON control message */
    sendControl(msg: object): void {
        if (!this.client?.isConnected()) return;

        const encoded = new TextEncoder().encode(JSON.stringify(msg));
        this.client.sendControlMessage(encoded);
    }

    /** Close the WebTransport connection */
    close(): void {
        this._connected = false;
        if (this.client) {
            this.client.disconnect();
            this.client = null;
        }
    }
}
