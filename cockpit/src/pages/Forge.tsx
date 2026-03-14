/**
 * Forge — Agent management and deployment interface
 *
 * Production-quality page for creating, managing, and deploying agent definitions.
 * Uses ForgeRegistry as the main content with AgentDetailPanel for details.
 */

import React, { useState, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAgents, useImportAgent, useExportAgent, type AgentDefinition } from '../hooks/useAgents';
import { useLicense } from '../hooks/useLicense';
import ForgeRegistry from './ForgeRegistry';
import AgentDetailPanel from '../components/AgentDetailPanel';
import DeployAgentModal from '../components/DeployAgentModal';

// ============================================================================
// Forge Page
// ============================================================================

const Forge: React.FC = () => {
    const navigate = useNavigate();
    const { isEnterprise } = useLicense();

    // State
    const [selectedAgent, setSelectedAgent] = useState<AgentDefinition | null>(null);
    const [deployAgent, setDeployAgent] = useState<AgentDefinition | null>(null);
    const [isPanelOpen, setIsPanelOpen] = useState(false);

    // Import functionality
    const fileInputRef = useRef<HTMLInputElement>(null);
    const { importAgent, isLoading: isImporting } = useImportAgent();
    const { exportAgent } = useExportAgent();

    // Data
    const { agents, refetch } = useAgents();

    // Handlers
    const handleViewDetails = (agent: AgentDefinition) => {
        setSelectedAgent(agent);
        setIsPanelOpen(true);
    };

    const handleDeploy = (agent: AgentDefinition) => {
        setDeployAgent(agent);
    };

    const handleEdit = (agent: AgentDefinition) => {
        navigate(`/forge/builder/${agent.id}`);
    };

    const handleClone = (agent: AgentDefinition) => {
        navigate(`/forge/builder?clone=${agent.id}`);
    };

    const handleDelete = async (_agent: AgentDefinition) => {
        // Delete is handled by the panel - we just refresh
        await refetch();
        setIsPanelOpen(false);
        setSelectedAgent(null);
    };

    const handleExport = async (agent: AgentDefinition) => {
        try {
            await exportAgent(agent.id, agent.name);
        } catch (err) {
            console.error('Export failed:', err);
        }
    };

    const handleClosePanel = () => {
        setIsPanelOpen(false);
        setSelectedAgent(null);
    };

    // Import handler
    const handleImport = async (e: React.ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        if (!file) return;

        try {
            await importAgent(file);
            await refetch();
        } catch (err) {
            console.error('Import failed:', err);
        }

        // Reset file input
        if (fileInputRef.current) {
            fileInputRef.current.value = '';
        }
    };

    // Find the selected agent from the agents list
    const currentAgent = selectedAgent
        ? agents.find(a => a.id === selectedAgent.id) || selectedAgent
        : null;

    // ============================================================================
    // Render
    // ============================================================================

    return (
        <div>
            {/* Hidden file input for import */}
            <input
                ref={fileInputRef}
                type="file"
                accept=".yaml,.yml,.json"
                style={{ display: 'none' }}
                onChange={handleImport}
            />

            {/* Main content - ForgeRegistry */}
            <ForgeRegistry
                onDeploy={handleDeploy}
                onConfigure={handleEdit}
                onDetails={handleViewDetails}
                onClone={handleClone}
            />

            {/* Agent Detail Panel */}
            {currentAgent && (
                <AgentDetailPanel
                    agent={currentAgent}
                    isOpen={isPanelOpen}
                    onClose={handleClosePanel}
                    onDeploy={handleDeploy}
                    onEdit={handleEdit}
                    onDelete={handleDelete}
                />
            )}

            {/* Deploy Agent Modal */}
            <DeployAgentModal
                isOpen={!!deployAgent}
                agent={deployAgent}
                onClose={() => setDeployAgent(null)}
                onDeployed={() => refetch()}
            />
        </div>
    );
};

export default Forge;
