/**
 * SystemPromptEditor — Rich textarea for agent system prompts
 *
 * Features:
 * - Monospace textarea with variable highlighting overlay
 * - Template library dropdown (Sysadmin, Log Analyzer, Security Scanner, Custom)
 * - Character count display
 * - Preview panel with highlighted template variables
 */

import React, { useState, useMemo, useCallback, useRef, useEffect } from 'react';

// ============================================================================
// Types
// ============================================================================

interface SystemPromptEditorProps {
    value: string;
    onChange: (value: string) => void;
}

// ============================================================================
// Constants
// ============================================================================

const TEMPLATE_VARIABLES = [
    '{satellite_name}',
    '{systeminfo}',
    '{runbooks}',
    '{hostname}',
    '{os}',
    '{timestamp}',
];

const PROMPT_TEMPLATES: Record<string, string> = {
    sysadmin: `You are a senior systems administrator managing the satellite "{satellite_name}".

System Information:
{systeminfo}

Available Runbooks:
{runbooks}

Your responsibilities:
1. Monitor system health and respond to alerts
2. Execute approved maintenance procedures from runbooks
3. Diagnose issues using available tools (read logs, check processes, inspect configs)
4. Report findings clearly with severity levels

Rules:
- Always check system state before making changes
- Follow runbook procedures exactly — do not improvise
- Escalate if a situation is outside your runbooks
- Log all actions taken`,

    'log-analyzer': `You are a log analysis specialist for "{satellite_name}".

System Context:
{systeminfo}

Your task is to analyze logs and identify:
1. Error patterns and their root causes
2. Performance anomalies and degradation trends
3. Security-relevant events (failed logins, privilege escalation, unusual access patterns)
4. Correlation between events across different log sources

Output format:
- Severity: CRITICAL / WARNING / INFO
- Summary: One-line description
- Details: Full analysis with timestamps
- Recommendation: Suggested action

Rules:
- Process logs chronologically
- Highlight timestamps in UTC
- Group related events together
- Do not modify any files — read-only analysis only`,

    'security-scanner': `You are a security assessment agent for "{satellite_name}".

System Information:
{systeminfo}

Perform the following checks:
1. Open ports and services — identify unnecessary exposure
2. User accounts — check for default credentials, inactive accounts
3. File permissions — world-readable sensitive files
4. Package versions — known CVEs in installed software
5. Configuration audit — SSH, firewall, logging settings
6. Process audit — unexpected or suspicious processes

Output each finding as:
- Finding ID: SEC-XXX
- Severity: CRITICAL / HIGH / MEDIUM / LOW
- Description: What was found
- Evidence: Command output / file content
- Remediation: How to fix

Rules:
- Read-only assessment — do not modify the system
- Do not attempt to exploit vulnerabilities
- Report findings even if uncertain — flag confidence level`,

    custom: '',
};

// ============================================================================
// Helpers
// ============================================================================

const VARIABLE_REGEX = /\{[a-z_]+\}/g;

function highlightVariables(text: string): React.ReactNode[] {
    const parts: React.ReactNode[] = [];
    let lastIndex = 0;
    let match: RegExpExecArray | null;
    const regex = new RegExp(VARIABLE_REGEX);

    while ((match = regex.exec(text)) !== null) {
        if (match.index > lastIndex) {
            parts.push(text.slice(lastIndex, match.index));
        }
        parts.push(
            <span key={match.index} className="spe-variable">{match[0]}</span>
        );
        lastIndex = regex.lastIndex;
    }
    if (lastIndex < text.length) {
        parts.push(text.slice(lastIndex));
    }
    return parts;
}

// ============================================================================
// Component
// ============================================================================

