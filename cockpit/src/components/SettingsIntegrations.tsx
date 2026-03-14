import React, { useState, useCallback, useEffect } from 'react';
import EnterpriseBadge from './EnterpriseBadge';
import { useLicense } from '../hooks/useLicense';

// ============================================================================
// Types
// ============================================================================

export interface ProviderConfig {
    id: string;
    name: string;
    apiKey: string;
    maskedKey: string;
    hasKey: boolean;
    connected: boolean;
    testing: boolean;
}

export interface SatelliteScope {
    id: string;
    name: string;
    providerId: string;
}

export type VaultBackend = 'local' | 'openbao' | 'hashicorp' | 'azure' | 'infisical';

const VAULT_BACKENDS: { id: VaultBackend; name: string; enterprise: boolean }[] = [
    { id: 'local', name: 'Local (Filesystem)', enterprise: false },
    { id: 'openbao', name: 'OpenBao', enterprise: true },
    { id: 'hashicorp', name: 'HashiCorp Vault', enterprise: true },
    { id: 'azure', name: 'Azure Key Vault', enterprise: true },
    { id: 'infisical', name: 'Infisical', enterprise: true },
];

const PROVIDERS = [
    { id: 'anthropic', name: 'Anthropic', icon: '🧠' },
    { id: 'openai', name: 'OpenAI', icon: '💬' },
    { id: 'google', name: 'Google', icon: '🔍' },
    { id: 'minimax', name: 'MiniMax', icon: '⚡' },
    { id: 'azure', name: 'Azure OpenAI', icon: '☁️' },
    { id: 'mistral', name: 'Mistral', icon: '🌊' },
    { id: 'deepseek', name: 'DeepSeek', icon: '🔭' },
    { id: 'xai', name: 'xAI (Grok)', icon: '🚀' },
    { id: 'ollama', name: 'Ollama', icon: '🦙' },
];

// ============================================================================
// Sub-components
// ============================================================================

/** Eye icon for show/hide password toggle */
const EyeIcon: React.FC<{ size?: number }> = ({ size = 16 }) => (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
        <circle cx="12" cy="12" r="3" />
    </svg>
);

const EyeOffIcon: React.FC<{ size?: number }> = ({ size = 16 }) => (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24" />
        <line x1="1" y1="1" x2="23" y2="23" />
    </svg>
);

/** Connection status indicator */
const ConnectionStatus: React.FC<{ connected: boolean; loading?: boolean }> = ({ connected, loading }) => (
    <span style={{
        display: 'inline-flex', alignItems: 'center', gap: '6px', fontSize: 12,
        color: loading ? 'var(--text-muted)' : connected ? 'var(--success)' : 'var(--text-muted)'
    }}>
        <span style={{
            width: 6, height: 6, borderRadius: '50%',
            backgroundColor: loading ? 'var(--text-muted)' : connected ? 'var(--success)' : 'var(--text-muted)',
            animation: loading ? 'pulse 1.5s ease-in-out infinite' : undefined,
        }} />
        {loading ? 'Testing...' : connected ? 'Connected' : 'Not configured'}
    </span>
);

/** Masked API key input with show/hide toggle */
const ApiKeyInput: React.FC<{
    value: string;
    onChange: (value: string) => void;
    placeholder?: string;
}> = ({ value, onChange, placeholder = 'sk-...' }) => {
    const [showKey, setShowKey] = useState(false);

    return (
        <div style={{ position: 'relative', display: 'flex', alignItems: 'center' }}>
            <input
                type={showKey ? 'text' : 'password'}
                value={value}
                onChange={(e) => onChange(e.target.value)}
                placeholder={placeholder}
                style={{
                    flex: 1,
                    padding: '8px 36px 8px 12px',
                    background: 'var(--bg)',
                    border: '1px solid var(--border)',
                    borderRadius: 'var(--radius-md)',
                    color: 'var(--text)',
                    fontSize: 13,
                    fontFamily: 'var(--font-mono)',
                }}
            />
            <button
                type="button"
                onClick={() => setShowKey(!showKey)}
                style={{
                    position: 'absolute',
                    right: 8,
                    background: 'none',
                    border: 'none',
                    cursor: 'pointer',
                    color: 'var(--text-muted)',
                    display: 'flex',
                    alignItems: 'center',
                    padding: 4,
                }}
                title={showKey ? 'Hide API key' : 'Show API key'}
            >
                {showKey ? <EyeOffIcon /> : <EyeIcon />}
            </button>
        </div>
    );
};

