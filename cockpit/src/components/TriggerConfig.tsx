/**
 * TriggerConfig — Trigger configuration form for agent scheduling
 *
 * Renders a form for configuring metric-based triggers:
 * - Metric selector (cpu_usage, memory_usage, disk_usage, gpu_usage, error_rate, custom)
 * - Operator selector (gt, lt, gte, lte, eq)
 * - Threshold numeric input
 * - Satellite selector with 'Any satellite' wildcard
 * - Cooldown minutes input (min 5, default 60)
 * - Save/Delete buttons calling appropriate API endpoints
 * - Shows last_triggered_at and cooldown status when active
 * - Enterprise-gated: shows upgrade message if license is community
 */

import React, { useState, useCallback, useEffect } from 'react';
import { getSatellites, apiRequest } from '../api/client';
import { useLicense } from '../hooks/useLicense';
import EnterpriseBadge from './EnterpriseBadge';

// ============================================================================
// Types (inline)
// ============================================================================

export type TriggerMetric = 'cpu_usage' | 'memory_usage' | 'disk_usage' | 'gpu_usage' | 'error_rate' | 'custom';
export type TriggerOperator = 'gt' | 'lt' | 'gte' | 'lte' | 'eq';

export interface TriggerConfig {
    metric: TriggerMetric;
    operator: TriggerOperator;
    threshold: number;
    satellite_id: string | null; // null = any satellite
    cooldown_minutes: number;
}

export interface TriggerStatus {
    enabled: boolean;
    last_triggered_at?: string;
    in_cooldown_until?: string;
}

interface TriggerConfigProps {
    agentId: string;
    trigger?: TriggerConfig;
    status?: TriggerStatus;
    onSave?: (config: TriggerConfig) => void;
    onDelete?: () => void;
}

// ============================================================================
// Constants
// ============================================================================

export const METRIC_OPTIONS: { value: TriggerMetric; label: string }[] = [
    { value: 'cpu_usage', label: 'CPU Usage' },
    { value: 'memory_usage', label: 'Memory Usage' },
    { value: 'disk_usage', label: 'Disk Usage' },
    { value: 'gpu_usage', label: 'GPU Usage' },
    { value: 'error_rate', label: 'Error Rate' },
    { value: 'custom', label: 'Custom' },
];

export const OPERATOR_OPTIONS: { value: TriggerOperator; label: string }[] = [
    { value: 'gt', label: '> (Greater than)' },
    { value: 'lt', label: '< (Less than)' },
    { value: 'gte', label: '>= (Greater or equal)' },
    { value: 'lte', label: '<= (Less or equal)' },
    { value: 'eq', label: '== (Equal)' },
];

export const DEFAULT_TRIGGER_CONFIG: TriggerConfig = {
    metric: 'cpu_usage',
    operator: 'gt',
    threshold: 80,
    satellite_id: null,
    cooldown_minutes: 60,
};

// ============================================================================
// API Functions
// ============================================================================

interface TriggerAPIResponse {
    enabled: boolean;
    metric: TriggerMetric;
    operator: TriggerOperator;
    threshold: number;
    satellite_id: string | null;
    cooldown_minutes: number;
    last_triggered_at?: string;
    in_cooldown_until?: string;
}

async function saveTrigger(agentId: string, config: TriggerConfig): Promise<TriggerAPIResponse> {
    return apiRequest<TriggerAPIResponse>(`/agents/${agentId}/trigger`, {
        method: 'PUT',
        body: JSON.stringify(config),
    });
}

async function deleteTrigger(agentId: string): Promise<{ status: string }> {
    return apiRequest<{ status: string }>(`/agents/${agentId}/trigger`, {
        method: 'DELETE',
    });
}

// ============================================================================
// Component
// ============================================================================