const SystemPromptEditor: React.FC<SystemPromptEditorProps> = ({ value, onChange }) => {
    const [selectedTemplate, setSelectedTemplate] = useState('custom');
    const [showPreview, setShowPreview] = useState(false);
    const textareaRef = useRef<HTMLTextAreaElement>(null);
    const overlayRef = useRef<HTMLDivElement>(null);

    // Sync scroll between textarea and overlay
    const handleScroll = useCallback(() => {
        if (textareaRef.current && overlayRef.current) {
            overlayRef.current.scrollTop = textareaRef.current.scrollTop;
            overlayRef.current.scrollLeft = textareaRef.current.scrollLeft;
        }
    }, []);

    // Handle template selection
    const handleTemplateChange = useCallback((e: React.ChangeEvent<HTMLSelectElement>) => {
        const key = e.target.value;
        setSelectedTemplate(key);
        if (key !== 'custom' && PROMPT_TEMPLATES[key]) {
            onChange(PROMPT_TEMPLATES[key]);
        }
    }, [onChange]);

    // Insert variable at cursor
    const insertVariable = useCallback((variable: string) => {
        const textarea = textareaRef.current;
        if (!textarea) return;
        const start = textarea.selectionStart;
        const end = textarea.selectionEnd;
        const newValue = value.slice(0, start) + variable + value.slice(end);
        onChange(newValue);
        // Restore cursor position after React re-render
        requestAnimationFrame(() => {
            textarea.selectionStart = textarea.selectionEnd = start + variable.length;
            textarea.focus();
        });
    }, [value, onChange]);

    // Highlighted overlay content
    const overlayContent = useMemo(() => highlightVariables(value), [value]);

    // Character count
    const charCount = value.length;

    return (
        <div className="spe">
            {/* Toolbar */}
            <div className="spe-toolbar">
                <div className="spe-toolbar__left">
                    <label className="spe-toolbar__label">Template</label>
                    <select
                        className="spe-toolbar__select"
                        value={selectedTemplate}
                        onChange={handleTemplateChange}
                    >
                        <option value="custom">Custom</option>
                        <option value="sysadmin">Sysadmin</option>
                        <option value="log-analyzer">Log Analyzer</option>
                        <option value="security-scanner">Security Scanner</option>
                    </select>
                </div>
                <div className="spe-toolbar__right">
                    <div className="spe-toolbar__variables">
                        {TEMPLATE_VARIABLES.map((v) => (
                            <button
                                key={v}
                                type="button"
                                className="spe-toolbar__var-btn"
                                onClick={() => insertVariable(v)}
                                title={`Insert ${v}`}
                            >
                                {v}
                            </button>
                        ))}
                    </div>
                    <button
                        type="button"
                        className={`spe-toolbar__preview-btn ${showPreview ? 'spe-toolbar__preview-btn--active' : ''}`}
                        onClick={() => setShowPreview(!showPreview)}
                    >
                        {showPreview ? 'Editor' : 'Preview'}
                    </button>
                </div>
            </div>

            {/* Editor / Preview */}
            {showPreview ? (
                <div className="spe-preview">
                    <pre className="spe-preview__content">{highlightVariables(value)}</pre>
                </div>
            ) : (
                <div className="spe-editor">
                    <div className="spe-editor__container">
                        {/* Highlight overlay */}
                        <div
                            ref={overlayRef}
                            className="spe-editor__overlay"
                            aria-hidden="true"
                        >
                            <pre>{overlayContent}</pre>
                        </div>
                        {/* Actual textarea */}
                        <textarea
                            ref={textareaRef}
                            className="spe-editor__textarea"
                            value={value}
                            onChange={(e) => {
                                onChange(e.target.value);
                                if (selectedTemplate !== 'custom') setSelectedTemplate('custom');
                            }}
                            onScroll={handleScroll}
                            placeholder="Enter your system prompt..."
                            spellCheck={false}
                        />
                    </div>
                </div>
            )}

            {/* Footer */}
            <div className="spe-footer">
                <span className="spe-footer__count">{charCount.toLocaleString()} characters</span>
            </div>
        </div>
    );
};

export default SystemPromptEditor;
