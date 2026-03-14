/**
 * GuardrailConfig — Guardrail settings panel for the agent builder wizard
 *
 * Controls: HITL toggle, Read-Only toggle, Timeout slider, Max Tool Calls, Max Turns.
 * Each control has a label and description. HITL shows EnterpriseBadge.
 * Outputs JSONB guardrails config via onChange callback.
 */

import React, { useCallback } from 'react';
import EnterpriseBadge from './EnterpriseBadge';

// ============================================================================
// Types
// ============================================================================

export interface GuardrailsConfig {
    hitl: boolean;
    read_only: boolean;
    timeout_minutes: number;
    max_tool_calls: number;
    max_turns: number;
}

interface GuardrailConfigProps {
    value: GuardrailsConfig;
    onChange: (config: GuardrailsConfig) => void;
    onReadOnlyChange?: (readOnly: boolean) => void;
}

// ============================================================================
// Defaults
// ============================================================================

export const DEFAULT_GUARDRAILS: GuardrailsConfig = {
    hitl: false,
    read_only: false,
    timeout_minutes: 15,
    max_tool_calls: 100,
    max_turns: 50,
};

// ============================================================================
// Component
// ============================================================================

const GuardrailConfig: React.FC<GuardrailConfigProps> = ({ value, onChange, onReadOnlyChange }) => {

    const update = useCallback(<K extends keyof GuardrailsConfig>(key: K, val: GuardrailsConfig[K]) => {
        const next = { ...value, [key]: val };
        onChange(next);
        if (key === 'read_only') {
            onReadOnlyChange?.(val as boolean);
        }
    }, [value, onChange, onReadOnlyChange]);

    const clamp = (v: number, min: number, max: number) => Math.min(max, Math.max(min, v));

    return (
        <div className="guardrail-config">
            {/* HITL Gate */}
            <div className="guardrail-config__control">
                <div className="guardrail-config__header">
                    <div className="guardrail-config__label-row">
                        <span className="guardrail-config__label">Human-in-the-Loop</span>
                        <EnterpriseBadge size="small" />
                    </div>
                    <span className="guardrail-config__desc">
                        Require human approval before executing tool calls
                    </span>
                </div>
                <button
                    type="button"
                    className={`toggle-switch ${value.hitl ? 'toggle-switch--on' : ''}`}
                    onClick={() => update('hitl', !value.hitl)}
                    role="switch"
                    aria-checked={value.hitl}
                    aria-label="Human-in-the-Loop toggle"
                >
                    <span className="toggle-switch__track">
                        <span className="toggle-switch__thumb" />
                    </span>
                </button>
            </div>

            {/* Read-Only Mode */}
            <div className="guardrail-config__control">
                <div className="guardrail-config__header">
                    <span className="guardrail-config__label">Read-Only Mode</span>
                    <span className="guardrail-config__desc">
                        Restrict the agent to read-only operations — disables write, edit, and patch tools
                    </span>
                </div>
                <button
                    type="button"
                    className={`toggle-switch ${value.read_only ? 'toggle-switch--on' : ''}`}
                    onClick={() => update('read_only', !value.read_only)}
                    role="switch"
                    aria-checked={value.read_only}
                    aria-label="Read-Only Mode toggle"
                >
                    <span className="toggle-switch__track">
                        <span className="toggle-switch__thumb" />
                    </span>
                </button>
            </div>

            {/* Timeout */}
            <div className="guardrail-config__control guardrail-config__control--slider">
                <div className="guardrail-config__header">
                    <div className="guardrail-config__label-row">
                        <span className="guardrail-config__label">Timeout</span>
                        <span className="guardrail-config__value-badge">{value.timeout_minutes} min</span>
                    </div>
                    <span className="guardrail-config__desc">
                        Maximum time the agent is allowed to run before being terminated
                    </span>
                </div>
                <input
                    type="range"
                    className="guardrail-config__slider"
                    min={1}
                    max={120}
                    step={1}
                    value={value.timeout_minutes}
                    onChange={(e) => update('timeout_minutes', parseInt(e.target.value, 10))}
                    aria-label="Timeout minutes"
                />
                <div className="guardrail-config__slider-labels">
                    <span>1 min</span>
                    <span>120 min</span>
                </div>
            </div>

            {/* Max Tool Calls */}
            <div className="guardrail-config__control">
                <div className="guardrail-config__header">
                    <span className="guardrail-config__label">Max Tool Calls</span>
                    <span className="guardrail-config__desc">
                        Maximum number of tool invocations allowed per run
                    </span>
                </div>
                <input
                    type="number"
                    className="guardrail-config__number"
                    min={1}
                    max={1000}
                    value={value.max_tool_calls}
                    onChange={(e) => update('max_tool_calls', clamp(parseInt(e.target.value, 10) || 1, 1, 1000))}
                    aria-label="Max tool calls"
                />
            </div>

            {/* Max Turns */}
            <div className="guardrail-config__control">
                <div className="guardrail-config__header">
                    <span className="guardrail-config__label">Max Turns</span>
                    <span className="guardrail-config__desc">
                        Maximum number of conversation turns before the agent must stop
                    </span>
                </div>
                <input
                    type="number"
                    className="guardrail-config__number"
                    min={1}
                    max={500}
                    value={value.max_turns}
                    onChange={(e) => update('max_turns', clamp(parseInt(e.target.value, 10) || 1, 1, 500))}
                    aria-label="Max turns"
                />
            </div>
        </div>
    );
};

export default GuardrailConfig;
