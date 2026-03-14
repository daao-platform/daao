/**
 * PipelineBuilder — Visual pipeline composer page
 * 
 * Allows operators to create/edit pipelines with a visual step composer.
 * Features:
 * - Pipeline metadata form (name, description, satellite, failure strategy)
 * - Ordered step list with drag/reorder support
 * - Add/remove step functionality
 * - Optional cron schedule configuration
 * - Enterprise feature gate for session_chaining
 * - Demo template for Security Audit Pipeline
 */

import React, { useState, useEffect, useCallback } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import PipelineStepCard, { type PipelineStepData } from '../components/PipelineStepCard';
import { useCreatePipeline, useUpdatePipeline, usePipeline } from '../hooks/usePipelines';
import { useLicense } from '../hooks/useLicense';
import { getSatellites, apiRequest } from '../api/client';
import type { Satellite } from '../api/client';

// ============================================================================
// Types
// ============================================================================

interface PipelineFormData {
    name: string;
    description: string;
    satellite_id: string;
    on_failure: 'abort' | 'continue' | 'retry';
    max_retries: number;
    schedule: string;
}

// Demo template: Security Audit Pipeline
const DEMO_TEMPLATE_STEPS: PipelineStepData[] = [
    {
        id: 'step-1',
        agent_id: '',
        input_mode: 'none',
        output_mode: 'pass',
    },
    {
        id: 'step-2',
        agent_id: '',
        input_mode: 'previous',
        output_mode: 'pass',
    },
    {
        id: 'step-3',
        agent_id: '',
        input_mode: 'previous',
        output_mode: 'collect',
    },
];

// ============================================================================
// Styles
// ============================================================================

const styles: Record<string, React.CSSProperties> = {
    container: {
        padding: '24px',
        maxWidth: '900px',
        margin: '0 auto',
    },
    header: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'flex-start',
        marginBottom: '24px',
        flexWrap: 'wrap',
        gap: '16px',
    },
    headerLeft: {
        display: 'flex',
        alignItems: 'center',
        gap: '12px',
    },
    title: {
        fontSize: '28px',
        fontWeight: 600,
        margin: 0,
        color: 'var(--text-primary)',
    },
    headerActions: {
        display: 'flex',
        gap: '12px',
    },
    section: {
        background: 'var(--bg-elevated)',
        borderRadius: '12px',
        border: '1px solid var(--border)',
        padding: '20px',
        marginBottom: '20px',
    },
    sectionTitle: {
        fontSize: '18px',
        fontWeight: 600,
        color: 'var(--text-primary)',
        marginBottom: '16px',
    },
    formGrid: {
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fit, minmax(250px, 1fr))',
        gap: '16px',
    },
    formGroup: {
        display: 'flex',
        flexDirection: 'column',
    },
    label: {
        fontSize: '13px',
        fontWeight: 500,
        color: 'var(--text-muted)',
        marginBottom: '6px',
    },
    input: {
        padding: '10px 12px',
        borderRadius: '6px',
        border: '1px solid var(--border)',
        background: 'var(--bg-primary)',
        color: 'var(--text-primary)',
        fontSize: '14px',
        outline: 'none',
    },
    textarea: {
        padding: '10px 12px',
        borderRadius: '6px',
        border: '1px solid var(--border)',
        background: 'var(--bg-primary)',
        color: 'var(--text-primary)',
        fontSize: '14px',
        outline: 'none',
        minHeight: '80px',
        resize: 'vertical',
    },
    select: {
        padding: '10px 12px',
        borderRadius: '6px',
        border: '1px solid var(--border)',
        background: 'var(--bg-primary)',
        color: 'var(--text-primary)',
        fontSize: '14px',
        outline: 'none',
        cursor: 'pointer',
    },
    stepsList: {
        display: 'flex',
        flexDirection: 'column',
        gap: '24px',
    },
    addStepButton: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        gap: '8px',
        padding: '12px',
        borderRadius: '8px',
        border: '2px dashed var(--border)',
        background: 'transparent',
        color: 'var(--text-muted)',
        fontSize: '14px',
        cursor: 'pointer',
        transition: 'all 0.2s',
    },
    scheduleHelp: {
        fontSize: '12px',
        color: 'var(--text-muted)',
        marginTop: '4px',
    },
    buttonRow: {
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '12px',
        marginTop: '24px',
    },
    lockedOverlay: {
        position: 'fixed',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        background: 'rgba(0, 0, 0, 0.7)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 1000,
    },
    lockedCard: {
        background: 'var(--bg-elevated)',
        borderRadius: '16px',
        padding: '40px',
        textAlign: 'center',
        maxWidth: '400px',
    },
    lockedTitle: {
        fontSize: '20px',
        fontWeight: 600,
        color: 'var(--text-primary)',
        marginBottom: '12px',
    },
    lockedDesc: {
        fontSize: '14px',
        color: 'var(--text-muted)',
        marginBottom: '24px',
    },
};

