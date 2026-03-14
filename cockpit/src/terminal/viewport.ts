/**
 * Mobile Viewport Manager
 * 
 * Handles dynamic viewport management for mobile devices.
 * Uses ResizeObserver to detect container size changes,
 * calculates cols/rows, and sends RESIZE events to server.
 * Also handles touch input for tap-to-type and virtual keyboard detection.
 */

import { Terminal } from '@xterm/xterm';

/**
 * Viewport configuration options
 */
export interface ViewportConfig {
  /** Terminal instance */
  terminal: Terminal;
  /** Font size in pixels (default: 14) */
  fontSize?: number;
  /** Font family for measurement (default: monospace) */
  fontFamily?: string;
  /** Minimum columns (default: 40) */
  minCols?: number;
  /** Minimum rows (default: 10) */
  minRows?: number;
  /** Callback when viewport changes */
  onResize?: (cols: number, rows: number) => void;
}

/**
 * Viewport dimensions
 */
export interface ViewportDimensions {
  cols: number;
  rows: number;
  width: number;
  height: number;
}

/**
 * Mobile Viewport Manager
 * 
 * Monitors container size using ResizeObserver and calculates
 * terminal dimensions (cols/rows) based on font metrics.
 */
export class MobileViewportManager {
  private terminal: Terminal;
  private container: HTMLElement | null = null;
  private resizeObserver: ResizeObserver | null = null;
  private config: Required<ViewportConfig>;
  private lastDimensions: ViewportDimensions | null = null;
  private virtualKeyboardVisible: boolean = false;
  private fontMeasureElement: HTMLElement | null = null;
  private touchInputHandler: ((e: TouchEvent) => void) | null = null;
  private boundHandleWindowResize: (() => void) | null = null;
  private boundHandleFocusIn: ((e: Event) => void) | null = null;
  private boundHandleFocusOut: ((e: Event) => void) | null = null;
  private boundHandleVisualViewportResize: (() => void) | null = null;

  constructor(config: ViewportConfig) {
    this.terminal = config.terminal;
    this.config = {
      fontSize: config.fontSize ?? 14,
      fontFamily: config.fontFamily ?? 'Menlo, Monaco, "Courier New", monospace',
      minCols: config.minCols ?? 40,
      minRows: config.minRows ?? 10,
      onResize: config.onResize ?? (() => {}),
      terminal: config.terminal,
    };
  }

  /**
   * Attach viewport manager to a container element
   */
  attach(container: HTMLElement): void {
    this.container = container;
    
    // Create hidden element for font measurement
    this.createMeasureElement();
    
    // Set up ResizeObserver for container monitoring
    this.setupResizeObserver();
    
    // Set up touch input handling for tap-to-type
    this.setupTouchInput();
    
    // Set up virtual keyboard detection
    this.setupVirtualKeyboardDetection();
    
    // Calculate initial dimensions
    this.calculateAndResize();
  }

  /**
   * Create hidden element for measuring font dimensions
   */
  private createMeasureElement(): void {
    if (!this.container) return;

    // Remove existing measure element if present
    if (this.fontMeasureElement) {
      this.fontMeasureElement.remove();
    }

    this.fontMeasureElement = document.createElement('div');
    this.fontMeasureElement.style.position = 'absolute';
    this.fontMeasureElement.style.visibility = 'hidden';
    this.fontMeasureElement.style.whiteSpace = 'pre';
    this.fontMeasureElement.style.fontFamily = this.config.fontFamily;
    this.fontMeasureElement.style.fontSize = `${this.config.fontSize}px`;
    this.fontMeasureElement.style.lineHeight = '1';
    this.fontMeasureElement.textContent = 'M'.repeat(80); // Measure 80 chars for accuracy
    
    this.container.appendChild(this.fontMeasureElement);
  }

