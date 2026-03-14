/**
 * Transport Negotiation
 *
 * Tries WebTransport first (HTTP/3 QUIC on :8446), then falls back
 * to WebSocket if the browser doesn't support WebTransport or the
 * QUIC connection fails.
 */

import type { TransportClient } from './types';
import { WebTransportTransport } from './WebTransportTransport';
import { WebSocketTransport } from './WebSocketTransport';

/**
 * Fetch the server certificate's SHA-256 hash from Nexus.
 * Chrome requires this for WebTransport with self-signed certs.
 * Returns the hash as a Uint8Array, or null on failure.
 */
async function fetchCertHash(host: string): Promise<Uint8Array | null> {
    try {
        const protocol = window.location.protocol;
        const res = await fetch(`${protocol}//${host}/api/v1/transport/cert-hash`);
        if (!res.ok) return null;
        const data = await res.json();
        if (!data.hash) return null;
        // Decode base64 → Uint8Array
        const binary = atob(data.hash);
        const bytes = new Uint8Array(binary.length);
        for (let i = 0; i < binary.length; i++) {
            bytes[i] = binary.charCodeAt(i);
        }
        return bytes;
    } catch {
        return null;
    }
}

/**
 * Create a connected transport for a session.
 *
 * @param sessionId  Session ID to connect to
 * @param host       Hostname (without protocol), e.g. window.location.host
 * @param authToken  Optional auth token for authenticated connections
 * @returns Connected TransportClient (either WebTransport or WebSocket)
 */
export async function createTransport(
    sessionId: string,
    host: string,
    authToken?: string,
): Promise<TransportClient> {
    // Strip port from host to get the bare hostname for WebTransport
    const hostname = host.split(':')[0];

    // Try WebTransport if the browser supports it
    if (typeof WebTransport !== 'undefined') {
        try {
            // Fetch cert hash for self-signed cert support
            const certHash = await fetchCertHash(host);
            const wt = new WebTransportTransport();
            const wtUrl = `https://${hostname}:8446/webtransport?session_id=${sessionId}`;
            // Pass cert hash so Chrome accepts self-signed certs
            if (certHash) {
                (wt as any)._certHash = certHash;
            }
            await wt.connect(wtUrl, sessionId, authToken);
            console.log('[Transport] Connected via WebTransport (QUIC/H3)');
            return wt;
        } catch (err) {
            console.warn('[Transport] WebTransport failed, falling back to WebSocket:', err);
        }
    }

    // Fallback to WebSocket
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocketTransport();
    const wsUrl = `${protocol}//${host}/api/v1/sessions/${sessionId}/stream`;
    await ws.connect(wsUrl, sessionId, authToken);
    console.log('[Transport] Connected via WebSocket');
    return ws;
}
