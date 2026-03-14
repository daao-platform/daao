/**
 * AgentRoutingConfig — Agent routing configuration for auto-dispatch
 *
 * Features:
 * - Toggle switch for auto-dispatch mode
 * - Required tags chip input
 * - Preferred tags chip input
 * - Enterprise-gated disabled state with tooltip
 */

import React, { useState, useCallback, useEffect } from 'react';
import './AgentRoutingConfig.css';

// ============================================================================
// Types
// ============================================================================

export interface RoutingConfig {
    mode: 'targeted' | 'auto-dispatch';
    require_tags?: string[];
    prefer_tags?: string[];
}

interface AgentRoutingConfigProps {
    /** Current routing config value */
    value: RoutingConfig | null;
    /** Callback when config changes */
    onChange: (config: RoutingConfig | null) => void;
    /** Whether the component is disabled (for community users) */
    disabled?: boolean;
}

// ============================================================================
// Component
// ============================================================================

export const AgentRoutingConfig: React.FC<AgentRoutingConfigProps> = ({
    value,
    onChange,
    disabled = false,
}) => {
    // Determine if auto-dispatch is enabled based on value
    const isAutoDispatch = value !== null && value.mode === 'auto-dispatch';

    // Internal state for tags
    const [requireTags, setRequireTags] = useState<string[]>(value?.require_tags || []);
    const [preferTags, setPreferTags] = useState<string[]>(value?.prefer_tags || []);
    const [requireTagInput, setRequireTagInput] = useState('');
    const [preferTagInput, setPreferTagInput] = useState('');

    // Sync internal state when value prop changes
    useEffect(() => {
        if (value) {
            setRequireTags(value.require_tags || []);
            setPreferTags(value.prefer_tags || []);
        } else {
            setRequireTags([]);
            setPreferTags([]);
        }
    }, [value]);

    // Add a tag to a list
    const addTag = useCallback((
        tag: string,
        currentTags: string[],
        setTags: React.Dispatch<React.SetStateAction<string[]>>
    ) => {
        const trimmedTag = tag.trim();
        if (trimmedTag && !currentTags.includes(trimmedTag)) {
            setTags([...currentTags, trimmedTag]);
        }
    }, []);

    // Remove a tag from a list
    const removeTag = useCallback((
        tagToRemove: string,
        currentTags: string[],
        setTags: React.Dispatch<React.SetStateAction<string[]>>
    ) => {
        setTags(currentTags.filter(tag => tag !== tagToRemove));
    }, []);

    // Handle toggle change
    const handleToggleChange = useCallback((enabled: boolean) => {
        if (enabled) {
            // Enable auto-dispatch with current tags
            onChange({
                mode: 'auto-dispatch',
                require_tags: requireTags,
                prefer_tags: preferTags,
            });
        } else {
            // Disable auto-dispatch - return to targeted mode
            onChange(null);
        }
    }, [onChange, requireTags, preferTags]);

    // Handle requiring adding a required tag
    const handleRequireTagKeyDown = useCallback((e: React.KeyboardEvent<HTMLInputElement>) => {
        if (e.key === 'Enter' && requireTagInput.trim()) {
            e.preventDefault();
            addTag(requireTagInput, requireTags, setRequireTags);
            setRequireTagInput('');

            // Update parent if auto-dispatch is enabled
            if (isAutoDispatch) {
                onChange({
                    mode: 'auto-dispatch',
                    require_tags: [...requireTags, requireTagInput.trim()],
                    prefer_tags: preferTags,
                });
            }
        }
    }, [requireTagInput, requireTags, preferTags, isAutoDispatch, addTag, onChange]);

    // Handle requiring removing a required tag
    const handleRemoveRequireTag = useCallback((tag: string) => {
        removeTag(tag, requireTags, setRequireTags);

        if (isAutoDispatch) {
            onChange({
                mode: 'auto-dispatch',
                require_tags: requireTags.filter(t => t !== tag),
                prefer_tags: preferTags,
            });
        }
    }, [requireTags, preferTags, isAutoDispatch, removeTag, onChange]);

    // Handle preferred tag input
    const handlePreferTagKeyDown = useCallback((e: React.KeyboardEvent<HTMLInputElement>) => {
        if (e.key === 'Enter' && preferTagInput.trim()) {
            e.preventDefault();
            addTag(preferTagInput, preferTags, setPreferTags);
            setPreferTagInput('');

            // Update parent if auto-dispatch is enabled
            if (isAutoDispatch) {
                onChange({
                    mode: 'auto-dispatch',
                    require_tags: requireTags,
                    prefer_tags: [...preferTags, preferTagInput.trim()],
                });
            }
        }
    }, [preferTagInput, preferTags, requireTags, isAutoDispatch, addTag, onChange]);

    // Handle removing a preferred tag
    const handleRemovePreferTag = useCallback((tag: string) => {
        removeTag(tag, preferTags, setPreferTags);

        if (isAutoDispatch) {
            onChange({
                mode: 'auto-dispatch',
                require_tags: requireTags,
                prefer_tags: preferTags.filter(t => t !== tag),
            });
        }
    }, [requireTags, preferTags, isAutoDispatch, removeTag, onChange]);

    return (
        <div className="routing-config">
            <div className="routing-config__header">
                <h3 className="routing-config__title">Routing</h3>
            </div>

            {/* Toggle Switch */}
            <div className="routing-config-toggle">
                <div className="routing-config-toggle__label">
                    <span className="routing-config-toggle__label-text">
                        Auto-dispatch to best available satellite
                    </span>
                    <span className="routing-config-toggle__label-hint">
                        Automatically route agent to optimal satellite based on tags
                    </span>
                </div>
                <div className="routing-config-toggle__tooltip">
                    <label className="routing-config-toggle__switch">
                        <input
                            type="checkbox"
                            checked={isAutoDispatch}
                            onChange={(e) => handleToggleChange(e.target.checked)}
                            disabled={disabled}
                        />
                        <span className="routing-config-toggle__slider"></span>
                    </label>
                    {disabled && (
                        <span className="routing-config-toggle__tooltip-text">
                            Coming soon — Enterprise
                        </span>
                    )}
                </div>
            </div>

            {/* Tag Inputs - shown when auto-dispatch is enabled */}
            {isAutoDispatch && (
                <div className="routing-config-tags">
                    {/* Required Tags */}
                    <div className="routing-config-tags__section">
                        <label className="routing-config-tags__label">
                            Required Tags
                            <span className="routing-config-tags__label-hint">
                                {' '}— Agent will ONLY run on satellites with ALL these tags
                            </span>
                        </label>
                        <div className="routing-config-tags__input-wrapper">
                            {requireTags.map((tag) => (
                                <span key={tag} className="routing-config-tags__chip">
                                    {tag}
                                    <button
                                        type="button"
                                        className="routing-config-tags__chip-remove"
                                        onClick={() => handleRemoveRequireTag(tag)}
                                        aria-label={`Remove ${tag}`}
                                    >
                                        ×
                                    </button>
                                </span>
                            ))}
                            <input
                                type="text"
                                className="routing-config-tags__input"
                                placeholder={requireTags.length === 0 ? "Enter tag and press Enter" : ""}
                                value={requireTagInput}
                                onChange={(e) => setRequireTagInput(e.target.value)}
                                onKeyDown={handleRequireTagKeyDown}
                            />
                        </div>
                        {requireTags.length === 0 && !requireTagInput && (
                            <span className="routing-config-tags__empty">
                                No required tags — will match any satellite
                            </span>
                        )}
                    </div>

                    {/* Preferred Tags */}
                    <div className="routing-config-tags__section">
                        <label className="routing-config-tags__label">
                            Preferred Tags
                            <span className="routing-config-tags__label-hint">
                                {' '}— Agent PREFERS satellites with these tags (if available)
                            </span>
                        </label>
                        <div className="routing-config-tags__input-wrapper">
                            {preferTags.map((tag) => (
                                <span key={tag} className="routing-config-tags__chip">
                                    {tag}
                                    <button
                                        type="button"
                                        className="routing-config-tags__chip-remove"
                                        onClick={() => handleRemovePreferTag(tag)}
                                        aria-label={`Remove ${tag}`}
                                    >
                                        ×
                                    </button>
                                </span>
                            ))}
                            <input
                                type="text"
                                className="routing-config-tags__input"
                                placeholder={preferTags.length === 0 ? "Enter tag and press Enter" : ""}
                                value={preferTagInput}
                                onChange={(e) => setPreferTagInput(e.target.value)}
                                onKeyDown={handlePreferTagKeyDown}
                            />
                        </div>
                        {preferTags.length === 0 && !preferTagInput && (
                            <span className="routing-config-tags__empty">
                                No preferred tags — will select any available satellite
                            </span>
                        )}
                    </div>
                </div>
            )}
        </div>
    );
};

export default AgentRoutingConfig;
