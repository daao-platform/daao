import React, { useState, useEffect, ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import { getSatellites, createSession, type Satellite } from '../api/client';
import { XIcon } from './Icons';

// Agent binary presets
const AGENT_PRESETS = ['claude', 'gemini', 'powershell', 'cmd', 'custom'] as const;
type AgentPreset = typeof AGENT_PRESETS[number];

// Form data type
interface FormData {
  name: string;
  satellite_id: string;
  agent_preset: AgentPreset;
  custom_binary: string;
  agent_args: string;
  working_dir: string;
  cols: number;
  rows: number;
}

// Component props
interface NewSessionModalProps {
  isOpen: boolean;
  onClose: () => void;
  onCreated?: () => void;
}

const NewSessionModal: React.FC<NewSessionModalProps> = ({ isOpen, onClose, onCreated }) => {
  const navigate = useNavigate();
  const [satellites, setSatellites] = useState<Satellite[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [formData, setFormData] = useState<FormData>({
    name: '',
    satellite_id: '',
    agent_preset: 'claude',
    custom_binary: '',
    agent_args: '',
    working_dir: '',
    cols: 80,
    rows: 24,
  });

  // Load satellites on mount
  useEffect(() => {
    if (isOpen) {
      getSatellites()
        .then(setSatellites)
        .catch((err) => {
          console.error('Failed to load satellites:', err);
          setError('Failed to load satellites');
        });
    }
  }, [isOpen]);

  // Handle input changes
  const handleChange = (field: keyof FormData, value: string | number) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
    setError(null);
  };

  // Handle form submission
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setLoading(true);

    try {
      // Parse agent args (comma-separated)
      const agentArgs = formData.agent_args
        ? formData.agent_args.split(',').map((arg) => arg.trim()).filter(Boolean)
        : undefined;

      const agentBinary = formData.agent_preset === 'custom'
        ? formData.custom_binary.trim()
        : formData.agent_preset;

      if (!agentBinary) {
        setError('Please enter a custom binary path');
        setLoading(false);
        return;
      }

      const newSession = await createSession({
        name: formData.name,
        satellite_id: formData.satellite_id,
        agent_binary: agentBinary,
        agent_args: agentArgs,
        working_dir: formData.working_dir || undefined,
        cols: formData.cols,
        rows: formData.rows,
      });

      // Navigate to the new session
      navigate(`/session/${newSession.id}`);
      onClose();

      // Call onCreated callback if provided
      if (onCreated) {
        onCreated();
      }

      // Reset form
      setFormData({
        name: '',
        satellite_id: '',
        agent_preset: 'claude',
        custom_binary: '',
        agent_args: '',
        working_dir: '',
        cols: 80,
        rows: 24,
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create session');
    } finally {
      setLoading(false);
    }
  };

  // Handle overlay click (close modal)
  const handleOverlayClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget) {
      onClose();
    }
  };

  // Don't render if not open
  if (!isOpen) {
    return null;
  }

  // Render form group helper
  const renderFormGroup = (label: string, fieldId: string, children: ReactNode, required?: boolean) => {
    return React.createElement('div', { className: 'form-group' },
      React.createElement('label', { htmlFor: fieldId, className: 'form-label' },
        label,
        required && React.createElement('span', { className: 'form-label--required' }, ' *')
      ),
      children
    );
  };

  // Render text input
  const renderTextInput = (fieldId: string, value: string | number, onChange: (value: string) => void, inputProps?: React.InputHTMLAttributes<HTMLInputElement>) => {
    return React.createElement('input', {
      id: fieldId,
      type: 'text',
      className: 'form-input',
      value: value,
      onChange: (e) => onChange(e.target.value),
      ...inputProps,
    });
  };

  // Render number input
  const renderNumberInput = (fieldId: string, value: number, onChange: (value: number) => void, inputProps?: React.InputHTMLAttributes<HTMLInputElement>) => {
    return React.createElement('input', {
      id: fieldId,
      type: 'number',
      className: 'form-input form-input--number',
      value: value,
      onChange: (e) => onChange(parseInt(e.target.value, 10) || 0),
      ...inputProps,
    });
  };

  // Render select
  const renderSelect = (fieldId: string, value: string, onChange: (value: string) => void, options: Array<{ value: string; label: string }>) => {
    return React.createElement('select', {
      id: fieldId,
      className: 'form-select',
      value: value,
      onChange: (e: React.ChangeEvent<HTMLSelectElement>) => onChange(e.target.value),
    },
      options.map((opt) =>
        React.createElement('option', { key: opt.value, value: opt.value }, opt.label)
      )
    );
  };

  // Render textarea
  const renderTextarea = (fieldId: string, value: string, onChange: (value: string) => void, placeholder?: string) => {
    return React.createElement('textarea', {
      id: fieldId,
      className: 'form-input form-textarea',
      value: value,
      onChange: (e: React.ChangeEvent<HTMLTextAreaElement>) => onChange(e.target.value),
      placeholder: placeholder,
      rows: 3,
    });
  };

  // Build the modal
  return React.createElement('div', { className: 'modal-overlay', onClick: handleOverlayClick },
    React.createElement('div', { className: 'modal', role: 'dialog', 'aria-modal': 'true', 'aria-labelledby': 'modal-title' },
      // Header
      React.createElement('div', { className: 'modal__header' },
        React.createElement('h2', { id: 'modal-title', className: 'modal__title' }, 'New Session'),
        React.createElement('button', {
          className: 'modal__close',
          onClick: onClose,
          'aria-label': 'Close modal',
          type: 'button',
        }, React.createElement(XIcon, { size: 20 }))
      ),

      // Body
      React.createElement('form', { onSubmit: handleSubmit },
        React.createElement('div', { className: 'modal__body' },
          // Error message
          error && React.createElement('div', { className: 'form-error' }, error),

          // Session name
          renderFormGroup('Session Name', 'session-name',
            renderTextInput('session-name', formData.name, (v) => handleChange('name', v), { required: true, placeholder: 'My Session' }),
            true
          ),

          // Satellite dropdown
          renderFormGroup('Satellite', 'satellite',
            React.createElement('div', null,
              renderSelect('satellite', formData.satellite_id, (v) => handleChange('satellite_id', v),
                [
                  { value: '', label: 'Select a satellite' },
                  ...satellites.map((s) => ({
                    value: s.id,
                    label: s.status === 'active' ? s.name : `${s.name} (${s.status} — unavailable)`,
                  })),
                ]
              ),
              formData.satellite_id && satellites.find(s => s.id === formData.satellite_id)?.status !== 'active' &&
                React.createElement('div', {
                  style: { fontSize: 12, color: 'var(--error, #ef4444)', marginTop: 4 },
                }, '⚠ This satellite is not active. Sessions cannot be created until the daemon connects.')
            ),
            true
          ),

          // Agent binary dropdown + custom input
          renderFormGroup('Agent', 'agent-binary',
            React.createElement('div', { className: 'form-row' },
              renderSelect('agent-binary', formData.agent_preset, (v) => handleChange('agent_preset', v),
                AGENT_PRESETS.map((preset) => ({ value: preset, label: preset === 'custom' ? 'Custom...' : preset }))
              ),
              formData.agent_preset === 'custom' && renderTextInput('agent-binary-custom', formData.custom_binary, (v) => handleChange('custom_binary', v), { placeholder: 'e.g. C:\\Windows\\System32\\cmd.exe' })
            ),
            true
          ),

          // Agent args
          renderFormGroup('Agent Args (optional)', 'agent-args',
            renderTextarea('agent-args', formData.agent_args, (v) => handleChange('agent_args', v), '--verbose, --model gpt-4')
          ),

          // Working directory
          renderFormGroup('Working Directory (optional)', 'working-dir',
            renderTextInput('working-dir', formData.working_dir, (v) => handleChange('working_dir', v), {
              placeholder: 'e.g. C:\\Users\\user\\myproject (leave blank for home directory)',
            }),
            false
          ),

          // Terminal dimensions
          renderFormGroup('Terminal Dimensions', 'terminal-dims',
            React.createElement('div', { className: 'form-row form-row--dims' },
              React.createElement('div', { className: 'form-group form-group--inline' },
                React.createElement('label', { htmlFor: 'cols', className: 'form-label' }, 'Cols'),
                renderNumberInput('cols', formData.cols, (v) => handleChange('cols', v), { min: 1, max: 500 })
              ),
              React.createElement('div', { className: 'form-group form-group--inline' },
                React.createElement('label', { htmlFor: 'rows', className: 'form-label' }, 'Rows'),
                renderNumberInput('rows', formData.rows, (v) => handleChange('rows', v), { min: 1, max: 200 })
              )
            )
          )
        ),

        // Footer
        React.createElement('div', { className: 'modal__footer' },
          React.createElement('button', {
            type: 'button',
            className: 'btn btn--secondary',
            onClick: onClose,
            disabled: loading,
          }, 'Cancel'),
          React.createElement('button', {
            type: 'submit',
            className: 'btn btn--primary',
            disabled: loading || !formData.name || !formData.satellite_id ||
              (formData.agent_preset === 'custom' && !formData.custom_binary.trim()) ||
              satellites.find(s => s.id === formData.satellite_id)?.status !== 'active',
          }, loading ? 'Creating...' : 'Create Session')
        )
      )
    )
  );
};

export default NewSessionModal;
