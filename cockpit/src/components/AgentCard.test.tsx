import React from 'react';
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import AgentCard from './AgentCard';
import type { AgentDefinition } from '../hooks/useAgents';

// ============================================================================
// Test Fixtures
// ============================================================================

const mockAgent: AgentDefinition = {
    id: 'agent-1',
    name: 'log-analyzer',
    display_name: 'Log Analyzer',
    description: 'Analyzes log files for errors and patterns, providing summaries and recommendations for debugging.',
    version: '1.0.0',
    type: 'specialist',
    category: 'operations',
    provider: 'openai',
    model: 'gpt-4o',
    system_prompt: 'You are a log analysis specialist.',
    is_builtin: false,
    is_enterprise: false,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
};

const noopFn = () => { };

// ============================================================================
// Tests
// ============================================================================

describe('AgentCard', () => {
    it('renders agent display name', () => {
        render(<AgentCard agent={mockAgent} onDeploy={noopFn} onConfigure={noopFn} onDetails={noopFn} />);
        expect(screen.getByText('Log Analyzer')).toBeDefined();
    });

    it('renders specialist badge', () => {
        render(<AgentCard agent={mockAgent} onDeploy={noopFn} onConfigure={noopFn} onDetails={noopFn} />);
        expect(screen.getByText('Specialist')).toBeDefined();
    });

    it('renders autonomous badge', () => {
        const autoAgent = { ...mockAgent, type: 'autonomous' };
        render(<AgentCard agent={autoAgent} onDeploy={noopFn} onConfigure={noopFn} onDetails={noopFn} />);
        expect(screen.getByText('Autonomous')).toBeDefined();
    });

    it('renders Enterprise badge when is_enterprise=true', () => {
        const entAgent = { ...mockAgent, is_enterprise: true };
        render(<AgentCard agent={entAgent} onDeploy={noopFn} onConfigure={noopFn} onDetails={noopFn} />);
        expect(screen.getByText('Coming Soon')).toBeDefined();
    });

    it('does not render Enterprise badge when is_enterprise=false', () => {
        render(<AgentCard agent={mockAgent} onDeploy={noopFn} onConfigure={noopFn} onDetails={noopFn} />);
        expect(screen.queryByText('Coming Soon')).toBeNull();
    });

    it('renders Core badge when is_builtin=true', () => {
        const builtinAgent = { ...mockAgent, is_builtin: true };
        render(<AgentCard agent={builtinAgent} onDeploy={noopFn} onConfigure={noopFn} onDetails={noopFn} />);
        expect(screen.getByText('Core')).toBeDefined();
    });

    it('fires onDeploy callback when Deploy button clicked', () => {
        const onDeploy = vi.fn();
        render(<AgentCard agent={mockAgent} onDeploy={onDeploy} onConfigure={noopFn} onDetails={noopFn} />);
        fireEvent.click(screen.getByText('Deploy'));
        expect(onDeploy).toHaveBeenCalledWith(mockAgent);
    });

    it('fires onConfigure callback when Edit button clicked', () => {
        const onConfigure = vi.fn();
        render(<AgentCard agent={mockAgent} onDeploy={noopFn} onConfigure={onConfigure} onDetails={noopFn} />);
        fireEvent.click(screen.getByText('Edit'));
        expect(onConfigure).toHaveBeenCalledWith(mockAgent);
    });

    it('fires onDetails callback when Details button clicked', () => {
        const onDetails = vi.fn();
        render(<AgentCard agent={mockAgent} onDeploy={noopFn} onConfigure={noopFn} onDetails={onDetails} />);
        fireEvent.click(screen.getByText('Details'));
        expect(onDetails).toHaveBeenCalledWith(mockAgent);
    });

    it('renders provider and model info', () => {
        render(<AgentCard agent={mockAgent} onDeploy={noopFn} onConfigure={noopFn} onDetails={noopFn} />);
        expect(screen.getByText('openai / gpt-4o')).toBeDefined();
    });

    it('renders category badge', () => {
        render(<AgentCard agent={mockAgent} onDeploy={noopFn} onConfigure={noopFn} onDetails={noopFn} />);
        expect(screen.getByText('operations')).toBeDefined();
    });
});