export default function TriggerConfig({
    agentId,
    trigger,
    status,
    onSave,
    onDelete,
}: TriggerConfigProps) {
    const { isCommunity } = useLicense();
    const [config, setConfig] = useState<TriggerConfig>(trigger || DEFAULT_TRIGGER_CONFIG);
    const [satellites, setSatellites] = useState<{ id: string; name: string }[]>([]);
    const [loading, setLoading] = useState(false);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState<string | null>(null);

    // Fetch satellites on mount
    useEffect(() => {
        getSatellites()
            .then((sats) => {
                setSatellites(sats.map((s) => ({ id: s.id, name: s.name })));
            })
            .catch(() => {
                // Silently fail - satellites are optional
            });
    }, []);

    // Update config when trigger prop changes
    useEffect(() => {
        if (trigger) {
            setConfig(trigger);
        }
    }, [trigger]);

    const updateConfig = useCallback(<K extends keyof TriggerConfig>(key: K, value: TriggerConfig[K]) => {
        setConfig((prev) => ({ ...prev, [key]: value }));
    }, []);

    const handleSave = async () => {
        setSaving(true);
        setError(null);
        try {
            await saveTrigger(agentId, config);
            onSave?.(config);
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Failed to save trigger');
        } finally {
            setSaving(false);
        }
    };

    const handleDelete = async () => {
        setSaving(true);
        setError(null);
        try {
            await deleteTrigger(agentId);
            onDelete?.();
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Failed to delete trigger');
        } finally {
            setSaving(false);
        }
    };

    const hasTrigger = status?.enabled || trigger !== undefined;
    const isInCooldown = status?.in_cooldown_until && new Date(status.in_cooldown_until) > new Date();

    // Enterprise gate
    if (isCommunity) {
        return (
            <div className="trigger-config trigger-config--locked">
                <div className="trigger-config__locked-message">
                    <EnterpriseBadge />
                    <p>
                        Triggers are planned for a future DAAO Enterprise release.
                    </p>
                </div>
            </div>
        );
    }

    return (
        <div className="trigger-config">
            <div className="trigger-config__header">
                <h3 className="trigger-config__title">Trigger Configuration</h3>
                {hasTrigger && (
                    <div className="trigger-config__status">
                        {status?.last_triggered_at && (
                            <span className="trigger-config__last-triggered">
                                Last triggered: {new Date(status.last_triggered_at).toLocaleString()}
                            </span>
                        )}
                        {isInCooldown && (
                            <span className="trigger-config__cooldown-badge badge badge--warning">
                                In Cooldown
                            </span>
                        )}
                    </div>
                )}
            </div>

            {error && (
                <div className="trigger-config__error">
                    {error}
                </div>
            )}

            <div className="trigger-config__form">
                {/* Metric Selector */}
                <div className="trigger-config__field">
                    <label className="trigger-config__label" htmlFor="trigger-metric">
                        Metric
                    </label>
                    <select
                        id="trigger-metric"
                        className="trigger-config__select"
                        value={config.metric}
                        onChange={(e) => updateConfig('metric', e.target.value as TriggerMetric)}
                    >
                        {METRIC_OPTIONS.map((opt) => (
                            <option key={opt.value} value={opt.value}>
                                {opt.label}
                            </option>
                        ))}
                    </select>
                </div>

                {/* Operator Selector */}
                <div className="trigger-config__field">
                    <label className="trigger-config__label" htmlFor="trigger-operator">
                        Operator
                    </label>
                    <select
                        id="trigger-operator"
                        className="trigger-config__select"
                        value={config.operator}
                        onChange={(e) => updateConfig('operator', e.target.value as TriggerOperator)}
                    >
                        {OPERATOR_OPTIONS.map((opt) => (
                            <option key={opt.value} value={opt.value}>
                                {opt.label}
                            </option>
                        ))}
                    </select>
                </div>

                {/* Threshold Input */}
                <div className="trigger-config__field">
                    <label className="trigger-config__label" htmlFor="trigger-threshold">
                        Threshold
                    </label>
                    <input
                        id="trigger-threshold"
                        type="number"
                        className="trigger-config__input"
                        value={config.threshold}
                        onChange={(e) => updateConfig('threshold', parseFloat(e.target.value) || 0)}
                        min={0}
                        max={100}
                        step={0.1}
                    />
                </div>

                {/* Satellite Selector */}
                <div className="trigger-config__field">
                    <label className="trigger-config__label" htmlFor="trigger-satellite">
                        Satellite
                    </label>
                    <select
                        id="trigger-satellite"
                        className="trigger-config__select"
                        value={config.satellite_id || ''}
                        onChange={(e) => updateConfig('satellite_id', e.target.value || null)}
                    >
                        <option value="">Any satellite</option>
                        {satellites.map((sat) => (
                            <option key={sat.id} value={sat.id}>
                                {sat.name}
                            </option>
                        ))}
                    </select>
                </div>

                {/* Cooldown Input */}
                <div className="trigger-config__field">
                    <label className="trigger-config__label" htmlFor="trigger-cooldown">
                        Cooldown (minutes)
                    </label>
                    <input
                        id="trigger-cooldown"
                        type="number"
                        className="trigger-config__input"
                        value={config.cooldown_minutes}
                        onChange={(e) => {
                            const val = parseInt(e.target.value, 10) || 60;
                            updateConfig('cooldown_minutes', Math.max(5, val));
                        }}
                        min={5}
                        max={1440}
                    />
                    <span className="trigger-config__hint">Minimum 5 minutes</span>
                </div>
            </div>

            {/* Action Buttons */}
            <div className="trigger-config__actions">
                <button
                    type="button"
                    className="btn btn--primary"
                    onClick={handleSave}
                    disabled={saving}
                >
                    {saving ? 'Saving...' : 'Save'}
                </button>
                {hasTrigger && (
                    <button
                        type="button"
                        className="btn btn--danger"
                        onClick={handleDelete}
                        disabled={saving}
                    >
                        {saving ? 'Deleting...' : 'Delete'}
                    </button>
                )}
            </div>
        </div>
    );
}
