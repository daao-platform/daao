import React from 'react';
import { useLicense } from '../hooks/useLicense';

/**
 * UpgradeCard — Compact card showing current license tier, usage, and upgrade CTA.
 * Shows in Settings and optionally in the sidebar.
 */
const UpgradeCard: React.FC = () => {
    const { license, isCommunity, loading } = useLicense();

    if (loading || !license) return null;

    const usageItems = [
        {
            label: 'Recordings',
            limit: license.max_recordings,
            unit: license.max_recordings === 0 ? 'Unlimited' : `/ ${license.max_recordings}`,
        },
        {
            label: 'Satellites',
            limit: license.max_satellites,
            unit: license.max_satellites === 0 ? 'Unlimited' : `/ ${license.max_satellites}`,
        },
        {
            label: 'Users',
            limit: license.max_users,
            unit: license.max_users === 0 ? 'Unlimited' : `/ ${license.max_users}`,
        },
        {
            label: 'Telemetry',
            limit: license.telemetry_retention_hours,
            unit: license.telemetry_retention_hours >= 24
                ? `${Math.floor(license.telemetry_retention_hours / 24)}d retention`
                : `${license.telemetry_retention_hours}h retention`,
        },
    ];

    return (
        <div className={`upgrade-card ${isCommunity ? 'upgrade-card--community' : 'upgrade-card--enterprise'}`}>
            {/* Header */}
            <div className="upgrade-card__header">
                <div className="upgrade-card__tier-badge">
                    {isCommunity ? (
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                            <path d="M12 2L2 7l10 5 10-5-10-5z" />
                            <path d="M2 17l10 5 10-5" />
                            <path d="M2 12l10 5 10-5" />
                        </svg>
                    ) : (
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                            <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
                        </svg>
                    )}
                    <span>DAAO {isCommunity ? 'Community' : 'Enterprise'}</span>
                </div>
                {!isCommunity && license.customer && (
                    <span className="upgrade-card__customer">{license.customer}</span>
                )}
            </div>

            {/* Limits */}
            <div className="upgrade-card__limits">
                {usageItems.map((item) => (
                    <div key={item.label} className="upgrade-card__limit-row">
                        <span className="upgrade-card__limit-label">{item.label}</span>
                        <span className="upgrade-card__limit-value">{item.unit}</span>
                    </div>
                ))}
            </div>

            {/* CTA */}
            {isCommunity && (
                <a
                    href="https://daao.dev/pricing"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="upgrade-card__cta"
                >
                    Enterprise Coming Soon →
                </a>
            )}

            {/* Enterprise expiry */}
            {!isCommunity && license.expires_at && license.expires_at > 0 && (
                <div className="upgrade-card__expiry">
                    License expires {new Date(license.expires_at * 1000).toLocaleDateString()}
                </div>
            )}
        </div>
    );
};

export default UpgradeCard;
