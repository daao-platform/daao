import React from 'react';
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import ToolPicker from './ToolPicker';

describe('ToolPicker', () => {
    const defaultValue = { allow: [], deny: [] };

    it('renders available tools in left column', () => {
        const onChange = vi.fn();
        render(<ToolPicker value={defaultValue} onChange={onChange} />);
        // All 7 predefined tools should be in the available column
        expect(screen.getByText('Execute')).toBeDefined();
        expect(screen.getByText('Read')).toBeDefined();
        expect(screen.getByText('Write')).toBeDefined();
        expect(screen.getByText('Process')).toBeDefined();
        expect(screen.getByText('Apply Patch')).toBeDefined();
        expect(screen.getByText('Edit')).toBeDefined();
        expect(screen.getByText('Web Search')).toBeDefined();
    });

    it('clicking a tool moves it to allowed column', () => {
        const onChange = vi.fn();
        render(<ToolPicker value={defaultValue} onChange={onChange} />);
        fireEvent.click(screen.getByText('Execute'));
        expect(onChange).toHaveBeenCalledWith({
            allow: ['exec'],
            deny: [],
        });
    });

    it('read-only mode disables write tools', () => {
        const onChange = vi.fn();
        render(<ToolPicker value={{ allow: ['write', 'read'], deny: [] }} onChange={onChange} />);
        // Toggle read-only mode
        const toggle = screen.getByRole('switch');
        fireEvent.click(toggle);
        // Should have called onChange with write tools denied
        expect(onChange).toHaveBeenCalled();
        const call = onChange.mock.calls[0][0];
        expect(call.deny).toContain('write');
        expect(call.deny).toContain('apply_patch');
        expect(call.deny).toContain('edit');
        expect(call.allow).not.toContain('write');
    });
});
