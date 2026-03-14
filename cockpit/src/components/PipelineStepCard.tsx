/**
 * PipelineStepCard — Individual step card component for the pipeline builder
 * 
 * Displays step configuration including agent selection, input/output modes,
 * and step management (reorder, remove).
 */

import React from 'react';
import { useAgents, type AgentDefinition } from '../hooks/useAgents';

export interface PipelineStepData {
    id: string;
    agent_id: string;
    input_mode: 'none' | 'previous' | 'manual';
    output_mode: 'none' | 'pass' | 'collect';
}

interface PipelineStepCardProps {
    stepNumber: number;
    step: PipelineStepData;
    onChange: (step: PipelineStepData) => void;
    onMoveUp: () => void;
    onMoveDown: () => void;
    onRemove: () => void;
    isFirst: boolean;
    isLast: boolean;
    totalSteps: number;
}

const styles: Record<string, React.CSSProperties> = {
    card: {
        background: 'var(--bg-elevated)',
        borderRadius: '12px',
        border: '1px solid var(--border)',
        padding: '16px',
        position: 'relative',
    },
    cardHeader: {
        display: 'flex',
        alignItems: 'center',
        gap: '12px',
        marginBottom: '16px',
    },
    stepBadge: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: '28px',
        height: '28px',
        borderRadius: '50%',
        background: 'var(--accent)',
        color: 'white',
        fontSize: '14px',
        fontWeight: 600,
    },
    stepTitle: {
        fontSize: '16px',
        fontWeight: 600,
        color: 'var(--text-primary)',
    },
    formGroup: {
        marginBottom: '12px',
    },
    label: {
        display: 'block',
        fontSize: '13px',
        fontWeight: 500,
        color: 'var(--text-muted)',
        marginBottom: '6px',
    },
    select: {
        width: '100%',
        padding: '8px 12px',
        borderRadius: '6px',
        border: '1px solid var(--border)',
        background: 'var(--bg-primary)',
        color: 'var(--text-primary)',
        fontSize: '14px',
        outline: 'none',
        cursor: 'pointer',
    },
    toggleGroup: {
        display: 'flex',
        gap: '8px',
    },
    toggleButton: {
        flex: 1,
        padding: '6px 12px',
        borderRadius: '6px',
        border: '1px solid var(--border)',
        background: 'var(--bg-primary)',
        color: 'var(--text-secondary)',
        fontSize: '13px',
        cursor: 'pointer',
        transition: 'all 0.2s',
    },
    toggleButtonActive: {
        background: 'var(--accent)',
        borderColor: 'var(--accent)',
        color: 'white',
    },
    buttonRow: {
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '8px',
        marginTop: '16px',
    },
    iconButton: {
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: '32px',
        height: '32px',
        borderRadius: '6px',
        border: '1px solid var(--border)',
        background: 'var(--bg-primary)',
        color: 'var(--text-secondary)',
        cursor: 'pointer',
        transition: 'all 0.2s',
    },
    removeButton: {
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: '6px 12px',
        borderRadius: '6px',
        border: '1px solid var(--error)',
        background: 'transparent',
        color: 'var(--error)',
        fontSize: '13px',
        cursor: 'pointer',
    },
    connector: {
        position: 'absolute',
        left: '50%',
        bottom: '-20px',
        width: '2px',
        height: '20px',
        background: 'var(--border)',
    },
    connectorArrow: {
        position: 'absolute',
        left: '50%',
        bottom: '-28px',
        transform: 'translateX(-50%)',
        width: 0,
        height: 0,
        borderLeft: '6px solid transparent',
        borderRight: '6px solid transparent',
        borderTop: '8px solid var(--border)',
    },
};

/** Arrow up icon */
const ArrowUpIcon: React.FC = () => (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
        <path d="M18 15l-6-6-6 6" />
    </svg>
);

/** Arrow down icon */
const ArrowDownIcon: React.FC = () => (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
        <path d="M6 9l6 6 6-6" />
    </svg>
);

/** Trash icon */
const TrashIcon: React.FC = () => (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
        <polyline points="3 6 5 6 21 6" />
        <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
    </svg>
);

