import React from 'react';
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import AgentRunView, { RunEvent, RunStatus } from './AgentRunView';

describe('AgentRunView', () => {
  it('renders empty state when no run is provided', () => {
    render(React.createElement(AgentRunView));
    expect(screen.getByText('No active agent run')).toBeDefined();
  });

  it('renders status badge for running state', () => {
    const run: RunEvent = {
      id: 'test-1',
      status: 'running',
      startedAt: new Date(),
      toolCalls: [],
      output: '',
    };
    
    render(React.createElement(AgentRunView, { run }));
    expect(screen.getByText('Running')).toBeDefined();
  });

  it('renders status badge for completed state', () => {
    const run: RunEvent = {
      id: 'test-2',
      status: 'completed',
      startedAt: new Date(),
      endedAt: new Date(),
      toolCalls: [],
      output: 'test output',
      tokenUsage: { input: 100, output: 200 },
    };
    
    render(React.createElement(AgentRunView, { run }));
    expect(screen.getAllByText('Completed').length).toBeGreaterThan(0);
  });

  it('renders status badge for failed state', () => {
    const run: RunEvent = {
      id: 'test-3',
      status: 'failed',
      startedAt: new Date(),
      endedAt: new Date(),
      toolCalls: [],
      output: '',
      error: 'Something went wrong',
    };
    
    render(React.createElement(AgentRunView, { run }));
    expect(screen.getAllByText('Failed').length).toBeGreaterThan(0);
  });

  it('renders status badge for timeout state', () => {
    const run: RunEvent = {
      id: 'test-4',
      status: 'timeout',
      startedAt: new Date(),
      endedAt: new Date(),
      toolCalls: [],
      output: '',
    };
    
    render(React.createElement(AgentRunView, { run }));
    expect(screen.getAllByText('Timeout').length).toBeGreaterThan(0);
  });

  it('renders tool call cards when toolCalls are provided', () => {
    const run: RunEvent = {
      id: 'test-5',
      status: 'running',
      startedAt: new Date(),
      toolCalls: [
        {
          id: 'tool-1',
          name: 'read_file',
          arguments: { path: '/test.txt' },
          result: 'file content',
          startedAt: new Date(),
        },
      ],
      output: '',
    };
    
    render(React.createElement(AgentRunView, { run }));
    expect(screen.getByText('read_file')).toBeDefined();
    expect(screen.getByText('Tool Calls (1)')).toBeDefined();
  });

  it('renders streaming output panel', () => {
    const run: RunEvent = {
      id: 'test-6',
      status: 'running',
      startedAt: new Date(),
      toolCalls: [],
      output: 'Streaming output text here',
    };
    
    render(React.createElement(AgentRunView, { run }));
    expect(screen.getByText('Output')).toBeDefined();
  });

  it('renders token usage counter', () => {
    const run: RunEvent = {
      id: 'test-7',
      status: 'completed',
      startedAt: new Date(),
      endedAt: new Date(),
      toolCalls: [],
      output: '',
      tokenUsage: { input: 1500, output: 2500 },
    };
    
    render(React.createElement(AgentRunView, { run }));
    expect(screen.getByText('Input:')).toBeDefined();
    expect(screen.getByText('Output:')).toBeDefined();
    expect(screen.getByText('Total:')).toBeDefined();
  });

  it('renders run summary panel on completion', () => {
    const run: RunEvent = {
      id: 'test-8',
      status: 'completed',
      startedAt: new Date(Date.now() - 10000),
      endedAt: new Date(),
      toolCalls: [],
      output: '',
      tokenUsage: { input: 100, output: 200 },
      result: 'Task completed successfully',
    };
    
    render(React.createElement(AgentRunView, { run }));
    expect(screen.getByText('Run Summary')).toBeDefined();
  });

  it('accepts className prop', () => {
    const run: RunEvent = {
      id: 'test-9',
      status: 'running',
      startedAt: new Date(),
      toolCalls: [],
      output: '',
    };
    
    const { container } = render(React.createElement(AgentRunView, { run, className: 'custom-class' }));
    expect(container.firstChild?.className).toContain('custom-class');
  });
});
