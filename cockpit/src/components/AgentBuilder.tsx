/**
 * AgentBuilder — 6-step wizard for creating and editing agents (Enterprise feature)
 *
 * Steps: Identity → Brain → Tools → Guardrails → Deployment → Review & Test
 * License-gated: Community users see UpgradeCard overlay.
 *
 * Modes:
 *   - Create:  /forge/builder         → useCreateAgent + optionally useDeployAgent
 *   - Edit:    /forge/builder/:agentId → useUpdateAgent (pre-filled from API)
 */

import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useCreateAgent, useDeployAgent, useUpdateAgent, useAgentDetail } from '../hooks/useAgents';
import { useLicense } from '../hooks/useLicense';
import { useToast } from './Toast';
import { ArrowLeftIcon } from './Icons';
import UpgradeCard from './UpgradeCard';
import SystemPromptEditor from './SystemPromptEditor';
import ToolPicker, { type ToolsConfig } from './ToolPicker';
import GuardrailConfig, { type GuardrailsConfig, DEFAULT_GUARDRAILS } from './GuardrailConfig';
import AgentRoutingConfig from './AgentRoutingConfig';

// ============================================================================
// Types
// ============================================================================

interface WizardData {
    // Step 1: Identity
    name: string;
    display_name: string;
    icon: string;
    description: string;
    category: string;
    // Step 2: Brain
    provider: string;
    model: string;
    system_prompt: string;
    // Step 3: Tools
    tools_config: ToolsConfig;
    // Step 4: Guardrails
    guardrails: GuardrailsConfig;
    // Step 5: Deployment
    satellite_ids: string[];
    schedule: string;
    routing: {
        mode: string;
        require_tags: string[];
        prefer_tags: string[];
    } | null;
}

interface StepErrors {
    [key: string]: string;
}

// ============================================================================
// Constants
// ============================================================================

const STEPS = [
    { id: 1, name: 'Identity', icon: '1' },
    { id: 2, name: 'Brain', icon: '2' },
    { id: 3, name: 'Tools', icon: '3' },
    { id: 4, name: 'Guardrails', icon: '4' },
    { id: 5, name: 'Deployment', icon: '5' },
    { id: 6, name: 'Review & Test', icon: '6' },
];

const CATEGORIES = [
    { value: 'infrastructure', label: 'Infrastructure' },
    { value: 'development', label: 'Development' },
    { value: 'security', label: 'Security' },
    { value: 'operations', label: 'Operations' },
];

const PROVIDERS = [
    { value: 'anthropic', label: 'Anthropic' },
    { value: 'openai', label: 'OpenAI' },
    { value: 'google', label: 'Google' },
    { value: 'minimax', label: 'MiniMax' },
    { value: 'azure', label: 'Azure OpenAI' },
    { value: 'mistral', label: 'Mistral' },
    { value: 'deepseek', label: 'DeepSeek' },
    { value: 'xai', label: 'xAI (Grok)' },
    { value: 'ollama', label: 'Ollama (Local)' },
];

const AGENT_ICONS = ['🤖', '🛡️', '🔧', '📊', '🔍', '⚙️', '🧠', '🚀', '🐛', '📝', '🔒', '🌐'];

const NAME_PATTERN = /^[a-z][a-z0-9-]*$/;

const INITIAL_DATA: WizardData = {
    name: '',
    display_name: '',
    icon: '🤖',
    description: '',
    category: 'infrastructure',
    provider: 'openai',
    model: '',
    system_prompt: '',
    tools_config: { allow: [], deny: [] },
    guardrails: { ...DEFAULT_GUARDRAILS },
    satellite_ids: [],
    schedule: '',
    routing: null,
};

// ============================================================================
// Step Indicator
// ============================================================================

