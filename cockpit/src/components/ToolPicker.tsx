/**
 * ToolPicker — Two-column allow/deny tool configuration
 *
 * Features:
 * - Available tools (left) / Allowed tools (right) columns
 * - Click-to-transfer between columns
 * - Description tooltips per tool
 * - "Read-only mode" toggle that auto-denies write tools
 * - Outputs JSONB: { allow: string[], deny: string[] }
 */

import React, { useState, useCallback, useEffect, useMemo } from 'react';

// ============================================================================
// Types
// ============================================================================

export interface ToolsConfig {
    allow: string[];
    deny: string[];
}

interface ToolPickerProps {
    value: ToolsConfig;
    onChange: (config: ToolsConfig) => void;
    readOnly?: boolean;
    onReadOnlyChange?: (readOnly: boolean) => void;
}

interface ToolInfo {
    id: string;
    label: string;
    description: string;
    isWrite: boolean;
}

// ============================================================================
// Constants
// ============================================================================

const TOOLS: ToolInfo[] = [
    { id: 'exec', label: 'Execute', description: 'Run shell commands and scripts on the satellite', isWrite: false },
    { id: 'read', label: 'Read', description: 'Read files, logs, and system information', isWrite: false },
    { id: 'write', label: 'Write', description: 'Create and write to files on the satellite', isWrite: true },
    { id: 'process', label: 'Process', description: 'Manage system processes (start, stop, restart)', isWrite: false },
    { id: 'apply_patch', label: 'Apply Patch', description: 'Apply diff patches to existing files', isWrite: true },
    { id: 'edit', label: 'Edit', description: 'Edit existing file contents in-place', isWrite: true },
    { id: 'web_search', label: 'Web Search', description: 'Search the web for documentation and solutions', isWrite: false },
];

const WRITE_TOOL_IDS = TOOLS.filter((t) => t.isWrite).map((t) => t.id);

// ============================================================================
// Sub-components
// ============================================================================

const ToolItem: React.FC<{
    tool: ToolInfo;
    isAllowed: boolean;
    isDenied: boolean;
    onClick: () => void;
}> = ({ tool, isAllowed, isDenied, onClick }) => (
    <button
        type="button"
        className={`tool-picker__item ${isAllowed ? 'tool-picker__item--allowed' : ''} ${isDenied ? 'tool-picker__item--denied' : ''}`}
        onClick={onClick}
        title={tool.description}
        disabled={isDenied}
    >
        <span className="tool-picker__item-name">{tool.label}</span>
        <span className="tool-picker__item-id">{tool.id}</span>
        {tool.isWrite && <span className="tool-picker__item-badge">write</span>}
    </button>
);

// ============================================================================
// Component
// ============================================================================

const ToolPicker: React.FC<ToolPickerProps> = ({ value, onChange, readOnly = false, onReadOnlyChange }) => {
    const [isReadOnly, setIsReadOnly] = useState(readOnly);

    // Sync with external readOnly prop
    useEffect(() => {
        setIsReadOnly(readOnly);
    }, [readOnly]);

    // Handle read-only toggle
    const handleReadOnlyToggle = useCallback(() => {
        const next = !isReadOnly;
        setIsReadOnly(next);
        onReadOnlyChange?.(next);

        if (next) {
            // Auto-deny write tools
            const newAllow = value.allow.filter((t) => !WRITE_TOOL_IDS.includes(t));
            const newDeny = [...new Set([...value.deny, ...WRITE_TOOL_IDS])];
            onChange({ allow: newAllow, deny: newDeny });
        } else {
            // Remove auto-denied write tools from deny list
            const newDeny = value.deny.filter((t) => !WRITE_TOOL_IDS.includes(t));
            onChange({ allow: value.allow, deny: newDeny });
        }
    }, [isReadOnly, value, onChange, onReadOnlyChange]);

    // Toggle a tool between available/allowed
    const toggleTool = useCallback((toolId: string) => {
        if (isReadOnly && WRITE_TOOL_IDS.includes(toolId)) return;

        if (value.allow.includes(toolId)) {
            // Remove from allow
            onChange({
                allow: value.allow.filter((t) => t !== toolId),
                deny: value.deny,
            });
        } else {
            // Add to allow, remove from deny if present
            onChange({
                allow: [...value.allow, toolId],
                deny: value.deny.filter((t) => t !== toolId),
            });
        }
    }, [value, onChange, isReadOnly]);

    // Split tools into available and allowed
    const availableTools = useMemo(
        () => TOOLS.filter((t) => !value.allow.includes(t.id)),
        [value.allow]
    );
    const allowedTools = useMemo(
        () => TOOLS.filter((t) => value.allow.includes(t.id)),
        [value.allow]
    );

    return (
        <div className="tool-picker">
            {/* Read-only toggle */}
            <div className="tool-picker__toggle-row">
                <label className="tool-picker__toggle-label">
                    <span className="tool-picker__toggle-text">Read-only mode</span>
                    <span className="tool-picker__toggle-desc">Deny all write, edit, and patch tools</span>
                </label>
                <button
                    type="button"
                    className={`toggle-switch ${isReadOnly ? 'toggle-switch--on' : ''}`}
                    onClick={handleReadOnlyToggle}
                    role="switch"
                    aria-checked={isReadOnly}
                >
                    <span className="toggle-switch__track">
                        <span className="toggle-switch__thumb" />
                    </span>
                </button>
            </div>

            {/* Two-column layout */}
            <div className="tool-picker__columns">
                {/* Available */}
                <div className="tool-picker__column">
                    <div className="tool-picker__column-header">
                        <span className="tool-picker__column-title">Available Tools</span>
                        <span className="tool-picker__column-count">{availableTools.length}</span>
                    </div>
                    <div className="tool-picker__list">
                        {availableTools.map((tool) => (
                            <ToolItem
                                key={tool.id}
                                tool={tool}
                                isAllowed={false}
                                isDenied={isReadOnly && tool.isWrite}
                                onClick={() => toggleTool(tool.id)}
                            />
                        ))}
                        {availableTools.length === 0 && (
                            <div className="tool-picker__empty">All tools allowed</div>
                        )}
                    </div>
                </div>

                {/* Transfer indicator */}
                <div className="tool-picker__divider">
                    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <polyline points="7 7 12 12 7 17" />
                        <polyline points="13 7 18 12 13 17" />
                    </svg>
                </div>

                {/* Allowed */}
                <div className="tool-picker__column">
                    <div className="tool-picker__column-header">
                        <span className="tool-picker__column-title">Allowed Tools</span>
                        <span className="tool-picker__column-count">{allowedTools.length}</span>
                    </div>
                    <div className="tool-picker__list">
                        {allowedTools.map((tool) => (
                            <ToolItem
                                key={tool.id}
                                tool={tool}
                                isAllowed={true}
                                isDenied={false}
                                onClick={() => toggleTool(tool.id)}
                            />
                        ))}
                        {allowedTools.length === 0 && (
                            <div className="tool-picker__empty">Click tools to allow</div>
                        )}
                    </div>
                </div>
            </div>
        </div>
    );
};

export default ToolPicker;
