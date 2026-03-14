import React, { createContext, useContext, useState, useEffect, useCallback, useMemo } from 'react';
import { oidcConfig, isOIDCConfigured, getOIDCEndpoints } from './authConfig';
import { generateCodeVerifier, generateCodeChallenge, generateState } from './pkce';

export interface UserInfo {
    sub: string;
    name?: string;
    email?: string;
    preferred_username?: string;
    groups?: string[];
    picture?: string;
    role: string;  // NEW — 'owner' | 'admin' | 'viewer'
}

export interface AuthContextType {
    /** Whether the user is authenticated */
    isAuthenticated: boolean;
    /** Whether auth is still loading/checking */
    isLoading: boolean;
    /** Whether OIDC is configured (false = local auth only) */
    isConfigured: boolean;
    /** Current user info */
    user: UserInfo | null;
    /** Access token for API calls */
    accessToken: string | null;
    /** Initiate OIDC login flow */
    login: () => Promise<void>;
    /** Local email/password login */
    localLogin: (email: string, password: string) => Promise<void>;
    /** Handle callback after auth redirect */
    handleCallback: (code: string, state: string) => Promise<void>;
    /** Logout */
    logout: () => void;
    /** User role from DB ('owner' | 'admin' | 'viewer') */
    role: string;
    /** Check if user has permission for a required role */
    hasPermission: (requiredRole: string) => boolean;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

// Session storage keys
const STORAGE_KEYS = {
    accessToken: 'oidc_access_token',
    refreshToken: 'oidc_refresh_token',
    idToken: 'oidc_id_token',
    userInfo: 'oidc_user_info',
    expiresAt: 'oidc_expires_at',
    codeVerifier: 'oidc_code_verifier',
    state: 'oidc_state',
} as const;

interface TokenResponse {
    access_token: string;
    id_token?: string;
    refresh_token?: string;
    token_type: string;
    expires_in: number;
    scope?: string;
}

export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const [isAuthenticated, setIsAuthenticated] = useState(false);
    const [isLoading, setIsLoading] = useState(true);
    const [user, setUser] = useState<UserInfo | null>(null);
    const [accessToken, setAccessToken] = useState<string | null>(null);
    const [role, setRole] = useState<string>('viewer');

    const isConfigured = isOIDCConfigured();
    const endpoints = useMemo(
        () => isConfigured ? getOIDCEndpoints(oidcConfig.issuerUrl) : null,
        [isConfigured]
    );

    // Role level for permission checking: viewer=0, admin=1, owner=2
    const roleLevel = (r: string): number => {
        return { viewer: 0, admin: 1, owner: 2 }[r] ?? -1;
    };

    // Check if user has permission for a required role
    const hasPermission = useCallback((required: string): boolean => {
        return roleLevel(role) >= roleLevel(required);
    }, [role]);

    // Check for existing session on mount
    useEffect(() => {
        if (!isConfigured) {
            // No OIDC — check for stored local JWT
            const localToken = sessionStorage.getItem(STORAGE_KEYS.accessToken);
            const localExpires = sessionStorage.getItem(STORAGE_KEYS.expiresAt);
            const localUserStr = sessionStorage.getItem(STORAGE_KEYS.userInfo);

            if (localToken && localExpires && Date.now() < parseInt(localExpires)) {
                setAccessToken(localToken);
                setIsAuthenticated(true);
                if (localUserStr) {
                    try {
                        const parsed = JSON.parse(localUserStr);
                        setUser(parsed);
                        if (parsed.role) setRole(parsed.role);
                    } catch { /* ignore */ }
                }
            }
            // If no token, stay unauthenticated — ProtectedRoute will redirect to login
            setIsLoading(false);
            return;
        }

        const token = sessionStorage.getItem(STORAGE_KEYS.accessToken);
        const expiresAt = sessionStorage.getItem(STORAGE_KEYS.expiresAt);
        const userInfoStr = sessionStorage.getItem(STORAGE_KEYS.userInfo);

        if (token && expiresAt && Date.now() < parseInt(expiresAt)) {
            setAccessToken(token);
            setIsAuthenticated(true);
            if (userInfoStr) {
                try {
                    const parsed = JSON.parse(userInfoStr);
                    setUser(parsed);
                    if (parsed.role) setRole(parsed.role);
                } catch { /* ignore */ }
            }
        }

        setIsLoading(false);
    }, [isConfigured]);

    // Initiate login — redirect to Authentik authorization endpoint
    const login = useCallback(async () => {
        if (!endpoints) return;

        const codeVerifier = generateCodeVerifier();
        const codeChallenge = await generateCodeChallenge(codeVerifier);
        const state = generateState();

        // Store PKCE verifier and state for callback validation
        sessionStorage.setItem(STORAGE_KEYS.codeVerifier, codeVerifier);
        sessionStorage.setItem(STORAGE_KEYS.state, state);

        const params = new URLSearchParams({
            response_type: 'code',
            client_id: oidcConfig.clientId,
            redirect_uri: oidcConfig.redirectUri,
            scope: oidcConfig.scopes.join(' '),
            state,
            code_challenge: codeChallenge,
            code_challenge_method: 'S256',
        });

        window.location.href = `${endpoints.authorization}?${params.toString()}`;
    }, [endpoints]);

