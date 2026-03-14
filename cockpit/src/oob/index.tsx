/**
 * OOB UI Renderer
 * 
 * Dynamic component loading and rendering for out-of-band UI payloads
 * received from WebTransport stream. Supports FileDiff, MarkdownPreview,
 * Confirmation modals, and more.
 */

import React, { useState, useCallback, useEffect, useRef } from 'react';
import { FileDiff, FileDiffProps, createFileDiffProps } from './components/FileDiff';
import { MarkdownPreview, MarkdownPreviewProps, createMarkdownPreviewProps } from './components/MarkdownPreview';
import { Confirmation, ConfirmationProps, createConfirmationProps } from './components/Confirmation';

// OOB Payload Types
export type OOBPayloadType = 
  | 'FileDiff' 
  | 'MarkdownPreview' 
  | 'Confirmation' 
  | 'FileTree'
  | 'Terminal'
  | 'Form'
  | 'Progress'
  | 'Error'
  | 'Info';

export interface OOBPayload {
  id: string;
  type: OOBPayloadType;
  timestamp?: number;
  [key: string]: any;
}

// FileTree component placeholder
interface FileTreeProps {
  root: string;
  files: Array<{
    name: string;
    path: string;
    type: 'file' | 'directory';
    children?: any[];
  }>;
  selectedPath?: string;
  onSelect?: (path: string) => void;
}

// Terminal component placeholder
interface TerminalProps {
  sessionId: string;
  onData?: (data: string) => void;
}

// Form component placeholder
interface FormProps {
  fields: Array<{
    name: string;
    label: string;
    type: 'text' | 'password' | 'email' | 'number' | 'checkbox' | 'select';
    options?: string[];
    required?: boolean;
    value?: any;
  }>;
  onSubmit: (values: Record<string, any>) => void;
  onCancel?: () => void;
}

// Progress component placeholder
interface ProgressProps {
  title: string;
  message?: string;
  value?: number;
  indeterminate?: boolean;
}

// Error component placeholder
interface ErrorProps {
  title?: string;
  message: string;
  code?: string;
  details?: string;
  onRetry?: () => void;
  onDismiss?: () => void;
}

// Info component placeholder
interface InfoProps {
  title?: string;
  message: string;
  type?: 'info' | 'success' | 'warning' | 'error';
  dismissible?: boolean;
  onDismiss?: () => void;
}

// Component registry
interface ComponentRegistry {
  [key: string]: {
    component: React.ComponentType<any>;
    createProps: (payload: OOBPayload) => any;
  };
}

// Forward declarations for components that will be registered
// These are defined later in the file but referenced in the registry
const FileTree: React.FC<any>;
const Terminal: React.FC<any>;
const Form: React.FC<any>;
const Progress: React.FC<any>;
const ErrorDisplay: React.FC<any>;
const InfoDisplay: React.FC<any>;

const componentRegistry: ComponentRegistry = {
  FileDiff: {
    component: FileDiff,
    createProps: createFileDiffProps,
  },
  MarkdownPreview: {
    component: MarkdownPreview,
    createProps: createMarkdownPreviewProps,
  },
  Confirmation: {
    component: (props: ConfirmationProps) => {
      // Confirmation needs special handling since it has callbacks
      return null; // Will be handled separately
    },
    createProps: () => ({}),
  },
  // Additional components registered for consistency
  FileTree: {
    component: FileTree as any,
    createProps: (payload: OOBPayload) => payload,
  },
  Terminal: {
    component: Terminal as any,
    createProps: (payload: OOBPayload) => payload,
  },
  Form: {
    component: Form as any,
    createProps: (payload: OOBPayload) => payload,
  },
  Progress: {
    component: Progress as any,
    createProps: (payload: OOBPayload) => payload,
  },
  Error: {
    component: ErrorDisplay as any,
    createProps: (payload: OOBPayload) => payload,
  },
  Info: {
    component: InfoDisplay as any,
    createProps: (payload: OOBPayload) => payload,
  },
};

// OOB UI Context for managing pending confirmations
interface OOBUIContextValue {
  payloads: OOBPayload[];
  pendingConfirmations: OOBPayload[];
  addPayload: (payload: OOBPayload) => void;
  removePayload: (id: string) => void;
  clearPayloads: () => void;
  confirm: (id: string) => void;
  cancel: (id: string) => void;
}

const OOBUIContext = React.createContext<OOBUIContextValue | null>(null);

