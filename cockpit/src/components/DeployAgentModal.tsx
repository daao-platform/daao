/**
 * DeployAgentModal — Deploy an agent to a satellite
 * 
 * Shows satellite selector, deployment confirmation, and result with run ID.
 */

import React, { useState, useEffect } from 'react';
import { XIcon } from './Icons';
import { useDeployAgent, type AgentDefinition } from '../hooks/useAgents';
import { getSatellites, type Satellite } from '../api/client';
import { useToast } from './Toast';

// ============================================================================
// Types
// ============================================================================

export interface DeployAgentModalProps {
    isOpen: boolean;
    agent: AgentDefinition | null;
    onClose: () => void;
    onDeployed: () => void;
}

// ============================================================================
// Component
// ============================================================================

const DeployAgentModal: React.FC<DeployAgentModalProps> = ({ isOpen, agent, onClose, onDeployed }) => {
    const { deploy, isDeploying } = useDeployAgent();
    const { showToast } = useToast();

    const [satellites, setSatellites] = useState<Satellite[]>([]);
    const [selectedSatellite, setSelectedSatellite] = useState('');
    const [loadingSatellites, setLoadingSatellites] = useState(false);
    const [deployResult, setDeployResult] = useState<{ runId: string } | null>(null);
    const [deployError, setDeployError] = useState<string | null>(null);

    // Load satellites when modal opens
    useEffect(() => {
        if (!isOpen) return;

        setSelectedSatellite('');
        setDeployResult(null);
        setDeployError(null);

        const loadSatellites = async () => {
            setLoadingSatellites(true);
            try {
                const sats = await getSatellites();
                const activeSats = (Array.isArray(sats) ? sats : []).filter(
                    (s: Satellite) => s.status === 'active'
                );
                setSatellites(activeSats);
                if (activeSats.length === 1) {
                    setSelectedSatellite(activeSats[0].id);
                }
            } catch (err) {
                console.error('Failed to fetch satellites:', err);
            } finally {
                setLoadingSatellites(false);
            }
        };

        loadSatellites();
    }, [isOpen]);

    // Handle Escape key
    useEffect(() => {
        const handleKeyDown = (e: KeyboardEvent) => {
            if (e.key === 'Escape' && isOpen) onClose();
        };
        document.addEventListener('keydown', handleKeyDown);
        return () => document.removeEventListener('keydown', handleKeyDown);
    }, [isOpen, onClose]);

    const handleDeploy = async () => {
        if (!agent || !selectedSatellite) return;

        setDeployError(null);
        const result = await deploy(agent.id, { satellite_id: selectedSatellite });

        if (result) {
            setDeployResult({ runId: result.run_id });
            showToast(`Agent "${agent.display_name || agent.name}" deployed successfully`, 'success');
            onDeployed();
        } else {
            setDeployError('Deployment failed. Please try again.');
        }
    };

    if (!isOpen || !agent) return null;

    const selectedSatName = satellites.find(s => s.id === selectedSatellite)?.name;

    return (
        <div className="modal-overlay" onClick={onClose}>
            <div className="modal" onClick={(e) => e.stopPropagation()} style={{ maxWidth: 480 }}>
                {/* Header */}
                <div className="modal__header">
                    <h2 className="modal__title">Deploy Agent</h2>
                    <button className="modal__close" onClick={onClose} type="button" aria-label="Close">
                        <XIcon size={20} />
                    </button>
                </div>

                {/* Body */}
                <div className="modal__body">
                    <p style={{ marginBottom: 16, color: 'var(--text-secondary)' }}>
                        Deploy <strong style={{ color: 'var(--text)' }}>{agent.display_name || agent.name}</strong> to a satellite.
                    </p>

                    {/* Deployment Result */}
                    {deployResult ? (
                        <div style={{ textAlign: 'center', padding: '16px 0' }}>
                            <div style={{ fontSize: 18, fontWeight: 600, color: 'var(--success)', marginBottom: 8 }}>
                                ✓ Deployment Initiated
                            </div>
                            <p style={{ color: 'var(--text-secondary)', marginBottom: 16 }}>
                                Run ID: <code style={{ color: 'var(--accent)' }}>{deployResult.runId}</code>
                            </p>
                            <a
                                href={`/forge/run/${deployResult.runId}`}
                                className="btn btn--primary btn--sm"
                                style={{ textDecoration: 'none', display: 'inline-flex' }}
                            >
                                View Run
                            </a>
                        </div>
                    ) : (
                        <>
                            {/* Satellite Selector */}
                            {loadingSatellites ? (
                                <div style={{ textAlign: 'center', padding: 16 }}>
                                    <div className="spinner" />
                                </div>
                            ) : satellites.length === 0 ? (
                                <div className="drawer-notice drawer-notice--warning">
                                    No active satellites available. Connect a satellite first.
                                </div>
                            ) : (
                                <div className="forge-form__group" style={{ marginBottom: 16 }}>
                                    <label className="forge-form__label">Select Satellite</label>
                                    <select
                                        className="forge-form__select"
                                        value={selectedSatellite}
                                        onChange={(e) => setSelectedSatellite(e.target.value)}
                                    >
                                        <option value="">Select a satellite...</option>
                                        {satellites.map((sat) => (
                                            <option key={sat.id} value={sat.id}>
                                                {sat.name} ({sat.status})
                                            </option>
                                        ))}
                                    </select>
                                </div>
                            )}

                            {/* Confirmation */}
                            {selectedSatellite && selectedSatName && (
                                <div className="drawer-notice drawer-notice--info" style={{ marginBottom: 16 }}>
                                    Agent <strong>{agent.display_name || agent.name}</strong> will be deployed to satellite <strong>{selectedSatName}</strong>.
                                </div>
                            )}

                            {/* Error */}
                            {deployError && (
                                <div className="drawer-notice drawer-notice--warning" style={{ marginBottom: 16 }}>
                                    {deployError}
                                </div>
                            )}
                        </>
                    )}
                </div>

                {/* Footer */}
                {!deployResult && (
                    <div className="modal__footer">
                        <button className="btn btn--outline" onClick={onClose}>Cancel</button>
                        <button
                            className="btn btn--primary"
                            onClick={handleDeploy}
                            disabled={!selectedSatellite || isDeploying}
                        >
                            {isDeploying ? 'Deploying...' : 'Deploy'}
                        </button>
                    </div>
                )}
                {deployResult && (
                    <div className="modal__footer">
                        <button className="btn btn--outline" onClick={onClose}>Close</button>
                    </div>
                )}
            </div>
        </div>
    );
};

export default DeployAgentModal;