const StepIndicator: React.FC<{ currentStep: number }> = ({ currentStep }) => (
    <div className="wizard-steps">
        {STEPS.map((step, index) => (
            <React.Fragment key={step.id}>
                <div className={`wizard-step ${currentStep === step.id ? 'wizard-step--active' : ''} ${currentStep > step.id ? 'wizard-step--completed' : ''}`}>
                    <div className="wizard-step__circle">
                        {currentStep > step.id ? (
                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round">
                                <polyline points="20 6 9 17 4 12" />
                            </svg>
                        ) : (
                            step.icon
                        )}
                    </div>
                    <span className="wizard-step__label">{step.name}</span>
                </div>
                {index < STEPS.length - 1 && (
                    <div className={`wizard-step__line ${currentStep > step.id ? 'wizard-step__line--completed' : ''}`} />
                )}
            </React.Fragment>
        ))}
    </div>
);

// ============================================================================
// Step Components
// ============================================================================

/** Step 1 — Identity */
const IdentityStep: React.FC<{
    data: WizardData;
    errors: StepErrors;
    onUpdate: (key: keyof WizardData, value: unknown) => void;
    isEditMode?: boolean;
}> = ({ data, errors, onUpdate, isEditMode }) => (
    <div className="wizard-form">
        <h3 className="wizard-form__title">Agent Identity</h3>
        <p className="wizard-form__desc">Give your agent a name, icon, and description.</p>

        <div className="wizard-form__grid">
            {/* Icon picker */}
            <div className="wizard-form__field wizard-form__field--full">
                <label className="wizard-form__label">Icon</label>
                <div className="wizard-icon-picker">
                    {AGENT_ICONS.map((icon) => (
                        <button
                            key={icon}
                            type="button"
                            className={`wizard-icon-picker__btn ${data.icon === icon ? 'wizard-icon-picker__btn--selected' : ''}`}
                            onClick={() => onUpdate('icon', icon)}
                        >
                            {icon}
                        </button>
                    ))}
                </div>
            </div>

            {/* Name */}
            <div className="wizard-form__field">
                <label className="wizard-form__label">
                    Name <span className="wizard-form__required">*</span>
                </label>
                <input
                    type="text"
                    className={`wizard-form__input ${errors.name ? 'wizard-form__input--error' : ''}`}
                    placeholder="my-agent"
                    value={data.name}
                    onChange={(e) => onUpdate('name', e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''))}
                    readOnly={isEditMode}
                    style={isEditMode ? { opacity: 0.6, cursor: 'not-allowed' } : undefined}
                />
                {errors.name && <span className="wizard-form__error">{errors.name}</span>}
                <span className="wizard-form__hint">{isEditMode ? 'Name cannot be changed after creation' : 'Lowercase letters, numbers, and hyphens only'}</span>
            </div>

            {/* Display Name */}
            <div className="wizard-form__field">
                <label className="wizard-form__label">
                    Display Name <span className="wizard-form__required">*</span>
                </label>
                <input
                    type="text"
                    className={`wizard-form__input ${errors.display_name ? 'wizard-form__input--error' : ''}`}
                    placeholder="My Agent"
                    value={data.display_name}
                    onChange={(e) => onUpdate('display_name', e.target.value)}
                />
                {errors.display_name && <span className="wizard-form__error">{errors.display_name}</span>}
            </div>

            {/* Category */}
            <div className="wizard-form__field">
                <label className="wizard-form__label">Category</label>
                <select
                    className="wizard-form__select"
                    value={data.category}
                    onChange={(e) => onUpdate('category', e.target.value)}
                >
                    {CATEGORIES.map((c) => (
                        <option key={c.value} value={c.value}>{c.label}</option>
                    ))}
                </select>
            </div>

            {/* Description */}
            <div className="wizard-form__field wizard-form__field--full">
                <label className="wizard-form__label">Description</label>
                <textarea
                    className="wizard-form__textarea"
                    placeholder="Describe what this agent does..."
                    value={data.description}
                    onChange={(e) => onUpdate('description', e.target.value)}
                    rows={3}
                />
            </div>
        </div>
    </div>
);

