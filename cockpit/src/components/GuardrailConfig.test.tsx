import React from 'react';
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import GuardrailConfig, { DEFAULT_GUARDRAILS } from './GuardrailConfig';

// Mock EnterpriseBadge
vi.mock('./EnterpriseBadge', () => ({
    default: () => <span data-testid="enterprise-badge">Enterprise</span>,
}));

describe('GuardrailConfig', () => {
    it('renders all 5 controls with labels', () => {
        const onChange = vi.fn();
        render(<GuardrailConfig value={DEFAULT_GUARDRAILS} onChange={onChange} />);
        expect(screen.getByText('Human-in-the-Loop')).toBeDefined();
        expect(screen.getByText('Read-Only Mode')).toBeDefined();
        expect(screen.getByText('Timeout')).toBeDefined();
        expect(screen.getByText('Max Tool Calls')).toBeDefined();
        expect(screen.getByText('Max Turns')).toBeDefined();
    });

    it('toggle HITL changes output', () => {
        const onChange = vi.fn();
        render(<GuardrailConfig value={DEFAULT_GUARDRAILS} onChange={onChange} />);
        const hitlToggle = screen.getByLabelText('Human-in-the-Loop toggle');
        fireEvent.click(hitlToggle);
        expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ hitl: true }));
    });

    it('timeout slider updates value and label', () => {
        const onChange = vi.fn();
        render(<GuardrailConfig value={DEFAULT_GUARDRAILS} onChange={onChange} />);
        const slider = screen.getByLabelText('Timeout minutes');
        fireEvent.change(slider, { target: { value: '30' } });
        expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ timeout_minutes: 30 }));
    });
});
