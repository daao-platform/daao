import React, { useState, useEffect, useCallback } from 'react';
import { getProposals, approveProposal, denyProposal, type Proposal } from '../api/client';

const riskColors: Record<string, string> = {
    critical: '#ff4757',
    high: '#ff6348',
    medium: '#ffa502',
    low: '#2ed573',
};

const statusColors: Record<string, string> = {
    pending: '#ffa502',
    approved: '#2ed573',
    denied: '#ff4757',
    expired: '#747d8c',
    cancelled: '#747d8c',
};

export default function Proposals() {
    const [proposals, setProposals] = useState<Proposal[]>([]);
    const [loading, setLoading] = useState(true);

    const fetchProposals = useCallback(async () => {
        try {
            const res = await getProposals();
            setProposals(res.proposals || []);
        } catch {
            setProposals([]);
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        fetchProposals();
        const interval = setInterval(fetchProposals, 5000);
        return () => clearInterval(interval);
    }, [fetchProposals]);

    const handleApprove = async (id: string) => {
        try {
            await approveProposal(id);
            fetchProposals();
        } catch (err) {
            console.error('Failed to approve proposal:', err);
        }
    };

    const handleDeny = async (id: string) => {
        try {
            await denyProposal(id);
            fetchProposals();
        } catch (err) {
            console.error('Failed to deny proposal:', err);
        }
    };

    const formatTime = (iso: string) => {
        const d = new Date(iso);
        return d.toLocaleString();
    };

    return (
        <div style={{ padding: '2rem' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '1.5rem' }}>
                <h1 style={{ margin: 0, fontSize: '1.75rem', fontWeight: 700 }}>
                    🛡️ HITL Proposals
                </h1>
            </div>

            {loading ? (
                <div style={{ textAlign: 'center', padding: '3rem', color: 'var(--text-muted)' }}>
                    Loading proposals…
                </div>
            ) : proposals.length === 0 ? (
                <div style={{
                    textAlign: 'center',
                    padding: '4rem 2rem',
                    background: 'var(--card-bg)',
                    borderRadius: '16px',
                    border: '1px solid var(--border)',
                    color: 'var(--text-muted)',
                }}>
                    <div style={{ fontSize: '3rem', marginBottom: '1rem' }}>🛡️</div>
                    <h2 style={{ margin: '0 0 0.5rem', color: 'var(--text)' }}>No proposals yet</h2>
                    <p>When agents request approval for high-risk actions, they'll appear here.</p>
                </div>
            ) : (
                <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
                    {proposals.map(p => (
                        <div
                            key={p.id}
                            style={{
                                padding: '1.25rem',
                                background: 'var(--card-bg)',
                                borderRadius: '12px',
                                border: '1px solid var(--border)',
                                display: 'flex',
                                flexDirection: 'column',
                                gap: '0.75rem',
                            }}
                        >
                            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                                <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
                                    <span style={{
                                        padding: '0.25rem 0.75rem',
                                        borderRadius: '999px',
                                        fontSize: '0.75rem',
                                        fontWeight: 600,
                                        background: `${riskColors[p.risk_level] || '#747d8c'}20`,
                                        color: riskColors[p.risk_level] || '#747d8c',
                                        textTransform: 'uppercase',
                                    }}>
                                        {p.risk_level}
                                    </span>
                                    <span style={{ fontWeight: 600, fontSize: '1rem' }}>{p.command}</span>
                                </div>
                                <span style={{
                                    padding: '0.25rem 0.75rem',
                                    borderRadius: '999px',
                                    fontSize: '0.75rem',
                                    fontWeight: 600,
                                    background: `${statusColors[p.status] || '#747d8c'}20`,
                                    color: statusColors[p.status] || '#747d8c',
                                }}>
                                    {p.status}
                                </span>
                            </div>

                            <p style={{ margin: 0, color: 'var(--text-muted)', fontSize: '0.9rem' }}>
                                {p.justification}
                            </p>

                            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', fontSize: '0.8rem', color: 'var(--text-muted)' }}>
                                <span>Session: <strong>{p.session_id}</strong></span>
                                <span>{formatTime(p.created_at)}</span>
                            </div>

                            {p.status === 'pending' && (
                                <div style={{ display: 'flex', gap: '0.75rem', marginTop: '0.25rem' }}>
                                    <button
                                        onClick={() => handleApprove(p.id)}
                                        style={{
                                            flex: 1,
                                            padding: '0.6rem',
                                            border: 'none',
                                            borderRadius: '8px',
                                            background: 'linear-gradient(135deg, #2ed573, #1abc9c)',
                                            color: '#fff',
                                            fontWeight: 600,
                                            cursor: 'pointer',
                                            fontSize: '0.875rem',
                                        }}
                                    >
                                        ✓ Approve
                                    </button>
                                    <button
                                        onClick={() => handleDeny(p.id)}
                                        style={{
                                            flex: 1,
                                            padding: '0.6rem',
                                            border: 'none',
                                            borderRadius: '8px',
                                            background: 'linear-gradient(135deg, #ff4757, #e74c3c)',
                                            color: '#fff',
                                            fontWeight: 600,
                                            cursor: 'pointer',
                                            fontSize: '0.875rem',
                                        }}
                                    >
                                        ✗ Deny
                                    </button>
                                </div>
                            )}
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
}
