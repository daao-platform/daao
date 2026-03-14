import React, { useState, useEffect, useCallback, useRef } from 'react';
import { apiRequest } from '../api/client';

// ============================================================================
// Types
// ============================================================================

export interface ContextFile {
  id: string;
  satellite_id: string;
  file_path: string;
  content: string;
  version: number;
  last_modified_by: string;
  created_at: string;
  updated_at: string;
  sync_status: 'synced' | 'pending' | 'conflict';
}

export interface ContextFileHistory {
  id: string;
  context_file_id: string;
  version: number;
  content: string;
  modified_by: string;
  modified_at: string;
}

interface ContextEditorProps {
  satelliteId: string;
  onError?: (error: string) => void;
}

// ============================================================================
// API Functions
// ============================================================================

async function getContextFiles(satelliteId: string): Promise<ContextFile[]> {
  const response = await apiRequest<{ files: ContextFile[], count: number }>(`/satellites/${satelliteId}/context`);
  return (response.files || []).map(f => ({ ...f, sync_status: 'synced' as const }));
}

async function getContextFileHistory(satelliteId: string, fileId: string): Promise<ContextFileHistory[]> {
  const response = await apiRequest<{ history: ContextFileHistory[], count: number }>(`/satellites/${satelliteId}/context/${fileId}/history`);
  return response.history || [];
}

async function saveContextFile(satelliteId: string, fileId: string, content: string): Promise<ContextFile> {
  return apiRequest<ContextFile>(`/satellites/${satelliteId}/context/${fileId}`, {
    method: 'PUT',
    body: JSON.stringify({ content }),
  });
}

async function createContextFile(satelliteId: string, filePath: string): Promise<ContextFile> {
  return apiRequest<ContextFile>(`/satellites/${satelliteId}/context`, {
    method: 'POST',
    body: JSON.stringify({ file_path: filePath, content: '' }),
  });
}

async function deleteContextFile(satelliteId: string, fileId: string): Promise<void> {
  await apiRequest(`/satellites/${satelliteId}/context/${fileId}`, {
    method: 'DELETE',
  });
}

// ============================================================================
// Sub-Components
// ============================================================================

/** Sync status indicator dot */
const SyncStatusDot: React.FC<{ status: ContextFile['sync_status'] }> = ({ status }) => {
  const statusClass = {
    synced: 'sync-status--synced',
    pending: 'sync-status--pending',
    conflict: 'sync-status--conflict',
  }[status];
  return <span className={`sync-status-dot ${statusClass}`} title={status} />;
};

const STANDARD_FILES: Array<{ name: string; description: string }> = [
  { name: 'systeminfo.md',    description: 'Role, services, hardware, network' },
  { name: 'runbooks.md',      description: 'SOPs and operational procedures' },
  { name: 'alerts.md',        description: 'Known alert conditions + resolution steps' },
  { name: 'topology.md',      description: 'Network relationships and dependencies' },
  { name: 'secrets-policy.md',description: 'Credential references (no actual values)' },
  { name: 'history.md',       description: 'Recent changes, deployments, incidents' },
  { name: 'monitoring.md',    description: 'Metrics, thresholds, and dashboards' },
  { name: 'dependencies.md',  description: 'Upstream/downstream service dependencies' },
];

