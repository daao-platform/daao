/**
 * FileDiff Component
 * 
 * Monaco diff viewer for displaying file differences in the sidebar.
 * Supports side-by-side and inline diff views.
 */

import React, { useRef, useEffect } from 'react';

// Monaco types (will be loaded dynamically)
interface Monaco {
  editor: {
    createDiffEditor: (container: HTMLElement, options?: any) => MonacoDiffEditor;
    create: (container: HTMLElement, options?: any) => MonacoEditor;
    DiffEditor: new (container: HTMLElement, options?: any) => MonacoDiffEditor;
    editor: {
      IMarkdownString: any;
    };
  };
}

interface MonacoDiffEditor {
  setModel: (originalModel: MonacoEditorModel, modifiedModel: MonacoEditorModel) => void;
  updateOptions: (options: any) => void;
  dispose: () => void;
}

interface MonacoEditor {
  getValue: () => string;
  setValue: (value: string) => void;
  updateOptions: (options: any) => void;
  dispose: () => void;
}

interface MonacoEditorModel {
  uri: { toString: () => string };
}

export interface FileDiffProps {
  originalContent: string;
  modifiedContent: string;
  originalLanguage?: string;
  modifiedLanguage?: string;
  originalFilename?: string;
  modifiedFilename?: string;
  viewMode?: 'side-by-side' | 'inline';
  readOnly?: boolean;
  onMount?: (editor: MonacoDiffEditor) => void;
}

// Lazy load Monaco editor
let monacoInstance: Monaco | null = null;
let monacoLoadingPromise: Promise<Monaco> | null = null;

async function loadMonaco(): Promise<Monaco> {
  if (monacoInstance) {
    return monacoInstance;
  }
  
  if (monacoLoadingPromise) {
    return monacoLoadingPromise;
  }
  
  monacoLoadingPromise = new Promise((resolve, reject) => {
    // Check if Monaco is already loaded globally
    const globalMonaco = (window as any).monaco;
    if (globalMonaco) {
      monacoInstance = globalMonaco as Monaco;
      resolve(monacoInstance);
      return;
    }
    
    // Load Monaco from CDN
    const script = document.createElement('script');
    script.src = 'https://cdnjs.cloudflare.com/ajax/libs/monaco-editor/0.45.0/min/vs/loader.min.js';
    script.onload = () => {
      const require = (window as any).require;
      require.config({
        paths: {
          vs: 'https://cdnjs.cloudflare.com/ajax/libs/monaco-editor/0.45.0/min/vs',
        },
      });
      
      require(['vs/editor/editor.main'], () => {
        monacoInstance = (window as any).monaco as Monaco;
        resolve(monacoInstance!);
      });
    };
    script.onerror = reject;
    document.head.appendChild(script);
  });
  
  return monacoLoadingPromise;
}

/**
 * FileDiff component with Monaco DiffEditor
 * 
 * Renders a Monaco diff viewer in the sidebar for comparing
 * original and modified file contents.
 */
export const FileDiff: React.FC<FileDiffProps> = ({
  originalContent,
  modifiedContent,
  originalLanguage = 'plaintext',
  modifiedLanguage = 'plaintext',
  originalFilename = 'Original',
  modifiedFilename = 'Modified',
  viewMode = 'side-by-side',
  readOnly = true,
  onMount,
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const editorRef = useRef<MonacoDiffEditor | null>(null);
  const [isLoading, setIsLoading] = React.useState(true);
  const [error, setError] = React.useState<string | null>(null);

  useEffect(() => {
    let isMounted = true;

    const initEditor = async () => {
      try {
        const monaco = await loadMonaco();
        
        if (!isMounted || !containerRef.current) {
          return;
        }

        // Create editor models
        const originalModel = monaco.editor.createModel(
          originalContent,
          originalLanguage
        );
        const modifiedModel = monaco.editor.createModel(
          modifiedContent,
          modifiedLanguage
        );

        // Create diff editor
        const diffEditor = monaco.editor.createDiffEditor(containerRef.current, {
          automaticLayout: true,
          readOnly,
          renderSideBySide: viewMode === 'side-by-side',
          minimap: { enabled: false },
          scrollBeyondLastLine: false,
          diffWordWrap: 'off',
          originalEditable: false,
        });

        diffEditor.setModel({
          original: originalModel,
          modified: modifiedModel,
        });

        if (isMounted) {
          editorRef.current = diffEditor;
          onMount?.(diffEditor);
          setIsLoading(false);
        }
      } catch (err) {
        if (isMounted) {
          setError(err instanceof Error ? err.message : 'Failed to load Monaco editor');
          setIsLoading(false);
        }
      }
    };

    initEditor();

    return () => {
      isMounted = false;
      if (editorRef.current) {
        editorRef.current.dispose();
        editorRef.current = null;
      }
    };
  }, [originalContent, modifiedContent, originalLanguage, modifiedLanguage, viewMode, readOnly]);

  if (error) {
    return (
      <div className="file-diff-error">
        <div className="error-message">Failed to load diff viewer: {error}</div>
        <div className="diff-fallback">
          <div className="diff-panel">
            <div className="diff-header">{originalFilename}</div>
            <pre className="diff-content">{originalContent}</pre>
          </div>
          <div className="diff-panel">
            <div className="diff-header">{modifiedFilename}</div>
            <pre className="diff-content">{modifiedContent}</pre>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="file-diff-container">
      {isLoading && (
        <div className="file-diff-loading">
          <span>Loading diff viewer...</span>
        </div>
      )}
      <div
        ref={containerRef}
        className="file-diff-editor"
        style={{ display: isLoading ? 'none' : 'block', height: '100%', minHeight: '400px' }}
      />
    </div>
  );
};

/**
 * Create a FileDiff component from payload data
 */
export function createFileDiffProps(payload: Record<string, any>): FileDiffProps {
  return {
    originalContent: payload.originalContent || payload.original || '',
    modifiedContent: payload.modifiedContent || payload.modified || '',
    originalLanguage: payload.originalLanguage || payload.language || 'plaintext',
    modifiedLanguage: payload.modifiedLanguage || payload.language || 'plaintext',
    originalFilename: payload.originalFilename || 'Original',
    modifiedFilename: payload.modifiedFilename || 'Modified',
    viewMode: payload.viewMode === 'inline' ? 'inline' : 'side-by-side',
    readOnly: payload.readOnly !== false,
  };
}

export default FileDiff;