/** Provider card component */
const ProviderCard: React.FC<{
    provider: typeof PROVIDERS[number];
    config: ProviderConfig;
    satellites: SatelliteScope[];
    onConfigChange: (config: ProviderConfig) => void;
    onTestConnection: () => void;
    onSatelliteChange: (satelliteId: string) => void;
}> = ({ provider, config, satellites, onConfigChange, onTestConnection, onSatelliteChange }) => {
    return (
        <div style={{
            background: 'var(--bg-elevated)',
            border: '1px solid var(--border)',
            borderRadius: 'var(--radius-lg)',
            padding: 'var(--space-lg)',
        }}>
            {/* Header */}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 'var(--space-md)' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-sm)' }}>
                    <span style={{ fontSize: 20 }}>{provider.icon}</span>
                    <span style={{ fontWeight: 600, fontSize: 15 }}>{provider.name}</span>
                </div>
                <ConnectionStatus connected={config.connected} loading={config.testing} />
            </div>

            {/* API Key Input */}
            <div style={{ marginBottom: 'var(--space-md)' }}>
                <label style={{
                    display: 'block',
                    fontSize: 11,
                    fontWeight: 600,
                    textTransform: 'uppercase',
                    letterSpacing: '0.05em',
                    color: 'var(--text-muted)',
                    marginBottom: 6,
                }}>
                    API Key
                </label>
                <ApiKeyInput
                    value={config.apiKey}
                    onChange={(apiKey) => onConfigChange({ ...config, apiKey })}
                    placeholder={config.hasKey ? config.maskedKey : (provider.id === 'ollama' ? 'http://localhost:11434' : 'sk-...')}
                />
            </div>

            {/* Satellite Scope Assignment */}
            <div style={{ marginBottom: 'var(--space-md)' }}>
                <label style={{
                    display: 'block',
                    fontSize: 11,
                    fontWeight: 600,
                    textTransform: 'uppercase',
                    letterSpacing: '0.05em',
                    color: 'var(--text-muted)',
                    marginBottom: 6,
                }}>
                    Satellite Scope
                </label>
                <select
                    value={satellites.find(s => s.providerId === provider.id)?.id || ''}
                    onChange={(e) => onSatelliteChange(e.target.value)}
                    style={{
                        width: '100%',
                        padding: '8px 12px',
                        background: 'var(--bg)',
                        border: '1px solid var(--border)',
                        borderRadius: 'var(--radius-md)',
                        color: 'var(--text)',
                        fontSize: 13,
                        cursor: 'pointer',
                    }}
                >
                    <option value="">Select a satellite...</option>
                    {satellites.map((sat) => (
                        <option key={sat.id} value={sat.id}>{sat.name}</option>
                    ))}
                </select>
            </div>

            {/* Test Connection Button */}
            <button
                className="btn btn--outline btn--sm"
                onClick={onTestConnection}
                disabled={config.testing || (!config.apiKey && !config.hasKey)}
                style={{
                    width: '100%',
                    opacity: config.testing || (!config.apiKey && !config.hasKey) ? 0.5 : 1,
                }}
            >
                {config.testing ? 'Testing...' : 'Test Connection'}
            </button>
        </div>
    );
};

/** Vault backend selector */
const VaultBackendSelector: React.FC<{
    value: VaultBackend;
    onChange: (backend: VaultBackend) => void;
}> = ({ value, onChange }) => {
    const { isCommunity } = useLicense();

    return (
        <div>
            <label style={{
                display: 'block',
                fontSize: 11,
                fontWeight: 600,
                textTransform: 'uppercase',
                letterSpacing: '0.05em',
                color: 'var(--text-muted)',
                marginBottom: 8,
            }}>
                Secrets Backend
            </label>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(180px, 1fr))', gap: 'var(--space-sm)' }}>
                {VAULT_BACKENDS.map((backend) => (
                    <div
                        key={backend.id}
                        onClick={() => !backend.enterprise && onChange(backend.id)}
                        style={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: 'var(--space-sm)',
                            padding: '10px 12px',
                            background: value === backend.id ? 'var(--accent-muted)' : 'var(--bg)',
                            border: `1px solid ${value === backend.id ? 'var(--accent)' : 'var(--border)'}`,
                            borderRadius: 'var(--radius-md)',
                            cursor: backend.enterprise ? 'not-allowed' : 'pointer',
                            opacity: backend.enterprise && isCommunity ? 0.6 : 1,
                            transition: 'all 150ms ease',
                        }}
                    >
                        <span style={{
                            width: 8,
                            height: 8,
                            borderRadius: '50%',
                            backgroundColor: value === backend.id ? 'var(--accent)' : 'var(--border)',
                        }} />
                        <span style={{ fontSize: 13, fontWeight: 500 }}>{backend.name}</span>
                        {backend.enterprise && <EnterpriseBadge size="small" showText={false} />}
                    </div>
                ))}
            </div>
        </div>
    );
};