/** Step 2 — Brain */
const BrainStep: React.FC<{
    data: WizardData;
    errors: StepErrors;
    onUpdate: (key: keyof WizardData, value: unknown) => void;
}> = ({ data, errors, onUpdate }) => (
    <div className="wizard-form">
        <h3 className="wizard-form__title">Brain Configuration</h3>
        <p className="wizard-form__desc">Choose the AI provider, model, and system prompt.</p>

        <div className="wizard-form__grid">
            {/* Provider */}
            <div className="wizard-form__field">
                <label className="wizard-form__label">
                    Provider <span className="wizard-form__required">*</span>
                </label>
                <select
                    className={`wizard-form__select ${errors.provider ? 'wizard-form__input--error' : ''}`}
                    value={data.provider}
                    onChange={(e) => onUpdate('provider', e.target.value)}
                >
                    {PROVIDERS.map((p) => (
                        <option key={p.value} value={p.value}>{p.label}</option>
                    ))}
                </select>
                {errors.provider && <span className="wizard-form__error">{errors.provider}</span>}
            </div>

            {/* Model */}
            <div className="wizard-form__field">
                <label className="wizard-form__label">
                    Model <span className="wizard-form__required">*</span>
                </label>
                <input
                    type="text"
                    className={`wizard-form__input ${errors.model ? 'wizard-form__input--error' : ''}`}
                    placeholder="gpt-4o"
                    value={data.model}
                    onChange={(e) => onUpdate('model', e.target.value)}
                />
                {errors.model && <span className="wizard-form__error">{errors.model}</span>}
            </div>

            {/* System Prompt */}
            <div className="wizard-form__field wizard-form__field--full">
                <label className="wizard-form__label">
                    System Prompt <span className="wizard-form__required">*</span>
                </label>
                <SystemPromptEditor
                    value={data.system_prompt}
                    onChange={(v) => onUpdate('system_prompt', v)}
                />
                {errors.system_prompt && <span className="wizard-form__error">{errors.system_prompt}</span>}
            </div>
        </div>
    </div>
);

/** Step 3 — Tools */
const ToolsStep: React.FC<{
    data: WizardData;
    onUpdate: (key: keyof WizardData, value: unknown) => void;
}> = ({ data, onUpdate }) => (
    <div className="wizard-form">
        <h3 className="wizard-form__title">Tool Configuration</h3>
        <p className="wizard-form__desc">Choose which tools this agent can use.</p>
        <ToolPicker
            value={data.tools_config}
            onChange={(config) => onUpdate('tools_config', config)}
            readOnly={data.guardrails.read_only}
        />
    </div>
);

/** Step 4 — Guardrails */
const GuardrailsStep: React.FC<{
    data: WizardData;
    onUpdate: (key: keyof WizardData, value: unknown) => void;
}> = ({ data, onUpdate }) => {
    const handleReadOnlyChange = useCallback((readOnly: boolean) => {
        // Sync read-only with tools_config
        if (readOnly) {
            const writeTools = ['write', 'apply_patch', 'edit'];
            const newAllow = data.tools_config.allow.filter((t) => !writeTools.includes(t));
            const newDeny = [...new Set([...data.tools_config.deny, ...writeTools])];
            onUpdate('tools_config', { allow: newAllow, deny: newDeny });
        }
    }, [data.tools_config, onUpdate]);

    return (
        <div className="wizard-form">
            <h3 className="wizard-form__title">Guardrails</h3>
            <p className="wizard-form__desc">Set safety limits and operational constraints.</p>
            <GuardrailConfig
                value={data.guardrails}
                onChange={(config) => onUpdate('guardrails', config)}
                onReadOnlyChange={handleReadOnlyChange}
            />
        </div>
    );
};

