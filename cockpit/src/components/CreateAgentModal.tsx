/**
 * CreateAgentModal — Modal form for creating a new agent definition
 * 
 * Two-column layout on desktop, stacked on mobile.
 * Validates required fields and name format (lowercase + hyphens).
 * Calls useCreateAgent() hook on submit.
 */

import React, { useState, useEffect, useCallback } from 'react';
import { XIcon } from './Icons';
import { useCreateAgent, type CreateAgentRequest } from '../hooks/useAgents';
import { useToast } from './Toast';

// ============================================================================
// Types
// ============================================================================

export interface CreateAgentModalProps {
    isOpen: boolean;
    onClose: () => void;
    onCreated: () => void;
}

interface FormErrors {
    name?: string;
    display_name?: string;
    type?: string;
    category?: string;
    provider?: string;
    model?: string;
    system_prompt?: string;
}

// ============================================================================
// Constants
// ============================================================================

const AGENT_TYPES = [
    { value: 'specialist', label: 'Specialist' },
    { value: 'autonomous', label: 'Autonomous' },
];

const CATEGORIES = [
    { value: 'operations', label: 'Operations' },
    { value: 'infrastructure', label: 'Infrastructure' },
    { value: 'security', label: 'Security' },
    { value: 'development', label: 'Development' },
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
    { value: 'ollama', label: 'Ollama' },
];

const NAME_PATTERN = /^[a-z][a-z0-9-]*$/;

// ============================================================================
// Component
// ============================================================================