// ============================================================================
// Main Component
// ============================================================================

const SettingsIntegrations: React.FC = () => {
    // Provider configurations backed by server-side encrypted storage
    const [providerConfigs, setProviderConfigs] = useState<ProviderConfig[]>([
        { id: 'anthropic', name: 'Anthropic', apiKey: '', maskedKey: '', hasKey: false, connected: false, testing: false },
        { id: 'openai', name: 'OpenAI', apiKey: '', maskedKey: '', hasKey: false, connected: false, testing: false },
        { id: 'google', name: 'Google', apiKey: '', maskedKey: '', hasKey: false, connected: false, testing: false },
        { id: 'minimax', name: 'MiniMax', apiKey: '', maskedKey: '', hasKey: false, connected: false, testing: false },
        { id: 'azure', name: 'Azure OpenAI', apiKey: '', maskedKey: '', hasKey: false, connected: false, testing: false },
        { id: 'mistral', name: 'Mistral', apiKey: '', maskedKey: '', hasKey: false, connected: false, testing: false },
        { id: 'deepseek', name: 'DeepSeek', apiKey: '', maskedKey: '', hasKey: false, connected: false, testing: false },
        { id: 'xai', name: 'xAI (Grok)', apiKey: '', maskedKey: '', hasKey: false, connected: false, testing: false },
        { id: 'ollama', name: 'Ollama', apiKey: '', maskedKey: '', hasKey: false, connected: false, testing: false },
    ]);
    const [loadingProviders, setLoadingProviders] = useState(true);

    // Satellite scope assignments (kept in local state — not sensitive)
    const [satelliteScopes] = useState<SatelliteScope[]>([
        { id: 'sat-1', name: 'Production US-East', providerId: '' },
        { id: 'sat-2', name: 'Production EU-West', providerId: '' },
        { id: 'sat-3', name: 'Development', providerId: '' },
    ]);

    // Vault backend selection (setting, not a secret)
    const [vaultBackend, setVaultBackend] = useState<VaultBackend>('local');

    // Saving state
    const [saving, setSaving] = useState(false);
    const [saveMessage, setSaveMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

    // Load provider configs from backend on mount
    useEffect(() => {
        const loadProviders = async () => {
            try {
                const token = sessionStorage.getItem('oidc_access_token') || localStorage.getItem('auth_token');
                const headers: HeadersInit = {
                    'Content-Type': 'application/json',
                    ...(token ? { Authorization: `Bearer ${token}` } : {}),
                };
                const res = await fetch('/api/v1/config/providers', { headers });
                if (res.ok) {
                    const data = await res.json();
                    if (data.providers) {
                        setProviderConfigs(prev =>
                            prev.map(p => {
                                const remote = data.providers.find((r: { id: string }) => r.id === p.id);
                                if (remote) {
                                    return {
                                        ...p,
                                        maskedKey: remote.masked_key || '',
                                        hasKey: remote.has_key || false,
                                        connected: remote.has_key || false,
                                    };
                                }
                                return p;
                            })
                        );
                    }
                }
            } catch (err) {
                console.error('Failed to load provider configs:', err);
            } finally {
                setLoadingProviders(false);
            }
        };
        loadProviders();
    }, []);

    // Update provider config (local state only — not sent until Save)
    const handleProviderConfigChange = useCallback((updatedConfig: ProviderConfig) => {
        setProviderConfigs((prev) =>
            prev.map((p) => (p.id === updatedConfig.id ? updatedConfig : p))
        );
    }, []);

    // Test connection for a provider
    const handleTestConnection = useCallback(async (providerId: string) => {
        const config = providerConfigs.find((p) => p.id === providerId);
        if (!config) return;

        // Update to testing state
        setProviderConfigs((prev) =>
            prev.map((p) => (p.id === providerId ? { ...p, testing: true } : p))
        );

        // Simulate connection test (in real app, this would call the API)
        setTimeout(() => {
            const isConnected = config.apiKey.length > 0;
            setProviderConfigs((prev) =>
                prev.map((p) =>
                    p.id === providerId
                        ? { ...p, testing: false, connected: isConnected }
                        : p
                )
            );
        }, 1500);
    }, [providerConfigs, setProviderConfigs]);

    // Update satellite scope assignment
    const handleSatelliteChange = useCallback((satelliteId: string) => {
        // This would update the scope in a real implementation
        console.log('Satellite changed:', satelliteId);
    }, []);

    // Save all configurations to the server
    const handleSave = useCallback(async () => {
        setSaving(true);
        setSaveMessage(null);

        try {
            const token = sessionStorage.getItem('oidc_access_token') || localStorage.getItem('auth_token');
            const headers: HeadersInit = {
                'Content-Type': 'application/json',
                ...(token ? { Authorization: `Bearer ${token}` } : {}),
            };

            const payload = {
                providers: providerConfigs.map((p) => ({
                    id: p.id,
                    api_key: p.apiKey, // Only sends new keys; empty = keep existing
                })),
            };

            const response = await fetch('/api/v1/config/providers', {
                method: 'PUT',
                headers,
                body: JSON.stringify(payload),
            });

            if (!response.ok) {
                throw new Error('Failed to save provider configuration');
            }

            const data = await response.json();

            // Update local state with server response (masked keys, hasKey flags)
            if (data.providers) {
                setProviderConfigs(prev =>
                    prev.map(p => {
                        const remote = data.providers.find((r: { id: string }) => r.id === p.id);
                        if (remote) {
                            return {
                                ...p,
                                apiKey: '', // Clear plaintext key from memory
                                maskedKey: remote.masked_key || '',
                                hasKey: remote.has_key || false,
                                connected: remote.has_key || false,
                            };
                        }
                        return p;
                    })
                );
            }

            setSaveMessage({ type: 'success', text: 'Settings saved securely' });
        } catch (error) {
            setSaveMessage({
                type: 'error',
                text: error instanceof Error ? error.message : 'Failed to save settings',
            });
        } finally {
            setSaving(false);
        }
    }, [providerConfigs]);

    return (
        <div>
            {/* Page Header */}
            <div className="page-header">
                <h1 className="page-header-title">Integrations</h1>
                <div className="page-header-subtitle">
                    Configure AI provider API keys and secrets management backend.
                </div>
            </div>

            {/* Provider Cards Grid */}
            <div className="settings-group">
                <div className="settings-group__title">AI Providers</div>
                <div style={{
                    display: 'grid',
                    gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))',
                    gap: 'var(--space-lg)',
                }}>
                    {PROVIDERS.map((provider) => {
                        const config = providerConfigs.find((p) => p.id === provider.id);
                        if (!config) return null;

                        return (
                            <ProviderCard
                                key={provider.id}
                                provider={provider}
                                config={config}
                                satellites={satelliteScopes}
                                onConfigChange={handleProviderConfigChange}
                                onTestConnection={() => handleTestConnection(provider.id)}
                                onSatelliteChange={handleSatelliteChange}
                            />
                        );
                    })}
                </div>
            </div>

            {/* Vault Backend Selection */}
            <div className="settings-group">
                <div className="settings-group__title">Secrets Backend</div>
                <div className="settings-card" style={{ padding: 'var(--space-lg)' }}>
                    <VaultBackendSelector
                        value={vaultBackend}
                        onChange={setVaultBackend}
                    />
                </div>
            </div>

            {/* Save Button */}
            <div style={{
                display: 'flex',
                alignItems: 'center',
                gap: 'var(--space-md)',
                marginTop: 'var(--space-xl)',
                paddingTop: 'var(--space-lg)',
                borderTop: '1px solid var(--border)',
            }}>
                <button
                    className="btn btn--primary"
                    onClick={handleSave}
                    disabled={saving}
                    style={{ minWidth: 120 }}
                >
                    {saving ? 'Saving...' : 'Save Changes'}
                </button>
                {saveMessage && (
                    <span style={{
                        fontSize: 13,
                        color: saveMessage.type === 'success' ? 'var(--success)' : 'var(--error)',
                    }}>
                        {saveMessage.text}
                    </span>
                )}
            </div>

            <style>{`
                @keyframes pulse {
                    0%, 100% { opacity: 1; }
                    50% { opacity: 0.5; }
                }
            `}</style>
        </div>
    );
};

export default SettingsIntegrations;