// ============================================================================
// Icons
// ============================================================================

/** Plus icon */
const PlusIcon: React.FC<{ size?: number }> = ({ size = 16 }) => (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
        <line x1="12" y1="5" x2="12" y2="19" />
        <line x1="5" y1="12" x2="19" y2="12" />
    </svg>
);

/** Lock icon */
const LockIcon: React.FC<{ size?: number }> = ({ size = 48 }) => (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
        <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
        <path d="M7 11V7a5 5 0 0 1 10 0v4" />
    </svg>
);

/** Pipeline icon */
const PipelineIcon: React.FC<{ size?: number }> = ({ size = 28 }) => (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
        <circle cx="12" cy="5" r="3" />
        <line x1="12" y1="8" x2="12" y2="12" />
        <circle cx="12" cy="16" r="3" />
        <line x1="12" y1="19" x2="12" y2="21" />
    </svg>
);

// ============================================================================
// Cron Helper
// ============================================================================

const CRON_EXAMPLES = [
    { label: 'Every hour', value: '0 * * * *' },
    { label: 'Every day at midnight', value: '0 0 * * *' },
    { label: 'Every Monday at 9am', value: '0 9 * * 1' },
    { label: 'Every week on Sunday', value: '0 0 * * 0' },
];

// ============================================================================
// Component
// ============================================================================

