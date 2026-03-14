import React, { createContext, useContext, useState, useCallback, ReactNode } from 'react';

// Toast types
type ToastType = 'success' | 'error' | 'info';

interface Toast {
  id: number;
  message: string;
  type: ToastType;
}

interface ToastContextValue {
  showToast: (message: string, type?: ToastType) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

interface ToastProviderProps {
  children: ReactNode;
}

export const ToastProvider: React.FC<ToastProviderProps> = ({ children }) => {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const showToast = useCallback((message: string, type: ToastType = 'info') => {
    const id = Date.now();
    setToasts((prev) => [...prev, { id, message, type }]);

    // Auto-dismiss after 3 seconds
    setTimeout(() => {
      setToasts((prev) => prev.filter((toast) => toast.id !== id));
    }, 3000);
  }, []);

  const removeToast = useCallback((id: number) => {
    setToasts((prev) => prev.filter((toast) => toast.id !== id));
  }, []);

  return (
    <ToastContext.Provider value={{ showToast }}>
      {children}
      {/* Desktop container — positioned via CSS */}
      <div className="toast-container">
        {toasts.map((toast) =>
          React.createElement(
            'div',
            {
              key: toast.id,
              className: `toast toast--${toast.type}`,
              onClick: () => removeToast(toast.id),
            },
            toast.message
          )
        )}
      </div>
      {/* Mobile container — shown via CSS media query */}
      <div className="toast-container toast-container--mobile">
        {toasts.map((toast) =>
          React.createElement(
            'div',
            {
              key: toast.id,
              className: `toast toast--${toast.type}`,
              onClick: () => removeToast(toast.id),
            },
            toast.message
          )
        )}
      </div>
    </ToastContext.Provider>
  );
};

export const useToast = (): ToastContextValue => {
  const context = useContext(ToastContext);
  if (!context) {
    // Return a no-op function if used outside provider
    return {
      showToast: () => {
        console.warn('useToast must be used within a ToastProvider');
      },
    };
  }
  return context;
};