/** Modal for adding a new context file */
const AddFileModal: React.FC<{
  isOpen: boolean;
  onClose: () => void;
  onSubmit: (fileName: string) => void;
  existingFiles: string[];
}> = ({ isOpen, onClose, onSubmit, existingFiles }) => {
  const [customName, setCustomName] = useState('');
  const [error, setError] = useState('');

  if (!isOpen) return null;

  const handleStandardPick = (name: string) => {
    onSubmit(name);
    onClose();
  };

  const handleCustomSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const name = customName.trim();
    if (!name) { setError('File name is required'); return; }
    if (!name.endsWith('.md')) { setError('File name must end with .md'); return; }
    onSubmit(name);
    setCustomName('');
    setError('');
    onClose();
  };

  const available = STANDARD_FILES.filter(f => !existingFiles.includes(f.name));

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__header">
          <h3>Add Context File</h3>
          <button className="modal__close" onClick={onClose} aria-label="Close">&times;</button>
        </div>
        <div className="modal__body">
          {available.length > 0 && (
            <>
              <p className="form-label">Standard files</p>
              <div className="context-editor__standard-files">
                {available.map(f => (
                  <button
                    key={f.name}
                    className="context-editor__standard-file-btn"
                    onClick={() => handleStandardPick(f.name)}
                    type="button"
                  >
                    <span className="context-editor__standard-file-name">{f.name}</span>
                    <span className="context-editor__standard-file-desc">{f.description}</span>
                  </button>
                ))}
              </div>
              <p className="form-label" style={{ marginTop: '1rem' }}>Or custom file</p>
            </>
          )}
          <form onSubmit={handleCustomSubmit}>
            <input
              type="text"
              className="form-input"
              placeholder="custom-file.md"
              value={customName}
              onChange={(e) => setCustomName(e.target.value)}
              autoFocus={available.length === 0}
            />
            {error && <p className="form-error">{error}</p>}
            <div className="modal__footer" style={{ paddingTop: '0.75rem' }}>
              <button type="button" className="btn btn--outline" onClick={onClose}>Cancel</button>
              <button type="submit" className="btn btn--primary">Create Custom</button>
            </div>
          </form>
        </div>
      </div>
    </div>
  );
};

/** Version history timeline component */
const HistoryTimeline: React.FC<{
  history: ContextFileHistory[];
  activeVersion?: number | null;
  onSelectVersion: (entry: ContextFileHistory) => void;
}> = ({ history, activeVersion, onSelectVersion }) => {
  if (history.length === 0) {
    return <p className="text-muted" style={{ fontSize: 12, padding: 8 }}>No version history yet.</p>;
  }

  return (
    <div className="history-timeline">
      {history.map((entry, index) => (
        <div
          key={entry.id}
          className={`history-timeline__entry ${activeVersion === entry.version ? 'history-timeline__entry--active' : ''}`}
          onClick={() => onSelectVersion(entry)}
        >
          <div className="history-timeline__dot" />
          {index < history.length - 1 && <div className="history-timeline__line" />}
          <div className="history-timeline__content">
            <div className="history-timeline__header">
              <span className="history-timeline__version">v{entry.version}</span>
              <span className="history-timeline__date">
                {new Date(entry.modified_at).toLocaleString()}
              </span>
            </div>
            <div className="history-timeline__meta">
              by <strong>{entry.modified_by}</strong>
            </div>
          </div>
        </div>
      ))}
    </div>
  );
};

// ============================================================================
// Main Context Editor Component
// ============================================================================

