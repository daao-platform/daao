/**
 * AgentDetailDrawer — Slide-in drawer for viewing agent details
 * 
 * Displays agent information in a slide-out drawer with tabbed navigation:
 * - Overview: Description, system prompt (full), tools (parsed from JSONB), guardrails (parsed from JSONB)
 * - Runs: Run history with tokens, cost, tool call count
 * - Configuration: Editable JSON for custom; read-only for built-in
 * - Deploy: Deploy to satellite (works for both built-in and custom)
 */

import React, { useState, useEffect, useCallback, ReactNode } from 'react';
import { XIcon } from './Icons';
import { useAgentDetail, useDeployAgent, useUpdateAgent, type AgentDefinition, type AgentRun } from '../hooks/useAgents';
import { getSatellites, type Satellite } from '../api/client';
import { useToast } from './Toast';

// ============================================================================
// Types
// ============================================================================

export interface AgentDetailDrawerProps {
  /** Whether the drawer is open */
  isOpen: boolean;
  /** Agent ID to display */
  agentId: string | null;
  /** Callback when drawer should close */
  onClose: () => void;
  /** Whether the agent is built-in (read-only config) */
  isBuiltIn?: boolean;
  /** Initial tab to open */
  initialTab?: TabId;
}

// Tab types
type TabId = 'overview' | 'runs' | 'configuration' | 'schedule' | 'deploy';

interface Tab {
  id: TabId;
  label: string;
}

// ============================================================================
// Constants
// ============================================================================

const TABS: Tab[] = [
  { id: 'overview', label: 'Overview' },
  { id: 'runs', label: 'Runs' },
  { id: 'configuration', label: 'Configuration' },
  { id: 'schedule', label: 'Schedule' },
  { id: 'deploy', label: 'Deploy' },
];

// ============================================================================
// Helper Functions
// ============================================================================

