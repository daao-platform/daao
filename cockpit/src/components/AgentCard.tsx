/**
 * AgentCard — Premium agent card component for the Forge page
 * 
 * Renders an agent definition as a glassmorphism card with:
 * - Category-tinted icon
 * - Name, type badge, provider/model info
 * - Truncated description
 * - Category/Core/Enterprise badges
 * - Deploy, Configure, Details action buttons
 */

import React from 'react';
import type { AgentDefinition } from '../hooks/useAgents';

// ============================================================================
// Types
// ============================================================================

export interface AgentCardProps {
    agent: AgentDefinition;
    onDeploy: (agent: AgentDefinition) => void;
    onConfigure: (agent: AgentDefinition) => void;
    onDetails: (agent: AgentDefinition) => void;
    onClone?: (agent: AgentDefinition) => void;
}

// ============================================================================
// Helpers
// ============================================================================

/** Get icon letter(s) from agent */
function getIconLetter(agent: AgentDefinition): string {
    if (agent.icon) return agent.icon;
    const name = agent.display_name || agent.name;
    return name.charAt(0).toUpperCase();
}

/** Get CSS class for category-tinted icon */
function getIconClass(category?: string): string {
    switch (category) {
        case 'infrastructure': return 'forge-card__icon--infrastructure';
        case 'development': return 'forge-card__icon--development';
        case 'security': return 'forge-card__icon--security';
        case 'operations': return 'forge-card__icon--operations';
        default: return 'forge-card__icon--default';
    }
}

/** Get badge class and label for agent type */
function getTypeBadge(type?: string): { className: string; label: string } | null {
    switch (type) {
        case 'specialist': return { className: 'badge badge--specialist', label: 'Specialist' };
        case 'autonomous': return { className: 'badge badge--autonomous', label: 'Autonomous' };
        default: return null;
    }
}

// ============================================================================
// Component
// ============================================================================

const AgentCard: React.FC<AgentCardProps> = ({ agent, onDeploy, onConfigure, onDetails, onClone }) => {
    const iconLetter = getIconLetter(agent);
    const iconClass = getIconClass(agent.category);
    const typeBadge = getTypeBadge(agent.type);

    return (
        <div className="forge-card" onClick={() => onDetails(agent)}>
            {/* Header: Icon + Name/Provider */}
            <div className="forge-card__header">
                <div className={`forge-card__icon ${iconClass}`}>
                    {iconLetter}
                </div>
                <div className="forge-card__title-row">
                    <div className="forge-card__name">{agent.display_name || agent.name}</div>
                    <div className="forge-card__provider">
                        {agent.provider && agent.model
                            ? `${agent.provider} / ${agent.model}`
                            : agent.provider || 'No provider set'}
                    </div>
                </div>
                {typeBadge && (
                    <span className={typeBadge.className}>{typeBadge.label}</span>
                )}
            </div>

            {/* Description */}
            <div className="forge-card__description">
                {agent.description || 'No description available.'}
            </div>

            {/* Badges */}
            <div className="forge-card__badges">
                {agent.category && (
                    <span className="badge badge--category">{agent.category}</span>
                )}
                {agent.is_builtin && (
                    <span className="badge badge--core">Core</span>
                )}
                {agent.is_enterprise && (
                    <span className="badge badge--enterprise">Coming Soon</span>
                )}
            </div>

            {/* Action Buttons */}
            <div className="forge-card__actions" onClick={(e) => e.stopPropagation()}>
                <button
                    className="btn btn--primary btn--sm"
                    onClick={() => onDeploy(agent)}
                >
                    Deploy
                </button>
                {agent.is_builtin && onClone ? (
                    <button
                        className="btn btn--outline btn--sm"
                        onClick={() => onClone(agent)}
                    >
                        Clone
                    </button>
                ) : (
                    <button
                        className="btn btn--outline btn--sm"
                        onClick={() => onConfigure(agent)}
                    >
                        Edit
                    </button>
                )}
                <button
                    className="btn btn--ghost btn--sm"
                    onClick={() => onDetails(agent)}
                >
                    Details
                </button>
            </div>
        </div>
    );
};

export default AgentCard;