const ContextEditor: React.FC<ContextEditorProps> = ({ satelliteId, onError }) => {
  const [files, setFiles] = useState<ContextFile[]>([]);
  const [activeTab, setActiveTab] = useState<string>('');
  const [selectedFile, setSelectedFile] = useState<ContextFile | null>(null);
  const [content, setContent] = useState('');
  const [originalContent, setOriginalContent] = useState('');
  const [history, setHistory] = useState<ContextFileHistory[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [hasChanges, setHasChanges] = useState(false);
  const [showHistory, setShowHistory] = useState(false);
  const [viewingVersion, setViewingVersion] = useState<ContextFileHistory | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // Load context files on mount
  useEffect(() => {
    loadContextFiles();
  }, [satelliteId]);

  // Load history when showing the panel or changing file
  useEffect(() => {
    if (showHistory && selectedFile) {
      loadHistory(selectedFile.id);
    }
  }, [showHistory, selectedFile]);

  // Update content when selected file changes
  useEffect(() => {
    if (selectedFile) {
      setContent(selectedFile.content);
      setOriginalContent(selectedFile.content);
      setHasChanges(false);
      setViewingVersion(null);
    }
  }, [selectedFile?.id]);

  // Keyboard shortcut: Ctrl+S to save
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault();
        if (hasChanges && !isSaving && selectedFile) {
          handleSave();
        }
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [hasChanges, isSaving, selectedFile, content]);

  const loadContextFiles = async () => {
    try {
      setIsLoading(true);
      const data = await getContextFiles(satelliteId);
      setFiles(data);
      if (data.length > 0 && !selectedFile) {
        setSelectedFile(data[0]);
        setActiveTab(data[0].file_path);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to load context files';
      onError?.(message);
    } finally {
      setIsLoading(false);
    }
  };

  const loadHistory = async (fileId: string) => {
    try {
      const data = await getContextFileHistory(satelliteId, fileId);
      setHistory(data);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to load history';
      onError?.(message);
    }
  };

  const handleSave = useCallback(async () => {
    if (!selectedFile || !hasChanges) return;

    try {
      setIsSaving(true);
      const updated = await saveContextFile(satelliteId, selectedFile.id, content);
      const updatedFile = { ...updated, sync_status: 'synced' as const };
      setFiles((prev) =>
        prev.map((f) => (f.id === updated.id ? updatedFile : f))
      );
      setSelectedFile(updatedFile);
      setOriginalContent(content);
      setHasChanges(false);
      // Refresh history if panel is open
      if (showHistory) {
        loadHistory(selectedFile.id);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to save';
      onError?.(message);
      // Mark as conflict/error on save failure
      setFiles((prev) =>
        prev.map((f) =>
          f.id === selectedFile.id ? { ...f, sync_status: 'conflict' } : f
        )
      );
    } finally {
      setIsSaving(false);
    }
  }, [selectedFile, hasChanges, content, satelliteId, showHistory]);

  const handleContentChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const newContent = e.target.value;
    setContent(newContent);
    const changed = newContent !== originalContent;
    setHasChanges(changed);
    if (selectedFile) {
      const newStatus = changed ? 'pending' : 'synced';
      setSelectedFile({ ...selectedFile, sync_status: newStatus });
      setFiles((prev) =>
        prev.map((f) =>
          f.id === selectedFile.id ? { ...f, sync_status: newStatus } : f
        )
      );
    }
  };

  const handleDiscard = () => {
    if (selectedFile) {
      setContent(originalContent);
      setHasChanges(false);
      setSelectedFile({ ...selectedFile, sync_status: 'synced' });
      setFiles((prev) =>
        prev.map((f) =>
          f.id === selectedFile.id ? { ...f, sync_status: 'synced' } : f
        )
      );
    }
  };

  const handleTabKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Tab') {
      e.preventDefault();
      const textarea = e.currentTarget;
      const start = textarea.selectionStart;
      const end = textarea.selectionEnd;
      const newValue = content.substring(0, start) + '  ' + content.substring(end);
      setContent(newValue);
      setHasChanges(newValue !== originalContent);
      // Restore cursor position after React re-render
      requestAnimationFrame(() => {
        textarea.selectionStart = textarea.selectionEnd = start + 2;
      });
    }
  };

  const handleAddFile = async (fileName: string) => {
    try {
      const newFile = await createContextFile(satelliteId, fileName);
      const fileWithStatus = { ...newFile, sync_status: 'synced' as const };
      setFiles((prev) => [...prev, fileWithStatus]);
      setSelectedFile(fileWithStatus);
      setActiveTab(fileName);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to create file';
      onError?.(message);
    }
  };

  const handleDeleteFile = async (fileId: string, filePath: string) => {
    if (!confirm(`Delete "${filePath}"? This cannot be undone.`)) return;
    try {
      await deleteContextFile(satelliteId, fileId);
      const remaining = files.filter((f) => f.id !== fileId);
      setFiles(remaining);
      if (selectedFile?.id === fileId) {
        if (remaining.length > 0) {
          setSelectedFile(remaining[0]);
          setActiveTab(remaining[0].file_path);
        } else {
          setSelectedFile(null);
          setActiveTab('');
        }
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to delete file';
      onError?.(message);
    }
  };

  const handleTabChange = (filePath: string) => {
    const file = files.find((f) => f.file_path === filePath);
    if (file) {
      setSelectedFile(file);
      setActiveTab(filePath);
      setViewingVersion(null);
    }
  };

  const handleViewVersion = (entry: ContextFileHistory) => {
    setViewingVersion(entry);
    setContent(entry.content);
  };

  const handleExitVersionView = () => {
    setViewingVersion(null);
    if (selectedFile) {
      setContent(originalContent);
    }
  };

  if (isLoading) {
    return (
      <div className="context-editor context-editor--loading">
        <div className="loading-spinner" />
        <p>Loading context files...</p>
      </div>
    );
  }

  return (
    <div className="context-editor">
      {/* Tab bar */}
      <div className="context-editor__tabs">
        {files.map((file) => (
          <button
            key={file.id}
            className={`context-editor__tab ${activeTab === file.file_path ? 'active' : ''}`}
            onClick={() => handleTabChange(file.file_path)}
          >
            <SyncStatusDot status={file.sync_status} />
            <span className="context-editor__tab-name">{file.file_path}</span>
            <span
              className="context-editor__tab-close"
              onClick={(e) => {
                e.stopPropagation();
                handleDeleteFile(file.id, file.file_path);
              }}
              title="Delete file"
            >
              ×
            </span>
          </button>
        ))}
        <button
          className="context-editor__tab context-editor__tab--add"
          onClick={() => setIsModalOpen(true)}
          title="Add new context file"
        >
          + Add file
        </button>
        {/* History toggle */}
        {selectedFile && (
          <button
            className="context-editor__tab"
            style={{ marginLeft: 'auto' }}
            onClick={() => setShowHistory(!showHistory)}
            title={showHistory ? 'Hide history' : 'Show history'}
          >
            🕐 History
          </button>
        )}
      </div>

      {/* Body: editor + optional history sidebar */}
      <div className="context-editor__body">
        {/* Main content area */}
        <div className="context-editor__main">
          {selectedFile ? (
            <div className="context-editor__content">
              {/* Toolbar */}
              <div className="context-editor__toolbar">
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <span className="context-editor__filename">{selectedFile.file_path}</span>
                  <span className="context-editor__shortcut-hint">Ctrl+S to save</span>
                </div>
                <div className="context-editor__actions">
                  <SyncStatusDot status={selectedFile.sync_status} />
                  <span className={`sync-status-label sync-status-label--${selectedFile.sync_status}`}>
                    {selectedFile.sync_status === 'synced' ? '🟢 Synced' :
                      selectedFile.sync_status === 'pending' ? '🟡 Unsaved' : '🔴 Error'}
                  </span>
                  {hasChanges && !viewingVersion && (
                    <button
                      className="btn btn--outline btn--sm"
                      onClick={handleDiscard}
                    >
                      Discard
                    </button>
                  )}
                  {!viewingVersion && (
                    <button
                      className="btn btn--primary btn--sm"
                      onClick={handleSave}
                      disabled={!hasChanges || isSaving}
                    >
                      {isSaving ? 'Saving...' : 'Save'}
                    </button>
                  )}
                </div>
              </div>

              {/* Version viewing banner */}
              {viewingVersion && (
                <div className="context-editor__history-banner">
                  <span>Viewing version {viewingVersion.version} — {new Date(viewingVersion.modified_at).toLocaleString()}</span>
                  <button onClick={handleExitVersionView}>← Back to current</button>
                </div>
              )}

              {/* Markdown textarea */}
              <textarea
                ref={textareaRef}
                className={`context-editor__textarea ${viewingVersion ? 'context-editor__textarea--readonly' : ''}`}
                value={content}
                onChange={viewingVersion ? undefined : handleContentChange}
                onKeyDown={viewingVersion ? undefined : handleTabKeyDown}
                readOnly={!!viewingVersion}
                placeholder={`# ${selectedFile.file_path}\n\nStart writing your context here...\n\n## Notes\n- Use Markdown syntax\n- This content will be synced to the satellite`}
                spellCheck={false}
              />
            </div>
          ) : (
            <div className="context-editor__empty">
              <p>No context files found for this satellite.</p>
              <button className="btn btn--primary" onClick={() => setIsModalOpen(true)}>
                + Add your first context file
              </button>
            </div>
          )}
        </div>

        {/* History sidebar */}
        {showHistory && selectedFile && (
          <div className="context-editor__history-sidebar">
            <div className="context-editor__history-header">
              <h4>Version History</h4>
              <button
                className="context-editor__history-toggle"
                onClick={() => setShowHistory(false)}
                title="Close history"
              >
                ✕
              </button>
            </div>
            <div className="context-editor__history-list">
              <HistoryTimeline
                history={history}
                activeVersion={viewingVersion?.version}
                onSelectVersion={handleViewVersion}
              />
            </div>
          </div>
        )}
      </div>

      {/* Add file modal */}
      <AddFileModal
        isOpen={isModalOpen}
        onClose={() => setIsModalOpen(false)}
        onSubmit={handleAddFile}
        existingFiles={files.map(f => f.file_path)}
      />
    </div>
  );
};

export default ContextEditor;