  /**
   * Set up ResizeObserver to monitor container size changes
   */
  private setupResizeObserver(): void {
    if (!this.container) return;

    this.resizeObserver = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const { width, height } = entry.contentRect;
        this.handleResize(width, height);
      }
    });

    this.resizeObserver.observe(this.container);
  }

  /**
   * Handle resize events from ResizeObserver
   */
  private handleResize(width: number, height: number): void {
    // Adjust for virtual keyboard if visible
    const adjustedHeight = this.virtualKeyboardVisible 
      ? this.getAvailableHeight() 
      : height;

    const dimensions = this.calculateDimensions(width, adjustedHeight);
    
    // Only trigger resize if dimensions actually changed
    if (this.lastDimensions && 
        this.lastDimensions.cols === dimensions.cols && 
        this.lastDimensions.rows === dimensions.rows) {
      return;
    }

    this.lastDimensions = dimensions;
    this.resize(dimensions.cols, dimensions.rows);
  }

  /**
   * Get available height accounting for virtual keyboard
   */
  private getAvailableHeight(): number {
    if (!this.container) return 0;
    
    const rect = this.container.getBoundingClientRect();
    const viewportHeight = window.visualViewport?.height ?? window.innerHeight;
    
    // Calculate the difference (virtual keyboard height)
    const keyboardHeight = viewportHeight - rect.bottom;
    
    return Math.max(0, rect.height - keyboardHeight);
  }

  /**
   * Calculate terminal columns and rows from container dimensions
   */
  private calculateDimensions(width: number, height: number): ViewportDimensions {
    if (!this.fontMeasureElement) {
      // Fallback to default dimensions if measure element not available
      return {
        cols: this.config.minCols,
        rows: this.config.minRows,
        width,
        height,
      };
    }

    // Get font character dimensions
    const charWidth = this.fontMeasureElement.getBoundingClientRect().width / 80;
    const charHeight = this.config.fontSize; // Approximate based on font size

    // Calculate cols and rows (subtract padding/margins)
    const padding = 16; // Account for terminal padding
    const cols = Math.max(
      this.config.minCols,
      Math.floor((width - padding) / charWidth)
    );
    const rows = Math.max(
      this.config.minRows,
      Math.floor((height - padding) / charHeight)
    );

    return {
      cols,
      rows,
      width,
      height,
    };
  }

  /**
   * Calculate and apply initial resize
   */
  private calculateAndResize(): void {
    if (!this.container) return;

    const rect = this.container.getBoundingClientRect();
    const dimensions = this.calculateDimensions(rect.width, rect.height);
    
    this.lastDimensions = dimensions;
    this.resize(dimensions.cols, dimensions.rows);
  }

  /**
   * Resize the terminal to the specified dimensions
   */
  private resize(cols: number, rows: number): void {
    // Resize the xterm terminal
    this.terminal.resize(cols, rows);
    
    // Call the resize callback with RESIZE event
    this.config.onResize(cols, rows);
  }

  /**
   * Set up touch input handling for tap-to-type functionality
   */
  private setupTouchInput(): void {
    if (!this.container) return;

    this.touchInputHandler = (e: TouchEvent) => {
      // Only handle single taps (not scrolls or multi-touch)
      if (e.touches.length !== 1) return;
      
      const touch = e.touches[0];
      const target = e.target as HTMLElement;
      
      // Check if tap is within terminal container
      if (!this.container?.contains(target)) return;

      // Prevent default to avoid zooming/scrolling
      e.preventDefault();
      
      // Focus the terminal for input
      this.terminal.focus();
    };

    this.container.addEventListener('touchstart', this.touchInputHandler, { passive: false });
    
    // Also handle touchend to ensure focus after tap
    this.container.addEventListener('touchend', () => {
      this.terminal.focus();
    }, { passive: true });
  }

  /**
   * Set up virtual keyboard detection and handling
   */
  private setupVirtualKeyboardDetection(): void {
    // Use Visual Viewport API for modern virtual keyboard detection
    if (window.visualViewport) {
      this.boundHandleVisualViewportResize = this.handleVisualViewportResize.bind(this);
      window.visualViewport.addEventListener('resize', this.boundHandleVisualViewportResize);
      window.visualViewport.addEventListener('scroll', this.boundHandleVisualViewportResize);
    }

    // Fallback: Use window resize event with heuristics
    this.boundHandleWindowResize = this.handleWindowResize.bind(this);
    window.addEventListener('resize', this.boundHandleWindowResize);

    // iOS specific: Detect keyboard show/hide via focused input
    if ('ontouchstart' in window) {
      this.boundHandleFocusIn = this.handleFocusIn.bind(this);
      this.boundHandleFocusOut = this.handleFocusOut.bind(this);
      document.addEventListener('focusin', this.boundHandleFocusIn);
      document.addEventListener('focusout', this.boundHandleFocusOut);
    }
  }

  /**
   * Handle Visual Viewport resize events
   */
  private handleVisualViewportResize(): void {
    if (!window.visualViewport || !this.container) return;

    const viewportHeight = window.visualViewport.height;
    const containerHeight = this.container.getBoundingClientRect().height;
    
    // If viewport is significantly smaller than container, keyboard is likely visible
    const keyboardThreshold = 0.8; // 80% of container height
    const newKeyboardVisible = viewportHeight < containerHeight * keyboardThreshold;
    
    if (newKeyboardVisible !== this.virtualKeyboardVisible) {
      this.virtualKeyboardVisible = newKeyboardVisible;
      this.handleVirtualKeyboardChange(newKeyboardVisible);
    }
  }

  /**
   * Handle window resize events (fallback)
   */
  private handleWindowResize(): void {
    if (!this.container) return;

    const rect = this.container.getBoundingClientRect();
    this.handleResize(rect.width, rect.height);
  }

  /**
   * Handle focus in events (iOS keyboard detection)
   */
  private handleFocusIn(e: Event): void {
    const target = e.target as HTMLElement;
    if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable) {
      this.virtualKeyboardVisible = true;
      this.handleVirtualKeyboardChange(true);
    }
  }

  /**
   * Handle focus out events (iOS keyboard detection)
   */
  private handleFocusOut(_e: Event): void {
    // Delay to check if keyboard is actually hiding
    setTimeout(() => {
      if (!document.activeElement || 
          (document.activeElement.tagName !== 'INPUT' && 
           document.activeElement.tagName !== 'TEXTAREA' && 
           !document.activeElement.isContentEditable)) {
        this.virtualKeyboardVisible = false;
        this.handleVirtualKeyboardChange(false);
      }
    }, 100);
  }

  /**
   * Handle virtual keyboard visibility change
   */
  private handleVirtualKeyboardChange(visible: boolean): void {
    if (!this.container) return;

    // Recalculate dimensions when keyboard shows/hides
    const rect = this.container.getBoundingClientRect();
    const newHeight = visible ? this.getAvailableHeight() : rect.height;
    
    this.handleResize(rect.width, newHeight);

    // Add/remove CSS class for styling
    if (visible) {
      this.container.classList.add('virtual-keyboard-visible');
    } else {
      this.container.classList.remove('virtual-keyboard-visible');
    }
  }

  /**
   * Get current viewport dimensions
   */
  getDimensions(): ViewportDimensions | null {
    return this.lastDimensions;
  }

  /**
   * Check if virtual keyboard is currently visible
   */
  isVirtualKeyboardVisible(): boolean {
    return this.virtualKeyboardVisible;
  }

  /**
   * Manually trigger a resize calculation
   */
  refresh(): void {
    if (!this.container) return;
    
    const rect = this.container.getBoundingClientRect();
    this.handleResize(rect.width, rect.height);
  }

  /**
   * Destroy the viewport manager and clean up
   */
  destroy(): void {
    // Remove ResizeObserver
    if (this.resizeObserver) {
      this.resizeObserver.disconnect();
      this.resizeObserver = null;
    }

    // Remove touch event listeners
    if (this.container && this.touchInputHandler) {
      this.container.removeEventListener('touchstart', this.touchInputHandler);
      this.container.removeEventListener('touchend', this.touchInputHandler);
    }

    // Remove keyboard detection listeners using stored bound references
    if (this.boundHandleWindowResize) {
      window.removeEventListener('resize', this.boundHandleWindowResize);
      this.boundHandleWindowResize = null;
    }

    if (this.boundHandleFocusIn) {
      document.removeEventListener('focusin', this.boundHandleFocusIn);
      this.boundHandleFocusIn = null;
    }

    if (this.boundHandleFocusOut) {
      document.removeEventListener('focusout', this.boundHandleFocusOut);
      this.boundHandleFocusOut = null;
    }

    if (window.visualViewport && this.boundHandleVisualViewportResize) {
      window.visualViewport.removeEventListener('resize', this.boundHandleVisualViewportResize);
      window.visualViewport.removeEventListener('scroll', this.boundHandleVisualViewportResize);
      this.boundHandleVisualViewportResize = null;
    }

    // Remove measure element
    if (this.fontMeasureElement) {
      this.fontMeasureElement.remove();
      this.fontMeasureElement = null;
    }

    // Clean up references
    this.container = null;
    this.lastDimensions = null;
  }
}

/**
 * Create a new MobileViewportManager instance
 */
export function createMobileViewportManager(config: ViewportConfig): MobileViewportManager {
  return new MobileViewportManager(config);
}

export default MobileViewportManager;