/** Format duration from start and end timestamps */
const formatDuration = (startedAt: string, endedAt?: string): string => {
  if (!startedAt) return 'N/A';
  const start = new Date(startedAt).getTime();
  const end = endedAt ? new Date(endedAt).getTime() : Date.now();
  const ms = end - start;

  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(2)}s`;
  const minutes = Math.floor(ms / 60000);
  const seconds = ((ms % 60000) / 1000).toFixed(0);
  return `${minutes}m ${seconds}s`;
};

/** Format date as relative time */
const formatRelativeTime = (dateStr: string): string => {
  if (!dateStr) return 'N/A';
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return 'Just now';
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.floor(hrs / 24);
  if (days < 7) return `${days}d ago`;
  return new Date(dateStr).toLocaleDateString();
};

/** Format date to readable string */
const formatDate = (dateStr: string): string => {
  if (!dateStr) return 'N/A';
  return new Date(dateStr).toLocaleString();
};

/** Parse JSONB field — handles string or object */
const parseJsonField = (field: unknown): Record<string, unknown> | null => {
  if (!field) return null;
  if (typeof field === 'object' && field !== null) return field as Record<string, unknown>;
  if (typeof field === 'string') {
    try {
      return JSON.parse(field);
    } catch {
      return null;
    }
  }
  return null;
};

/** Format cost */
const formatCost = (cost?: number): string => {
  if (cost === undefined || cost === null) return '—';
  return `$${cost.toFixed(4)}`;
};

/** Format token count */
const formatTokens = (tokens?: number): string => {
  if (tokens === undefined || tokens === null) return '—';
  return tokens.toLocaleString();
};

// ============================================================================
// Sub-Components
// ============================================================================

/** Status badge component */
const RunStatusBadge: React.FC<{ status: AgentRun['status'] }> = ({ status }) => {
  const getStatusClass = () => {
    switch (status) {
      case 'completed': return 'status-badge--completed';
      case 'running': return 'status-badge--running';
      case 'failed': return 'status-badge--failed';
      case 'pending': return 'status-badge--pending';
      case 'cancelled': return 'status-badge--cancelled';
      default: return '';
    }
  };

  return React.createElement('span', {
    className: `status-badge ${getStatusClass()}`,
  }, status);
};

/** Overview Tab Content */
const OverviewTab: React.FC<{ agent: AgentDefinition | null }> = ({ agent }) => {
  if (!agent) {
    return React.createElement('div', { className: 'drawer-tab-content--loading' }, 'Loading...');
  }

  // Parse tools_config from JSONB
  const toolsConfig = parseJsonField(agent.tools_config);
  const toolAllow = toolsConfig?.allow as string[] | undefined;
  const toolDeny = toolsConfig?.deny as string[] | undefined;
  const toolsList = toolAllow || (toolsConfig ? Object.keys(toolsConfig) : []);

  // Parse guardrails from JSONB
  const guardrails = parseJsonField(agent.guardrails);
  const hitlEnabled = guardrails?.hitl_enabled as boolean | undefined;
  const readOnly = guardrails?.read_only as boolean | undefined;
  const timeout = guardrails?.timeout as number | undefined;
  const maxToolCalls = guardrails?.max_tool_calls as number | undefined;

  return React.createElement('div', { className: 'drawer-tab-content' },
    // Description
    React.createElement('div', { className: 'drawer-section' },
      React.createElement('h4', { className: 'drawer-section-title' }, 'Description'),
      React.createElement('p', { className: 'drawer-section-text' }, agent.description || 'No description available.')
    ),

    // System Prompt (full, scrollable)
    React.createElement('div', { className: 'drawer-section' },
      React.createElement('h4', { className: 'drawer-section-title' }, 'System Prompt'),
      React.createElement('pre', {
        className: 'drawer-code-block drawer-code-block--full',
        style: { maxHeight: '200px', overflow: 'auto' },
      }, agent.system_prompt || 'No system prompt defined.')
    ),

    // Tools
    React.createElement('div', { className: 'drawer-section' },
      React.createElement('h4', { className: 'drawer-section-title' }, 'Tools'),
      toolsList.length > 0
        ? React.createElement('div', null,
          toolAllow && React.createElement('div', { style: { marginBottom: 8 } },
            React.createElement('span', { style: { fontSize: 12, color: 'var(--text-muted)', marginRight: 8 } }, 'Allow:'),
            React.createElement('div', { className: 'drawer-badges', style: { display: 'inline-flex' } },
              toolAllow.map((tool, i) =>
                React.createElement('span', { key: i, className: 'badge badge--tool' }, tool)
              )
            )
          ),
          toolDeny && React.createElement('div', null,
            React.createElement('span', { style: { fontSize: 12, color: 'var(--text-muted)', marginRight: 8 } }, 'Deny:'),
            React.createElement('div', { className: 'drawer-badges', style: { display: 'inline-flex' } },
              toolDeny.map((tool, i) =>
                React.createElement('span', { key: i, className: 'badge badge--guardrail' }, tool)
              )
            )
          ),
          !toolAllow && !toolDeny && React.createElement('div', { className: 'drawer-badges' },
            toolsList.map((tool, i) =>
              React.createElement('span', { key: i, className: 'badge badge--tool' }, String(tool))
            )
          )
        )
        : React.createElement('span', { className: 'text-muted' }, 'No tools configured')
    ),

    // Guardrails
    React.createElement('div', { className: 'drawer-section' },
      React.createElement('h4', { className: 'drawer-section-title' }, 'Guardrails'),
      guardrails
        ? React.createElement('div', { className: 'drawer-meta-grid' },
          React.createElement('div', { className: 'drawer-meta-item' },
            React.createElement('span', { className: 'drawer-meta-label' }, 'Human-in-the-Loop'),
            React.createElement('span', { className: 'drawer-meta-value' }, hitlEnabled ? '✓ Enabled' : '✗ Disabled')
          ),
          React.createElement('div', { className: 'drawer-meta-item' },
            React.createElement('span', { className: 'drawer-meta-label' }, 'Read-only'),
            React.createElement('span', { className: 'drawer-meta-value' }, readOnly ? '✓ Yes' : '✗ No')
          ),
          timeout !== undefined && React.createElement('div', { className: 'drawer-meta-item' },
            React.createElement('span', { className: 'drawer-meta-label' }, 'Timeout'),
            React.createElement('span', { className: 'drawer-meta-value' }, `${timeout}s`)
          ),
          maxToolCalls !== undefined && React.createElement('div', { className: 'drawer-meta-item' },
            React.createElement('span', { className: 'drawer-meta-label' }, 'Max Tool Calls'),
            React.createElement('span', { className: 'drawer-meta-value' }, String(maxToolCalls))
          )
        )
        : React.createElement('span', { className: 'text-muted' }, 'No guardrails configured')
    ),

    // Metadata
    React.createElement('div', { className: 'drawer-section' },
      React.createElement('h4', { className: 'drawer-section-title' }, 'Metadata'),
      React.createElement('div', { className: 'drawer-meta-grid' },
        React.createElement('div', { className: 'drawer-meta-item' },
          React.createElement('span', { className: 'drawer-meta-label' }, 'Provider'),
          React.createElement('span', { className: 'drawer-meta-value' }, agent.provider || 'N/A')
        ),
        React.createElement('div', { className: 'drawer-meta-item' },
          React.createElement('span', { className: 'drawer-meta-label' }, 'Model'),
          React.createElement('span', { className: 'drawer-meta-value' }, agent.model || 'N/A')
        ),
        React.createElement('div', { className: 'drawer-meta-item' },
          React.createElement('span', { className: 'drawer-meta-label' }, 'Version'),
          React.createElement('span', { className: 'drawer-meta-value' }, agent.version)
        ),
        React.createElement('div', { className: 'drawer-meta-item' },
          React.createElement('span', { className: 'drawer-meta-label' }, 'Author'),
          React.createElement('span', { className: 'drawer-meta-value' }, agent.author || 'Unknown')
        ),
        React.createElement('div', { className: 'drawer-meta-item' },
          React.createElement('span', { className: 'drawer-meta-label' }, 'Created'),
          React.createElement('span', { className: 'drawer-meta-value' }, formatDate(agent.created_at))
        ),
        React.createElement('div', { className: 'drawer-meta-item' },
          React.createElement('span', { className: 'drawer-meta-label' }, 'Updated'),
          React.createElement('span', { className: 'drawer-meta-value' }, formatDate(agent.updated_at))
        )
      )
    )
  );
};

/** Runs Tab Content */
const RunsTab: React.FC<{ runs: AgentRun[]; isLoading: boolean }> = ({ runs, isLoading }) => {
  if (isLoading) {
    return React.createElement('div', { className: 'drawer-tab-content--loading' }, 'Loading runs...');
  }

  if (!runs || runs.length === 0) {
    return React.createElement('div', { className: 'drawer-tab-content' },
      React.createElement('div', { className: 'drawer-empty' },
        React.createElement('div', { style: { fontSize: 14, color: 'var(--text-muted)' } }, 'No runs yet'),
        React.createElement('div', { style: { fontSize: 12, color: 'var(--text-muted)', marginTop: 4 } }, 'Deploy this agent to see run history')
      )
    );
  }

  return React.createElement('div', { className: 'drawer-tab-content' },
    React.createElement('table', { className: 'drawer-table' },
      React.createElement('thead', null,
        React.createElement('tr', null,
          React.createElement('th', null, 'Status'),
          React.createElement('th', null, 'Started'),
          React.createElement('th', null, 'Duration'),
          React.createElement('th', null, 'Tokens'),
          React.createElement('th', null, 'Cost'),
          React.createElement('th', null, 'Tool Calls')
        )
      ),
      React.createElement('tbody', null,
        runs.map((run) =>
          React.createElement('tr', { key: run.id },
            React.createElement('td', null, React.createElement(RunStatusBadge, { status: run.status })),
            React.createElement('td', null, formatRelativeTime(run.started_at)),
            React.createElement('td', null, formatDuration(run.started_at, run.ended_at)),
            React.createElement('td', null, formatTokens(run.total_tokens)),
            React.createElement('td', null, formatCost(run.estimated_cost)),
            React.createElement('td', null, run.tool_call_count !== undefined ? String(run.tool_call_count) : '—')
          )
        )
      )
    )
  );
};

/** Configuration Tab Content */
const ConfigurationTab: React.FC<{ agent: AgentDefinition | null; isBuiltIn: boolean }> = ({ agent, isBuiltIn }) => {
  const { updateAgent, isUpdating } = useUpdateAgent();
  const { showToast } = useToast();
  const [configText, setConfigText] = useState('');
  const [saveError, setSaveError] = useState<string | null>(null);

  // Provider/model state for built-in agents
  const [provider, setProvider] = useState('');
  const [model, setModel] = useState('');

  useEffect(() => {
    if (agent) {
      setConfigText(JSON.stringify(agent, null, 2));
      setProvider(agent.provider === 'configurable' ? '' : (agent.provider || ''));
      setModel(agent.model === 'default' ? '' : (agent.model || ''));
    }
  }, [agent]);

  if (!agent) {
    return React.createElement('div', { className: 'drawer-tab-content--loading' }, 'Loading configuration...');
  }

  const handleSaveConfig = async () => {
    setSaveError(null);
    try {
      const parsed = JSON.parse(configText);
      const result = await updateAgent(agent.id, parsed);
      if (result) {
        showToast('Configuration saved', 'success');
      } else {
        setSaveError('Failed to save configuration');
      }
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Invalid JSON');
    }
  };

  const handleSaveProviderModel = async () => {
    setSaveError(null);
    if (!provider.trim() || !model.trim()) {
      setSaveError('Both provider and model are required');
      return;
    }
    const result = await updateAgent(agent.id, { provider: provider.trim(), model: model.trim() });
    if (result) {
      showToast('Provider and model updated', 'success');
    } else {
      setSaveError('Failed to save');
    }
  };

  if (isBuiltIn) {
    // Built-in agents: show focused Provider/Model config + read-only details
    return React.createElement('div', { className: 'drawer-tab-content' },
      React.createElement('div', { className: 'drawer-notice drawer-notice--info', style: { marginBottom: 16 } },
        'This is a built-in agent. Only provider and model can be configured. Clone this agent to fully customize it.'
      ),

      // Provider/Model form
      React.createElement('div', { className: 'drawer-section' },
        React.createElement('h4', { className: 'drawer-section-title' }, 'LLM Configuration'),
        React.createElement('div', { style: { display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, marginBottom: 12 } },
          React.createElement('div', null,
            React.createElement('label', { className: 'drawer-meta-label', style: { display: 'block', marginBottom: 4 } }, 'Provider'),
            React.createElement('select', {
              className: 'forge-form__select',
              value: provider,
              onChange: (e: React.ChangeEvent<HTMLSelectElement>) => setProvider(e.target.value),
              style: { width: '100%' },
            },
              React.createElement('option', { value: '' }, 'Select provider...'),
              React.createElement('option', { value: 'anthropic' }, 'Anthropic'),
              React.createElement('option', { value: 'openai' }, 'OpenAI'),
              React.createElement('option', { value: 'google' }, 'Google'),
              React.createElement('option', { value: 'minimax' }, 'MiniMax'),
              React.createElement('option', { value: 'azure' }, 'Azure OpenAI'),
              React.createElement('option', { value: 'mistral' }, 'Mistral'),
              React.createElement('option', { value: 'deepseek' }, 'DeepSeek'),
              React.createElement('option', { value: 'xai' }, 'xAI (Grok)'),
              React.createElement('option', { value: 'ollama' }, 'Ollama (Local)'),
            )
          ),
          React.createElement('div', null,
            React.createElement('label', { className: 'drawer-meta-label', style: { display: 'block', marginBottom: 4 } }, 'Model'),
            React.createElement('input', {
              type: 'text',
              className: 'forge-form__input',
              placeholder: 'e.g., gpt-4o, claude-3-sonnet',
              value: model,
              onChange: (e: React.ChangeEvent<HTMLInputElement>) => setModel(e.target.value),
              style: { width: '100%' },
            })
          )
        ),
        React.createElement('div', { style: { display: 'flex', alignItems: 'center', gap: 8 } },
          React.createElement('button', {
            className: 'btn btn--primary btn--sm',
            onClick: handleSaveProviderModel,
            disabled: isUpdating || (!provider.trim() || !model.trim()),
          }, isUpdating ? 'Saving...' : 'Save'),
          saveError && React.createElement('span', {
            style: { color: 'var(--danger)', fontSize: 13 },
          }, saveError)
        )
      ),

      // Read-only config reference
      React.createElement('div', { className: 'drawer-section', style: { marginTop: 16 } },
        React.createElement('h4', { className: 'drawer-section-title' }, 'Full Configuration (Read-only)'),
        React.createElement('pre', {
          className: 'drawer-code-block drawer-code-block--full',
          style: { maxHeight: '40vh', overflow: 'auto' },
        }, configText)
      )
    );
  }

  // Custom agents: full editable config
  return React.createElement('div', { className: 'drawer-tab-content' },
    React.createElement('div', { className: 'drawer-config-actions' },
      React.createElement('button', {
        className: 'btn btn--primary btn--sm',
        onClick: handleSaveConfig,
        disabled: isUpdating,
      }, isUpdating ? 'Saving...' : 'Save Changes'),
      saveError && React.createElement('span', {
        style: { color: 'var(--danger)', fontSize: 13, marginLeft: 8 },
      }, saveError)
    ),
    React.createElement('textarea', {
      className: 'drawer-config-textarea',
      value: configText,
      onChange: (e: React.ChangeEvent<HTMLTextAreaElement>) => setConfigText(e.target.value),
      spellCheck: false,
    })
  );
};

/** Deploy Tab Content */
const DeployTab: React.FC<{
  agentId: string;
  agentName: string;
  deploy: (agentId: string, request?: { satellite_id: string }) => Promise<unknown>;
  isDeploying: boolean;
}> = ({ agentId, agentName, deploy, isDeploying }) => {
  const [satellites, setSatellites] = useState<Satellite[]>([]);
  const [selectedSatellite, setSelectedSatellite] = useState<string>('');
  const [deployResult, setDeployResult] = useState<string | null>(null);
  const { showToast } = useToast();

  // Load satellites
  useEffect(() => {
    getSatellites()
      .then((sats) => {
        const active = (Array.isArray(sats) ? sats : []).filter(s => s.status === 'active');
        setSatellites(active);
      })
      .catch((err) => console.error('Failed to load satellites:', err));
  }, []);

  const handleDeploy = async () => {
    if (!selectedSatellite) return;

    try {
      const result = await deploy(agentId, { satellite_id: selectedSatellite }) as { run_id?: string } | null;
      if (result) {
        const runId = result.run_id || 'unknown';
        setDeployResult(runId);
        showToast(`Agent "${agentName}" deployed successfully`, 'success');
      } else {
        setDeployResult(null);
        showToast('Deployment failed', 'error');
      }
    } catch (err) {
      setDeployResult(null);
      showToast(`Error: ${err instanceof Error ? err.message : 'Unknown error'}`, 'error');
    }
  };

  return React.createElement('div', { className: 'drawer-tab-content' },
    // Satellite selector
    React.createElement('div', { className: 'drawer-section' },
      React.createElement('h4', { className: 'drawer-section-title' }, 'Select Satellite'),
      React.createElement('select', {
        className: 'forge-form__select',
        value: selectedSatellite,
        onChange: (e: React.ChangeEvent<HTMLSelectElement>) => setSelectedSatellite(e.target.value),
        style: { width: '100%' },
      },
        React.createElement('option', { value: '' }, 'Select a satellite...'),
        satellites.map((satellite) =>
          React.createElement('option', { key: satellite.id, value: satellite.id }, satellite.name)
        )
      ),
      satellites.length === 0 && React.createElement('div', { className: 'drawer-notice drawer-notice--warning', style: { marginTop: 8 } },
        'No active satellites available'
      )
    ),

    // Deploy button
    React.createElement('div', { className: 'drawer-section' },
      React.createElement('button', {
        className: 'btn btn--primary',
        onClick: handleDeploy,
        disabled: !selectedSatellite || isDeploying,
        style: { width: '100%' },
      }, isDeploying ? 'Deploying...' : 'Deploy'),

      deployResult && React.createElement('div', {
        style: { marginTop: 12, textAlign: 'center' },
      },
        React.createElement('div', { style: { color: 'var(--success)', fontWeight: 600, marginBottom: 4 } }, '✓ Deployment Initiated'),
        React.createElement('div', { style: { fontSize: 13, color: 'var(--text-secondary)' } },
          'Run ID: ',
          React.createElement('a', {
            href: `/forge/run/${deployResult}`,
            style: { color: 'var(--accent)' },
          }, deployResult)
        )
      )
    )
  );
};

/** Schedule Tab Content — embeds ScheduleConfig and TriggerConfig components */
const ScheduleTab: React.FC<{ agent: AgentDefinition | null; isBuiltIn: boolean }> = ({ agent, isBuiltIn }) => {
  // Dynamically import the components to avoid circular dependencies
  const [ScheduleConfig, setScheduleConfig] = useState<React.ComponentType<any> | null>(null);
  const [TriggerConfigComponent, setTriggerConfigComponent] = useState<React.ComponentType<any> | null>(null);

  useEffect(() => {
    import('./ScheduleConfig').then(mod => setScheduleConfig(() => mod.ScheduleConfig || mod.default));
    import('./TriggerConfig').then(mod => setTriggerConfigComponent(() => mod.default));
  }, []);

  if (!agent) {
    return React.createElement('div', { className: 'drawer-tab-content--loading' }, 'Loading...');
  }

  if (isBuiltIn) {
    return React.createElement('div', { className: 'drawer-tab-content' },
      React.createElement('div', { className: 'drawer-notice drawer-notice--info' },
        'Scheduling is not available for built-in agents.'
      )
    );
  }

  // Parse existing schedule/trigger from agent
  const schedule = agent.schedule ? (() => { try { return JSON.parse(agent.schedule as string); } catch { return null; } })() : null;
  const trigger = agent.trigger ? (() => { try { return JSON.parse(agent.trigger as string); } catch { return null; } })() : null;

  return React.createElement('div', { className: 'drawer-tab-content' },
    // Schedule section
    React.createElement('div', { className: 'drawer-section' },
      React.createElement('h4', { className: 'drawer-section-title' }, 'Cron Schedule'),
      ScheduleConfig
        ? React.createElement(ScheduleConfig, { agentId: agent.id, initialSchedule: schedule })
        : React.createElement('div', { className: 'drawer-tab-content--loading' }, 'Loading schedule config...')
    ),

    // Trigger section
    React.createElement('div', { className: 'drawer-section', style: { marginTop: 24 } },
      React.createElement('h4', { className: 'drawer-section-title' }, 'Event Trigger'),
      TriggerConfigComponent
        ? React.createElement(TriggerConfigComponent, { agentId: agent.id, trigger })
        : React.createElement('div', { className: 'drawer-tab-content--loading' }, 'Loading trigger config...')
    )
  );
};

// ============================================================================
// Main Component
// ============================================================================

const AgentDetailDrawer: React.FC<AgentDetailDrawerProps> = ({
  isOpen,
  agentId,
  onClose,
  isBuiltIn = false,
  initialTab,
}) => {
  const [activeTab, setActiveTab] = useState<TabId>(initialTab || 'overview');
  const { agent, runs, isLoading } = useAgentDetail(agentId || '');
  const { deploy, isDeploying } = useDeployAgent();

  // Handle Escape key
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && isOpen) {
        onClose();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose]);

  // Handle backdrop click
  const handleBackdropClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget) {
      onClose();
    }
  };

  // Reset tab when agent changes
  useEffect(() => {
    if (agentId) {
      setActiveTab(initialTab || 'overview');
    }
  }, [agentId, initialTab]);

  // Don't render if not open
  if (!isOpen) {
    return null;
  }

  // Render tab content based on active tab
  const renderTabContent = (): ReactNode => {
    switch (activeTab) {
      case 'overview':
        return React.createElement(OverviewTab, { agent });
      case 'runs':
        return React.createElement(RunsTab, { runs, isLoading });
      case 'configuration':
        return React.createElement(ConfigurationTab, { agent, isBuiltIn });
      case 'schedule':
        return React.createElement(ScheduleTab, { agent, isBuiltIn });
      case 'deploy':
        return React.createElement(DeployTab, {
          agentId: agentId || '',
          agentName: agent?.display_name || agent?.name || 'Agent',
          deploy: async (id: string, req?: { satellite_id: string }) => deploy(id, req),
          isDeploying
        });
      default:
        return null;
    }
  };

  return React.createElement('div', {
    className: 'drawer-overlay',
    onClick: handleBackdropClick
  },
    React.createElement('div', {
      className: 'drawer',
      role: 'dialog',
      'aria-modal': 'true',
      'aria-labelledby': 'drawer-title'
    },
      // Header
      React.createElement('div', { className: 'drawer__header' },
        React.createElement('h2', {
          id: 'drawer-title',
          className: 'drawer__title'
        }, agent?.display_name || agent?.name || 'Agent Details'),
        React.createElement('button', {
          className: 'drawer__close',
          onClick: onClose,
          'aria-label': 'Close drawer',
          type: 'button',
        }, React.createElement(XIcon, { size: 20 }))
      ),

      // Tabs
      React.createElement('div', { className: 'drawer__tabs' },
        TABS.map((tab) =>
          React.createElement('button', {
            key: tab.id,
            className: `drawer__tab ${activeTab === tab.id ? 'drawer__tab--active' : ''}`,
            onClick: () => setActiveTab(tab.id),
            type: 'button',
          }, tab.label)
        )
      ),

      // Content
      React.createElement('div', { className: 'drawer__content' },
        isLoading && agentId ?
          React.createElement('div', { className: 'drawer-loading' }, 'Loading...') :
          renderTabContent()
      )
    )
  );
};

export default AgentDetailDrawer;
