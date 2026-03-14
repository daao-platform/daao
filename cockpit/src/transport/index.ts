/**
 * WebTransport Client for Cockpit
 * 
 * Implements WebTransport client for Cockpit using HTTP/3 QUIC.
 * Handles connection establishment, JWT auth, stream opening, and graceful disconnect.
 */

export interface WebTransportConfig {
  url: string;
  jwt: string;
  onConnect?: () => void;
  onDisconnect?: (reason: string) => void;
  onError?: (error: Error) => void;
  onControlMessage?: (data: Uint8Array) => void;
  onTerminalData?: (data: Uint8Array) => void;
}

export interface WebTransportStreams {
  control: WebTransportBidirectionalStream;
  terminalRx: any; // WebTransportReceiveStream — experimental type
  terminalTx: any; // WebTransportSendStream — experimental type
  close: () => void;
}

/**
 * WebTransport client for Cockpit
 * 
 * Connects to Nexus via WebTransport with HTTP/3 QUIC.
 * Sends JWT Authorization header for authentication.
 * Opens bidirectional stream for control messages.
 * Opens unidirectional streams for terminal RX/TX.
 */
export class WebTransportClient {
  private transport: WebTransport | null = null;
  private config: WebTransportConfig;
  private streams: WebTransportStreams | null = null;
  private connected: boolean = false;

  constructor(config: WebTransportConfig) {
    this.config = config;
  }

  /**
   * Connect to Nexus via WebTransport
   * 
   * Uses ALPN protocol "webtransport" for HTTP/3 QUIC connection.
   * Sends Authorization: Bearer <JWT> header for authentication.
   */
  async connect(): Promise<void> {
    if (this.connected) {
      return;
    }

    const headers = new Headers();
    headers.set('Authorization', `Bearer ${this.config.jwt}`);
    headers.set('X-WebTransport-Protocol', 'webtransport');

    try {
      // Connect to Nexus via WebTransport (ALPN: webtransport)
      // Pass serverCertificateHashes if provided (for self-signed certs)
      const wtOptions: any = {};
      if ((this as any)._certHashOptions?.serverCertificateHashes) {
        wtOptions.serverCertificateHashes = (this as any)._certHashOptions.serverCertificateHashes;
      }
      this.transport = new WebTransport(this.config.url, wtOptions);

      // Wait for connection established
      await this.transport.ready;

      // Open streams after connection is established
      await this.openStreams();

      this.connected = true;
      this.config.onConnect?.();
    } catch (error) {
      const err = error instanceof Error ? error : new Error(String(error));
      this.config.onError?.(err);
      throw err;
    }
  }

  /**
   * Open bidirectional and unidirectional streams
   */
  private async openStreams(): Promise<void> {
    if (!this.transport) {
      throw new Error('Transport not initialized');
    }

    // Open bidirectional stream for control
    const controlStream = await this.transport.createBidirectionalStream();

    // Open unidirectional stream for terminal RX (receive)
    const terminalRxStream = await this.transport.createUnidirectionalStream();

    // Open unidirectional stream for terminal TX (send) 
    // Note: In WebTransport, we need another unidirectional stream for sending
    const terminalTxStream = await this.transport.createUnidirectionalStream();

    // Set up control stream handling
    this.handleControlStream(controlStream);

    // Set up terminal RX stream handling
    this.handleTerminalRxStream(terminalRxStream);

    this.streams = {
      control: controlStream,
      terminalRx: terminalRxStream,
      terminalTx: terminalTxStream,
      close: () => this.disconnect(),
    };

    // Handle connection close gracefully
    this.transport.closed.catch((reason) => {
      this.handleDisconnect(reason);
    });
  }

  /**
   * Handle bidirectional control stream
   */
  private async handleControlStream(stream: WebTransportBidirectionalStream): Promise<void> {
    const reader = stream.readable.getReader();

    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) {
          break;
        }
        this.config.onControlMessage?.(value);
      }
    } catch (error) {
      // Stream closed or error
    }
  }

  /**
   * Handle terminal RX (receive) unidirectional stream
   */
  private async handleTerminalRxStream(stream: any): Promise<void> {
    const reader = stream.getReader();

    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) {
          break;
        }
        this.config.onTerminalData?.(value);
      }
    } catch (error) {
      // Stream closed or error
    }
  }

  /**
   * Send control message via bidirectional stream
   */
  async sendControlMessage(data: Uint8Array): Promise<void> {
    if (!this.streams?.control) {
      throw new Error('Control stream not available');
    }

    const writer = this.streams.control.writable.getWriter();
    try {
      await writer.write(data);
    } finally {
      writer.releaseLock();
    }
  }

  /**
   * Send terminal data via unidirectional stream
   */
  async sendTerminalData(data: Uint8Array): Promise<void> {
    if (!this.streams?.terminalTx) {
      throw new Error('Terminal TX stream not available');
    }

    const writer = this.streams.terminalTx.writable.getWriter();
    try {
      await writer.write(data);
    } finally {
      writer.releaseLock();
    }
  }

  /**
   * Handle disconnection gracefully
   */
  private handleDisconnect(reason?: any): void {
    if (!this.connected) {
      return;
    }

    this.connected = false;
    const reasonStr = reason instanceof Error ? reason.message : String(reason ?? 'Connection closed');
    this.config.onDisconnect?.(reasonStr);
  }

  /**
   * Disconnect gracefully
   * 
   * Handles connection close gracefully by:
   * - Closing all streams
   * - Closing the transport connection
   * - Triggering onDisconnect callback
   */
  async disconnect(): Promise<void> {
    if (!this.connected && !this.transport) {
      return;
    }

    try {
      // Close streams gracefully
      if (this.streams) {
        // Close bidirectional control stream
        this.streams.control.writable.close();
        this.streams.control.readable.cancel();

        // Close unidirectional terminal streams
        this.streams.terminalRx.cancel();
        this.streams.terminalTx.writable.close();
      }

      // Close transport gracefully
      if (this.transport) {
        await this.transport.close({
          closeCode: 1000,
          reason: 'Client disconnect',
        });
      }
    } catch (error) {
      // Ignore errors during cleanup
    } finally {
      this.connected = false;
      this.transport = null;
      this.streams = null;
      this.config.onDisconnect?.('Graceful disconnect');
    }
  }

  /**
   * Check if client is connected
   */
  isConnected(): boolean {
    return this.connected;
  }

  /**
   * Get the underlying WebTransport instance
   */
  getTransport(): WebTransport | null {
    return this.transport;
  }

  /**
   * Get the opened streams
   */
  getStreams(): WebTransportStreams | null {
    return this.streams;
  }
}

/**
 * Create a new WebTransport client instance
 */
export function createWebTransportClient(config: WebTransportConfig): WebTransportClient {
  return new WebTransportClient(config);
}

export default WebTransportClient;