    // Handle callback — exchange authorization code for tokens
    const handleCallback = useCallback(async (code: string, state: string) => {
        if (!endpoints) throw new Error('OIDC not configured');

        // Validate state
        const savedState = sessionStorage.getItem(STORAGE_KEYS.state);
        if (state !== savedState) {
            throw new Error('Invalid state parameter — possible CSRF attack');
        }

        // Get PKCE verifier
        const codeVerifier = sessionStorage.getItem(STORAGE_KEYS.codeVerifier);
        if (!codeVerifier) {
            throw new Error('Missing PKCE code verifier');
        }

        // Exchange code for tokens
        const tokenResponse = await fetch(endpoints.token, {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
            body: new URLSearchParams({
                grant_type: 'authorization_code',
                client_id: oidcConfig.clientId,
                code,
                redirect_uri: oidcConfig.redirectUri,
                code_verifier: codeVerifier,
            }).toString(),
        });

        if (!tokenResponse.ok) {
            const errText = await tokenResponse.text();
            throw new Error(`Token exchange failed: ${errText}`);
        }

        const tokens: TokenResponse = await tokenResponse.json();

        // Calculate expiration
        const expiresAt = Date.now() + tokens.expires_in * 1000;

        // Store tokens
        sessionStorage.setItem(STORAGE_KEYS.accessToken, tokens.access_token);
        sessionStorage.setItem(STORAGE_KEYS.expiresAt, expiresAt.toString());
        if (tokens.id_token) sessionStorage.setItem(STORAGE_KEYS.idToken, tokens.id_token);
        if (tokens.refresh_token) sessionStorage.setItem(STORAGE_KEYS.refreshToken, tokens.refresh_token);

        // Set HttpOnly auth cookie for SSE endpoints (EventSource can't send headers)
        try {
            await fetch('/api/v1/auth/cookie', {
                method: 'POST',
                headers: { Authorization: `Bearer ${tokens.access_token}` },
                credentials: 'include',
            });
        } catch (err) {
            console.warn('Failed to set auth cookie:', err);
        }

        // Clean up PKCE artifacts
        sessionStorage.removeItem(STORAGE_KEYS.codeVerifier);
        sessionStorage.removeItem(STORAGE_KEYS.state);

        setAccessToken(tokens.access_token);

        // Fetch user info
        try {
            const userInfoResponse = await fetch(endpoints.userinfo, {
                headers: { Authorization: `Bearer ${tokens.access_token}` },
            });

            if (userInfoResponse.ok) {
                const userInfo: UserInfo = await userInfoResponse.json();

                // Fetch DB-sourced user role
                let finalUserInfo = userInfo;
                try {
                    const meResponse = await fetch('/api/v1/users/me', {
                        headers: { Authorization: `Bearer ${tokens.access_token}` },
                    });
                    if (meResponse.ok) {
                        const meData = await meResponse.json();
                        finalUserInfo = { ...userInfo, role: meData.role };
                        setRole(meData.role);
                    }
                } catch (meErr) {
                    console.warn('Failed to fetch /api/v1/users/me:', meErr);
                }

                setUser(finalUserInfo);
                sessionStorage.setItem(STORAGE_KEYS.userInfo, JSON.stringify(finalUserInfo));
            }
        } catch (err) {
            console.warn('Failed to fetch user info:', err);
        }

        setIsAuthenticated(true);
    }, [endpoints]);

    // Logout — clear tokens and optionally redirect to Authentik end-session
    const logout = useCallback(() => {
        const idToken = sessionStorage.getItem(STORAGE_KEYS.idToken);

        // Clear auth cookie (SSE endpoints use this for auth)
        try {
            fetch('/api/v1/auth/cookie', { method: 'DELETE', credentials: 'include' });
        } catch { /* ignore */ }

        // Clear all stored tokens
        Object.values(STORAGE_KEYS).forEach(key => sessionStorage.removeItem(key));

        setAccessToken(null);
        setUser(null);
        setIsAuthenticated(false);
        setRole('viewer');

        // Redirect to Authentik end-session endpoint if available
        if (endpoints?.endSession && idToken) {
            const params = new URLSearchParams({
                id_token_hint: idToken,
                post_logout_redirect_uri: oidcConfig.postLogoutRedirectUri,
            });
            window.location.href = `${endpoints.endSession}?${params.toString()}`;
        } else {
            window.location.href = '/login';
        }
    }, [endpoints]);

    // Local email/password login
    const localLogin = useCallback(async (email: string, password: string) => {
        const response = await fetch('/api/v1/auth/login', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ email, password }),
        });

        if (!response.ok) {
            const errData = await response.json().catch(() => ({ error: 'Login failed' }));
            throw new Error(errData.error || 'Invalid email or password');
        }

        const data = await response.json();
        const token = data.token;
        const userInfo: UserInfo = {
            sub: data.user.id,
            email: data.user.email,
            name: data.user.name,
            role: data.user.role,
            picture: data.user.avatar_url,
        };

        // Store token with 24h expiry (matching backend JWT expiry)
        const expiresAt = Date.now() + 24 * 60 * 60 * 1000;
        sessionStorage.setItem(STORAGE_KEYS.accessToken, token);
        sessionStorage.setItem(STORAGE_KEYS.expiresAt, expiresAt.toString());
        sessionStorage.setItem(STORAGE_KEYS.userInfo, JSON.stringify(userInfo));

        setAccessToken(token);
        setUser(userInfo);
        setRole(userInfo.role);
        setIsAuthenticated(true);
    }, []);

    const contextValue = useMemo<AuthContextType>(() => ({
        isAuthenticated,
        isLoading,
        isConfigured,
        user,
        accessToken,
        login,
        localLogin,
        handleCallback,
        logout,
        role,
        hasPermission,
    }), [isAuthenticated, isLoading, isConfigured, user, accessToken, login, localLogin, handleCallback, logout, role, hasPermission]);

    return (
        <AuthContext.Provider value={contextValue}>
            {children}
        </AuthContext.Provider>
    );
};

/**
 * Hook to access auth context
 */
export function useAuth(): AuthContextType {
    const context = useContext(AuthContext);
    if (!context) {
        throw new Error('useAuth must be used within an AuthProvider');
    }
    return context;
}

export default AuthProvider;