export const useOOBUI = () => {
  const context = React.useContext(OOBUIContext);
  if (!context) {
    throw new Error('useOOBUI must be used within an OOBUIRoot provider');
  }
  return context;
};

// FileTree component
const FileTree: React.FC<FileTreeProps> = ({ root, files, selectedPath, onSelect }) => {
  const renderFile = (file: any, depth: number = 0) => {
    const isSelected = selectedPath === file.path;
    const indent = depth * 16;

    return (
      <div key={file.path} className="file-tree-item-wrapper">
        <div
          className={`file-tree-item ${file.type} ${isSelected ? 'selected' : ''}`}
          style={{ paddingLeft: `${indent + 8}px` }}
          onClick={() => onSelect?.(file.path)}
        >
          <span className="file-icon">
            {file.type === 'directory' ? '📁' : '📄'}
          </span>
          <span className="file-name">{file.name}</span>
        </div>
        {file.type === 'directory' && file.children && (
          <div className="file-tree-children">
            {file.children.map((child: any) => renderFile(child, depth + 1))}
          </div>
        )}
      </div>
    );
  };

  return (
    <div className="file-tree-container">
      <div className="file-tree-root">{root}</div>
      <div className="file-tree-content">
        {files.map((file) => renderFile(file))}
      </div>
    </div>
  );
};

// Terminal component
const Terminal: React.FC<TerminalProps> = ({ sessionId, onData }) => {
  const [output, setOutput] = useState<string>('');

  useEffect(() => {
    // Terminal rendering would be connected to a real terminal
    // This is a placeholder
  }, [sessionId]);

  return (
    <div className="terminal-container">
      <div className="terminal-header">
        <span>Terminal - {sessionId}</span>
      </div>
      <div className="terminal-output">
        <pre>{output}</pre>
      </div>
    </div>
  );
};

// Form component
const Form: React.FC<FormProps> = ({ fields, onSubmit, onCancel }) => {
  const [values, setValues] = useState<Record<string, any>>(() => {
    const initial: Record<string, any> = {};
    fields.forEach((field) => {
      initial[field.name] = field.value ?? (field.type === 'checkbox' ? false : '');
    });
    return initial;
  });

  const handleChange = (name: string, value: any) => {
    setValues((prev) => ({ ...prev, [name]: value }));
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSubmit(values);
  };

  return (
    <form className="oob-form" onSubmit={handleSubmit}>
      {fields.map((field) => (
        <div key={field.name} className="form-field">
          <label htmlFor={field.name}>
            {field.label}
            {field.required && <span className="required">*</span>}
          </label>
          {field.type === 'select' ? (
            <select
              id={field.name}
              value={values[field.name]}
              onChange={(e) => handleChange(field.name, e.target.value)}
              required={field.required}
            >
              <option value="">Select...</option>
              {field.options?.map((opt) => (
                <option key={opt} value={opt}>{opt}</option>
              ))}
            </select>
          ) : field.type === 'checkbox' ? (
            <input
              type="checkbox"
              id={field.name}
              checked={values[field.name]}
              onChange={(e) => handleChange(field.name, e.target.checked)}
            />
          ) : (
            <input
              type={field.type}
              id={field.name}
              value={values[field.name]}
              onChange={(e) => handleChange(field.name, e.target.value)}
              required={field.required}
            />
          )}
        </div>
      ))}
      <div className="form-actions">
        {onCancel && (
          <button type="button" className="btn btn-secondary" onClick={onCancel}>
            Cancel
          </button>
        )}
        <button type="submit" className="btn btn-primary">Submit</button>
      </div>
    </form>
  );
};

// Progress component
const Progress: React.FC<ProgressProps> = ({ title, message, value, indeterminate }) => {
  const progressValue = indeterminate ? undefined : Math.min(100, Math.max(0, value ?? 0));

  return (
    <div className="progress-container">
      <div className="progress-title">{title}</div>
      {message && <div className="progress-message">{message}</div>}
      <div className="progress-bar">
        {indeterminate ? (
          <div className="progress-bar-indeterminate" />
        ) : (
          <div className="progress-bar-value" style={{ width: `${progressValue}%` }} />
        )}
      </div>
      {progressValue !== undefined && (
        <div className="progress-percentage">{progressValue}%</div>
      )}
    </div>
  );
};

