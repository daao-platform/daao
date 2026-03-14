import React, { useEffect, useRef, useState, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { FitAddon } from '@xterm/addon-fit';
import { XtermTerminal, TerminalConfig } from '../terminal';
import { ArrowLeftIcon, MoreIcon, WifiIcon, XIcon, PauseIcon } from '../components/Icons';
import { createTransport } from '../transport/negotiate';
import type { TransportClient } from '../transport/types';
import {
    getSession,
    attachSession,
    detachSession,
    suspendSession,
    killSession,
    renameSession,
    startRecording,
    stopRecording,
    SESSION_STATES,
    type Session,
} from '../api/client';

/**
 * Tokyo Night terminal theme — matches the Stitch designs
 */
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

/**
 * TerminalView — Immersive terminal with agent panel
 * 
 * Desktop: 60/40 split (terminal + agent panel)
 * Mobile: Fullscreen terminal with peekable bottom sheet
 */
const TerminalView: React.FC = () => {
    const { sessionId } = useParams<{ sessionId: string }>();
    const navigate = useNavigate();
    const terminalRef = useRef<HTMLDivElement>(null);
    const xtermRef = useRef<XtermTerminal | null>(null);
    const xtermJsRef = useRef<any>(null); // xterm.js Terminal instance
    const fitAddonRef = useRef<FitAddon | null>(null);
    const transportRef = useRef<TransportClient | null>(null);
    const [transportType, setTransportType] = useState<'websocket' | 'webtransport' | null>(null);

    // Session state
    const [session, setSession] = useState<Session | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    // Connection state
    const [connected, setConnected] = useState(false);
    const [reconnecting, setReconnecting] = useState(false);
    const [terminated, setTerminated] = useState(false);
    const [latency, setLatency] = useState<number | null>(null);
    const [termSize, setTermSize] = useState({ cols: 120, rows: 36 });
    const pingTime = useRef<number>(0);

    // Rename state
    const [isRenaming, setIsRenaming] = useState(false);
    const [renameValue, setRenameValue] = useState('');

    // Recording state
    const [isRecording, setIsRecording] = useState(false);
    const [recordingId, setRecordingId] = useState<string | null>(null);

    // Load session info on mount
    useEffect(() => {
        if (!sessionId) {
            setError('No session ID provided');
            setLoading(false);
            return;
        }

        const loadSession = async () => {
            try {
                setLoading(true);
                setError(null);
                const sess = await getSession(sessionId);
                setSession(sess);
            } catch (err) {
                if (err instanceof Error) {
                    if (err.message.includes('404') || err.message.includes('not found')) {
                        setError('Session not found');
                    } else {
                        setError(err.message);
                    }
                } else {
                    setError('Failed to load session');
                }
            } finally {
                setLoading(false);
            }
        };

        loadSession();
    }, [sessionId]);

    // Connect to session via transport (WebTransport with WebSocket fallback)
    useEffect(() => {
        if (!sessionId || !session) return;
        // Don't connect if already known terminated
        if (session.state === SESSION_STATES.TERMINATED) {
            setTerminated(true);
            return;
        }

        let isMounted = true;
        let pingInterval: ReturnType<typeof setInterval> | null = null;

        const connectTransport = async () => {
            if (!isMounted) return;

            try {
                // Read auth token from storage (same pattern as api/client.ts)
                const authToken = sessionStorage.getItem('oidc_access_token') || localStorage.getItem('auth_token') || undefined;
                const transport = await createTransport(sessionId, window.location.host, authToken);
                if (!isMounted) {
                    transport.close();
                    return;
                }

                transportRef.current = transport;
                setTransportType(transport.transportType);

                // Set up callbacks for ongoing events
                transport.onConnect = () => {
                    // This fires on *reconnection* (WebSocket backoff reconnect)
                    if (!isMounted) return;
                    setConnected(true);
                    setReconnecting(false);
                };

                transport.onControlMessage = (msg: any) => {
                    if (!isMounted) return;
                    if (msg.type === 'pong') {
                        setLatency(Math.round(performance.now() - pingTime.current));
                        return;
                    }
                    if (msg.type === 'terminated') {
                        setTerminated(true);
                        setSession(s => s ? { ...s, state: SESSION_STATES.TERMINATED } : s);
                        transport.close();
                        return;
                    }
                };

                transport.onTerminalData = (data: string | ArrayBuffer) => {
                    if (!isMounted || !xtermJsRef.current) return;
                    if (typeof data === 'string') {
                        xtermJsRef.current.write(data);
                    } else {
                        xtermJsRef.current.write(new Uint8Array(data));
                    }
                };

                transport.onDisconnect = (reason: string) => {
                    if (!isMounted) return;
                    setConnected(false);
                    if (pingInterval) {
                        clearInterval(pingInterval);
                        pingInterval = null;
                    }
                    if (reason === 'reconnecting') {
                        setReconnecting(true);
                    }
                };

                transport.onError = (error: Error) => {
                    console.error('Transport error:', error);
                };

                // Transport is already connected (createTransport resolves after connect)
                setConnected(true);
                setReconnecting(false);

                // Send initial terminal dimensions to PTY — FitAddon fires onResize
                // ~100ms after terminal init, but the transport may not have been
                // connected yet, so the resize was silently dropped. Sync now.
                if (xtermJsRef.current) {
                    const term = xtermJsRef.current;
                    const cols = (term as any).cols || 80;
                    const rows = (term as any).rows || 24;
                    transport.sendControl({ type: 'resize', cols, rows });
                }

                // Attach session (idempotent — server returns current state)
                attachSession(sessionId!).catch(err =>
                    console.warn('Failed to attach on connect:', err)
                );

                // Start RTT measurement
                const sendPing = () => {
                    if (transport.connected) {
                        pingTime.current = performance.now();
                        transport.sendControl({ type: 'ping' });
                    }
                };
                sendPing();
                pingInterval = setInterval(sendPing, 3000);

            } catch (err) {
                console.error('Failed to connect transport:', err);
                if (isMounted) {
                    setReconnecting(true);
                    // Retry after delay
                    setTimeout(() => connectTransport(), 3000);
                }
            }
        };

        connectTransport();

        return () => {
            isMounted = false;
            if (pingInterval) clearInterval(pingInterval);
            if (transportRef.current) transportRef.current.close();
        };
    }, [sessionId, session?.id]); // eslint-disable-line react-hooks/exhaustive-deps

    // Handle Detach
    const handleDetach = useCallback(async () => {
        if (!sessionId) return;

        try {
            await detachSession(sessionId);
            navigate('/sessions');
        } catch (err) {
            console.error('Failed to detach session:', err);
        }
    }, [sessionId, navigate]);

    // Handle Suspend
    const handleSuspend = useCallback(async () => {
        if (!sessionId) return;

        try {
            await suspendSession(sessionId);
            // Update local session state
            if (session) {
                setSession({ ...session, state: SESSION_STATES.SUSPENDED });
            }
        } catch (err) {
            console.error('Failed to suspend session:', err);
        }
    }, [sessionId, session]);

    // Handle Kill
    const handleKill = useCallback(async () => {
        if (!sessionId) return;

        try {
            await killSession(sessionId);
            navigate('/sessions');
        } catch (err) {
            console.error('Failed to kill session:', err);
        }
    }, [sessionId, navigate]);

    // Handle Start Recording
    const handleRecord = useCallback(async () => {
        if (!sessionId || isRecording) return;
        try {
            // Pass the actual terminal dimensions so the .cast header is accurate
            const term = xtermRef.current?.getTerminal();
            const cols = term?.cols || 80;
            const rows = term?.rows || 24;
            const result = await startRecording(sessionId, cols, rows);
            setIsRecording(true);
            setRecordingId(result.recording_id);
        } catch (err) {
            console.error('Failed to start recording:', err);
        }
    }, [sessionId, isRecording]);

    // Handle Stop Recording
    const handleStopRecord = useCallback(async () => {
        if (!sessionId || !isRecording) return;
        try {
            await stopRecording(sessionId);
            setIsRecording(false);
            setRecordingId(null);
        } catch (err) {
            console.error('Failed to stop recording:', err);
        }
    }, [sessionId, isRecording]);

    // Initialize xterm.js terminal
    useEffect(() => {
        if (!terminalRef.current || !session) return;

        const config: TerminalConfig = {
            // Don't hardcode cols/rows — FitAddon will compute them from
            // the container size and send a resize event to the PTY.
            scrollback: 10000,
            cursorStyle: 'block',
            cursorBlink: true,
            fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', Consolas, monospace",
            fontSize: 13,
        };

        const xterm = new XtermTerminal(config, {
            onReady: (terminal) => {
                // Store the xterm.js Terminal instance for WebSocket data
                xtermJsRef.current = terminal;

                // Apply Tokyo Night theme
                (terminal as any).options.theme = tokyoNightTheme;

                // Load FitAddon to auto-size the terminal to its container
                const fitAddon = new FitAddon();
                terminal.loadAddon(fitAddon);
                fitAddonRef.current = fitAddon;
                // Initial fit after the terminal is attached and rendered
                setTimeout(() => fitAddon.fit(), 100);

                // Write welcome message
                terminal.writeln('\x1b[38;2;45;212;191m✓\x1b[0m Connected to session \x1b[1m' + (session.name || sessionId?.slice(0, 8)) + '\x1b[0m');
                terminal.writeln('\x1b[38;2;148;163;184m  ' + (session.agent_binary || 'agent') + '\x1b[0m');
                terminal.writeln('');
                terminal.writeln('\x1b[38;2;122;162;247m$\x1b[0m ');
            },
            onResize: (cols, rows) => {
                setTermSize({ cols, rows });
                // Send resize to server via transport
                transportRef.current?.sendControl({ type: 'resize', cols, rows });
            },
            onData: (data) => {
                // Send keyboard input via transport
                transportRef.current?.send(data);
            },
        });

        xterm.attach(terminalRef.current);
        xtermRef.current = xterm;

        // Focus terminal
        setTimeout(() => xterm.focus(), 100);

        return () => {
            xterm.destroy();
            xtermRef.current = null;
        };
    }, [session, sessionId]);

    // ResizeObserver — catches ALL container size changes:
    // window resize, sidebar toggle, route transitions, etc.
    useEffect(() => {
        if (!terminalRef.current) return;
        const container = terminalRef.current;
        const observer = new ResizeObserver(() => {
            if (fitAddonRef.current) {
                requestAnimationFrame(() => fitAddonRef.current?.fit());
            }
        });
        observer.observe(container);
        return () => observer.disconnect();
    }, [session]);

    // Render loading state
    if (loading) {
        return (
            <div className="terminal-view">
                <div className="terminal-loading">
                    <div className="spinner" />
                    <p>Loading session...</p>
                </div>
            </div>
        );
    }

    // Render error state
    if (error) {
        return (
            <div className="terminal-view">
                <div className="terminal-toolbar">
                    <button
                        className="btn btn--icon"
                        onClick={() => navigate('/sessions')}
                        aria-label="Back to sessions"
                    >
                        <ArrowLeftIcon size={20} />
                    </button>
                    <span className="terminal-toolbar__title">Error</span>
                </div>
                <div className="terminal-error">
                    <div className="error-icon">⚠</div>
                    <h3>Session Not Found</h3>
                    <p>{error}</p>
                    <button
                        className="btn btn--primary"
                        onClick={() => navigate('/sessions')}
                    >
                        Back to Sessions
                    </button>
                </div>
            </div>
        );
    }

    // Get session state badge
    const getStateBadge = () => {
        const state = (terminated || session?.state === SESSION_STATES.TERMINATED)
            ? SESSION_STATES.TERMINATED
            : session?.state;
        if (state === SESSION_STATES.RUNNING) {
            return <span className="badge badge--running"><span className="badge__dot" />RUNNING</span>;
        } else if (state === SESSION_STATES.SUSPENDED) {
            return <span className="badge badge--suspended"><span className="badge__dot" />SUSPENDED</span>;
        } else if (state === SESSION_STATES.DETACHED) {
            return <span className="badge badge--detached">DETACHED</span>;
        } else if (state === SESSION_STATES.PROVISIONING) {
            return <span className="badge badge--provisioning">PROVISIONING</span>;
        } else if (state === SESSION_STATES.TERMINATED) {
            return <span className="badge badge--terminated">TERMINATED</span>;
        }
        return <span className="badge">{state}</span>;
    };

    return (
        <div className="terminal-view">
            {/* Top Bar */}
            <div className="terminal-toolbar">
                <button
                    className="btn btn--icon"
                    onClick={() => navigate('/sessions')}
                    aria-label="Back to sessions"
                >
                    <ArrowLeftIcon size={20} />
                </button>
                <div className="terminal-toolbar__title">
                    {isRenaming ? (
                        <input
                            className="terminal-toolbar__rename-input"
                            value={renameValue}
                            autoFocus
                            onChange={e => setRenameValue(e.target.value)}
                            onKeyDown={async (e) => {
                                if (e.key === 'Enter') {
                                    const trimmed = renameValue.trim();
                                    if (trimmed && sessionId) {
                                        try {
                                            await renameSession(sessionId, trimmed);
                                            setSession(s => s ? { ...s, name: trimmed } : s);
                                        } catch (err) {
                                            console.error('Failed to rename session:', err);
                                        }
                                    }
                                    setIsRenaming(false);
                                } else if (e.key === 'Escape') {
                                    setIsRenaming(false);
                                }
                            }}
                            onBlur={async () => {
                                const trimmed = renameValue.trim();
                                if (trimmed && sessionId && trimmed !== session?.name) {
                                    try {
                                        await renameSession(sessionId, trimmed);
                                        setSession(s => s ? { ...s, name: trimmed } : s);
                                    } catch (err) {
                                        console.error('Failed to rename session:', err);
                                    }
                                }
                                setIsRenaming(false);
                            }}
                        />
                    ) : (
                        <span
                            className="terminal-toolbar__name"
                            onClick={() => {
                                setRenameValue(session?.name || '');
                                setIsRenaming(true);
                            }}
                            title="Click to rename"
                        >
                            {session?.name || 'Unknown Session'}
                        </span>
                    )}
                    <span className="terminal-toolbar__satellite">{session?.agent_binary || 'agent'}</span>
                </div>
                {getStateBadge()}
                <div className="terminal-toolbar__actions">
                    {/* Recording controls */}
                    {isRecording ? (
                        <button
                            className="btn btn--outline btn--sm"
                            onClick={handleStopRecord}
                            title="Stop recording"
                            style={{ color: '#f7768e', borderColor: '#f7768e44' }}
                        >
                            <span style={{
                                display: 'inline-block',
                                width: 8,
                                height: 8,
                                borderRadius: '50%',
                                background: '#f7768e',
                                marginRight: 6,
                                animation: 'blink 1s ease-in-out infinite',
                            }} />
                            REC
                        </button>
                    ) : (
                        <button
                            className="btn btn--outline btn--sm"
                            onClick={handleRecord}
                            title="Start recording"
                            disabled={terminated || session?.state !== SESSION_STATES.RUNNING}
                        >
                            ⏺ Record
                        </button>
                    )}
                    <button className="btn btn--outline btn--sm" onClick={handleDetach}>Detach</button>
                    <button className="btn btn--outline btn--sm" onClick={handleSuspend}>
                        <PauseIcon size={14} />
                    </button>
                    <button className="btn btn--danger btn--sm" onClick={handleKill}>
                        <XIcon size={14} />
                    </button>
                </div>
            </div>

            {/* Reconnection UI */}
            {reconnecting && !terminated && (
                <div className="terminal-reconnect">
                    <div className="spinner spinner--sm" />
                    <span>Reconnecting...</span>
                </div>
            )}

            {/* Terminated overlay */}
            {terminated && (
                <div className="terminal-reconnect" style={{
                    background: 'rgba(239, 68, 68, 0.1)',
                    borderBottom: '1px solid rgba(239, 68, 68, 0.3)',
                    color: 'var(--text-secondary)',
                }}>
                    <span>Session ended — process exited</span>
                    <button
                        className="btn btn--outline btn--sm"
                        style={{ marginLeft: 12, fontSize: 12 }}
                        onClick={() => navigate('/sessions')}
                    >
                        Back to Sessions
                    </button>
                </div>
            )}

            {/* Main Split */}
            <div className="terminal-split">
                {/* Terminal Panel */}
                <div className="terminal-panel">
                    <div ref={terminalRef} className="terminal-container" />
                </div>

                {/* Agent Panel (Desktop only) */}
                <div className="agent-panel">
                    <div className="agent-panel__section">
                        <div className="agent-panel__header">Session Info</div>
                        <div className="agent-panel__agent-card">
                            <div className="agent-avatar" style={{ background: '#F9731622', color: '#F97316' }}>S</div>
                            <div>
                                <div style={{ fontWeight: 600, fontSize: 14 }}>{session?.name || 'Session'}</div>
                                <div style={{ fontSize: 12, color: 'var(--text-muted)' }}>ID: {sessionId?.slice(0, 8)}</div>
                            </div>
                        </div>
                        <div className="session-info-grid">
                            <div className="info-item">
                                <span className="info-label">Agent</span>
                                <span className="info-value">{session?.agent_binary || 'N/A'}</span>
                            </div>
                            <div className="info-item">
                                <span className="info-label">Satellite</span>
                                <span className="info-value">{session?.satellite_id || 'N/A'}</span>
                            </div>
                        </div>
                    </div>

                    <div className="agent-panel__section">
                        <div className="agent-panel__header">Quick Actions</div>
                        <div className="quick-actions">
                            <button className="btn btn--outline btn--sm flex-1" onClick={handleDetach}>Detach</button>
                            <button className="btn btn--outline btn--sm flex-1" onClick={handleSuspend}>Suspend</button>
                            <button className="btn btn--danger btn--sm flex-1" onClick={handleKill}>Kill</button>
                        </div>
                    </div>
                </div>
            </div>

            {/* Status Bar (Desktop) */}
            <div className="terminal-statusbar">
                <div className="status-bar__item">
                    <span className={`status-dot ${connected ? 'status-dot--online' : 'status-dot--offline'}`} />
                    {connected ? 'Connected' : 'Disconnected'}
                </div>
                {transportType && (
                    <div className="status-bar__item" style={{ opacity: 0.7 }}>
                        {transportType === 'webtransport' ? '⚡ WebTransport' : '🔌 WebSocket'}
                    </div>
                )}
                {reconnecting && <div className="status-bar__item status-bar__item--warning">Reconnecting...</div>}
                <div className={`status-bar__item status-bar__latency--${latency === null ? 'good' : latency <= 50 ? 'good' : latency <= 150 ? 'fair' : 'poor'}`}>
                    Latency: {latency !== null ? `${latency}ms` : '—'}
                </div>
                {isRecording && (
                    <div className="status-bar__item" style={{ color: '#f7768e' }}>
                        <span style={{
                            display: 'inline-block',
                            width: 6,
                            height: 6,
                            borderRadius: '50%',
                            background: '#f7768e',
                            marginRight: 4,
                            animation: 'blink 1s ease-in-out infinite',
                        }} />
                        Recording
                    </div>
                )}
                <div className="status-bar__item text-mono">{termSize.cols}×{termSize.rows}</div>
            </div>
        </div>
    );
};

export default TerminalView;
