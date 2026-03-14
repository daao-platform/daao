import React from 'react';
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import CreateAgentModal from './CreateAgentModal';

// Mock useToast
vi.mock('./Toast', () => ({
    useToast: () => ({ showToast: vi.fn() }),
}));

// Mock useCreateAgent
vi.mock('../hooks/useAgents', () => ({
    useCreateAgent: () => ({
        createAgent: vi.fn().mockResolvedValue(null),
        isCreating: false,
        error: null,
    }),
}));

describe('CreateAgentModal', () => {
    const onClose = vi.fn();
    const onCreated = vi.fn();

    it('renders form fields when open', () => {
        render(<CreateAgentModal isOpen={true} onClose={onClose} onCreated={onCreated} />);
        expect(screen.getAllByText('Create Agent').length).toBeGreaterThan(0);
        expect(screen.getByPlaceholderText('my-agent')).toBeDefined();
        expect(screen.getByPlaceholderText('My Agent')).toBeDefined();
        expect(screen.getByPlaceholderText('gpt-4o')).toBeDefined();
    });

    it('does not render when isOpen=false', () => {
        const { container } = render(<CreateAgentModal isOpen={false} onClose={onClose} onCreated={onCreated} />);
        expect(container.innerHTML).toBe('');
    });

    it('closes on Escape key', () => {
        const closeSpy = vi.fn();
        render(<CreateAgentModal isOpen={true} onClose={closeSpy} onCreated={onCreated} />);
        fireEvent.keyDown(document, { key: 'Escape' });
        expect(closeSpy).toHaveBeenCalled();
    });
});
