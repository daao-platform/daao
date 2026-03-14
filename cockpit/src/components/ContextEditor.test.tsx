import React from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';

// Mock the API client
const mockApiRequest = vi.fn();
vi.mock('../api/client', () => ({
    apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}));

import ContextEditor from './ContextEditor';

const mockFiles = [
    {
        id: 'file-1',
        satellite_id: 'sat-1',
        file_path: 'systeminfo.md',
        content: '# System Info\n\nSome content here.',
        version: 1,
        last_modified_by: 'user@cockpit',
        created_at: '2026-03-07T10:00:00Z',
        updated_at: '2026-03-07T10:00:00Z',
    },
    {
        id: 'file-2',
        satellite_id: 'sat-1',
        file_path: 'runbooks.md',
        content: '# Runbooks',
        version: 2,
        last_modified_by: 'admin@cockpit',
        created_at: '2026-03-06T10:00:00Z',
        updated_at: '2026-03-07T09:00:00Z',
    },
];

function setupDefaultMock() {
    mockApiRequest.mockImplementation((url: string, opts?: RequestInit) => {
        const method = opts?.method || 'GET';
        if (url === '/satellites/sat-1/context' && method === 'GET') {
            return Promise.resolve({ files: mockFiles, count: mockFiles.length });
        }
        if (url === '/satellites/sat-1/context' && method === 'POST') {
            return Promise.resolve({
                id: 'file-3', satellite_id: 'sat-1', file_path: 'new-file.md',
                content: '', version: 1, last_modified_by: 'user@cockpit',
                created_at: '2026-03-07T12:00:00Z', updated_at: '2026-03-07T12:00:00Z',
            });
        }
        if (url.startsWith('/satellites/sat-1/context/') && method === 'PUT') {
            return Promise.resolve({ ...mockFiles[0], version: 2, content: 'modified' });
        }
        if (url.includes('/history')) {
            return Promise.resolve({ history: [], count: 0 });
        }
        if (method === 'DELETE') {
            return Promise.resolve(undefined);
        }
        return Promise.resolve({});
    });
}

/** Helper: wait for files to load by checking for the tab-name elements */
async function waitForFilesLoaded() {
    await waitFor(() => {
        // systeminfo.md appears twice (tab + toolbar), so use getAllByText
        expect(screen.getAllByText('systeminfo.md').length).toBeGreaterThanOrEqual(1);
    });
}

describe('ContextEditor', () => {
    beforeEach(() => {
        vi.clearAllMocks();
        setupDefaultMock();
    });

    it('renders loading state initially', () => {
        mockApiRequest.mockImplementation(() => new Promise(() => { }));
        render(<ContextEditor satelliteId="sat-1" />);
        expect(screen.getByText('Loading context files...')).toBeDefined();
    });

    it('renders tabs for each context file', async () => {
        render(<ContextEditor satelliteId="sat-1" />);
        await waitForFilesLoaded();
        // systeminfo.md appears in tab + toolbar = 2
        expect(screen.getAllByText('systeminfo.md').length).toBe(2);
        // runbooks.md only appears in the tab (not active, so no toolbar)
        expect(screen.getAllByText('runbooks.md').length).toBeGreaterThanOrEqual(1);
    });

    it('renders empty state when no files exist', async () => {
        mockApiRequest.mockResolvedValue({ files: [], count: 0 });
        render(<ContextEditor satelliteId="sat-1" />);
        await waitFor(() => {
            expect(screen.getByText('No context files found for this satellite.')).toBeDefined();
        });
    });

    it('shows Add File modal on button click', async () => {
        render(<ContextEditor satelliteId="sat-1" />);
        await waitForFilesLoaded();
        fireEvent.click(screen.getByText('+ Add file'));
        expect(screen.getByText('Add Context File')).toBeDefined();
        expect(screen.getByPlaceholderText('custom-file.md')).toBeDefined();
    });

    it('Save button is disabled when no changes', async () => {
        render(<ContextEditor satelliteId="sat-1" />);
        await waitForFilesLoaded();
        const saveBtn = screen.getByText('Save');
        expect((saveBtn as HTMLButtonElement).disabled).toBe(true);
    });

    it('editing shows Unsaved sync status', async () => {
        render(<ContextEditor satelliteId="sat-1" />);
        await waitForFilesLoaded();
        const textarea = document.querySelector('.context-editor__textarea') as HTMLTextAreaElement;
        fireEvent.change(textarea, { target: { value: 'changed content' } });
        await waitFor(() => {
            expect(screen.getByText('🟡 Unsaved')).toBeDefined();
        });
    });

    it('Save triggers PUT API call', async () => {
        render(<ContextEditor satelliteId="sat-1" />);
        await waitForFilesLoaded();
        const textarea = document.querySelector('.context-editor__textarea') as HTMLTextAreaElement;
        fireEvent.change(textarea, { target: { value: 'modified' } });
        fireEvent.click(screen.getByText('Save'));
        await waitFor(() => {
            expect(mockApiRequest).toHaveBeenCalledWith(
                '/satellites/sat-1/context/file-1',
                expect.objectContaining({ method: 'PUT' })
            );
        });
    });

    it('Create file sends POST API call', async () => {
        render(<ContextEditor satelliteId="sat-1" />);
        await waitForFilesLoaded();
        fireEvent.click(screen.getByText('+ Add file'));
        const input = screen.getByPlaceholderText('custom-file.md');
        fireEvent.change(input, { target: { value: 'new-file.md' } });
        fireEvent.click(screen.getByText('Create Custom'));
        await waitFor(() => {
            expect(mockApiRequest).toHaveBeenCalledWith(
                '/satellites/sat-1/context',
                expect.objectContaining({ method: 'POST' })
            );
        });
    });

    it('calls onError when file load fails', async () => {
        const onError = vi.fn();
        mockApiRequest.mockRejectedValue(new Error('Network error'));
        render(<ContextEditor satelliteId="sat-1" onError={onError} />);
        await waitFor(() => {
            expect(onError).toHaveBeenCalledWith('Network error');
        });
    });
});
