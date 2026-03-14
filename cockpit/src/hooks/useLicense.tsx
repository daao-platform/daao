import React, { createContext, useContext, useEffect, useState, useCallback } from 'react';
import { getLicenseInfo, type LicenseInfo } from '../api/client';

// ============================================================================
// Context + Provider
// ============================================================================

interface LicenseContextValue {
    license: LicenseInfo | null;
    loading: boolean;
    error: Error | null;
    isCommunity: boolean;
    isEnterprise: boolean;
    hasFeature: (featureId: string) => boolean;
    isNearLimit: (current: number, field: 'max_recordings' | 'max_satellites' | 'max_users') => boolean;
    isAtLimit: (current: number, field: 'max_recordings' | 'max_satellites' | 'max_users') => boolean;
    refetch: () => void;
}

const LicenseContext = createContext<LicenseContextValue>({
    license: null,
    loading: true,
    error: null,
    isCommunity: true,
    isEnterprise: false,
    hasFeature: () => false,
    isNearLimit: () => false,
    isAtLimit: () => false,
    refetch: () => { },
});

export function LicenseProvider({ children }: { children: React.ReactNode }) {
    const [license, setLicense] = useState<LicenseInfo | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<Error | null>(null);

    const fetchLicense = useCallback(() => {
        setLoading(true);
        getLicenseInfo()
            .then((data) => {
                setLicense(data);
                setError(null);
            })
            .catch((err) => {
                setError(err instanceof Error ? err : new Error(String(err)));
                // Default to community on error
                setLicense({
                    tier: 'community',
                    max_users: 3,
                    max_satellites: 5,
                    max_recordings: 50,
                    telemetry_retention_hours: 1,
                    enterprise_features: [],
                });
            })
            .finally(() => setLoading(false));
    }, []);

    useEffect(() => {
        fetchLicense();
    }, [fetchLicense]);

    const isCommunity = !license || license.tier === 'community';
    const isEnterprise = !!license && license.tier === 'enterprise';

    const hasFeature = useCallback(
        (featureId: string) => {
            if (!license || isCommunity) return false;
            return isEnterprise && license.enterprise_features.some((f) => f.ID === featureId);
        },
        [license, isCommunity, isEnterprise],
    );

    const isNearLimit = useCallback(
        (current: number, field: 'max_recordings' | 'max_satellites' | 'max_users') => {
            if (!license) return false;
            const limit = license[field];
            if (limit <= 0) return false; // unlimited
            return current >= limit * 0.8;
        },
        [license],
    );

    const isAtLimit = useCallback(
        (current: number, field: 'max_recordings' | 'max_satellites' | 'max_users') => {
            if (!license) return false;
            const limit = license[field];
            if (limit <= 0) return false; // unlimited
            return current >= limit;
        },
        [license],
    );

    return (
        <LicenseContext.Provider
            value={{
                license,
                loading,
                error,
                isCommunity,
                isEnterprise,
                hasFeature,
                isNearLimit,
                isAtLimit,
                refetch: fetchLicense,
            }}
        >
            {children}
        </LicenseContext.Provider>
    );
}

/**
 * useLicense — Access the license context from any component.
 * Must be used within a <LicenseProvider>.
 */
export function useLicense(): LicenseContextValue {
    return useContext(LicenseContext);
}