const PipelineBuilder: React.FC = () => {
    const navigate = useNavigate();
    const { id: pipelineId } = useParams<{ id?: string }>();
    const isEditMode = !!pipelineId;

    // License check for enterprise feature
    const { hasFeature, isEnterprise } = useLicense();
    const hasSessionChaining = hasFeature('session_chaining');

    // Form state
    const [formData, setFormData] = useState<PipelineFormData>({
        name: '',
        description: '',
        satellite_id: '',
        on_failure: 'abort',
        max_retries: 0,
        schedule: '',
    });

    // Steps state
    const [steps, setSteps] = useState<PipelineStepData[]>([
        { id: 'step-1', agent_id: '', input_mode: 'none', output_mode: 'pass' },
    ]);

    // Loading/saving state
    const [isSaving, setIsSaving] = useState(false);
    const [satellites, setSatellites] = useState<Satellite[]>([]);
    const [isLoadingSatellites, setIsLoadingSatellites] = useState(true);

    // API hooks
    const { createPipeline } = useCreatePipeline();
    const { updatePipeline } = useUpdatePipeline();
    const { pipeline: existingPipeline, isLoading: isLoadingPipeline } = usePipeline(pipelineId || '');

    // Fetch satellites on mount
    useEffect(() => {
        getSatellites()
            .then(setSatellites)
            .catch((err) => console.error('Failed to load satellites:', err))
            .finally(() => setIsLoadingSatellites(false));
    }, []);

    // Load existing pipeline in edit mode
    useEffect(() => {
        if (isEditMode && existingPipeline) {
            setFormData({
                name: existingPipeline.name,
                description: existingPipeline.description || '',
                satellite_id: existingPipeline.satellite_id,
                on_failure: existingPipeline.on_failure,
                max_retries: existingPipeline.max_retries,
                schedule: existingPipeline.schedule || '',
            });
            setSteps(
                existingPipeline.steps.map((s, idx) => ({
                    id: `step-${idx + 1}`,
                    agent_id: s.agent_id,
                    input_mode: s.input_mode,
                    output_mode: s.output_mode,
                }))
            );
        }
    }, [isEditMode, existingPipeline]);

    // Generate unique step ID
    const generateStepId = () => {
        return `step-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
    };

    // Handle form field changes
    const handleFieldChange = (field: keyof PipelineFormData, value: string | number) => {
        setFormData((prev) => ({ ...prev, [field]: value }));
    };

    // Handle step changes
    const handleStepChange = (index: number, updatedStep: PipelineStepData) => {
        setSteps((prev) => {
            const newSteps = [...prev];
            newSteps[index] = updatedStep;
            return newSteps;
        });
    };

    // Move step up
    const handleMoveStepUp = (index: number) => {
        if (index === 0) return;
        setSteps((prev) => {
            const newSteps = [...prev];
            [newSteps[index - 1], newSteps[index]] = [newSteps[index], newSteps[index - 1]];
            return newSteps;
        });
    };

    // Move step down
    const handleMoveStepDown = (index: number) => {
        if (index === steps.length - 1) return;
        setSteps((prev) => {
            const newSteps = [...prev];
            [newSteps[index], newSteps[index + 1]] = [newSteps[index + 1], newSteps[index]];
            return newSteps;
        });
    };

    // Remove step
    const handleRemoveStep = (index: number) => {
        setSteps((prev) => prev.filter((_, i) => i !== index));
    };

    // Add new step
    const handleAddStep = () => {
        setSteps((prev) => [
            ...prev,
            { id: generateStepId(), agent_id: '', input_mode: 'none', output_mode: 'pass' },
        ]);
    };

    // Load demo template
    const handleLoadDemoTemplate = () => {
        setFormData((prev) => ({
            ...prev,
            name: 'Security Audit Pipeline',
            description: 'Automated security scanning pipeline with three agents: Security Scanner, Log Analyzer, and Virtual Sysadmin',
        }));
        setSteps(DEMO_TEMPLATE_STEPS.map((s, idx) => ({ ...s, id: `step-${idx + 1}` })));
    };

    // Handle save
    const handleSave = async () => {
        if (!formData.name.trim()) {
            alert('Please enter a pipeline name');
            return;
        }
        if (!formData.satellite_id) {
            alert('Please select a satellite');
            return;
        }
        if (steps.length === 0) {
            alert('Please add at least one step');
            return;
        }

        setIsSaving(true);
        try {
            const pipelineData = {
                name: formData.name,
                description: formData.description,
                satellite_id: formData.satellite_id,
                on_failure: formData.on_failure,
                max_retries: formData.on_failure === 'retry' ? formData.max_retries : 0,
                schedule: formData.schedule || undefined,
                steps: steps.map((s, idx) => ({
                    step_order: idx + 1,
                    agent_id: s.agent_id,
                    input_mode: s.input_mode,
                    output_mode: s.output_mode,
                    config: {},
                })),
            };

            if (isEditMode && pipelineId) {
                await updatePipeline(pipelineId, pipelineData);
            } else {
                await createPipeline(pipelineData);
            }

            navigate('/pipelines');
        } catch (err) {
            console.error('Failed to save pipeline:', err);
            alert('Failed to save pipeline. Please try again.');
        } finally {
            setIsSaving(false);
        }
    };

    // Handle cancel
    const handleCancel = () => {
        navigate('/pipelines');
    };

    // Show locked overlay for non-enterprise users
    if (!isEnterprise || !hasSessionChaining) {
        return (
            <div style={styles.lockedOverlay}>
                <div style={styles.lockedCard}>
                    <div style={{ color: 'var(--accent)', marginBottom: '16px' }}>
                        <LockIcon />
                    </div>
                    <h2 style={styles.lockedTitle}>Coming Soon</h2>
                    <p style={styles.lockedDesc}>
                        Pipeline chaining is planned for a future DAAO Enterprise release. Stay tuned.
                    </p>
                    <button className="btn btn--primary" onClick={() => navigate('/settings')}>
                        View Plans
                    </button>
                </div>
            </div>
        );
    }

    return (
        <div style={styles.container}>
            {/* Page Header */}
            <div id="pipeline-builder-header" style={styles.header}>
                <div style={styles.headerLeft}>
                    <span style={{ color: 'var(--accent)' }}>
                        <PipelineIcon />
                    </span>
                    <h1 style={styles.title}>
                        {isEditMode ? 'Edit Pipeline' : 'Create Pipeline'}
                    </h1>
                </div>
                <div style={styles.headerActions}>
                    <button
                        id="load-demo-template"
                        className="btn btn--outline btn--sm"
                        onClick={handleLoadDemoTemplate}
                    >
                        Load Demo Template
                    </button>
                </div>
            </div>

            {/* Pipeline Metadata Section */}
            <div id="pipeline-metadata-section" style={styles.section}>
                <h2 style={styles.sectionTitle}>Pipeline Settings</h2>
                <div style={styles.formGrid}>
                    <div style={styles.formGroup}>
                        <label style={styles.label} htmlFor="pipeline-name">
                            Name *
                        </label>
                        <input
                            id="pipeline-name"
                            type="text"
                            value={formData.name}
                            onChange={(e) => handleFieldChange('name', e.target.value)}
                            placeholder="My Pipeline"
                            style={styles.input}
                        />
                    </div>

                    <div style={styles.formGroup}>
                        <label style={styles.label} htmlFor="pipeline-satellite">
                            Satellite *
                        </label>
                        <select
                            id="pipeline-satellite"
                            value={formData.satellite_id}
                            onChange={(e) => handleFieldChange('satellite_id', e.target.value)}
                            style={styles.select}
                            disabled={isLoadingSatellites}
                        >
                            <option value="">Select a satellite...</option>
                            {satellites.map((sat) => (
                                <option key={sat.id} value={sat.id}>
                                    {sat.name}
                                </option>
                            ))}
                        </select>
                    </div>

                    <div style={styles.formGroup}>
                        <label style={styles.label} htmlFor="pipeline-on-failure">
                            On Failure
                        </label>
                        <select
                            id="pipeline-on-failure"
                            value={formData.on_failure}
                            onChange={(e) => handleFieldChange('on_failure', e.target.value)}
                            style={styles.select}
                        >
                            <option value="abort">Stop</option>
                            <option value="continue">Skip Remaining</option>
                            <option value="retry">Retry</option>
                        </select>
                    </div>

                    {formData.on_failure === 'retry' && (
                        <div style={styles.formGroup}>
                            <label style={styles.label} htmlFor="pipeline-max-retries">
                                Max Retries
                            </label>
                            <input
                                id="pipeline-max-retries"
                                type="number"
                                min="0"
                                max="10"
                                value={formData.max_retries}
                                onChange={(e) => handleFieldChange('max_retries', parseInt(e.target.value) || 0)}
                                style={styles.input}
                            />
                        </div>
                    )}
                </div>

                <div style={{ ...styles.formGroup, marginTop: '16px' }}>
                    <label style={styles.label} htmlFor="pipeline-description">
                        Description
                    </label>
                    <textarea
                        id="pipeline-description"
                        value={formData.description}
                        onChange={(e) => handleFieldChange('description', e.target.value)}
                        placeholder="Describe what this pipeline does..."
                        style={styles.textarea}
                    />
                </div>
            </div>

            {/* Schedule Section (Optional) */}
            <div id="pipeline-schedule-section" style={styles.section}>
                <h2 style={styles.sectionTitle}>Schedule (Optional)</h2>
                <div style={styles.formGroup}>
                    <label style={styles.label} htmlFor="pipeline-cron">
                        Cron Expression
                    </label>
                    <input
                        id="pipeline-cron"
                        type="text"
                        value={formData.schedule}
                        onChange={(e) => handleFieldChange('schedule', e.target.value)}
                        placeholder="0 0 * * *"
                        style={styles.input}
                    />
                    <div style={styles.scheduleHelp}>
                        Example: {CRON_EXAMPLES.map((ex, i) => (
                            <span key={ex.value}>
                                {i > 0 ? ' | ' : ''}
                                <code>{ex.value}</code> ({ex.label})
                            </span>
                        ))}
                    </div>
                </div>
            </div>

            {/* Steps Section */}
            <div id="pipeline-steps-section" style={styles.section}>
                <h2 style={styles.sectionTitle}>Pipeline Steps</h2>
                <div style={styles.stepsList}>
                    {steps.map((step, index) => (
                        <PipelineStepCard
                            key={step.id}
                            stepNumber={index + 1}
                            step={step}
                            onChange={(updated) => handleStepChange(index, updated)}
                            onMoveUp={() => handleMoveStepUp(index)}
                            onMoveDown={() => handleMoveStepDown(index)}
                            onRemove={() => handleRemoveStep(index)}
                            isFirst={index === 0}
                            isLast={index === steps.length - 1}
                            totalSteps={steps.length}
                        />
                    ))}

                    <button
                        id="add-step-button"
                        type="button"
                        style={styles.addStepButton}
                        onClick={handleAddStep}
                    >
                        <PlusIcon size={16} />
                        Add Step
                    </button>
                </div>
            </div>

            {/* Action Buttons */}
            <div style={styles.buttonRow}>
                <button
                    id="cancel-button"
                    type="button"
                    className="btn btn--outline"
                    onClick={handleCancel}
                >
                    Cancel
                </button>
                <button
                    id="save-button"
                    type="button"
                    className="btn btn--primary"
                    onClick={handleSave}
                    disabled={isSaving}
                >
                    {isSaving ? 'Saving...' : isEditMode ? 'Update Pipeline' : 'Create Pipeline'}
                </button>
            </div>
        </div>
    );
};

export default PipelineBuilder;
