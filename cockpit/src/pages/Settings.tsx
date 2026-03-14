import React, { useState, useEffect, useRef, useCallback } from 'react';
import { ChevronRightIcon, UserIcon } from '../components/Icons';
import { useLocalStorage } from '../hooks';
import { useLicense } from '../hooks/useLicense';
import { useAuth } from '../auth/AuthProvider';
import { createPushNotificationManager, PushNotificationManager } from '../push';
import { getSessions, getSatellites, listAllRecordings, getRecordingConfig, setRecordingConfig } from '../api/client';
import UpgradeCard from '../components/UpgradeCard';
import EnterpriseBadge from '../components/EnterpriseBadge';

// ============================================================================
// Color scheme definitions with preview colors
// ============================================================================
const COLOR_SCHEMES = [
    {
        name: 'Tokyo Night',
        key: 'tokyo-night',
        bg: '#0F172A',
        surface: '#1E293B',
        accent: '#2DD4BF',
        text: '#F1F5F9',
    },
    {
        name: 'Dracula',
        key: 'dracula',
        bg: '#282A36',
        surface: '#343746',
        accent: '#BD93F9',
        text: '#F8F8F2',
    },
    {
        name: 'Monokai',
        key: 'monokai',
        bg: '#272822',
        surface: '#2E2F2A',
        accent: '#A6E22E',
        text: '#F8F8F2',
    },
    {
        name: 'Solarized',
        key: 'solarized',
        bg: '#002B36',
        surface: '#073642',
        accent: '#2AA198',
        text: '#FDF6E3',
    },
] as const;

const FONT_SIZE_OPTIONS = [
    { label: 'Small', value: 12, desc: 'Compact' },
    { label: 'Default', value: 14, desc: 'Recommended' },
    { label: 'Medium', value: 16, desc: 'Comfortable' },
    { label: 'Large', value: 18, desc: 'Easy reading' },
    { label: 'XL', value: 20, desc: 'Accessibility' },
];

// ============================================================================
// Sub-components
// ============================================================================

const Toggle: React.FC<{ value: boolean; onChange: (v: boolean) => void }> = ({ value, onChange }) => (
    <div
        className={`toggle${value ? ' active' : ''}`}
        onClick={() => onChange(!value)}
        role="switch"
        aria-checked={value}
        tabIndex={0}
        onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') onChange(!value); }}
    />
);

const SettingsRow: React.FC<{
    label: string;
    value?: string;
    toggle?: boolean;
    toggleValue?: boolean;
    onToggle?: (v: boolean) => void;
    onClick?: () => void;
    subtitle?: string;
}> = ({ label, value, toggle, toggleValue, onToggle, onClick, subtitle }) => (
    <div className="settings-row" onClick={onClick} style={onClick ? { cursor: 'pointer' } : undefined}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <span className="settings-row__label">{label}</span>
            {subtitle && <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>{subtitle}</span>}
        </div>
        <span className="settings-row__value">
            {toggle && onToggle ? (
                <Toggle value={toggleValue || false} onChange={onToggle} />
            ) : (
                <>
                    {value && <span>{value}</span>}
                    {onClick && <ChevronRightIcon size={16} className="settings-row__chevron" />}
                </>
            )}
        </span>
    </div>
);

/** Connection Status */
const ConnectionStatus: React.FC<{ connected: boolean; loading?: boolean }> = ({ connected, loading }) => (
    <span style={{
        display: 'inline-flex', alignItems: 'center', gap: '6px', fontSize: 13,
        color: loading ? 'var(--text-muted)' : connected ? 'var(--success)' : 'var(--error)'
    }}>
        <span style={{
            width: 8, height: 8, borderRadius: '50%',
            backgroundColor: loading ? 'var(--text-muted)' : connected ? 'var(--success)' : 'var(--error)',
            animation: loading ? 'pulse 1.5s ease-in-out infinite' : undefined,
        }} />
        {loading ? 'Checking...' : connected ? 'Connected' : 'Disconnected'}
    </span>
);

