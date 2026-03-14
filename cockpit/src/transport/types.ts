/**
 * Transport Client Interface
 * 
 * Unified abstraction over WebSocket and WebTransport connections.
 * Both transport implementations satisfy this interface so the UI
 * components can be transport-agnostic.
 */

export interface TransportClient {
    /** Connect to a session stream endpoint */
    connect(url: string, sessionId: string, authToken?: string): Promise<void>;

    /** Send terminal data (keyboard input) */
    send(data: string | ArrayBuffer): void;

    /** Send a JSON control message (ping, resize, etc.) */
    sendControl(msg: object): void;

    /** Close the connection */
    close(): void;

    /** Whether the transport is currently connected */
    readonly connected: boolean;

    /** Which transport type is in use */
    readonly transportType: 'websocket' | 'webtransport';

    // ---- Callbacks (set by consumer) ----

    /** Called when connection is established */
    onConnect?: () => void;

    /** Called when connection is lost */
    onDisconnect?: (reason: string) => void;

    /** Called when terminal output data arrives */
    onTerminalData?: (data: string | ArrayBuffer) => void;

    /** Called when a JSON control message arrives (pong, terminated, etc.) */
    onControlMessage?: (msg: any) => void;

    /** Called on transport error */
    onError?: (error: Error) => void;
}
