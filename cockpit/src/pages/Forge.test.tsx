import React from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';

// Track navigation
const mockNavigate = vi.fn();
vi.mock('react-router-dom', async (importOriginal) => {
    const actual = await importOriginal() as Record<string, unknown>;
    return {
        ...actual,
        useNavigate: () => mockNavigate,
    };
});

// Mock hooks
vi.mock('../hooks/useAgents', () => ({
    useAgents: vi.fn(),
    useImportAgent: () => ({ importAgent: vi.fn(), isLoading: false }),
    useExportAgent: () => ({ exportAgent: vi.fn() }),
}));

vi.mock('../hooks/useLicense', () => ({
    useLicense: () => ({
        isCommunity: false,
        isEnterprise: true,
        loading: false,
    }),
}));

// Mock sub-components
vi.mock('./ForgeRegistry', () => ({
    default: ({ onDeploy, onConfigure, onDetails }: {
        onDeploy?: (a: { id: string }) => void;
        onConfigure?: (a: { id: string }) => void;
        onDetails?: (a: { id: string }) => void;
    }) => (
        <div data-testid="forge-registry">
            <button data-testid="deploy-btn" onClick={() => onDeploy?.({ id: 'test-1' })}>Deploy</button>
            <button data-testid="configure-btn" onClick={() => onConfigure?.({ id: 'test-1' })}>Edit</button>
            <button data-testid="details-btn" onClick={() => onDetails?.({ id: 'test-1' })}>Details</button>
        </div>
    ),
}));

vi.mock('../components/AgentDetailPanel', () => ({
    default: ({ isOpen, agent }: { isOpen: boolean; agent: { id: string } }) =>
        isOpen ? <div data-testid="detail-panel">{agent.id}</div> : null,
}));

vi.mock('../components/DeployAgentModal', () => ({
    default: ({ isOpen, agent }: { isOpen: boolean; agent: { id: string } | null }) =>
        isOpen ? <div data-testid="deploy-modal">{agent?.id}</div> : null,
}));

import Forge from './Forge';
import { useAgents } from '../hooks/useAgents';

const mockUseAgents = useAgents as ReturnType<typeof vi.fn>;

describe('Forge', () => {
    beforeEach(() => {
        vi.clearAllMocks();
        mockUseAgents.mockReturnValue({
            agents: [
                { id: 'test-1', name: 'test-agent', display_name: 'Test Agent' },
            ],
            isLoading: false,
            error: null,
            refetch: vi.fn(),
        });
    });

    it('renders ForgeRegistry component', () => {
        render(<MemoryRouter><Forge /></MemoryRouter>);
        expect(screen.getByTestId('forge-registry')).toBeDefined();
    });

    it('opens detail panel when Details is clicked', () => {
        render(<MemoryRouter><Forge /></MemoryRouter>);
        fireEvent.click(screen.getByTestId('details-btn'));
        expect(screen.getByTestId('detail-panel')).toBeDefined();
    });

    it('opens deploy modal when Deploy is clicked', () => {
        render(<MemoryRouter><Forge /></MemoryRouter>);
        fireEvent.click(screen.getByTestId('deploy-btn'));
        expect(screen.getByTestId('deploy-modal')).toBeDefined();
    });

    it('navigates to builder when Edit is clicked', () => {
        render(<MemoryRouter><Forge /></MemoryRouter>);
        fireEvent.click(screen.getByTestId('configure-btn'));
        expect(mockNavigate).toHaveBeenCalledWith('/forge/builder/test-1');
    });
});
