import React, { useState } from 'react';
import { useAuth } from '../auth/AuthProvider';
import { useNavigate } from 'react-router-dom';

/**
 * Login Page — handles both OIDC SSO and local email/password auth
 */
const LoginPage: React.FC = () => {
    const { login, localLogin, isLoading, isConfigured, isAuthenticated } = useAuth();
    const navigate = useNavigate();

    const [email, setEmail] = useState('');
    const [password, setPassword] = useState('');
    const [error, setError] = useState<string | null>(null);
    const [submitting, setSubmitting] = useState(false);

    // If already authenticated, redirect to dashboard
    if (isAuthenticated) {
        navigate('/', { replace: true });
        return null;
    }

    // OIDC login handler
    const handleOIDCLogin = async () => {
        try {
            await login();
        } catch (err) {
            console.error('Login failed:', err);
        }
    };

    // Local login handler
    const handleLocalLogin = async (e: React.FormEvent) => {
        e.preventDefault();
        if (!email.trim() || !password) return;

        setSubmitting(true);
        setError(null);

        try {
            await localLogin(email.trim(), password);
            navigate('/', { replace: true });
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Login failed');
        } finally {
            setSubmitting(false);
        }
    };

    return (
        <div className="login-page">
            <div className="login-card">
                <div className="login-logo">
                    <div className="login-logo__icon">D</div>
                    <h1 className="login-logo__title">DAAO</h1>
                    <p className="login-logo__subtitle">Distributed AI Agent Orchestration</p>
                </div>

                <div className="login-divider" />

                {/* Show OIDC button when configured */}
                {isConfigured && (
                    <>
                        <button
                            className="btn btn--primary btn--full login-btn"
                            onClick={handleOIDCLogin}
                            disabled={isLoading}
                        >
                            {isLoading ? (
                                <span className="login-btn__loading">
                                    <span className="spinner spinner--sm" />
                                    Connecting...
                                </span>
                            ) : (
                                <>
                                    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                                        <path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4" />
                                        <polyline points="10 17 15 12 10 7" />
                                        <line x1="15" y1="12" x2="3" y2="12" />
                                    </svg>
                                    Sign in with SSO
                                </>
                            )}
                        </button>
                        <div style={{ margin: 'var(--space-lg) 0', textAlign: 'center', fontSize: 12, color: 'var(--text-muted)' }}>
                            — or sign in with email —
                        </div>
                    </>
                )}

                {/* Local login form */}
                <form onSubmit={handleLocalLogin}>
                    {error && (
                        <div className="form-error" style={{ marginBottom: 'var(--space-md)', textAlign: 'left' }}>
                            {error}
                        </div>
                    )}

                    <div className="form-group" style={{ marginBottom: 'var(--space-md)', textAlign: 'left' }}>
                        <label htmlFor="login-email" className="form-label">Email</label>
                        <input
                            id="login-email"
                            type="email"
                            className="form-input"
                            placeholder="you@example.com"
                            value={email}
                            onChange={(e) => setEmail(e.target.value)}
                            required
                            autoFocus={!isConfigured}
                            autoComplete="email"
                        />
                    </div>

                    <div className="form-group" style={{ marginBottom: 'var(--space-lg)', textAlign: 'left' }}>
                        <label htmlFor="login-password" className="form-label">Password</label>
                        <input
                            id="login-password"
                            type="password"
                            className="form-input"
                            placeholder="••••••••"
                            value={password}
                            onChange={(e) => setPassword(e.target.value)}
                            required
                            autoComplete="current-password"
                        />
                    </div>

                    <button
                        type="submit"
                        className="btn btn--primary btn--full login-btn"
                        disabled={submitting || !email.trim() || !password}
                    >
                        {submitting ? (
                            <span className="login-btn__loading">
                                <span className="spinner spinner--sm" />
                                Signing in...
                            </span>
                        ) : (
                            'Sign In'
                        )}
                    </button>
                </form>

                <p className="login-footer">
                    {isConfigured ? 'OpenID Connect + Local Auth' : 'Local Authentication'}
                </p>
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

        .login-logo {
          margin-bottom: var(--space-xl);
        }

        .login-logo__icon {
          width: 56px;
          height: 56px;
          background: var(--accent-muted);
          color: var(--accent);
          border-radius: var(--radius-lg);
          display: flex;
          align-items: center;
          justify-content: center;
          font-size: 24px;
          font-weight: 700;
          margin: 0 auto var(--space-lg);
          letter-spacing: -0.05em;
        }

        .login-logo__title {
          font-size: 28px;
          font-weight: 700;
          letter-spacing: -0.03em;
          color: var(--text);
          margin-bottom: var(--space-xs);
        }

        .login-logo__subtitle {
          font-size: 13px;
          color: var(--text-muted);
        }

        .login-divider {
          height: 1px;
          background: var(--border);
          margin: var(--space-xl) 0;
        }

        .login-btn {
          padding: var(--space-md) var(--space-xl);
          font-size: 15px;
          font-weight: 600;
          min-height: 48px;
          gap: var(--space-md);
        }

        .login-btn__loading {
          display: flex;
          align-items: center;
          gap: var(--space-md);
        }

        .login-footer {
          margin-top: var(--space-xl);
          font-size: 11px;
          color: var(--text-muted);
          letter-spacing: 0.02em;
        }
      `}</style>
        </div>
    );
};

export default LoginPage;
