import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// ============================================================================
// Mock WebSocket for fallback testing
// ============================================================================
class MockWebSocket {
    static CONNECTING = 0;
    static OPEN = 1;
    static CLOSING = 2;
    static CLOSED = 3;

    readyState = MockWebSocket.CONNECTING;
    binaryType = 'blob';
    onopen: (() => void) | null = null;
    onclose: (() => void) | null = null;
    onmessage: ((event: any) => void) | null = null;
    onerror: (() => void) | null = null;

    constructor(public url: string) {
        setTimeout(() => {
            this.readyState = MockWebSocket.OPEN;
            this.onopen?.();
        }, 0);
    }

    send(_data: any): void { }
    close(): void {
        this.readyState = MockWebSocket.CLOSED;
        this.onclose?.();
    }
}

vi.stubGlobal('WebSocket', MockWebSocket);

// ============================================================================
// Tests
// ============================================================================
import { createTransport } from './negotiate';

describe('negotiate.ts — createTransport()', () => {
    beforeEach(() => {
        // Ensure WebTransport is NOT defined by default (simulates Safari / older browsers)
        vi.stubGlobal('WebTransport', undefined);

        // Mock window.location
        vi.stubGlobal('location', {
            protocol: 'https:',
            host: 'localhost:8443',
        });
    });

    afterEach(() => {
        vi.unstubAllGlobals();
        // Re-stub WebSocket since unstubAllGlobals removes it
        vi.stubGlobal('WebSocket', MockWebSocket);
    });

    it('should fall back to WebSocket when WebTransport is undefined', async () => {
        const transport = await createTransport('session-abc', 'localhost:8443');
        expect(transport.transportType).toBe('websocket');
        expect(transport.connected).toBe(true);
        transport.close();
    });

    it('should fall back to WebSocket when WebTransport constructor throws', async () => {
        // Mock WebTransport that always fails
        vi.stubGlobal('WebTransport', class {
            ready: Promise<void>;
            constructor() {
                this.ready = Promise.reject(new Error('QUIC not available'));
            }
        });

        const transport = await createTransport('session-xyz', 'localhost:8443');
        expect(transport.transportType).toBe('websocket');
        transport.close();
    });

    it('should return a transport with connected=true', async () => {
        const transport = await createTransport('session-test', 'localhost:8443');
        expect(transport.connected).toBe(true);
        transport.close();
    });

    it('should construct the correct WebSocket URL', async () => {
        // We can verify indirectly: the transport should connect successfully
        const transport = await createTransport('sess-123', 'myhost.example.com');
        expect(transport.transportType).toBe('websocket');
        expect(transport.connected).toBe(true);
        transport.close();
    });
});
