/**
 * OIDC Configuration for Authentik
 * 
 * Reads OIDC settings from Vite environment variables at build time.
 * Set these in your .env file or docker-compose environment.
 */

export interface OIDCConfig {
    /** OIDC Issuer URL — e.g. https://auth.example.com/application/o/daao/ */
    issuerUrl: string;
    /** OAuth2 Client ID (public client) */
    clientId: string;
    /** Redirect URI after authentication */
    redirectUri: string;
    /** Scopes to request */
    scopes: string[];
    /** Post-logout redirect URI */
    postLogoutRedirectUri: string;
}

// Read from Vite env vars, with sensible defaults for local development
export const oidcConfig: OIDCConfig = {
    issuerUrl: import.meta.env.VITE_OIDC_ISSUER_URL || '',
    clientId: import.meta.env.VITE_OIDC_CLIENT_ID || '',
    redirectUri: import.meta.env.VITE_OIDC_REDIRECT_URI || `${window.location.origin}/auth/callback`,
    scopes: ['openid', 'profile', 'email'],
    postLogoutRedirectUri: import.meta.env.VITE_OIDC_POST_LOGOUT_URI || window.location.origin,
};

/**
 * Check if OIDC authentication is configured
 */
export function isOIDCConfigured(): boolean {
    return !!(oidcConfig.issuerUrl && oidcConfig.clientId);
}

/**
 * OIDC Discovery endpoints derived from issuer URL
 */
export function getOIDCEndpoints(issuerUrl: string) {
    // Remove trailing slash
    const base = issuerUrl.replace(/\/$/, '');
    return {
        discovery: `${base}/.well-known/openid-configuration`,
        authorization: `${base}/authorize/`,
        token: `${base}/token/`,
        userinfo: `${base}/userinfo/`,
        endSession: `${base}/end-session/`,
        jwks: `${base}/jwks/`,
    };
}