/** Expandable panel for inline settings pickers */
const ExpandablePanel: React.FC<{
    label: string;
    value: string;
    open: boolean;
    onToggle: () => void;
    children: React.ReactNode;
}> = ({ label, value, open, onToggle, children }) => (
    <div>
        <div className="settings-row" onClick={onToggle} style={{ cursor: 'pointer' }}>
            <span className="settings-row__label">{label}</span>
            <span className="settings-row__value">
                <span>{value}</span>
                <span style={{
                    transform: open ? 'rotate(90deg)' : 'rotate(0)',
                    transition: 'transform 200ms ease',
                    display: 'inline-flex',
                }}>
                    <ChevronRightIcon size={16} className="settings-row__chevron" />
                </span>
            </span>
        </div>
        {open && (
            <div style={{
                padding: '0 var(--space-lg) var(--space-lg)',
                animation: 'fadeIn 200ms ease',
            }}>
                {children}
            </div>
        )}
    </div>
);

/** Modal overlay */
const Modal: React.FC<{
    open: boolean;
    onClose: () => void;
    title: string;
    children: React.ReactNode;
}> = ({ open, onClose, title, children }) => {
    if (!open) return null;
    return (
        <div className="modal-overlay" onClick={onClose}>
            <div className="modal-content" onClick={(e) => e.stopPropagation()}>
                <div className="modal-header">
                    <h3>{title}</h3>
                    <button className="btn btn--icon btn--outline" onClick={onClose}>✕</button>
                </div>
                {children}
            </div>
        </div>
    );
};

