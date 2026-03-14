import { useState, useEffect, useRef, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import { getRecording, getRecordingStreamUrl, type Recording } from '../api/client';

/**
 * RecordingPlayer — plays back a recorded terminal session using xterm.js.
 *
 * Uses a full xterm.js Terminal to correctly render ANSI escape sequences,
 * colors, cursor positioning, and all terminal control codes.
 *
 * The terminal fills the available viewport space using FitAddon.
 * For old recordings with incorrect header dimensions, this ensures content
 * renders without wrapping. For new recordings with correct dimensions,
 * the content fills the screen naturally.
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

interface AsciicastEvent {
    offset: number; // seconds
    type: string;
    data: string;
}

interface AsciicastHeader {
    version: number;
    width: number;
    height: number;
    timestamp?: number;
    title?: string;
}

export default function RecordingPlayer() {
    const { recordingId } = useParams<{ recordingId: string }>();
    const navigate = useNavigate();

    const [recording, setRecording] = useState<Recording | null>(null);
    const [events, setEvents] = useState<AsciicastEvent[]>([]);
    const [header, setHeader] = useState<AsciicastHeader | null>(null);
    const [currentIndex, setCurrentIndex] = useState(0);
    const [isPlaying, setIsPlaying] = useState(false);
    const [speed, setSpeed] = useState(1);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(true);

    const terminalRef = useRef<HTMLDivElement>(null);
    const xtermRef = useRef<Terminal | null>(null);
    const fitAddonRef = useRef<FitAddon | null>(null);
    const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const currentIndexRef = useRef(0); // ref for timer callbacks

    // Keep the ref in sync with state
    useEffect(() => {
        currentIndexRef.current = currentIndex;
    }, [currentIndex]);

    // Fetch recording metadata
    useEffect(() => {
        if (!recordingId) return;
        (async () => {
            try {
                const rec = await getRecording(recordingId);
                setRecording(rec);
            } catch (err) {
                if (err instanceof Error && (err.message.includes('not found') || err.message.includes('404'))) {
                    setError('Recording not found — it may have been deleted.');
                } else {
                    setError('Failed to load recording metadata');
                }
                console.error(err);
            }
        })();
    }, [recordingId]);

    // Fetch and parse .cast file
    useEffect(() => {
        if (!recordingId) return;
        (async () => {
            try {
                const url = getRecordingStreamUrl(recordingId);
                const resp = await fetch(url);
                if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
                const text = await resp.text();
                const lines = text.trim().split('\n');

                // Parse header (line 0)
                if (lines.length > 0) {
                    try {
                        const h = JSON.parse(lines[0]);
                        setHeader(h);
                    } catch { /* skip */ }
                }

                // Parse events (line 1+)
                const parsed: AsciicastEvent[] = [];
                for (let i = 1; i < lines.length; i++) {
                    try {
                        const arr = JSON.parse(lines[i]);
                        if (Array.isArray(arr) && arr.length >= 3) {
                            parsed.push({
                                offset: arr[0],
                                type: arr[1],
                                data: arr[2],
                            });
                        }
                    } catch { /* skip malformed */ }
                }

                setEvents(parsed);
                setLoading(false);
            } catch (err) {
                setError('Failed to load recording data');
                setLoading(false);
                console.error(err);
            }
        })();
    }, [recordingId]);

    // Initialize xterm.js terminal — fill available viewport with FitAddon
    useEffect(() => {
        if (!terminalRef.current || loading) return;

        const term = new Terminal({
            scrollback: 50000,
            cursorStyle: 'block',
            cursorBlink: false,
            disableStdin: true, // read-only playback
            fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', Consolas, monospace",
            fontSize: 14,
            theme: tokyoNightTheme,
        });

        const fitAddon = new FitAddon();
        term.loadAddon(fitAddon);
        term.open(terminalRef.current);
        fitAddonRef.current = fitAddon;
        xtermRef.current = term;

        // Fit to container after mount
        setTimeout(() => fitAddon.fit(), 50);

        return () => {
            term.dispose();
            xtermRef.current = null;
            fitAddonRef.current = null;
        };
    }, [loading]);

    // Handle window resize
    useEffect(() => {
        const handleResize = () => {
            if (fitAddonRef.current) {
                fitAddonRef.current.fit();
            }
        };
        window.addEventListener('resize', handleResize);
        return () => window.removeEventListener('resize', handleResize);
    }, []);
    // Write events up to a given index into the terminal
    const replayUpTo = useCallback((targetIndex: number) => {
        const term = xtermRef.current;
        if (!term) return;
        term.reset();
        for (let i = 0; i < targetIndex && i < events.length; i++) {
            if (events[i].type === 'o') {
                term.write(events[i].data);
            }
        }
    }, [events]);

    // Playback loop using refs to avoid stale closures
    const scheduleNext = useCallback(() => {
        const idx = currentIndexRef.current;
        if (idx >= events.length) {
            setIsPlaying(false);
            return;
        }

        const event = events[idx];
        if (event.type === 'o' && xtermRef.current) {
            xtermRef.current.write(event.data);
        }

        const nextIdx = idx + 1;
        setCurrentIndex(nextIdx);
        currentIndexRef.current = nextIdx;

        if (nextIdx < events.length) {
            const nextEvent = events[nextIdx];
            let delay = (nextEvent.offset - event.offset) / speed;
            if (delay > 5) delay = 5;
            if (delay < 0) delay = 0;
            timerRef.current = setTimeout(scheduleNext, delay * 1000);
        } else {
            setIsPlaying(false);
        }
    }, [events, speed]);

    // Start/stop playback
    useEffect(() => {
        if (isPlaying) {
            scheduleNext();
        }
        return () => {
            if (timerRef.current) clearTimeout(timerRef.current);
        };
    }, [isPlaying, scheduleNext]);

    const handlePlay = () => {
        if (currentIndex >= events.length) {
            // Restart from beginning
            setCurrentIndex(0);
            currentIndexRef.current = 0;
            if (xtermRef.current) xtermRef.current.reset();
        }
        setIsPlaying(true);
    };

    const handlePause = () => {
        setIsPlaying(false);
        if (timerRef.current) clearTimeout(timerRef.current);
    };

    const handleRestart = () => {
        setIsPlaying(false);
        if (timerRef.current) clearTimeout(timerRef.current);
        setCurrentIndex(0);
        currentIndexRef.current = 0;
        if (xtermRef.current) xtermRef.current.reset();
    };

    const handleSeek = (e: React.ChangeEvent<HTMLInputElement>) => {
        const targetIndex = parseInt(e.target.value);
        setIsPlaying(false);
        if (timerRef.current) clearTimeout(timerRef.current);
        setCurrentIndex(targetIndex);
        currentIndexRef.current = targetIndex;
        replayUpTo(targetIndex);
    };

    const progress = events.length > 0 ? (currentIndex / events.length) * 100 : 0;
    const currentTime = currentIndex > 0 && currentIndex <= events.length
        ? events[Math.min(currentIndex - 1, events.length - 1)].offset
        : 0;
    const totalTime = events.length > 0 ? events[events.length - 1].offset : 0;

    const formatTime = (seconds: number) => {
        const m = Math.floor(seconds / 60);
        const s = Math.floor(seconds % 60);
        return `${m}:${s.toString().padStart(2, '0')}`;
    };

    if (error) {
        return (
            <div style={{
                display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center',
                height: '100vh', background: '#0f1117', color: 'var(--text)', gap: '1rem',
            }}>
                <div style={{ fontSize: '3rem' }}>🎬</div>
                <h2 style={{ margin: 0 }}>Recording Player</h2>
                <p style={{ color: 'var(--danger)', margin: 0 }}>{error}</p>
                <div style={{ display: 'flex', gap: '0.75rem', marginTop: '0.5rem' }}>
                    <button onClick={() => navigate('/recordings')} className="btn btn--primary btn--sm">
                        View All Recordings
                    </button>
                    <button onClick={() => navigate(-1)} className="btn btn--outline btn--sm">
                        ← Back
                    </button>
                </div>
            </div>
        );
    }

    return (
        <div style={{ display: 'flex', flexDirection: 'column', height: '100vh', background: '#0f1117' }}>
            {/* Header */}
            <div style={{
                display: 'flex', alignItems: 'center', gap: '1rem',
                padding: '0.75rem 1.5rem', flexShrink: 0,
                background: 'var(--bg)', borderBottom: '1px solid var(--border)',
            }}>
                <button
                    onClick={() => navigate(-1)}
                    className="btn btn--outline btn--sm"
                >
                    ← Back
                </button>
                <h2 style={{ margin: 0, color: 'var(--text)', fontSize: '1rem', fontWeight: 600 }}>
                    🎬 Recording Playback
                </h2>
                {recording && (
                    <span style={{ color: 'var(--text-muted)', fontSize: '0.8rem' }}>
                        {recording.filename} · {formatTime(totalTime)} · {(recording.size_bytes / 1024).toFixed(1)}KB
                    </span>
                )}
                {header && (
                    <span style={{ color: 'var(--text-muted)', fontSize: '0.75rem', marginLeft: 'auto' }}>
                        {header.width}×{header.height}
                        {header.title && ` · ${header.title}`}
                    </span>
                )}
            </div>

            {/* Terminal (xterm.js) — fills available space */}
            <div style={{
                flex: 1, overflow: 'hidden',
                padding: '0.5rem',
            }}>
                {loading ? (
                    <div style={{ color: 'var(--text-muted)', textAlign: 'center', paddingTop: '3rem' }}>
                        Loading recording...
                    </div>
                ) : (
                    <div
                        ref={terminalRef}
                        style={{
                            width: '100%',
                            height: '100%',
                            borderRadius: '8px',
                            overflow: 'hidden',
                        }}
                    />
                )}
            </div>

            {/* Controls */}
            <div style={{
                display: 'flex', alignItems: 'center', gap: '1rem',
                padding: '0.75rem 1.5rem',
                background: 'var(--bg)', borderTop: '1px solid var(--border)',
                flexShrink: 0,
            }}>
                {/* Play/Pause */}
                <button
                    onClick={isPlaying ? handlePause : handlePlay}
                    disabled={loading || events.length === 0}
                    className={`btn ${isPlaying ? 'btn--outline' : 'btn--primary'} btn--sm`}
                    style={{ minWidth: '80px' }}
                >
                    {isPlaying ? '⏸ Pause' : '▶ Play'}
                </button>

                {/* Restart */}
                <button onClick={handleRestart} className="btn btn--outline btn--sm">
                    ⏮
                </button>

                {/* Timeline */}
                <input
                    type="range"
                    min="0"
                    max={events.length}
                    value={currentIndex}
                    onChange={handleSeek}
                    style={{ flex: 1, accentColor: 'var(--accent)' }}
                />

                {/* Time display */}
                <span style={{
                    color: 'var(--text-muted)', fontSize: '0.8rem',
                    minWidth: '100px', textAlign: 'center', fontFamily: 'monospace',
                }}>
                    {formatTime(currentTime)} / {formatTime(totalTime)}
                </span>

                {/* Speed */}
                <select
                    value={speed}
                    onChange={e => setSpeed(parseFloat(e.target.value))}
                    style={{
                        background: 'var(--bg-surface)',
                        border: '1px solid var(--border)',
                        borderRadius: '8px',
                        padding: '0.4rem 0.5rem',
                        color: 'var(--text)',
                        fontSize: '0.8rem',
                    }}
                >
                    <option value={0.25}>0.25x</option>
                    <option value={0.5}>0.5x</option>
                    <option value={1}>1x</option>
                    <option value={2}>2x</option>
                    <option value={4}>4x</option>
                    <option value={8}>8x</option>
                    <option value={16}>16x</option>
                </select>

                {/* Progress */}
                <span style={{ color: 'var(--text-muted)', fontSize: '0.75rem', minWidth: '40px' }}>
                    {progress.toFixed(0)}%
                </span>
            </div>
        </div>
    );
}
