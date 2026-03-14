/**
 * Xterm.js Terminal Integration for Cockpit
 * 
 * Handles terminal rendering, ring buffer hydration on connect,
 * real-time output streaming, and keyboard input forwarding.
 */

import { Terminal } from '@xterm/xterm';

// Import xterm CSS
import '@xterm/xterm/css/xterm.css';

export interface TerminalConfig {
  /** Initial columns */
  cols?: number;
  /** Initial rows */
  rows?: number;
  /** Scrollback buffer size (default: 10000 lines) */
  scrollback?: number;
  /** Cursor style */
  cursorStyle?: 'block' | 'underline' | 'bar';
  /** Cursor blink */
  cursorBlink?: boolean;
  /** Font family */
  fontFamily?: string;
  /** Font size */
  fontSize?: number;
}

export interface TerminalCallbacks {
  /** Called when terminal is ready */
  onReady?: (terminal: Terminal) => void;
  /** Called on data input from terminal */
  onData?: (data: string) => void;
  /** Called on key press */
  onKey?: (key: string, event: KeyboardEvent) => void;
  /** Called on resize */
  onResize?: (cols: number, rows: number) => void;
  /** Called when connected to server */
  onConnect?: () => void;
  /** Called when disconnected */
  onDisconnect?: (reason: string) => void;
  /** Called on error */
  onError?: (error: Error) => void;
}

/**
 * Terminal connection state
 */
export interface TerminalState {
  /** Session ID */
  sessionId: string;
  /** WebSocket/WebTransport connection */
  connected: boolean;
  /** Current dimensions */
  cols: number;
  rows: number;
}

/**
 * Ring buffer snapshot for hydration
 */
export interface RingBufferSnapshot {
  /** Raw terminal data as string */
  data: string;
  /** Cursor position */
  cursorCol: number;
  cursorRow: number;
  /** Scroll position */
  scrollOffset: number;
  /** Timestamp of snapshot */
  timestamp: number;
}

/**
 * Xterm.js Terminal Manager
 * 
 * Manages terminal instance, WebTransport connection, and data flow.
 * Handles ring buffer hydration on connect and scrollback preservation.
 */
export class XtermTerminal {
  private terminal: Terminal | null = null;
  private config: TerminalConfig;
  private callbacks: TerminalCallbacks;
  private scrollbackBuffer: string[] = [];
  private maxScrollback: number;

  constructor(config: TerminalConfig = {}, callbacks: TerminalCallbacks = {}) {
    this.config = {
      cols: config.cols ?? 80,
      rows: config.rows ?? 24,
      scrollback: config.scrollback ?? 10000,
      cursorStyle: config.cursorStyle ?? 'block',
      cursorBlink: config.cursorBlink ?? true,
      fontFamily: config.fontFamily ?? 'Menlo, Monaco, "Courier New", monospace',
      fontSize: config.fontSize ?? 14,
      ...config,
    };
    this.callbacks = callbacks;
    this.maxScrollback = this.config.scrollback ?? 10000;
  }

  /**
   * Attach terminal to a DOM element
   */
  attach(container: HTMLElement): void {
    // Create Xterm.js terminal instance
    this.terminal = new Terminal({
      cols: this.config.cols,
      rows: this.config.rows,
      cursorStyle: this.config.cursorStyle,
      cursorBlink: this.config.cursorBlink,
      fontFamily: this.config.fontFamily,
      fontSize: this.config.fontSize,
      scrollback: this.maxScrollback,
      convertEol: false, // ConPTY already sends \r\n — enabling this caused double \r\r\n
      allowTransparency: false,
      drawBoldTextInBrightColors: true,
      screenReaderMode: false,
      overviewRulerWidth: 0,
      theme: {
        background: '#1e1e1e',
        foreground: '#d4d4d4',
        cursor: '#ffffff',
        cursorAccent: '#1e1e1e',
        selectionBackground: '#264f78',
        selectionForeground: '#d4d4d4',
        black: '#000000',
        red: '#cd3131',
        green: '#0dbc79',
        yellow: '#e5e510',
        blue: '#2472c8',
        magenta: '#bc3fbc',
        cyan: '#11a8cd',
        white: '#e5e5e5',
        brightBlack: '#666666',
        brightRed: '#f14c4c',
        brightGreen: '#23d18b',
        brightYellow: '#f5f543',
        brightBlue: '#3b8eea',
        brightMagenta: '#d670d6',
        brightCyan: '#29b8db',
        brightWhite: '#e5e5e5',
      },
    });

    // Open terminal in container
    this.terminal.open(container);

    // Set up input handlers
    this.setupInputHandlers();

    // Set up resize handler
    this.setupResizeHandler();

    // Notify ready
    this.callbacks.onReady?.(this.terminal);
  }