// ============================================================================
// Main Settings Component
// ============================================================================
const Settings: React.FC = () => {
    // Preferences stored in localStorage
    const [theme, setTheme] = useLocalStorage<string>('settings-theme', 'dark');
    const [colorScheme, setColorScheme] = useLocalStorage<string>('settings-color-scheme', 'tokyo-night');
    const [notificationsEnabled, setNotificationsEnabled] = useLocalStorage<boolean>('settings-notifications', true);
    const [terminalFontSize, setTerminalFontSize] = useLocalStorage<number>('settings-font-size', 14);

    // Notification sub-toggles
    const [sessionChanges, setSessionChanges] = useLocalStorage<boolean>('settings-notif-session', true);
    const [agentErrors, setAgentErrors] = useLocalStorage<boolean>('settings-notif-agent-errors', true);
    const [dmsWarnings, setDmsWarnings] = useLocalStorage<boolean>('settings-notif-dms', true);
    const [autoReconnect, setAutoReconnect] = useLocalStorage<boolean>('settings-auto-reconnect', true);

    // Expandable panel states
    const [themeOpen, setThemeOpen] = useState(false);
    const [colorSchemeOpen, setColorSchemeOpen] = useState(false);
    const [fontSizeOpen, setFontSizeOpen] = useState(false);

    // Modal states
    const [sshKeysModalOpen, setSshKeysModalOpen] = useState(false);
    const [apiTokensModalOpen, setApiTokensModalOpen] = useState(false);

    // SSH Keys state
    const [sshKeys, setSshKeys] = useLocalStorage<Array<{ name: string; fingerprint: string; created: string }>>('settings-ssh-keys', []);
    const [newKeyName, setNewKeyName] = useState('');
    const [newKeyValue, setNewKeyValue] = useState('');

    // API Tokens state
    const [apiTokens, setApiTokens] = useLocalStorage<Array<{ name: string; token: string; created: string; lastUsed: string }>>('settings-api-tokens', []);
    const [newTokenName, setNewTokenName] = useState('');
    const [generatedToken, setGeneratedToken] = useState('');

    // Password change state
    const [currentPassword, setCurrentPassword] = useState('');
    const [newPassword, setNewPassword] = useState('');
    const [confirmPassword, setConfirmPassword] = useState('');
    const [passwordLoading, setPasswordLoading] = useState(false);
    const [passwordMessage, setPasswordMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

    // Auth context
    const { accessToken, user } = useAuth();

    // Push notification manager
    const pushManagerRef = useRef<PushNotificationManager | null>(null);

    // Connection status
    const [nexusConnected, setNexusConnected] = useState(false);
    const [connectionLoading, setConnectionLoading] = useState(true);
    const [sessionCount, setSessionCount] = useState(0);
    const [sessionLoading, setSessionLoading] = useState(true);
    const [satelliteCount, setSatelliteCount] = useState(0);
    const [recordingCount, setRecordingCount] = useState(0);

    // Recording config
    const [recordingEnabled, setRecordingEnabled] = useState(true);

    // License context
    const { license, isCommunity } = useLicense();

    // Apply theme to document
    useEffect(() => { document.documentElement.setAttribute('data-theme', theme); }, [theme]);
    useEffect(() => { document.documentElement.setAttribute('data-color-scheme', colorScheme); }, [colorScheme]);

    // Initialize push manager
    useEffect(() => {
        pushManagerRef.current = createPushNotificationManager({
            vapidPublicKey: 'BEl62iUYgUivxIkv69yViEuiBIa-Ib9-SkvMeAtA3LFgDzkrxZJjSgSnfckjBJuBkr3qBUYIHBQFLXYp5Nksh8U',
        });
    }, []);

    // Health check
    const checkHealth = useCallback(async () => {
        setConnectionLoading(true);
        try {
            const response = await fetch('/health');
            setNexusConnected(response.ok);
        } catch { setNexusConnected(false); }
        finally { setConnectionLoading(false); }
    }, []);

    useEffect(() => { checkHealth(); }, [checkHealth]);

    // Fetch session count
    useEffect(() => {
        (async () => {
            setSessionLoading(true);
            try { const s = await getSessions(); setSessionCount(s.items?.length ?? 0); }
            catch { setSessionCount(0); }
            finally { setSessionLoading(false); }
        })();
        // Also fetch satellite and recording counts
        (async () => {
            try { const sats = await getSatellites(); setSatelliteCount(sats.length); }
            catch { /* */ }
        })();
        (async () => {
            try { const recs = await listAllRecordings(); setRecordingCount(recs.length); }
            catch { /* */ }
        })();
    }, []);

    // Fetch recording config
    useEffect(() => {
        (async () => {
            try {
                const config = await getRecordingConfig();
                setRecordingEnabled(config.recording_enabled);
            } catch { /* default to true */ }
        })();
    }, []);

    // Push notification toggle
    const handlePushToggle = async (enabled: boolean) => {
        setNotificationsEnabled(enabled);
        if (!pushManagerRef.current) return;
        if (enabled) await pushManagerRef.current.subscribe();
        else await pushManagerRef.current.unsubscribe();
    };

    // Recording toggle
    const handleRecordingToggle = async (enabled: boolean) => {
        setRecordingEnabled(enabled);
        try {
            await setRecordingConfig(enabled);
        } catch {
            setRecordingEnabled(!enabled); // revert on failure
        }
    };

    // SSH Key management
    const handleAddSshKey = () => {
        if (!newKeyName.trim() || !newKeyValue.trim()) return;
        const fingerprint = 'SHA256:' + btoa(newKeyValue.slice(0, 20)).slice(0, 43);
        setSshKeys([...sshKeys, {
            name: newKeyName.trim(),
            fingerprint,
            created: new Date().toISOString().split('T')[0],
        }]);
        setNewKeyName('');
        setNewKeyValue('');
    };

    const handleRemoveSshKey = (index: number) => {
        setSshKeys(sshKeys.filter((_, i) => i !== index));
    };

    // API Token management
    const handleGenerateToken = () => {
        if (!newTokenName.trim()) return;
        const token = 'daao_' + Array.from(crypto.getRandomValues(new Uint8Array(24)))
            .map(b => b.toString(16).padStart(2, '0')).join('');
        setGeneratedToken(token);
        setApiTokens([...apiTokens, {
            name: newTokenName.trim(),
            token: token.slice(0, 12) + '...' + token.slice(-4),
            created: new Date().toISOString().split('T')[0],
            lastUsed: 'Never',
        }]);
        setNewTokenName('');
    };

    const handleRevokeToken = (index: number) => {
        setApiTokens(apiTokens.filter((_, i) => i !== index));
    };

    const copyToClipboard = (text: string) => {
        navigator.clipboard.writeText(text).catch(() => {
            const el = document.createElement('textarea');
            el.value = text;
            document.body.appendChild(el);
            el.select();
            document.execCommand('copy');
            document.body.removeChild(el);
        });
    };

    // Password change handler
    const handleChangePassword = async (e: React.FormEvent) => {
        e.preventDefault();
        if (newPassword !== confirmPassword) {
            setPasswordMessage({ type: 'error', text: 'New passwords do not match' });
            return;
        }
        if (newPassword.length < 8) {
            setPasswordMessage({ type: 'error', text: 'Password must be at least 8 characters' });
            return;
        }
        setPasswordLoading(true);
        setPasswordMessage(null);
        try {
            const res = await fetch('/api/v1/auth/change-password', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    Authorization: `Bearer ${accessToken}`,
                },
                body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
            });
            if (!res.ok) {
                const err = await res.json().catch(() => ({ error: 'Failed to change password' }));
                throw new Error(err.error || 'Failed to change password');
            }
            setPasswordMessage({ type: 'success', text: 'Password changed successfully' });
            setCurrentPassword('');
            setNewPassword('');
            setConfirmPassword('');
        } catch (err) {
            setPasswordMessage({ type: 'error', text: err instanceof Error ? err.message : 'Failed to change password' });
        } finally {
            setPasswordLoading(false);
        }
    };

    // User info — prefer auth context, fall back to OIDC session storage
    const getUserDisplayName = () => {
        // Try auth context first (local auth)
        if (user?.name) return user.name;
        if (user?.email) return user.email.split('@')[0];
        // Try OIDC session
        try {
            const u = sessionStorage.getItem('oidc_user_info');
            if (u) { const p = JSON.parse(u); return p.name || p.preferred_username || 'User'; }
        } catch { /* */ }
        return 'User';
    };
    const getUserEmail = () => {
        if (user?.email) return user.email;
        try {
            const u = sessionStorage.getItem('oidc_user_info');
            if (u) return JSON.parse(u).email || '';
        } catch { /* */ }
        return '';
    };
    const isOidc = !!sessionStorage.getItem('oidc_user_info');

    return (
        <div>
            <div className="page-header">
                <h1 className="page-header-title">Settings</h1>
                <div className="page-header-subtitle">Configure your DAAO experience and manage infrastructure policies.</div>
            </div>

            {/* ===== Two-Column Hero: User + License ===== */}
            <div className="settings-hero">
                {/* Left: User Card */}
                <div className="settings-card settings-hero__card">
                    <div className="settings-hero__user">
                        <div style={{
                            width: 52, height: 52, borderRadius: 'var(--radius-full)',
                            background: 'var(--bg-elevated)', display: 'flex',
                            alignItems: 'center', justifyContent: 'center',
                        }}>
                            <UserIcon size={24} />
                        </div>
                        <div>
                            <div style={{ fontWeight: 700, fontSize: 16, color: 'var(--text)' }}>{getUserDisplayName()}</div>
                            <div style={{ fontSize: 12, color: 'var(--text-muted)', marginTop: 2 }}>
                                {getUserEmail()}
                            </div>
                        </div>
                    </div>
                    <div style={{ marginTop: 12, fontSize: 12, color: 'var(--text-muted)', display: 'flex', alignItems: 'center', gap: 6 }}>
                        <span style={{ width: 6, height: 6, borderRadius: '50%', background: 'var(--success)', display: 'inline-block' }} />
                        {isOidc ? 'SSO/OIDC' : 'Local account'}
                        {user?.role && <span style={{ marginLeft: 8, padding: '1px 8px', borderRadius: 'var(--radius-sm)', background: 'var(--bg-elevated)', fontSize: 11, fontWeight: 600, textTransform: 'uppercase' }}>{user.role}</span>}
                    </div>
                </div>

                {/* Right: License Card */}
                <div className="settings-card settings-hero__card settings-hero__license">
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 16 }}>
                        <div>
                            <div style={{ fontWeight: 700, fontSize: 16, color: 'var(--text)' }}>
                                DAAO {isCommunity ? 'Community' : 'Enterprise'}
                            </div>
                            <div style={{ fontSize: 12, color: 'var(--text-muted)', marginTop: 2 }}>
                                {isCommunity ? 'Free tier' : 'Licensed'}
                            </div>
                        </div>
                        <div style={{
                            width: 36, height: 36, borderRadius: '50%',
                            background: isCommunity ? 'rgba(45, 212, 191, 0.15)' : 'rgba(45, 212, 191, 0.3)',
                            display: 'flex', alignItems: 'center', justifyContent: 'center',
                            fontSize: 18,
                        }}>
                            {isCommunity ? '🌱' : '⭐'}
                        </div>
                    </div>

                    {license && (
                        <div style={{ display: 'flex', flexDirection: 'column', gap: 10, marginBottom: 16 }}>
                            {license.max_recordings > 0 && (
                                <div className="settings-usage-row">
                                    <span>Recordings</span>
                                    <span>{recordingCount} / {license.max_recordings}</span>
                                </div>
                            )}
                            {license.max_satellites > 0 && (
                                <div className="settings-usage-row">
                                    <span>Satellites</span>
                                    <span>{satelliteCount} / {license.max_satellites}</span>
                                </div>
                            )}
                            {license.max_users > 0 && (
                                <div className="settings-usage-row">
                                    <span>Users</span>
                                    <span>1 / {license.max_users}</span>
                                </div>
                            )}
                            <div className="settings-usage-row">
                                <span>Telemetry Retention</span>
                                <span>1h</span>
                            </div>
                        </div>
                    )}

                    {isCommunity && (
                        <a href="https://daao.dev/pricing" target="_blank" rel="noopener noreferrer"
                            className="btn btn--primary" style={{ width: '100%', justifyContent: 'center', textDecoration: 'none', gap: 6 }}>
                            ENTERPRISE COMING SOON <span>↗</span>
                        </a>
                    )}

                    {isCommunity && (
                        <div style={{ marginTop: 12, fontSize: 11, color: 'var(--text-muted)', display: 'flex', gap: 6, alignItems: 'flex-start' }}>
                            <span style={{ color: 'var(--warning)', flexShrink: 0 }}>⚠</span>
                            <span>Enterprise features are in development. Visit our site to register interest and get notified when available.</span>
                        </div>
                    )}
                </div>
            </div>

            {/* ===== Appearance ===== */}
            <div className="settings-group">
                <div className="settings-group__title">Appearance</div>
                <div className="settings-card" style={{ padding: '20px 24px' }}>
                    {/* Interface Theme */}
                    <div style={{ marginBottom: 20 }}>
                        <div style={{ fontSize: 10, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.1em', color: 'var(--text-muted)', marginBottom: 10 }}>
                            Interface Theme
                        </div>
                        <div style={{ display: 'flex', gap: 8 }}>
                            {(['light', 'dark', 'system'] as const).map((t) => (
                                <button
                                    key={t}
                                    className={`btn btn--sm ${theme === t ? 'btn--primary' : 'btn--outline'}`}
                                    onClick={() => setTheme(t === 'system' ? 'dark' : t)}
                                    style={{ minWidth: 72, gap: 4 }}
                                >
                                    {t === 'light' ? '☀️' : t === 'dark' ? '🌙' : '💻'} {t.charAt(0).toUpperCase() + t.slice(1)}
                                </button>
                            ))}
                        </div>
                    </div>

                    {/* Syntax Highlighting */}
                    <div style={{ marginBottom: 20 }}>
                        <div style={{ fontSize: 10, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.1em', color: 'var(--text-muted)', marginBottom: 10 }}>
                            Syntax Highlighting
                        </div>
                        <div style={{ display: 'flex', gap: 16, alignItems: 'center' }}>
                            {COLOR_SCHEMES.map((scheme) => (
                                <div key={scheme.key} style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 6, cursor: 'pointer' }}
                                    onClick={() => setColorScheme(scheme.key)}>
                                    <div style={{
                                        width: 32, height: 32, borderRadius: '50%',
                                        background: scheme.accent,
                                        border: colorScheme === scheme.key ? '3px solid var(--text)' : '3px solid transparent',
                                        boxShadow: colorScheme === scheme.key ? `0 0 0 2px ${scheme.accent}` : 'none',
                                        transition: 'all 200ms ease',
                                    }} />
                                    <span style={{ fontSize: 10, color: colorScheme === scheme.key ? 'var(--text)' : 'var(--text-muted)' }}>
                                        {scheme.name.split(' ')[0]}
                                    </span>
                                </div>
                            ))}
                        </div>
                    </div>

                    {/* Terminal Font Size */}
                    <div>
                        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                            <span style={{ fontSize: 10, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.1em', color: 'var(--text-muted)' }}>
                                Terminal Font Size
                            </span>
                            <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--accent)' }}>{terminalFontSize}px</span>
                        </div>
                        <input
                            type="range"
                            min={10}
                            max={24}
                            step={1}
                            value={terminalFontSize}
                            onChange={(e) => setTerminalFontSize(parseInt(e.target.value))}
                            style={{ width: '100%', accentColor: 'var(--accent)' }}
                        />
                        <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 10, color: 'var(--text-muted)', marginTop: 2 }}>
                            <span>10px</span>
                            <span>24px</span>
                        </div>
                    </div>
                </div>
            </div>

            {/* ===== Enterprise Features Grid ===== */}
            {isCommunity && license && license.enterprise_features.length > 0 && (
                <div className="settings-group">
                    <div className="settings-group__title">Enterprise Features (Coming Soon)</div>
                    <div className="settings-features-grid">
                        {license.enterprise_features.map((feat) => (
                            <div key={feat.ID} className="settings-feature-card">
                                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                                    <span style={{ fontSize: 16 }}>
                                        {feat.ID === 'hitl_guardrails' ? '🛡️' : feat.ID === 'rbac' ? '🔒' : feat.ID === 'siem' ? '📊' : '🔍'}
                                    </span>
                                    <span style={{ fontWeight: 600, fontSize: 13, color: 'var(--text)' }}>{feat.Name}</span>
                                </div>
                                <EnterpriseBadge size="small" tooltip={`${feat.Name} — Coming Soon`} />
                            </div>
                        ))}
                    </div>
                </div>
            )}

            {/* ===== Notifications ===== */}
            <div className="settings-group">
                <div className="settings-group__title">Notifications</div>
                <div className="settings-card">
                    <SettingsRow label="Push Notifications" toggle toggleValue={notificationsEnabled} onToggle={handlePushToggle} />
                    <SettingsRow label="Session Changes" toggle toggleValue={sessionChanges} onToggle={setSessionChanges} />
                    <SettingsRow label="Agent Errors" toggle toggleValue={agentErrors} onToggle={setAgentErrors} />
                    <SettingsRow label="DMS Warnings" toggle toggleValue={dmsWarnings} onToggle={setDmsWarnings} />
                </div>
            </div>

            {/* ===== Recording ===== */}
            <div className="settings-group">
                <div className="settings-group__title">Recording</div>
                <div className="settings-card">
                    <SettingsRow
                        label="Auto-record sessions"
                        subtitle="When enabled, new sessions are recorded by default"
                        toggle
                        toggleValue={recordingEnabled}
                        onToggle={handleRecordingToggle}
                    />
                </div>
            </div>

            {/* ===== Connection ===== */}
            <div className="settings-group">
                <div className="settings-group__title">Connection</div>
                <div className="settings-card">
                    <SettingsRow label="Nexus Status" onClick={checkHealth} subtitle="Tap to refresh" />
                    <div className="settings-row">
                        <span className="settings-row__label">Status</span>
                        <ConnectionStatus connected={nexusConnected} loading={connectionLoading} />
                    </div>
                    <SettingsRow label="Satellite Transport" value="gRPC / TLS" subtitle="Bidirectional streaming to Nexus" />
                    <SettingsRow label="Cockpit Transport" value="WebSocket" subtitle="Terminal & real-time updates" />
                    <SettingsRow label="Auto-Reconnect" toggle toggleValue={autoReconnect} onToggle={setAutoReconnect} subtitle="Reconnect automatically on disconnect" />
                </div>
            </div>

            {/* ===== Security ===== */}
            <div className="settings-group">
                <div className="settings-group__title">Security</div>
                <div className="settings-card">
                    <SettingsRow label="Authentication" value={isOidc ? 'SSO/OIDC' : 'Local'} subtitle={isOidc ? 'OpenID Connect' : 'Email & password'} />
                </div>
            </div>

            {/* ===== Change Password ===== */}
            <div className="settings-group">
                <div className="settings-group__title">Change Password</div>
                <div className="settings-card" style={{ padding: '20px 24px' }}>
                    <form onSubmit={handleChangePassword} style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                        {passwordMessage && (
                            <div style={{
                                padding: '8px 12px',
                                borderRadius: 'var(--radius-md)',
                                fontSize: 13,
                                background: passwordMessage.type === 'success' ? 'rgba(45, 212, 191, 0.15)' : 'rgba(239, 68, 68, 0.15)',
                                color: passwordMessage.type === 'success' ? 'var(--success)' : 'var(--error)',
                                border: `1px solid ${passwordMessage.type === 'success' ? 'var(--success)' : 'var(--error)'}`,
                            }}>
                                {passwordMessage.text}
                            </div>
                        )}
                        <input
                            type="password"
                            placeholder="Current password"
                            value={currentPassword}
                            onChange={(e) => setCurrentPassword(e.target.value)}
                            required
                            style={{
                                padding: 'var(--space-sm) var(--space-md)',
                                background: 'var(--bg)', border: '1px solid var(--border)',
                                borderRadius: 'var(--radius-md)', color: 'var(--text)', fontSize: 13,
                            }}
                        />
                        <input
                            type="password"
                            placeholder="New password (min 8 characters)"
                            value={newPassword}
                            onChange={(e) => setNewPassword(e.target.value)}
                            required
                            minLength={8}
                            style={{
                                padding: 'var(--space-sm) var(--space-md)',
                                background: 'var(--bg)', border: '1px solid var(--border)',
                                borderRadius: 'var(--radius-md)', color: 'var(--text)', fontSize: 13,
                            }}
                        />
                        <input
                            type="password"
                            placeholder="Confirm new password"
                            value={confirmPassword}
                            onChange={(e) => setConfirmPassword(e.target.value)}
                            required
                            style={{
                                padding: 'var(--space-sm) var(--space-md)',
                                background: 'var(--bg)', border: '1px solid var(--border)',
                                borderRadius: 'var(--radius-md)', color: 'var(--text)', fontSize: 13,
                            }}
                        />
                        <button
                            type="submit"
                            className="btn btn--primary"
                            disabled={passwordLoading || !currentPassword || !newPassword || !confirmPassword}
                            style={{ alignSelf: 'flex-start' }}
                        >
                            {passwordLoading ? 'Changing...' : 'Change Password'}
                        </button>
                    </form>
                </div>
            </div>

            {/* ===== About ===== */}
            <div className="settings-group">
                <div className="settings-group__title">About</div>
                <div className="settings-card">
                    <SettingsRow
                        label="Active Sessions"
                        value={sessionLoading ? 'Loading...' : `${sessionCount} session${sessionCount !== 1 ? 's' : ''}`}
                    />
                    <SettingsRow label="Version" value="v0.1.0" onClick={() => copyToClipboard('DAAO v0.1.0')} subtitle="Tap to copy" />
                </div>
            </div>

            <div style={{ textAlign: 'center', padding: '24px 0', fontSize: 12, color: 'var(--text-muted)' }}>
                DAAO v0.1.0 • Open Source
            </div>

            {/* ========== SSH Keys Modal ========== */}
            <Modal open={sshKeysModalOpen} onClose={() => setSshKeysModalOpen(false)} title="SSH Keys">
                <div style={{ padding: 'var(--space-lg)' }}>
                    {/* Existing keys */}
                    {sshKeys.length > 0 ? (
                        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-sm)', marginBottom: 'var(--space-xl)' }}>
                            {sshKeys.map((key, i) => (
                                <div key={i} style={{
                                    display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                                    padding: 'var(--space-md)', background: 'var(--bg)', borderRadius: 'var(--radius-md)',
                                    border: '1px solid var(--border)',
                                }}>
                                    <div>
                                        <div style={{ fontWeight: 600, fontSize: 14 }}>{key.name}</div>
                                        <div style={{ fontSize: 12, color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
                                            {key.fingerprint}
                                        </div>
                                        <div style={{ fontSize: 11, color: 'var(--text-muted)', marginTop: 2 }}>
                                            Added {key.created}
                                        </div>
                                    </div>
                                    <button className="btn btn--sm btn--danger" onClick={() => handleRemoveSshKey(i)}>
                                        Remove
                                    </button>
                                </div>
                            ))}
                        </div>
                    ) : (
                        <div style={{
                            textAlign: 'center', padding: 'var(--space-xl)', color: 'var(--text-muted)',
                            fontSize: 13, marginBottom: 'var(--space-lg)',
                        }}>
                            No SSH keys registered yet
                        </div>
                    )}

                    {/* Add new key */}
                    <div style={{ borderTop: '1px solid var(--border)', paddingTop: 'var(--space-lg)' }}>
                        <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 'var(--space-md)' }}>Add SSH Key</div>
                        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-md)' }}>
                            <input
                                type="text"
                                placeholder="Key name (e.g. Work Laptop)"
                                value={newKeyName}
                                onChange={(e) => setNewKeyName(e.target.value)}
                                style={{
                                    padding: 'var(--space-sm) var(--space-md)',
                                    background: 'var(--bg)', border: '1px solid var(--border)',
                                    borderRadius: 'var(--radius-md)', color: 'var(--text)',
                                    fontSize: 13,
                                }}
                            />
                            <textarea
                                placeholder="Paste your public key (ssh-rsa AAAA...)"
                                value={newKeyValue}
                                onChange={(e) => setNewKeyValue(e.target.value)}
                                rows={3}
                                style={{
                                    padding: 'var(--space-sm) var(--space-md)',
                                    background: 'var(--bg)', border: '1px solid var(--border)',
                                    borderRadius: 'var(--radius-md)', color: 'var(--text)',
                                    fontSize: 12, fontFamily: 'var(--font-mono)', resize: 'vertical',
                                }}
                            />
                            <button
                                className="btn btn--primary"
                                onClick={handleAddSshKey}
                                disabled={!newKeyName.trim() || !newKeyValue.trim()}
                                style={{ opacity: (!newKeyName.trim() || !newKeyValue.trim()) ? 0.5 : 1 }}
                            >
                                Add Key
                            </button>
                        </div>
                    </div>
                </div>
            </Modal>

            {/* ========== API Tokens Modal ========== */}
            <Modal open={apiTokensModalOpen} onClose={() => { setApiTokensModalOpen(false); setGeneratedToken(''); }} title="API Tokens">
                <div style={{ padding: 'var(--space-lg)' }}>
                    {/* Generated token banner */}
                    {generatedToken && (
                        <div style={{
                            padding: 'var(--space-md)', background: 'var(--accent-muted)',
                            border: '1px solid var(--accent)', borderRadius: 'var(--radius-md)',
                            marginBottom: 'var(--space-lg)',
                        }}>
                            <div style={{ fontSize: 12, fontWeight: 600, color: 'var(--accent)', marginBottom: 'var(--space-xs)' }}>
                                ⓘ Copy your token now — it won't be shown again
                            </div>
                            <div style={{
                                display: 'flex', alignItems: 'center', gap: 'var(--space-sm)',
                            }}>
                                <code style={{
                                    flex: 1, padding: 'var(--space-sm)', background: 'var(--bg)',
                                    borderRadius: 'var(--radius-sm)', fontSize: 12,
                                    fontFamily: 'var(--font-mono)', wordBreak: 'break-all',
                                    color: 'var(--text)',
                                }}>
                                    {generatedToken}
                                </code>
                                <button className="btn btn--sm btn--primary" onClick={() => copyToClipboard(generatedToken)}>
                                    Copy
                                </button>
                            </div>
                        </div>
                    )}

                    {/* Existing tokens */}
                    {apiTokens.length > 0 ? (
                        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-sm)', marginBottom: 'var(--space-xl)' }}>
                            {apiTokens.map((tok, i) => (
                                <div key={i} style={{
                                    display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                                    padding: 'var(--space-md)', background: 'var(--bg)', borderRadius: 'var(--radius-md)',
                                    border: '1px solid var(--border)',
                                }}>
                                    <div>
                                        <div style={{ fontWeight: 600, fontSize: 14 }}>{tok.name}</div>
                                        <div style={{ fontSize: 12, color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
                                            {tok.token}
                                        </div>
                                        <div style={{ fontSize: 11, color: 'var(--text-muted)', marginTop: 2 }}>
                                            Created {tok.created} • Last used: {tok.lastUsed}
                                        </div>
                                    </div>
                                    <button className="btn btn--sm btn--danger" onClick={() => handleRevokeToken(i)}>
                                        Revoke
                                    </button>
                                </div>
                            ))}
                        </div>
                    ) : (
                        <div style={{
                            textAlign: 'center', padding: 'var(--space-xl)', color: 'var(--text-muted)',
                            fontSize: 13, marginBottom: 'var(--space-lg)',
                        }}>
                            No API tokens generated yet
                        </div>
                    )}

                    {/* Generate new token */}
                    <div style={{ borderTop: '1px solid var(--border)', paddingTop: 'var(--space-lg)' }}>
                        <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 'var(--space-md)' }}>Generate Token</div>
                        <div style={{ display: 'flex', gap: 'var(--space-sm)' }}>
                            <input
                                type="text"
                                placeholder="Token name (e.g. CI Pipeline)"
                                value={newTokenName}
                                onChange={(e) => setNewTokenName(e.target.value)}
                                style={{
                                    flex: 1, padding: 'var(--space-sm) var(--space-md)',
                                    background: 'var(--bg)', border: '1px solid var(--border)',
                                    borderRadius: 'var(--radius-md)', color: 'var(--text)', fontSize: 13,
                                }}
                                onKeyDown={(e) => { if (e.key === 'Enter') handleGenerateToken(); }}
                            />
                            <button
                                className="btn btn--primary"
                                onClick={handleGenerateToken}
                                disabled={!newTokenName.trim()}
                                style={{ opacity: !newTokenName.trim() ? 0.5 : 1 }}
                            >
                                Generate
                            </button>
                        </div>
                    </div>
                </div>
            </Modal>

            <style>{`
                @keyframes fadeIn {
                    from { opacity: 0; transform: translateY(-4px); }
                    to { opacity: 1; transform: translateY(0); }
                }
            `}</style>
        </div>
    );
};

export default Settings;
