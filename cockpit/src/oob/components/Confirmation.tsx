/**
 * Confirmation Component
 * 
 * Modal dialog component that blocks AI until user provides an answer.
 * Displays a confirmation dialog with Accept/Decline options.
 */

import React, { useState, useEffect, useCallback, useRef } from 'react';

export interface ConfirmationProps {
  id: string;
  title?: string;
  message: string;
  details?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  confirmVariant?: 'primary' | 'danger' | 'warning' | 'success';
  cancelVariant?: 'secondary' | 'outline' | 'ghost';
  showDetails?: boolean;
  defaultFocus?: 'confirm' | 'cancel';
  onConfirm: (id: string) => void;
  onCancel: (id: string) => void;
  isLoading?: boolean;
  disabled?: boolean;
}

/**
 * Confirmation Modal Component
 * 
 * A modal dialog that blocks execution until user responds.
 * Used for AI to request user confirmation before proceeding.
 */
export const Confirmation: React.FC<ConfirmationProps> = ({
  id,
  title = 'Confirm',
  message,
  details,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  confirmVariant = 'primary',
  cancelVariant = 'secondary',
  showDetails = false,
  defaultFocus = 'confirm',
  onConfirm,
  onCancel,
  isLoading = false,
  disabled = false,
}) => {
  const [isVisible, setIsVisible] = useState(false);
  const [showDetailsExpanded, setShowDetailsExpanded] = useState(showDetails);
  const confirmButtonRef = useRef<HTMLButtonElement>(null);
  const cancelButtonRef = useRef<HTMLButtonElement>(null);

  // Handle escape key
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        handleCancel();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, []);

  // Focus management
  useEffect(() => {
    setIsVisible(true);
    
    // Focus the appropriate button after render
    setTimeout(() => {
      if (defaultFocus === 'confirm' && confirmButtonRef.current) {
        confirmButtonRef.current.focus();
      } else if (defaultFocus === 'cancel' && cancelButtonRef.current) {
        cancelButtonRef.current.focus();
      }
    }, 100);
  }, [defaultFocus]);

  // Prevent background scrolling when modal is open
  useEffect(() => {
    document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = 'unset';
    };
  }, []);

  const handleConfirm = useCallback(() => {
    if (disabled || isLoading) return;
    setIsVisible(false);
    onConfirm(id);
  }, [id, disabled, isLoading, onConfirm]);

  const handleCancel = useCallback(() => {
    if (disabled || isLoading) return;
    setIsVisible(false);
    onCancel(id);
  }, [id, disabled, isLoading, onCancel]);

  // Get variant classes
  const getConfirmVariantClass = () => {
    switch (confirmVariant) {
      case 'danger': return 'btn-danger';
      case 'warning': return 'btn-warning';
      case 'success': return 'btn-success';
      default: return 'btn-primary';
    }
  };

  const getCancelVariantClass = () => {
    switch (cancelVariant) {
      case 'outline': return 'btn-outline';
      case 'ghost': return 'btn-ghost';
      default: return 'btn-secondary';
    }
  };

  return (
    <div className="modal-overlay" onClick={handleCancel}>
      <div 
        className={`modal-container confirmation-modal ${isVisible ? 'visible' : ''}`}
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby="confirmation-title"
        aria-describedby="confirmation-message"
      >
        {/* Header */}
        <div className="modal-header">
          <h2 id="confirmation-title" className="modal-title">
            {title}
          </h2>
          <button 
            className="modal-close"
            onClick={handleCancel}
            aria-label="Close"
            disabled={disabled || isLoading}
          >
            ×
          </button>
        </div>

        {/* Body */}
        <div className="modal-body">
          <p id="confirmation-message" className="confirmation-message">
            {message}
          </p>
          
          {details && (
            <div className="confirmation-details-wrapper">
              <button
                className="details-toggle"
                onClick={() => setShowDetailsExpanded(!showDetailsExpanded)}
                type="button"
              >
                {showDetailsExpanded ? '▼ Hide' : '▶ Show'} Details
              </button>
              
              {showDetailsExpanded && (
                <div className="confirmation-details">
                  <pre>{details}</pre>
                </div>
              )}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="modal-footer">
          <button
            ref={cancelButtonRef}
            className={`btn ${getCancelVariantClass()}`}
            onClick={handleCancel}
            disabled={disabled || isLoading}
            type="button"
          >
            {isLoading ? 'Loading...' : cancelLabel}
          </button>
          
          <button
            ref={confirmButtonRef}
            className={`btn ${getConfirmVariantClass()}`}
            onClick={handleConfirm}
            disabled={disabled || isLoading}
            type="button"
          >
            {isLoading ? 'Loading...' : confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
};

/**
 * Create Confirmation props from payload data
 */
export function createConfirmationProps(
  payload: Record<string, any>,
  onConfirm: (id: string) => void,
  onCancel: (id: string) => void
): ConfirmationProps {
  return {
    id: payload.id || `confirm-${Date.now()}`,
    title: payload.title,
    message: payload.message || payload.prompt || 'Are you sure?',
    details: payload.details,
    confirmLabel: payload.confirmLabel || 'Confirm',
    cancelLabel: payload.cancelLabel || 'Cancel',
    confirmVariant: payload.confirmVariant || 'primary',
    cancelVariant: payload.cancelVariant || 'secondary',
    showDetails: payload.showDetails || false,
    defaultFocus: payload.defaultFocus || 'confirm',
    onConfirm,
    onCancel,
    isLoading: payload.isLoading || false,
    disabled: payload.disabled || false,
  };
}

/**
 * PendingConfirmations Component
 * 
 * Manages multiple pending confirmation dialogs.
 * Renders them in a stack, one at a time.
 */
interface PendingConfirmation {
  id: string;
  props: ConfirmationProps;
}

interface PendingConfirmationsProps {
  confirmations: PendingConfirmation[];
  onConfirm: (id: string) => void;
  onCancel: (id: string) => void;
}

export const PendingConfirmations: React.FC<PendingConfirmationsProps> = ({
  confirmations,
  onConfirm,
  onCancel,
}) => {
  if (confirmations.length === 0) {
    return null;
  }

  // Show only the most recent confirmation
  const currentConfirmation = confirmations[confirmations.length - 1];
  
  return (
    <Confirmation
      {...currentConfirmation.props}
      onConfirm={onConfirm}
      onCancel={onCancel}
    />
  );
};

export default Confirmation;