  /**
   * Set up keyboard input handlers
   */
  private setupInputHandlers(): void {
    if (!this.terminal) return;

    // Handle data input (main input handler)
    this.terminal.onData((data: string) => {
      // Forward keyboard input
      this.callbacks.onData?.(data);
    });

    // Handle key events
    this.terminal.onKey(({ key, domEvent }) => {
      this.callbacks.onKey?.(key, domEvent);
    });
  }

  /**
   * Set up resize handler
   */
  private setupResizeHandler(): void {
    if (!this.terminal) return;

    this.terminal.onResize(({ cols, rows }) => {
      this.callbacks.onResize?.(cols, rows);
    });
  }


  /**
   * Handle incoming terminal data from server
   */
  private handleTerminalData(data: Uint8Array): void {
    if (!this.terminal) return;

    // Decode data to string
    const text = new TextDecoder().decode(data);

    // Write to terminal
    this.terminal.write(text);

    // Store in scrollback buffer for reconnection
    this.appendToScrollback(text);
  }

  /**
   * Hydrate terminal with ring buffer snapshot
   * 
   * Called when reconnecting to restore terminal state
   */
  hydrate(snapshot: RingBufferSnapshot): void {
    if (!this.terminal) return;

    // Clear terminal
    this.terminal.clear();

    // Write historical buffer data
    this.terminal.write(snapshot.data);

    // Restore scroll position
    if (snapshot.scrollOffset > 0) {
      // Scroll back to show historical content
      this.terminal.scrollToBottom();
      this.terminal.scrollLines(-snapshot.scrollOffset);
    }

    // Restore cursor position if available
    if (snapshot.cursorCol >= 0 && snapshot.cursorRow >= 0) {
      this.terminal.resize(
        Math.max(this.terminal.cols, snapshot.cursorCol + 1),
        Math.max(this.terminal.rows, snapshot.cursorRow + 1)
      );
    }
  }

  /**
   * Append data to scrollback buffer
   */
  private appendToScrollback(data: string): void {
    this.scrollbackBuffer.push(data);

    // Trim to max scrollback size
    while (this.scrollbackBuffer.length > this.maxScrollback) {
      this.scrollbackBuffer.shift();
    }
  }

  /**
   * Get current scrollback buffer as string
   */
  getScrollback(): string {
    return this.scrollbackBuffer.join('');
  }

  /**
   * Get snapshot of current terminal state for reconnection
   */
  getSnapshot(): RingBufferSnapshot {
    if (!this.terminal) {
      return {
        data: '',
        cursorCol: 0,
        cursorRow: 0,
        scrollOffset: 0,
        timestamp: Date.now(),
      };
    }

    // Get buffer lines from xterm
    const buffer = this.terminal.buffer.active;
    const lines: string[] = [];

    // Get visible buffer content
    for (let i = 0; i < buffer.length; i++) {
      const line = buffer.getLine(i);
      if (line) {
        lines.push(line.translateToString());
      }
    }

    // Get cursor position
    const cursorCol = buffer.cursorX;
    const cursorRow = buffer.cursorY;

    // Calculate scroll offset (how far back from bottom)
    const scrollOffset = Math.max(0, buffer.length - buffer.viewportY - 1);

    return {
      data: lines.join('\r\n'),
      cursorCol,
      cursorRow,
      scrollOffset,
      timestamp: Date.now(),
    };
  }

  /**
   * Send resize request to server
   */
  resize(cols: number, rows: number): void {
    if (!this.terminal) return;

    this.terminal.resize(cols, rows);
    this.callbacks.onResize?.(cols, rows);
  }

  /**
   * Send keyboard input
   */
  sendInput(data: string): void {
    this.callbacks.onData?.(data);
  }

  /**
   * Get the underlying Xterm.js instance
   */
  getTerminal(): Terminal | null {
    return this.terminal;
  }

  /**
   * Disconnect and clean up
   */
  async disconnect(): Promise<void> {
    // No-op — transport is managed externally
  }

  /**
   * Destroy terminal instance
   */
  destroy(): void {
    this.disconnect();

    if (this.terminal) {
      this.terminal.dispose();
      this.terminal = null;
    }

    this.scrollbackBuffer = [];
  }

  /**
   * Focus the terminal
   */
  focus(): void {
    this.terminal?.focus();
  }

  /**
   * Clear the terminal
   */
  clear(): void {
    this.terminal?.clear();
    this.scrollbackBuffer = [];
  }
}

/**
 * Create a new Xterm.js terminal instance
 */
export function createXtermTerminal(
  config?: TerminalConfig,
  callbacks?: TerminalCallbacks
): XtermTerminal {
  return new XtermTerminal(config, callbacks);
}

/**
 * Default terminal configuration
 */
export const defaultTerminalConfig: TerminalConfig = {
  cols: 80,
  rows: 24,
  scrollback: 10000,
  cursorStyle: 'block',
  cursorBlink: true,
  fontFamily: 'Menlo, Monaco, "Courier New", monospace',
  fontSize: 14,
};

export default XtermTerminal;
