import React from 'react';
import { useLicense } from '../hooks/useLicense';

interface EnterpriseBadgeProps {
    /** Optional tooltip text override */
    tooltip?: string;
    /** Optional: show inline description text */
    showText?: boolean;
    /** Size variant */
    size?: 'small' | 'default';
}

/**
 * EnterpriseBadge — Small inline badge showing a lock icon + "Enterprise".
 * Renders nothing if the current license is enterprise tier.
 */
const EnterpriseBadge: React.FC<EnterpriseBadgeProps> = ({
    tooltip = 'Coming Soon — DAAO Enterprise',
    showText = true,
    size = 'default',
}) => {
    const { isEnterprise } = useLicense();

    // Don't show badge if already on enterprise
    if (isEnterprise) return null;

    return (
        <span
            className={`enterprise-badge enterprise-badge--${size}`}
            title={tooltip}
        >
            <svg
                width={size === 'small' ? 10 : 12}
                height={size === 'small' ? 10 : 12}
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2.5"
                strokeLinecap="round"
                strokeLinejoin="round"
            >
                <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
                <path d="M7 11V7a5 5 0 0 1 10 0v4" />
            </svg>
            {showText && <span className="enterprise-badge__text">Coming Soon</span>}
        </span>
    );
};

export default EnterpriseBadge;
