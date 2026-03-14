import React, { useState, useEffect, useRef, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { FitAddon } from '@xterm/addon-fit';
import { Terminal } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';
import {
    getSessions,
    attachSession,
    SESSION_STATES,
    type Session,
    type SessionsPaginatedResponse,
} from '../api/client';
import { useApi } from '../hooks';
import { createTransport } from '../transport/negotiate';
import type { TransportClient } from '../transport/types';
import {
    ArrowLeftIcon,
    GridViewIcon,
    MaximizeIcon,
    MinimizeIcon,
    XIcon,
    TerminalIcon,
} from '../components/Icons';

// ============================================================================
// Tokyo Night Theme — shared with TerminalView
// ============================================================================
const tokyoNightTheme = {
    background: '#1a1b26',
    foreground: '#c0caf5',
    cursor: '#c0caf5',
    cursorAccent: '#1a1b26',
    selectionBackground: '#33467c',
    selectionForeground: '#c0caf5',
    black: '#15161e',
    red: '#f7768e',
    green: '#9ece6a',
    yellow: '#e0af68',
    blue: '#7aa2f7',
    magenta: '#bb9af7',
    cyan: '#7dcfff',
    white: '#a9b1d6',
    brightBlack: '#414868',
    brightRed: '#f7768e',
    brightGreen: '#9ece6a',
    brightYellow: '#e0af68',
    brightBlue: '#7aa2f7',
    brightMagenta: '#bb9af7',
    brightCyan: '#7dcfff',
    brightWhite: '#c0caf5',
};

// ============================================================================
// Layout Presets
// ============================================================================
type LayoutPreset = '1x1' | '2x1' | '2x2' | '3x2';

interface LayoutConfig {
    cols: number;
    rows: number;
    label: string;
    paneCount: number;
}

const LAYOUTS: Record<LayoutPreset, LayoutConfig> = {
    '1x1': { cols: 1, rows: 1, label: '1', paneCount: 1 },
    '2x1': { cols: 2, rows: 1, label: '2', paneCount: 2 },
    '2x2': { cols: 2, rows: 2, label: '4', paneCount: 4 },
    '3x2': { cols: 3, rows: 2, label: '6', paneCount: 6 },
};

// ============================================================================
// Terminal Pane Component
// ============================================================================
interface TerminalPaneProps {
    session: Session | null;
    paneIndex: number;
    isMaximized: boolean;
    layout: LayoutPreset;
    onAssignSession: (paneIndex: number) => void;
    onMaximize: (paneIndex: number) => void;
    onMinimize: () => void;
    onRemove: (paneIndex: number) => void;
}

const TerminalPane: React.FC<TerminalPaneProps> = ({
    session,
    paneIndex,
    isMaximized,
    layout,
    onAssignSession,
    onMaximize,
    onMinimize,
    onRemove,
}) => {
    const containerRef = useRef<HTMLDivElement>(null);
    const terminalRef = useRef<Terminal | null>(null);
    const fitAddonRef = useRef<FitAddon | null>(null);
    const transportRef = useRef<TransportClient | null>(null);
    const [connected, setConnected] = useState(false);
    const [latency, setLatency] = useState<number | null>(null);
    const [activeTransportType, setActiveTransportType] = useState<string | null>(null);
    const pingTimeRef = useRef<number>(0);

    // Connect via transport when session is assigned
    useEffect(() => {
        if (!session || !containerRef.current) return;

        // Create terminal
        const terminal = new Terminal({
            scrollback: 5000,
            cursorStyle: 'block',
            cursorBlink: true,
            fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', Consolas, monospace",
            fontSize: 12,
            theme: tokyoNightTheme,
            convertEol: false, // ConPTY already sends \r\n
            allowTransparency: false,
            drawBoldTextInBrightColors: true,
        });

        const fitAddon = new FitAddon();
        terminal.loadAddon(fitAddon);

        terminal.open(containerRef.current);
        terminalRef.current = terminal;
        fitAddonRef.current = fitAddon;

        // Fit after initial render
        setTimeout(() => fitAddon.fit(), 50);

        let pingInterval: ReturnType<typeof setInterval> | null = null;
        let disposed = false;

        const connectPane = async () => {
            if (disposed) return;
            try {
                const transport = await createTransport(session.id, window.location.host);
                if (disposed) {
                    transport.close();
                    return;
                }
                transportRef.current = transport;
                setActiveTransportType(transport.transportType);

                transport.onConnect = () => {
                    // Fires on *reconnection* (WebSocket backoff reconnect)
                    setConnected(true);
                };

                transport.onControlMessage = (msg: any) => {
                    if (msg.type === 'pong') {
                        setLatency(Math.round(performance.now() - pingTimeRef.current));
                        return;
                    }
                    if (msg.type === 'terminated') {
                        setConnected(false);
                        return;
                    }
                };

                transport.onTerminalData = (data: string | ArrayBuffer) => {
                    if (!terminalRef.current) return;
                    if (typeof data === 'string') {
                        terminalRef.current.write(data);
                    } else {
                        terminalRef.current.write(new Uint8Array(data));
                    }
                };

                transport.onDisconnect = () => {
                    setConnected(false);
                    if (pingInterval) clearInterval(pingInterval);
                };

                transport.onError = () => { };

                // Already connected from createTransport
                setConnected(true);

                // Send initial terminal dimensions to PTY — FitAddon fires
                // before transport is ready, so the first resize is dropped.
                if (terminalRef.current) {
                    const cols = terminalRef.current.cols || 80;
                    const rows = terminalRef.current.rows || 24;
                    transport.sendControl({ type: 'resize', cols, rows });
                }

                attachSession(session.id).catch(() => { });

                // Start ping for latency measurement
                const sendPing = () => {
                    if (transport.connected) {
                        pingTimeRef.current = performance.now();
                        transport.sendControl({ type: 'ping' });
                    }
                };
                sendPing();
                pingInterval = setInterval(sendPing, 10000);

            } catch (err) {
                console.error('Multi-pane transport error:', err);
            }
        };

        connectPane();

        // Handle keyboard input
        const dataDisposable = terminal.onData((data) => {
            transportRef.current?.send(data);
        });

        // Handle resize
        const resizeDisposable = terminal.onResize(({ cols, rows }) => {
            transportRef.current?.sendControl({ type: 'resize', cols, rows });
        });

        return () => {
            disposed = true;
            dataDisposable.dispose();
            resizeDisposable.dispose();
            if (pingInterval) clearInterval(pingInterval);
            if (transportRef.current) transportRef.current.close();
            terminal.dispose();
            terminalRef.current = null;
            fitAddonRef.current = null;
            transportRef.current = null;
        };
    }, [session]);

    // Re-fit on maximize/minimize and layout changes
    useEffect(() => {
        if (fitAddonRef.current) {
            // Small delay to let CSS grid transition complete
            setTimeout(() => fitAddonRef.current?.fit(), 100);
        }
    }, [isMaximized, layout]);

    // ResizeObserver — catches ALL container size changes:
    // window resize, layout switches, sidebar toggle, maximize/minimize
    useEffect(() => {
        if (!containerRef.current) return;
        const observer = new ResizeObserver(() => {
            // Debounce slightly to avoid fitting during rapid resize
            if (fitAddonRef.current) {
                requestAnimationFrame(() => fitAddonRef.current?.fit());
            }
        });
        observer.observe(containerRef.current);
        return () => observer.disconnect();
    }, [session]); // Re-create observer when session changes (terminal re-mounts)

    // Empty pane
    if (!session) {
        return (
            <div className="multi-pane multi-pane--empty" onClick={() => onAssignSession(paneIndex)}>
                <div className="multi-pane__empty-content">
                    <TerminalIcon size={32} />
                    <span>Click to assign session</span>
                </div>
            </div>
        );
    }

    // State badge color
    const getStateColor = () => {
        switch (session.state) {
            case SESSION_STATES.RUNNING: return 'var(--green)';
            case SESSION_STATES.SUSPENDED: return 'var(--yellow)';
            case SESSION_STATES.DETACHED: return 'var(--blue)';
            case SESSION_STATES.TERMINATED: return 'var(--red)';
            default: return 'var(--text-muted)';
        }
    };

    return (
        <div className={`multi-pane${isMaximized ? ' multi-pane--maximized' : ''}`}>
            {/* Pane header */}
            <div className="multi-pane__header">
                <div className="multi-pane__title">
                    <span
                        className="multi-pane__dot"
                        style={{ background: connected ? 'var(--green)' : 'var(--red)' }}
                    />
                    <span className="multi-pane__name">{session.name || session.id.slice(0, 8)}</span>
                    <span className="multi-pane__state" style={{ color: getStateColor() }}>
                        {session.state}
                    </span>
                </div>
                <div className="multi-pane__actions">
                    {latency !== null && (
                        <span className="multi-pane__latency">{latency}ms</span>
                    )}
                    {activeTransportType && (
                        <span className="multi-pane__latency" style={{ opacity: 0.6, fontSize: 10 }}>
                            {activeTransportType === 'webtransport' ? 'WT' : 'WS'}
                        </span>
                    )}
                    <button
                        className="multi-pane__btn"
                        onClick={() => isMaximized ? onMinimize() : onMaximize(paneIndex)}
                        title={isMaximized ? 'Minimize' : 'Maximize'}
                    >
                        {isMaximized ? <MinimizeIcon size={14} /> : <MaximizeIcon size={14} />}
                    </button>
                    <button
                        className="multi-pane__btn"
                        onClick={() => onRemove(paneIndex)}
                        title="Remove"
                    >
                        <XIcon size={14} />
                    </button>
                </div>
            </div>

            {/* Terminal container */}
            <div ref={containerRef} className="multi-pane__terminal" />
        </div>
    );
};

// ============================================================================
// Session Picker Modal
// ============================================================================
interface SessionPickerProps {
    sessions: Session[];
    assignedSessionIds: Set<string>;
    onSelect: (session: Session) => void;
    onClose: () => void;
}

const SessionPicker: React.FC<SessionPickerProps> = ({
    sessions,
    assignedSessionIds,
    onSelect,
    onClose,
}) => {
    const availableSessions = sessions.filter(
        s => s.state !== SESSION_STATES.TERMINATED && !assignedSessionIds.has(s.id)
    );

    return (
        <div className="session-picker__overlay" onClick={onClose}>
            <div className="session-picker" onClick={e => e.stopPropagation()}>
                <div className="session-picker__header">
                    <h3>Assign Session</h3>
                    <button className="multi-pane__btn" onClick={onClose}>
                        <XIcon size={18} />
                    </button>
                </div>
                <div className="session-picker__list">
                    {availableSessions.length === 0 ? (
                        <div className="session-picker__empty">
                            <TerminalIcon size={24} />
                            <span>No available sessions</span>
                            <span className="session-picker__hint">
                                All active sessions are already assigned to panes,
                                or no sessions are running.
                            </span>
                        </div>
                    ) : (
                        availableSessions.map(session => (
                            <button
                                key={session.id}
                                className="session-picker__item"
                                onClick={() => onSelect(session)}
                            >
                                <div className="session-picker__item-info">
                                    <span className="session-picker__item-name">
                                        {session.name || session.id.slice(0, 8)}
                                    </span>
                                    <span className="session-picker__item-agent">
                                        {session.agent_binary || 'agent'}
                                    </span>
                                </div>
                                <span
                                    className="session-picker__item-state"
                                    style={{
                                        color: session.state === SESSION_STATES.RUNNING
                                            ? 'var(--green)'
                                            : 'var(--yellow)',
                                    }}
                                >
                                    {session.state}
                                </span>
                            </button>
                        ))
                    )}
                </div>
            </div>
        </div>
    );
};

// ============================================================================
// Multi-Session Dashboard Page
// ============================================================================
const MultiSessionDashboard: React.FC = () => {
    const navigate = useNavigate();
    const [layout, setLayout] = useState<LayoutPreset>('2x2');
    const [panes, setPanes] = useState<(Session | null)[]>([null, null, null, null]);
    const [maximizedPane, setMaximizedPane] = useState<number | null>(null);
    const [pickerTarget, setPickerTarget] = useState<number | null>(null);

    // Fetch all sessions for the session picker
    const { data: sessionsData, refetch } = useApi<SessionsPaginatedResponse>(
        () => getSessions({ limit: 50 })
    );
    const allSessions = sessionsData?.items || [];

    // Auto-populate panes with active sessions on first load
    const hasAutoPopulated = useRef(false);
    useEffect(() => {
        if (hasAutoPopulated.current || !allSessions.length) return;
        hasAutoPopulated.current = true;

        const activeSessions = allSessions.filter(
            s => s.state === SESSION_STATES.RUNNING || s.state === SESSION_STATES.DETACHED
        );

        const layoutConfig = LAYOUTS[layout];
        const newPanes: (Session | null)[] = [];
        for (let i = 0; i < layoutConfig.paneCount; i++) {
            newPanes.push(activeSessions[i] || null);
        }
        setPanes(newPanes);
    }, [allSessions, layout]);

    // Change layout
    const handleLayoutChange = useCallback((newLayout: LayoutPreset) => {
        const newConfig = LAYOUTS[newLayout];
        setPanes(prev => {
            const newPanes: (Session | null)[] = [];
            for (let i = 0; i < newConfig.paneCount; i++) {
                newPanes.push(prev[i] || null);
            }
            return newPanes;
        });
        setLayout(newLayout);
        setMaximizedPane(null);
    }, []);

    // Assign session to pane
    const handleAssignSession = useCallback((paneIndex: number) => {
        setPickerTarget(paneIndex);
    }, []);

    const handleSelectSession = useCallback((session: Session) => {
        if (pickerTarget === null) return;
        setPanes(prev => {
            const newPanes = [...prev];
            newPanes[pickerTarget] = session;
            return newPanes;
        });
        setPickerTarget(null);
    }, [pickerTarget]);

    // Remove session from pane
    const handleRemove = useCallback((paneIndex: number) => {
        setPanes(prev => {
            const newPanes = [...prev];
            newPanes[paneIndex] = null;
            return newPanes;
        });
        if (maximizedPane === paneIndex) setMaximizedPane(null);
    }, [maximizedPane]);

    // Get assigned session IDs for filtering picker
    const assignedSessionIds = new Set(
        panes.filter((p): p is Session => p !== null).map(p => p.id)
    );

    const layoutConfig = LAYOUTS[layout];

    return (
        <div className="multi-dashboard">
            {/* Toolbar */}
            <div className="multi-dashboard__toolbar">
                <div className="multi-dashboard__toolbar-left">
                    <button
                        className="btn btn--icon"
                        onClick={() => navigate('/sessions')}
                        title="Back to Sessions"
                    >
                        <ArrowLeftIcon size={20} />
                    </button>
                    <h2 className="multi-dashboard__title">
                        <GridViewIcon size={20} />
                        Multi-View
                    </h2>
                </div>

                <div className="multi-dashboard__toolbar-center">
                    {(Object.keys(LAYOUTS) as LayoutPreset[]).map(preset => (
                        <button
                            key={preset}
                            className={`multi-dashboard__layout-btn${layout === preset ? ' active' : ''}`}
                            onClick={() => handleLayoutChange(preset)}
                            title={`${LAYOUTS[preset].label} panes`}
                        >
                            {LAYOUTS[preset].label}
                        </button>
                    ))}
                </div>

                <div className="multi-dashboard__toolbar-right">
                    <span className="multi-dashboard__count">
                        {panes.filter(p => p !== null).length} / {layoutConfig.paneCount} sessions
                    </span>
                    <button
                        className="btn btn--outline btn--sm"
                        onClick={() => { refetch(); }}
                    >
                        Refresh
                    </button>
                </div>
            </div>

            {/* Grid */}
            <div
                className="multi-dashboard__grid"
                style={{
                    gridTemplateColumns: maximizedPane !== null
                        ? '1fr'
                        : `repeat(${layoutConfig.cols}, 1fr)`,
                    gridTemplateRows: maximizedPane !== null
                        ? '1fr'
                        : `repeat(${layoutConfig.rows}, 1fr)`,
                }}
            >
                {panes.map((session, index) => {
                    // If a pane is maximized, only render that pane
                    if (maximizedPane !== null && maximizedPane !== index) return null;

                    return (
                        <TerminalPane
                            key={`pane-${index}-${session?.id || 'empty'}`}
                            session={session}
                            paneIndex={index}
                            isMaximized={maximizedPane === index}
                            layout={layout}
                            onAssignSession={handleAssignSession}
                            onMaximize={setMaximizedPane}
                            onMinimize={() => setMaximizedPane(null)}
                            onRemove={handleRemove}
                        />
                    );
                })}
            </div>

            {/* Session Picker Modal */}
            {pickerTarget !== null && (
                <SessionPicker
                    sessions={allSessions}
                    assignedSessionIds={assignedSessionIds}
                    onSelect={handleSelectSession}
                    onClose={() => setPickerTarget(null)}
                />
            )}
        </div>
    );
};

export default MultiSessionDashboard;