// ErrorDisplay component
const ErrorDisplay: React.FC<ErrorProps> = ({ title, message, code, details, onRetry, onDismiss }) => {
  return (
    <div className="error-container">
      <div className="error-icon">⚠️</div>
      <div className="error-title">{title || 'Error'}</div>
      <div className="error-message">{message}</div>
      {code && <div className="error-code">Code: {code}</div>}
      {details && <pre className="error-details">{details}</pre>}
      <div className="error-actions">
        {onRetry && (
          <button className="btn btn-primary" onClick={onRetry}>Retry</button>
        )}
        {onDismiss && (
          <button className="btn btn-secondary" onClick={onDismiss}>Dismiss</button>
        )}
      </div>
    </div>
  );
};

// InfoDisplay component
const InfoDisplay: React.FC<InfoProps> = ({ title, message, type = 'info', dismissible, onDismiss }) => {
  return (
    <div className={`info-container info-${type}`}>
      <div className="info-icon">
        {type === 'success' ? '✓' : type === 'warning' ? '⚠' : type === 'error' ? '✕' : 'ℹ'}
      </div>
      <div className="info-content">
        {title && <div className="info-title">{title}</div>}
        <div className="info-message">{message}</div>
      </div>
      {dismissible && onDismiss && (
        <button className="info-dismiss" onClick={onDismiss}>×</button>
      )}
    </div>
  );
};

// Dynamic component renderer
interface DynamicRendererProps {
  payload: OOBPayload;
  onConfirm?: (id: string) => void;
  onCancel?: (id: string) => void;
}

const DynamicRenderer: React.FC<DynamicRendererProps> = ({ payload, onConfirm, onCancel }) => {
  const { type, id } = payload;

  // Handle Confirmation specially since it needs callbacks
  if (type === 'Confirmation' && onConfirm && onCancel) {
    const props = createConfirmationProps(payload, onConfirm, onCancel);
    return <Confirmation {...props} />;
  }

  // Get component from registry
  const registryEntry = componentRegistry[type];
  if (registryEntry) {
    const Component = registryEntry.component;
    const props = registryEntry.createProps(payload);
    return <Component {...props} />;
  }

  // Handle additional component types
  switch (type) {
    case 'FileTree':
      return <FileTree {...payload} />;
    case 'Terminal':
      return <Terminal {...payload} />;
    case 'Form':
      return <Form {...payload} />;
    case 'Progress':
      return <Progress {...payload} />;
    case 'Error':
      return <ErrorDisplay {...payload} />;
    case 'Info':
      return <InfoDisplay {...payload} />;
    default:
      return (
        <div className="unknown-oob-type">
          Unknown OOB type: {type}
        </div>
      );
  }
};

// OOB UI Root component
interface OOBUIRootProps {
  children: React.ReactNode;
  onPayload?: (payload: OOBPayload) => void;
  onConfirm?: (id: string, payload: OOBPayload) => void;
  onCancel?: (id: string, payload: OOBPayload) => void;
}

export const OOBUIRoot: React.FC<OOBUIRootProps> = ({ 
  children, 
  onPayload,
  onConfirm: externalOnConfirm,
  onCancel: externalOnCancel,
}) => {
  const [payloads, setPayloads] = useState<OOBPayload[]>([]);
  const [pendingConfirmations, setPendingConfirmations] = useState<OOBPayload[]>([]);

  // Handle incoming payloads
  const addPayload = useCallback((payload: OOBPayload) => {
    const enrichedPayload = {
      ...payload,
      timestamp: payload.timestamp ?? Date.now(),
    };
    
    setPayloads((prev) => [...prev, enrichedPayload]);
    
    // If it's a confirmation, track it specially
    if (enrichedPayload.type === 'Confirmation') {
      setPendingConfirmations((prev) => [...prev, enrichedPayload]);
    }
    
    onPayload?.(enrichedPayload);
  }, [onPayload]);

  const removePayload = useCallback((id: string) => {
    setPayloads((prev) => prev.filter((p) => p.id !== id));
    setPendingConfirmations((prev) => prev.filter((p) => p.id !== id));
  }, []);

  const clearPayloads = useCallback(() => {
    setPayloads([]);
    setPendingConfirmations([]);
  }, []);

  const handleConfirm = useCallback((id: string) => {
    const confirmation = pendingConfirmations.find((p) => p.id === id);
    if (confirmation) {
      externalOnConfirm?.(id, confirmation);
      removePayload(id);
    }
  }, [pendingConfirmations, externalOnConfirm, removePayload]);

  const handleCancel = useCallback((id: string) => {
    const confirmation = pendingConfirmations.find((p) => p.id === id);
    if (confirmation) {
      externalOnCancel?.(id, confirmation);
      removePayload(id);
    }
  }, [pendingConfirmations, externalOnCancel, removePayload]);

  const contextValue: OOBUIContextValue = {
    payloads,
    pendingConfirmations,
    addPayload,
    removePayload,
    clearPayloads,
    confirm: handleConfirm,
    cancel: handleCancel,
  };

  return (
    <OOBUIContext.Provider value={contextValue}>
      {children}
      {/* Render pending confirmations */}
      <PendingConfirmations
        confirmations={pendingConfirmations.map((p) => ({
          id: p.id,
          props: createConfirmationProps(p, handleConfirm, handleCancel),
        }))}
        onConfirm={handleConfirm}
        onCancel={handleCancel}
      />
    </OOBUIContext.Provider>
  );
};