const PipelineStepCard: React.FC<PipelineStepCardProps> = ({
    stepNumber,
    step,
    onChange,
    onMoveUp,
    onMoveDown,
    onRemove,
    isFirst,
    isLast,
    totalSteps,
}) => {
    const { agents, isLoading: agentsLoading } = useAgents();

    const handleAgentChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
        onChange({ ...step, agent_id: e.target.value });
    };

    const handleInputModeChange = (mode: 'none' | 'previous' | 'manual') => {
        onChange({ ...step, input_mode: mode });
    };

    const handleOutputModeChange = (mode: 'none' | 'pass' | 'collect') => {
        onChange({ ...step, output_mode: mode });
    };

    const handleRemove = () => {
        if (totalSteps > 2) {
            if (confirm('Are you sure you want to remove this step?')) {
                onRemove();
            }
        } else {
            onRemove();
        }
    };

    return (
        <div style={styles.card}>
            {/* Step header with number badge */}
            <div style={styles.cardHeader}>
                <div style={styles.stepBadge}>{stepNumber}</div>
                <span style={styles.stepTitle}>Step {stepNumber}</span>
            </div>

            {/* Agent selector */}
            <div style={styles.formGroup}>
                <label style={styles.label}>Agent</label>
                <select
                    id={`step-${stepNumber}-agent`}
                    value={step.agent_id}
                    onChange={handleAgentChange}
                    style={styles.select}
                    disabled={agentsLoading}
                >
                    <option value="">Select an agent...</option>
                    {agents.map((agent: AgentDefinition) => (
                        <option key={agent.id} value={agent.id}>
                            {agent.display_name || agent.name}
                        </option>
                    ))}
                </select>
            </div>

            {/* Input mode toggle */}
            <div style={styles.formGroup}>
                <label style={styles.label}>Input Mode</label>
                <div style={styles.toggleGroup}>
                    <button
                        id={`step-${stepNumber}-input-none`}
                        type="button"
                        style={{
                            ...styles.toggleButton,
                            ...(step.input_mode === 'none' ? styles.toggleButtonActive : {}),
                        }}
                        onClick={() => handleInputModeChange('none')}
                    >
                        No Input
                    </button>
                    <button
                        id={`step-${stepNumber}-input-previous`}
                        type="button"
                        style={{
                            ...styles.toggleButton,
                            ...(step.input_mode === 'previous' ? styles.toggleButtonActive : {}),
                        }}
                        onClick={() => handleInputModeChange('previous')}
                    >
                        Previous Output
                    </button>
                    <button
                        id={`step-${stepNumber}-input-manual`}
                        type="button"
                        style={{
                            ...styles.toggleButton,
                            ...(step.input_mode === 'manual' ? styles.toggleButtonActive : {}),
                        }}
                        onClick={() => handleInputModeChange('manual')}
                    >
                        Manual
                    </button>
                </div>
            </div>

            {/* Output mode selector */}
            <div style={styles.formGroup}>
                <label style={styles.label}>Output Mode</label>
                <select
                    id={`step-${stepNumber}-output-mode`}
                    value={step.output_mode}
                    onChange={(e) => handleOutputModeChange(e.target.value as 'none' | 'pass' | 'collect')}
                    style={styles.select}
                >
                    <option value="pass">Pass to Next</option>
                    <option value="collect">Report</option>
                    <option value="none">Both</option>
                </select>
            </div>

            {/* Action buttons */}
            <div style={styles.buttonRow}>
                <button
                    id={`step-${stepNumber}-move-up`}
                    type="button"
                    style={{
                        ...styles.iconButton,
                        opacity: isFirst ? 0.5 : 1,
                        cursor: isFirst ? 'not-allowed' : 'pointer',
                    }}
                    onClick={onMoveUp}
                    disabled={isFirst}
                    title="Move up"
                >
                    <ArrowUpIcon />
                </button>
                <button
                    id={`step-${stepNumber}-move-down`}
                    type="button"
                    style={{
                        ...styles.iconButton,
                        opacity: isLast ? 0.5 : 1,
                        cursor: isLast ? 'not-allowed' : 'pointer',
                    }}
                    onClick={onMoveDown}
                    disabled={isLast}
                    title="Move down"
                >
                    <ArrowDownIcon />
                </button>
                <button
                    id={`step-${stepNumber}-remove`}
                    type="button"
                    style={styles.removeButton}
                    onClick={handleRemove}
                    title="Remove step"
                >
                    <TrashIcon />
                </button>
            </div>

            {/* Visual connector to next step */}
            {!isLast && (
                <>
                    <div style={styles.connector} />
                    <div style={styles.connectorArrow} />
                </>
            )}
        </div>
    );
};

export default PipelineStepCard;