/** Step 5 — Deployment */
const DeploymentStep: React.FC<{
    data: WizardData;
    onUpdate: (key: keyof WizardData, value: unknown) => void;
}> = ({ data, onUpdate }) => {
    const [satellites, setSatellites] = useState<Array<{ id: string; name: string }>>([]);
    const [loadingSats, setLoadingSats] = useState(false);

    React.useEffect(() => {
        setLoadingSats(true);
        fetch('/api/v1/satellites')
            .then((r) => r.json())
            .then((d) => setSatellites(d.satellites || []))
            .catch(() => setSatellites([]))
            .finally(() => setLoadingSats(false));
    }, []);

    const toggleSatellite = useCallback((id: string) => {
        const current = data.satellite_ids;
        const next = current.includes(id) ? current.filter((s) => s !== id) : [...current, id];
        onUpdate('satellite_ids', next);
    }, [data.satellite_ids, onUpdate]);

    const handleRoutingChange = useCallback((routing: { mode: string; require_tags?: string[]; prefer_tags?: string[] } | null) => {
        onUpdate('routing', routing);
    }, [onUpdate]);

    return (
        <div className="wizard-form">
            <h3 className="wizard-form__title">Deployment</h3>
            <p className="wizard-form__desc">Choose target satellites and schedule (optional).</p>

            <div className="wizard-form__field">
                <label className="wizard-form__label">Target Satellites</label>
                {loadingSats ? (
                    <div className="wizard-form__loading">Loading satellites...</div>
                ) : satellites.length === 0 ? (
                    <div className="wizard-form__empty-hint">
                        No satellites registered. You can deploy later from the Forge page.
                    </div>
                ) : (
                    <div className="wizard-satellite-grid">
                        {satellites.map((sat) => (
                            <button
                                key={sat.id}
                                type="button"
                                className={`wizard-satellite-card ${data.satellite_ids.includes(sat.id) ? 'wizard-satellite-card--selected' : ''}`}
                                onClick={() => toggleSatellite(sat.id)}
                            >
                                <span className="wizard-satellite-card__indicator">
                                    {data.satellite_ids.includes(sat.id) ? '✓' : '○'}
                                </span>
                                <span className="wizard-satellite-card__name">{sat.name}</span>
                            </button>
                        ))}
                    </div>
                )}
            </div>

            <div className="wizard-form__field">
                <label className="wizard-form__label">Schedule (Cron Expression)</label>
                <input
                    type="text"
                    className="wizard-form__input"
                    placeholder="Optional — e.g., 0 */6 * * * (every 6 hours)"
                    value={data.schedule}
                    onChange={(e) => onUpdate('schedule', e.target.value)}
                />
                <span className="wizard-form__hint">Leave empty for on-demand execution</span>
            </div>

            <AgentRoutingConfig
                value={data.routing as { mode: 'targeted' | 'auto-dispatch'; require_tags?: string[]; prefer_tags?: string[] } | null}
                onChange={handleRoutingChange}
            />
        </div>
    );
};