// OOB Renderer for displaying non-confirmation payloads
interface OOBRendererProps {
  payloads?: OOBPayload[];
  className?: string;
}

export const OOBRenderer: React.FC<OOBRendererProps> = ({ payloads = [], className = '' }) => {
  // Use context payloads if not provided as prop
  const contextPayloads = payloads.length > 0 ? payloads : useOOBUI().payloads;
  const { onConfirm, onCancel } = useOOBUI();

  // Filter out confirmations (they're handled separately)
  const visiblePayloads = contextPayloads.filter((p) => p.type !== 'Confirmation');

  if (visiblePayloads.length === 0) {
    return null;
  }

  return (
    <div className={`oob-renderer ${className}`}>
      {visiblePayloads.map((payload) => (
        <div key={payload.id} className={`oob-payload oob-${payload.type.toLowerCase()}`}>
          <DynamicRenderer 
            payload={payload} 
            onConfirm={onConfirm}
            onCancel={onCancel}
          />
        </div>
      ))}
    </div>
  );
};

// Hook for receiving payloads from WebTransport
export const useOOBPayloadReceiver = (
  stream: ReadableStream<Uint8Array> | null,
  onPayload?: (payload: OOBPayload) => void
) => {
  const addPayload = useOOBUI().addPayload;
  const readerRef = useRef<ReadableStreamDefaultReader<Uint8Array> | null>(null);

  useEffect(() => {
    if (!stream) return;

    const reader = stream.getReader();
    readerRef.current = reader;

    const processMessages = async () => {
      try {
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          // Parse the message (assuming JSON)
          const text = new TextDecoder().decode(value);
          try {
            const payload = JSON.parse(text) as OOBPayload;
            addPayload(payload);
            onPayload?.(payload);
          } catch (parseError) {
            console.error('Failed to parse OOB payload:', parseError);
          }
        }
      } catch (error) {
        console.error('Error reading OOB stream:', error);
      }
    };

    processMessages();

    return () => {
      reader.cancel();
    };
  }, [stream, addPayload, onPayload]);

  return readerRef.current;
};

// Utility function to create OOB payloads
export const createOOBPayload = (
  type: OOBPayloadType,
  data: Record<string, any>
): OOBPayload => {
  return {
    id: data.id || `${type}-${Date.now()}`,
    type,
    ...data,
  };
};

// Utility function to send OOB response
export const sendOOBResponse = async (
  stream: WritableStream<Uint8Array> | null,
  response: Record<string, any>
): Promise<void> => {
  if (!stream) {
    console.warn('No stream available for OOB response');
    return;
  }

  const writer = stream.getWriter();
  try {
    const data = JSON.stringify(response);
    await writer.write(new TextEncoder().encode(data));
  } finally {
    writer.releaseLock();
  }
};

// Export all components and types
export {
  FileDiff,
  MarkdownPreview,
  Confirmation,
  FileTree,
  Terminal,
  Form,
  Progress,
  ErrorDisplay as Error,
  InfoDisplay as Info,
};

export type {
  OOBPayload,
  OOBPayloadType,
  FileDiffProps,
  MarkdownPreviewProps,
  ConfirmationProps,
  FileTreeProps,
  TerminalProps,
  FormProps,
  ProgressProps,
  ErrorProps,
  InfoProps,
};

export default {
  OOBUIRoot,
  OOBRenderer,
  useOOBPayloadReceiver,
  useOOBUI,
  createOOBPayload,
  sendOOBResponse,
  FileDiff,
  MarkdownPreview,
  Confirmation,
};
