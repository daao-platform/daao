import React, { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useAuth } from '../auth/AuthProvider';

/**
 * AuthCallback — handles the OIDC redirect callback
 *
 * Exchanges the authorization code for tokens, then redirects to the dashboard.
 */
const AuthCallback: React.FC = () => {
    const navigate = useNavigate();
    const [searchParams] = useSearchParams();
    const { handleCallback } = useAuth();
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        const code = searchParams.get('code');
        const state = searchParams.get('state');
        const errorParam = searchParams.get('error');
        const errorDescription = searchParams.get('error_description');

        if (errorParam) {
            setError(`Authentication failed: ${errorDescription || errorParam}`);
            return;
        }

        if (!code || !state) {
            setError('Missing authorization code or state parameter');
            return;
        }

        const processCallback = async () => {
            try {
                await handleCallback(code, state);
                navigate('/', { replace: true });
            } catch (err) {
                setError(err instanceof Error ? err.message : 'Authentication failed');
            }
        };

        processCallback();
    }, [searchParams, handleCallback, navigate]);

    if (error) {
        return (
            <div className="login-page">
                <div className="login-card" style={{ maxWidth: 420 }}>
                    <div style={{ color: 'var(--danger)', fontSize: 40, marginBottom: 'var(--space-lg)' }}>⚠</div>
                    <h2 style={{ marginBottom: 'var(--space-md)' }}>Authentication Error</h2>
                    <p style={{ color: 'var(--text-muted)', fontSize: 13, marginBottom: 'var(--space-xl)' }}>{error}</p>
                    <button
                        className="btn btn--primary btn--full"
                        onClick={() => navigate('/login', { replace: true })}
                    >
                        Try Again
                    </button>
                </div>

                <style>{`
          .login-page {
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            background: var(--bg);
            padding: var(--space-lg);
          }
          .login-card {
            background: var(--bg-surface);
            border: 1px solid var(--border);
            border-radius: var(--radius-xl);
            padding: var(--space-3xl);
            width: 100%;
            text-align: center;
          }
        `}</style>
            </div>
        );
    }

    return (
        <div className="login-page">
            <div className="login-card">
                <div className="spinner" style={{ margin: '0 auto var(--space-lg)' }} />
                <p style={{ color: 'var(--text-muted)', fontSize: 14 }}>Completing authentication...</p>
            </div>

            <style>{`
        .login-page {
          min-height: 100vh;
          display: flex;
          align-items: center;
          justify-content: center;
          background: var(--bg);
          padding: var(--space-lg);
        }
        .login-card {
          background: var(--bg-surface);
          border: 1px solid var(--border);
          border-radius: var(--radius-xl);
          padding: var(--space-3xl);
          width: 100%;
          max-width: 380px;
          text-align: center;
        }
      `}</style>
        </div>
    );
};

export default AuthCallback;