/** Step 6 — Review & Test */
const ReviewStep: React.FC<{
    data: WizardData;
    isSubmitting: boolean;
}> = ({ data }) => {
    const toolCount = data.tools_config.allow.length;
    const deniedCount = data.tools_config.deny.length;

    return (
        <div className="wizard-form">
            <h3 className="wizard-form__title">Review & Test</h3>
            <p className="wizard-form__desc">Review your agent configuration before saving.</p>

            <div className="wizard-review">
                {/* Identity */}
                <div className="wizard-review__section">
                    <h4 className="wizard-review__heading">Identity</h4>
                    <div className="wizard-review__grid">
                        <div className="wizard-review__item">
                            <span className="wizard-review__key">Icon</span>
                            <span className="wizard-review__val">{data.icon}</span>
                        </div>
                        <div className="wizard-review__item">
                            <span className="wizard-review__key">Name</span>
                            <span className="wizard-review__val wizard-review__val--mono">{data.name}</span>
                        </div>
                        <div className="wizard-review__item">
                            <span className="wizard-review__key">Display Name</span>
                            <span className="wizard-review__val">{data.display_name}</span>
                        </div>
                        <div className="wizard-review__item">
                            <span className="wizard-review__key">Category</span>
                            <span className="wizard-review__val">{data.category}</span>
                        </div>
                        {data.description && (
                            <div className="wizard-review__item wizard-review__item--full">
                                <span className="wizard-review__key">Description</span>
                                <span className="wizard-review__val">{data.description}</span>
                            </div>
                        )}
                    </div>
                </div>

                {/* Brain */}
                <div className="wizard-review__section">
                    <h4 className="wizard-review__heading">Brain</h4>
                    <div className="wizard-review__grid">
                        <div className="wizard-review__item">
                            <span className="wizard-review__key">Provider</span>
                            <span className="wizard-review__val">{data.provider}</span>
                        </div>
                        <div className="wizard-review__item">
                            <span className="wizard-review__key">Model</span>
                            <span className="wizard-review__val wizard-review__val--mono">{data.model}</span>
                        </div>
                        <div className="wizard-review__item wizard-review__item--full">
                            <span className="wizard-review__key">System Prompt</span>
                            <pre className="wizard-review__prompt">{data.system_prompt.slice(0, 200)}{data.system_prompt.length > 200 ? '…' : ''}</pre>
                        </div>
                    </div>
                </div>

                {/* Tools */}
                <div className="wizard-review__section">
                    <h4 className="wizard-review__heading">Tools</h4>
                    <div className="wizard-review__grid">
                        <div className="wizard-review__item">
                            <span className="wizard-review__key">Allowed</span>
                            <span className="wizard-review__val">{toolCount > 0 ? data.tools_config.allow.join(', ') : 'None selected'}</span>
                        </div>
                        {deniedCount > 0 && (
                            <div className="wizard-review__item">
                                <span className="wizard-review__key">Denied</span>
                                <span className="wizard-review__val">{data.tools_config.deny.join(', ')}</span>
                            </div>
                        )}
                    </div>
                </div>

                {/* Guardrails */}
                <div className="wizard-review__section">
                    <h4 className="wizard-review__heading">Guardrails</h4>
                    <div className="wizard-review__grid">
                        <div className="wizard-review__item">
                            <span className="wizard-review__key">HITL</span>
                            <span className="wizard-review__val">{data.guardrails.hitl ? 'Enabled' : 'Disabled'}</span>
                        </div>
                        <div className="wizard-review__item">
                            <span className="wizard-review__key">Read-Only</span>
                            <span className="wizard-review__val">{data.guardrails.read_only ? 'Yes' : 'No'}</span>
                        </div>
                        <div className="wizard-review__item">
                            <span className="wizard-review__key">Timeout</span>
                            <span className="wizard-review__val">{data.guardrails.timeout_minutes} min</span>
                        </div>
                        <div className="wizard-review__item">
                            <span className="wizard-review__key">Max Tool Calls</span>
                            <span className="wizard-review__val">{data.guardrails.max_tool_calls}</span>
                        </div>
                        <div className="wizard-review__item">
                            <span className="wizard-review__key">Max Turns</span>
                            <span className="wizard-review__val">{data.guardrails.max_turns}</span>
                        </div>
                    </div>
                </div>

                {/* Deployment */}
                <div className="wizard-review__section">
                    <h4 className="wizard-review__heading">Deployment</h4>
                    <div className="wizard-review__grid">
                        <div className="wizard-review__item">
                            <span className="wizard-review__key">Satellites</span>
                            <span className="wizard-review__val">{data.satellite_ids.length > 0 ? `${data.satellite_ids.length} selected` : 'None (deploy later)'}</span>
                        </div>
                        {data.schedule && (
                            <div className="wizard-review__item">
                                <span className="wizard-review__key">Schedule</span>
                                <span className="wizard-review__val wizard-review__val--mono">{data.schedule}</span>
                            </div>
                        )}
                    </div>
                </div>
            </div>
        </div>
    );
};

// ============================================================================
// Validation
// ============================================================================