const CreateAgentModal: React.FC<CreateAgentModalProps> = ({ isOpen, onClose, onCreated }) => {
    const { createAgent, isCreating, error: apiError } = useCreateAgent();
    const { showToast } = useToast();

    // Form state
    const [name, setName] = useState('');
    const [displayName, setDisplayName] = useState('');
    const [description, setDescription] = useState('');
    const [type, setType] = useState('');
    const [category, setCategory] = useState('');
    const [provider, setProvider] = useState('');
    const [model, setModel] = useState('');
    const [systemPrompt, setSystemPrompt] = useState('');
    const [errors, setErrors] = useState<FormErrors>({});

    // Reset form when modal opens
    useEffect(() => {
        if (isOpen) {
            setName('');
            setDisplayName('');
            setDescription('');
            setType('');
            setCategory('');
            setProvider('');
            setModel('');
            setSystemPrompt('');
            setErrors({});
        }
    }, [isOpen]);

    // Handle Escape key
    useEffect(() => {
        const handleKeyDown = (e: KeyboardEvent) => {
            if (e.key === 'Escape' && isOpen) {
                onClose();
            }
        };
        document.addEventListener('keydown', handleKeyDown);
        return () => document.removeEventListener('keydown', handleKeyDown);
    }, [isOpen, onClose]);

    // Validate form
    const validate = useCallback((): boolean => {
        const newErrors: FormErrors = {};

        if (!name.trim()) {
            newErrors.name = 'Name is required';
        } else if (!NAME_PATTERN.test(name)) {
            newErrors.name = 'Must be lowercase letters, numbers, and hyphens only';
        }

        if (!displayName.trim()) {
            newErrors.display_name = 'Display name is required';
        }

        if (!type) {
            newErrors.type = 'Type is required';
        }

        if (!category) {
            newErrors.category = 'Category is required';
        }

        if (!provider) {
            newErrors.provider = 'Provider is required';
        }

        if (!model.trim()) {
            newErrors.model = 'Model is required';
        }

        if (!systemPrompt.trim()) {
            newErrors.system_prompt = 'System prompt is required';
        }

        setErrors(newErrors);
        return Object.keys(newErrors).length === 0;
    }, [name, displayName, type, category, provider, model, systemPrompt]);

    // Handle submit
    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        if (!validate()) return;

        const request: CreateAgentRequest = {
            name: name.trim(),
            display_name: displayName.trim(),
            description: description.trim() || undefined,
            type,
            category,
            provider,
            model: model.trim(),
            system_prompt: systemPrompt.trim(),
        };

        const result = await createAgent(request);
        if (result) {
            showToast(`Agent "${result.display_name}" created successfully`, 'success');
            onCreated();
            onClose();
        }
    };

    if (!isOpen) return null;

    return (
        <div className="modal-overlay" onClick={onClose}>
            <div className="modal" onClick={(e) => e.stopPropagation()} style={{ maxWidth: 640 }}>
                {/* Header */}
                <div className="modal__header">
                    <h2 className="modal__title">Create Agent</h2>
                    <button className="modal__close" onClick={onClose} type="button" aria-label="Close">
                        <XIcon size={20} />
                    </button>
                </div>

                {/* Body */}
                <div className="modal__body">
                    <form className="forge-form" onSubmit={handleSubmit}>
                        {/* API Error */}
                        {apiError && (
                            <div className="drawer-notice drawer-notice--warning">
                                {apiError.message}
                            </div>
                        )}

                        {/* Row 1: Name + Display Name */}
                        <div className="forge-form__row">
                            <div className="forge-form__group">
                                <label className="forge-form__label forge-form__label--required">Name</label>
                                <input
                                    className={`forge-form__input ${errors.name ? 'forge-form__input--error' : ''}`}
                                    type="text"
                                    value={name}
                                    onChange={(e) => setName(e.target.value)}
                                    placeholder="my-agent"
                                    disabled={isCreating}
                                />
                                {errors.name && <span className="forge-form__error">{errors.name}</span>}
                            </div>
                            <div className="forge-form__group">
                                <label className="forge-form__label forge-form__label--required">Display Name</label>
                                <input
                                    className={`forge-form__input ${errors.display_name ? 'forge-form__input--error' : ''}`}
                                    type="text"
                                    value={displayName}
                                    onChange={(e) => setDisplayName(e.target.value)}
                                    placeholder="My Agent"
                                    disabled={isCreating}
                                />
                                {errors.display_name && <span className="forge-form__error">{errors.display_name}</span>}
                            </div>
                        </div>

                        {/* Row 2: Type + Category */}
                        <div className="forge-form__row">
                            <div className="forge-form__group">
                                <label className="forge-form__label forge-form__label--required">Type</label>
                                <select
                                    className={`forge-form__select ${errors.type ? 'forge-form__select--error' : ''}`}
                                    value={type}
                                    onChange={(e) => setType(e.target.value)}
                                    disabled={isCreating}
                                >
                                    <option value="">Select type...</option>
                                    {AGENT_TYPES.map((t) => (
                                        <option key={t.value} value={t.value}>{t.label}</option>
                                    ))}
                                </select>
                                {errors.type && <span className="forge-form__error">{errors.type}</span>}
                            </div>
                            <div className="forge-form__group">
                                <label className="forge-form__label forge-form__label--required">Category</label>
                                <select
                                    className={`forge-form__select ${errors.category ? 'forge-form__select--error' : ''}`}
                                    value={category}
                                    onChange={(e) => setCategory(e.target.value)}
                                    disabled={isCreating}
                                >
                                    <option value="">Select category...</option>
                                    {CATEGORIES.map((c) => (
                                        <option key={c.value} value={c.value}>{c.label}</option>
                                    ))}
                                </select>
                                {errors.category && <span className="forge-form__error">{errors.category}</span>}
                            </div>
                        </div>

                        {/* Row 3: Provider + Model */}
                        <div className="forge-form__row">
                            <div className="forge-form__group">
                                <label className="forge-form__label forge-form__label--required">Provider</label>
                                <select
                                    className={`forge-form__select ${errors.provider ? 'forge-form__select--error' : ''}`}
                                    value={provider}
                                    onChange={(e) => setProvider(e.target.value)}
                                    disabled={isCreating}
                                >
                                    <option value="">Select provider...</option>
                                    {PROVIDERS.map((p) => (
                                        <option key={p.value} value={p.value}>{p.label}</option>
                                    ))}
                                </select>
                                {errors.provider && <span className="forge-form__error">{errors.provider}</span>}
                            </div>
                            <div className="forge-form__group">
                                <label className="forge-form__label forge-form__label--required">Model</label>
                                <input
                                    className={`forge-form__input ${errors.model ? 'forge-form__input--error' : ''}`}
                                    type="text"
                                    value={model}
                                    onChange={(e) => setModel(e.target.value)}
                                    placeholder="gpt-4o"
                                    disabled={isCreating}
                                />
                                {errors.model && <span className="forge-form__error">{errors.model}</span>}
                            </div>
                        </div>

                        {/* Description */}
                        <div className="forge-form__group">
                            <label className="forge-form__label">Description</label>
                            <textarea
                                className="forge-form__textarea"
                                value={description}
                                onChange={(e) => setDescription(e.target.value)}
                                placeholder="What does this agent do?"
                                maxLength={500}
                                disabled={isCreating}
                            />
                        </div>

                        {/* System Prompt */}
                        <div className="forge-form__group">
                            <label className="forge-form__label forge-form__label--required">System Prompt</label>
                            <textarea
                                className={`forge-form__textarea forge-form__textarea--mono ${errors.system_prompt ? 'forge-form__textarea--error' : ''}`}
                                value={systemPrompt}
                                onChange={(e) => setSystemPrompt(e.target.value)}
                                placeholder="You are a helpful agent that..."
                                disabled={isCreating}
                            />
                            {errors.system_prompt && <span className="forge-form__error">{errors.system_prompt}</span>}
                        </div>

                        {/* Footer */}
                        <div className="modal__footer">
                            <button type="button" className="btn btn--outline" onClick={onClose} disabled={isCreating}>
                                Cancel
                            </button>
                            <button type="submit" className="btn btn--primary" disabled={isCreating}>
                                {isCreating ? 'Creating...' : 'Create Agent'}
                            </button>
                        </div>
                    </form>
                </div>
            </div>
        </div>
    );
};

export default CreateAgentModal;
