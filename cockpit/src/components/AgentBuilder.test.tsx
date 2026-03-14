import React from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

// Mock react-router-dom
const mockNavigate = vi.fn();
vi.mock('react-router-dom', () => ({
    useNavigate: () => mockNavigate,
    useParams: () => ({}),
    useSearchParams: () => [new URLSearchParams(), vi.fn()],
}));

// Mock hooks
vi.mock('../hooks/useAgents', () => ({
    useCreateAgent: () => ({
        createAgent: vi.fn().mockResolvedValue({ id: '1', name: 'test' }),
        isCreating: false,
        error: null,
    }),
    useDeployAgent: () => ({
        deploy: vi.fn().mockResolvedValue(null),
        isDeploying: false,
        error: null,
    }),
    useUpdateAgent: () => ({
        updateAgent: vi.fn().mockResolvedValue({ id: '1', name: 'test' }),
        isUpdating: false,
        error: null,
    }),
    useAgentDetail: () => ({
        agent: null,
        runs: [],
        isLoading: false,
        error: null,
    }),
}));

const mockUseLicense = vi.fn();
vi.mock('../hooks/useLicense', () => ({
    useLicense: () => mockUseLicense(),
}));

vi.mock('./Toast', () => ({
    useToast: () => ({ showToast: vi.fn() }),
}));

vi.mock('./Icons', () => ({
    ArrowLeftIcon: () => <span>←</span>,
}));

vi.mock('./UpgradeCard', () => ({
    default: () => <div data-testid="upgrade-card">UpgradeCard</div>,
}));

vi.mock('./EnterpriseBadge', () => ({
    default: () => <span data-testid="enterprise-badge">Enterprise</span>,
}));

import AgentBuilder from './AgentBuilder';

describe('AgentBuilder', () => {
    beforeEach(() => {
        vi.clearAllMocks();
        mockUseLicense.mockReturnValue({
            isCommunity: false,
            isEnterprise: true,
            loading: false,
            license: { tier: 'enterprise' },
        });
    });

    it('renders step 1 (Identity) by default', () => {
        render(<AgentBuilder />);
        expect(screen.getByText('Agent Identity')).toBeDefined();
        expect(screen.getByPlaceholderText('my-agent')).toBeDefined();
        expect(screen.getByPlaceholderText('My Agent')).toBeDefined();
    });

    it('next button advances to step 2', () => {
        render(<AgentBuilder />);
        // Fill required fields
        fireEvent.change(screen.getByPlaceholderText('my-agent'), { target: { value: 'test-agent' } });
        fireEvent.change(screen.getByPlaceholderText('My Agent'), { target: { value: 'Test Agent' } });
        fireEvent.click(screen.getByText('Next →'));
        expect(screen.getByText('Brain Configuration')).toBeDefined();
    });

    it('back button returns to previous step', () => {
        render(<AgentBuilder />);
        // Advance to step 2
        fireEvent.change(screen.getByPlaceholderText('my-agent'), { target: { value: 'test-agent' } });
        fireEvent.change(screen.getByPlaceholderText('My Agent'), { target: { value: 'Test Agent' } });
        fireEvent.click(screen.getByText('Next →'));
        expect(screen.getByText('Brain Configuration')).toBeDefined();
        // Go back
        fireEvent.click(screen.getByText('← Back'));
        expect(screen.getByText('Agent Identity')).toBeDefined();
    });

    it('step validation blocks advance on empty fields', () => {
        render(<AgentBuilder />);
        fireEvent.click(screen.getByText('Next →'));
        // Should still be on step 1 with error messages
        expect(screen.getByText('Name is required')).toBeDefined();
        expect(screen.getByText('Display name is required')).toBeDefined();
    });

    it('all 6 steps render without crash', () => {
        render(<AgentBuilder />);
        // Step 1
        expect(screen.getByText('Agent Identity')).toBeDefined();
        fireEvent.change(screen.getByPlaceholderText('my-agent'), { target: { value: 'test-agent' } });
        fireEvent.change(screen.getByPlaceholderText('My Agent'), { target: { value: 'Test Agent' } });
        fireEvent.click(screen.getByText('Next →'));

        // Step 2
        expect(screen.getByText('Brain Configuration')).toBeDefined();
        fireEvent.change(screen.getByPlaceholderText('gpt-4o'), { target: { value: 'gpt-4o' } });
        // Fill system prompt via the textarea
        const textarea = document.querySelector('.spe-editor__textarea') as HTMLTextAreaElement;
        if (textarea) fireEvent.change(textarea, { target: { value: 'You are a helpful agent.' } });
        fireEvent.click(screen.getByText('Next →'));

        // Step 3
        expect(screen.getByText('Tool Configuration')).toBeDefined();
        fireEvent.click(screen.getByText('Next →'));

        // Step 4
        expect(screen.getAllByText('Guardrails').length).toBeGreaterThan(0);
        fireEvent.click(screen.getByText('Next →'));

        // Step 5
        expect(screen.getAllByText('Deployment').length).toBeGreaterThan(0);
        fireEvent.click(screen.getByText('Next →'));

        // Step 6
        expect(screen.getAllByText(/Review/).length).toBeGreaterThan(0);
    });

    it('license gate shows UpgradeCard for Community', () => {
        mockUseLicense.mockReturnValue({
            isCommunity: true,
            isEnterprise: false,
            loading: false,
            license: { tier: 'community' },
        });
        render(<AgentBuilder />);
        expect(screen.getByTestId('upgrade-card')).toBeDefined();
        expect(screen.getByText('Coming Soon')).toBeDefined();
    });
});