function validateStep(step: number, data: WizardData): StepErrors {
    const errors: StepErrors = {};
    switch (step) {
        case 1:
            if (!data.name) errors.name = 'Name is required';
            else if (!NAME_PATTERN.test(data.name)) errors.name = 'Must start with letter, lowercase + hyphens only';
            if (!data.display_name) errors.display_name = 'Display name is required';
            break;
        case 2:
            if (!data.provider) errors.provider = 'Provider is required';
            if (!data.model) errors.model = 'Model is required';
            if (!data.system_prompt.trim()) errors.system_prompt = 'System prompt is required';
            break;
        // Steps 3-6 don't have required validation
    }
    return errors;
}

// ============================================================================
// Main Component
// ============================================================================

const AgentBuilder: React.FC = () => {
    const navigate = useNavigate();
    const { agentId } = useParams<{ agentId?: string }>();
    const [searchParams] = useSearchParams();
    const cloneSourceId = searchParams.get('clone');
    const { isCommunity } = useLicense();
    const { createAgent, isCreating } = useCreateAgent();
    const { updateAgent, isUpdating } = useUpdateAgent();
    const { deploy, isDeploying } = useDeployAgent();
    const { showToast } = useToast();

    const isEditMode = !!agentId;
    const isCloneMode = !!cloneSourceId;
    // In clone mode, fetch the source agent to pre-fill; in edit mode, fetch the agent to edit
    const fetchId = agentId || cloneSourceId || '';
    const { agent: existingAgent, isLoading: isLoadingAgent } = useAgentDetail(fetchId);

    const [currentStep, setCurrentStep] = useState(1);
    const [data, setData] = useState<WizardData>({ ...INITIAL_DATA });
    const [errors, setErrors] = useState<StepErrors>({});
    const [isSubmitted, setIsSubmitted] = useState(false);
    const [dataLoaded, setDataLoaded] = useState(false);

    // Pre-fill wizard data from existing agent in edit or clone mode
    useEffect(() => {
        if ((isEditMode || isCloneMode) && existingAgent && !dataLoaded) {
            // Parse tools_config
            let toolsConfig: ToolsConfig = { allow: [], deny: [] };
            if (existingAgent.tools_config) {
                try {
                    const parsed = typeof existingAgent.tools_config === 'string'
                        ? JSON.parse(existingAgent.tools_config)
                        : existingAgent.tools_config;
                    if (parsed.allow || parsed.deny) {
                        toolsConfig = { allow: parsed.allow || [], deny: parsed.deny || [] };
                    }
                } catch { /* use defaults */ }
            }

            // Parse guardrails
            let guardrails: GuardrailsConfig = { ...DEFAULT_GUARDRAILS };
            if (existingAgent.guardrails) {
                try {
                    const parsed = typeof existingAgent.guardrails === 'string'
                        ? JSON.parse(existingAgent.guardrails)
                        : existingAgent.guardrails;
                    guardrails = {
                        hitl: parsed.hitl_enabled ?? parsed.hitl ?? DEFAULT_GUARDRAILS.hitl,
                        read_only: parsed.read_only ?? DEFAULT_GUARDRAILS.read_only,
                        timeout_minutes: parsed.timeout_minutes ?? parsed.timeout ?? DEFAULT_GUARDRAILS.timeout_minutes,
                        max_tool_calls: parsed.max_tool_calls ?? DEFAULT_GUARDRAILS.max_tool_calls,
                        max_turns: parsed.max_turns ?? DEFAULT_GUARDRAILS.max_turns,
                    };
                } catch { /* use defaults */ }
            }

            setData({
                name: isCloneMode ? `${existingAgent.name}-copy` : (existingAgent.name || ''),
                display_name: isCloneMode ? `${existingAgent.display_name || existingAgent.name} (Copy)` : (existingAgent.display_name || ''),
                icon: existingAgent.icon || '🤖',
                description: existingAgent.description || '',
                category: existingAgent.category || 'infrastructure',
                provider: existingAgent.provider === 'configurable' ? 'openai' : (existingAgent.provider || 'openai'),
                model: existingAgent.model === 'default' ? '' : (existingAgent.model || ''),
                system_prompt: existingAgent.system_prompt || '',
                tools_config: toolsConfig,
                guardrails: guardrails,
                satellite_ids: [],
                schedule: existingAgent.schedule || '',
                routing: existingAgent.routing ? JSON.parse(existingAgent.routing) : null,
            });
            setDataLoaded(true);
        }
    }, [isEditMode, isCloneMode, existingAgent, dataLoaded]);

    const isSubmitting = isCreating || isUpdating || isDeploying;

    // Update a single field
    const handleUpdate = useCallback((key: keyof WizardData, value: unknown) => {
        setData((prev) => ({ ...prev, [key]: value }));
        // Clear error for this field
        setErrors((prev) => {
            if (prev[key]) {
                const next = { ...prev };
                delete next[key];
                return next;
            }
            return prev;
        });
    }, []);

    // Navigate forward
    const handleNext = useCallback(() => {
        const stepErrors = validateStep(currentStep, data);
        if (Object.keys(stepErrors).length > 0) {
            setErrors(stepErrors);
            return;
        }
        setErrors({});
        setCurrentStep((s) => Math.min(s + 1, 6));
    }, [currentStep, data]);

    // Navigate backward
    const handleBack = useCallback(() => {
        setErrors({});
        setCurrentStep((s) => Math.max(s - 1, 1));
    }, []);

    // Final submit
    const handleSubmit = useCallback(async () => {
        const stepErrors = validateStep(1, data);
        const brainErrors = validateStep(2, data);
        const allErrors = { ...stepErrors, ...brainErrors };
        if (Object.keys(allErrors).length > 0) {
            setErrors(allErrors);
            showToast('Please fix validation errors before submitting', 'error');
            return;
        }

        if (isEditMode && agentId) {
            // --- Update existing agent ---
            const result = await updateAgent(agentId, {
                display_name: data.display_name,
                description: data.description || undefined,
                category: data.category,
                provider: data.provider,
                model: data.model,
                system_prompt: data.system_prompt,
                tools_config: data.tools_config as unknown as Record<string, unknown>,
                guardrails: data.guardrails as unknown as Record<string, unknown>,
                routing: data.routing ? JSON.stringify(data.routing) : undefined,
            });

            if (!result) {
                showToast('Failed to update agent', 'error');
                return;
            }

            showToast(`Agent "${data.display_name}" updated successfully!`, 'success');
            setIsSubmitted(true);
            navigate('/forge');
        } else {
            // --- Create new agent ---
            const agent = await createAgent({
                name: data.name,
                display_name: data.display_name,
                description: data.description || undefined,
                type: 'specialist',
                category: data.category,
                provider: data.provider,
                model: data.model,
                system_prompt: data.system_prompt,
                icon: data.icon,
                tools_config: data.tools_config as unknown as Record<string, unknown>,
                guardrails: data.guardrails as unknown as Record<string, unknown>,
                schedule: data.schedule || undefined,
                routing: data.routing ? JSON.stringify(data.routing) : undefined,
            });

            if (!agent) {
                showToast('Failed to create agent', 'error');
                return;
            }

            showToast(`Agent "${data.display_name}" created successfully!`, 'success');
            setIsSubmitted(true);

            // Deploy to selected satellites
            if (data.satellite_ids.length > 0) {
                let deployedCount = 0;
                for (const satId of data.satellite_ids) {
                    const result = await deploy(agent.id, { satellite_id: satId });
                    if (result) deployedCount++;
                }
                if (deployedCount > 0) {
                    showToast(`Deployed to ${deployedCount} satellite(s)`, 'success');
                }
            }

            navigate('/forge');
        }
    }, [data, isEditMode, agentId, createAgent, updateAgent, deploy, showToast, navigate]);

    // ========================================================================
    // Render
    // ========================================================================

    // License gate
    if (isCommunity) {
        return (
            <div className="wizard-page">
                <div className="wizard-page__header">
                    <button className="wizard-page__back" onClick={() => navigate('/forge')}>
                        <ArrowLeftIcon size={18} />
                        <span>Back to Forge</span>
                    </button>
                    <h1 className="wizard-page__title">{isEditMode ? 'Edit Agent' : 'Agent Builder'}</h1>
                </div>
                <div className="wizard-gate">
                    <div className="wizard-gate__overlay">
                        <h2 className="wizard-gate__heading">Coming Soon</h2>
                        <p className="wizard-gate__desc">
                            The Agent Builder wizard is planned for a future DAAO Enterprise release.
                            Community users can create agents using the simple modal on the Forge page.
                        </p>
                        <UpgradeCard />
                    </div>
                </div>
            </div>
        );
    }

    // Show loading state when fetching agent data for edit or clone
    if ((isEditMode || isCloneMode) && isLoadingAgent) {
        return (
            <div className="wizard-page">
                <div className="wizard-page__header">
                    <button className="wizard-page__back" onClick={() => navigate('/forge')}>
                        <ArrowLeftIcon size={18} />
                        <span>Back to Forge</span>
                    </button>
                    <h1 className="wizard-page__title">Edit Agent</h1>
                </div>
                <div style={{ textAlign: 'center', padding: '3rem', color: 'var(--text-muted)' }}>
                    Loading agent data...
                </div>
            </div>
        );
    }

    return (
        <div className="wizard-page">
            {/* Header */}
            <div className="wizard-page__header">
                <button className="wizard-page__back" onClick={() => navigate('/forge')}>
                    <ArrowLeftIcon size={18} />
                    <span>Back to Forge</span>
                </button>
                <h1 className="wizard-page__title">{isEditMode ? 'Edit Agent' : isCloneMode ? 'Clone Agent' : 'Agent Builder'}</h1>
            </div>

            {/* Step Indicator */}
            <StepIndicator currentStep={currentStep} />

            {/* Step Content */}
            <div className="wizard-content">
                {currentStep === 1 && <IdentityStep data={data} errors={errors} onUpdate={handleUpdate} isEditMode={isEditMode} />}
                {currentStep === 2 && <BrainStep data={data} errors={errors} onUpdate={handleUpdate} />}
                {currentStep === 3 && <ToolsStep data={data} onUpdate={handleUpdate} />}
                {currentStep === 4 && <GuardrailsStep data={data} onUpdate={handleUpdate} />}
                {currentStep === 5 && <DeploymentStep data={data} onUpdate={handleUpdate} />}
                {currentStep === 6 && <ReviewStep data={data} isSubmitting={isSubmitting} />}
            </div>

            {/* Navigation */}
            <div className="wizard-nav">
                <button
                    type="button"
                    className="btn btn--ghost"
                    onClick={handleBack}
                    disabled={currentStep === 1}
                >
                    ← Back
                </button>
                <div className="wizard-nav__right">
                    {currentStep < 6 ? (
                        <button
                            type="button"
                            className="btn btn--primary"
                            onClick={handleNext}
                        >
                            Next →
                        </button>
                    ) : (
                        <button
                            type="button"
                            className="btn btn--primary"
                            onClick={handleSubmit}
                            disabled={isSubmitting || isSubmitted}
                        >
                            {isSubmitting
                                ? (isEditMode ? 'Saving…' : 'Creating…')
                                : isSubmitted
                                    ? (isEditMode ? 'Saved ✓' : 'Created ✓')
                                    : (isEditMode ? 'Save Changes' : 'Create Agent')}
                        </button>
                    )}
                </div>
            </div>
        </div>
    );
};

// ============================================================================
// Page Wrapper
// ============================================================================

/**
 * AgentBuilderPage — Top-level page component for the /forge/builder route
 */
export const AgentBuilderPage: React.FC = () => (
    <AgentBuilder />
);

export default AgentBuilder;
