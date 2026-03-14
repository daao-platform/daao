import React, { useState, useEffect, useRef } from 'react';
import { getSessionPreview } from '../api/client';

interface TerminalPreviewProps {
    sessionId: string;
    /** Refresh interval in ms (default: 10000) */
    refreshInterval?: number;
}

/**
 * TerminalPreview — Renders a mini terminal preview thumbnail for a session card.
 * Fetches the last ~12 lines of terminal output from the ring buffer API
 * and displays them in a dark monospace container.
 */
const TerminalPreview: React.FC<TerminalPreviewProps> = ({
    sessionId,
    refreshInterval = 10000,
}) => {
    const [text, setText] = useState<string>('');
    const [hasContent, setHasContent] = useState(false);
    const mountedRef = useRef(true);

    useEffect(() => {
        mountedRef.current = true;

        const fetchPreview = async () => {
            try {
                const res = await getSessionPreview(sessionId);
                if (mountedRef.current) {
                    setText(res.text || '');
                    setHasContent(res.has_content);
                }
            } catch {
                // Silently ignore — preview is non-critical
            }
        };

        fetchPreview();
        const interval = setInterval(fetchPreview, refreshInterval);

        return () => {
            mountedRef.current = false;
            clearInterval(interval);
        };
    }, [sessionId, refreshInterval]);

    if (!hasContent) {
        return (
            <div className="terminal-preview terminal-preview--empty">
                <span className="terminal-preview__dots">
                    <span />
                    <span />
                    <span />
                </span>
                <div className="terminal-preview__placeholder">
                    Awaiting output…
                </div>
            </div>
        );
    }

    return (
        <div className="terminal-preview">
            <span className="terminal-preview__dots">
                <span />
                <span />
                <span />
            </span>
            <pre className="terminal-preview__text">{text}</pre>
        </div>
    );
};

export default TerminalPreview;
