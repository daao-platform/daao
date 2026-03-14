/// <reference types="vite/client" />

interface ImportMetaEnv {
    readonly VITE_OIDC_ISSUER_URL: string;
    readonly VITE_OIDC_CLIENT_ID: string;
    readonly VITE_OIDC_REDIRECT_URI: string;
    readonly VITE_OIDC_POST_LOGOUT_URI: string;
}

interface ImportMeta {
    readonly env: ImportMetaEnv;
}
