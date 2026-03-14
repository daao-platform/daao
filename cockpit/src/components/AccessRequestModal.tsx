/**
 * AccessRequestModal — Modal for requesting elevated permissions
 * 
 * Shows a form for viewers/admins to request a higher role.
 * Only shows roles above the current user's role.
 * POSTs to /api/v1/users/{userId}/request-access
 */

import React, { useState, useEffect } from 'react';
import { XIcon } from './Icons';
import { apiRequest } from '../api/client';

// ============================================================================
// Types
// ============================================================================

export interface AccessRequestModalProps {
    isOpen: boolean;
    onClose: () => void;
    currentRole: string;
}

// Role hierarchy: viewer=0, admin=1, owner=2
const ROLE_LEVELS: Record<string, number> = {
    viewer: 0,
    admin: 1,
    owner: 2,
};

// Roles available in the system
const ALL_ROLES = [
    { value: 'viewer', label: 'Viewer' },
    { value: 'admin', label: 'Admin' },
    { value: 'owner', label: 'Owner' },
];

// ============================================================================
// Component
// ============================================================================

const AccessRequestModal: React.FC<AccessRequestModalProps> = ({ isOpen, onClose, currentRole }) => {
    const [selectedRole, setSelectedRole] = useState('');
    const [reason, setReason] = useState('');
    const [isSubmitting, setIsSubmitting] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [success, setSuccess] = useState(false);

    // Get roles above current user's role
    const currentLevel = ROLE_LEVELS[currentRole] ?? 0;
    const availableRoles = ALL_ROLES.filter((r) => (ROLE_LEVELS[r.value] ?? -1) > currentLevel);

    // Reset form when modal opens
    useEffect(() => {
        if (isOpen) {
            setSelectedRole('');
            setReason('');
            setError(null);
            setSuccess(false);
        }
    }, [isOpen]);

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

    // Get current user ID from session storage
    const getCurrentUserId = (): string | null => {
        const userInfoStr = sessionStorage.getItem('oidc_user_info');
        if (userInfoStr) {
            try {
                const userInfo = JSON.parse(userInfoStr);
                return userInfo.sub || null;
            } catch {
                return null;
            }
        }
        return null;
    };

    // Handle form submission
    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();

        if (!selectedRole) {
            setError('Please select a role');
            return;
        }

        if (!reason.trim()) {
            setError('Please provide a reason for your request');
            return;
        }

        const userId = getCurrentUserId();
        if (!userId) {
            setError('Unable to identify current user');
            return;
        }

        setIsSubmitting(true);
        setError(null);

        try {
            await apiRequest(`/users/${userId}/request-access`, {
                method: 'POST',
                body: JSON.stringify({
                    requested_role: selectedRole,
                    reason: reason.trim(),
                }),
            });

            setSuccess(true);

            // Close modal after showing success message
            setTimeout(() => {
                onClose();
            }, 2000);
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Failed to submit request');
        } finally {
            setIsSubmitting(false);
        }
    };

    if (!isOpen) return null;

    // No roles available to request
    if (availableRoles.length === 0) {
        return (
            <div className="modal-overlay" onClick={onClose}>
                <div className="modal" onClick={(e) => e.stopPropagation()} style={{ maxWidth: 480 }}>
                    <div className="modal__header">
                        <h2 className="modal__title">Request Access</h2>
                        <button className="modal__close" onClick={onClose} type="button" aria-label="Close">
                            <XIcon size={20} />
                        </button>
                    </div>
                    <div className="modal__body">
                        <p className="text-muted">
                            You currently have the highest role available ({currentRole}).
                            No elevated access is available to request.
                        </p>
                    </div>
                    <div className="modal__footer">
                        <button type="button" className="btn btn--primary" onClick={onClose}>
                            Close
                        </button>
                    </div>
                </div>
            </div>
        );
    }

    return (
        <div className="modal-overlay" onClick={onClose}>
            <div className="modal" onClick={(e) => e.stopPropagation()} style={{ maxWidth: 480 }}>
                {/* Header */}
                <div className="modal__header">
                    <h2 className="modal__title">Request Access</h2>
                    <button className="modal__close" onClick={onClose} type="button" aria-label="Close">
                        <XIcon size={20} />
                    </button>
                </div>

                {/* Body */}
                <div className="modal__body">
                    {success ? (
                        <div className="drawer-notice drawer-notice--success">
                            Your access request has been submitted successfully. The owners have been notified.
                        </div>
                    ) : (
                        <form className="forge-form" onSubmit={handleSubmit}>
                            {/* Error message */}
                            {error && (
                                <div className="drawer-notice drawer-notice--warning">
                                    {error}
                                </div>
                            )}

                            {/* Role Selection */}
                            <div className="forge-form__group">
                                <label className="forge-form__label forge-form__label--required">
                                    Requested Role
                                </label>
                                <select
                                    className="forge-form__select"
                                    value={selectedRole}
                                    onChange={(e) => setSelectedRole(e.target.value)}
                                    disabled={isSubmitting}
                                >
                                    <option value="">Select a role...</option>
                                    {availableRoles.map((role) => (
                                        <option key={role.value} value={role.value}>
                                            {role.label}
                                        </option>
                                    ))}
                                </select>
                                <span className="forge-form__help">
                                    You are currently a {currentRole}. This request will be sent to all owners.
                                </span>
                            </div>

                            {/* Reason */}
                            <div className="forge-form__group">
                                <label className="forge-form__label forge-form__label--required">
                                    Reason / Justification
                                </label>
                                <textarea
                                    className="forge-form__textarea"
                                    value={reason}
                                    onChange={(e) => setReason(e.target.value)}
                                    placeholder="Why do you need elevated access?"
                                    rows={4}
                                    disabled={isSubmitting}
                                />
                            </div>

                            {/* Footer */}
                            <div className="modal__footer">
                                <button
                                    type="button"
                                    className="btn btn--outline"
                                    onClick={onClose}
                                    disabled={isSubmitting}
                                >
                                    Cancel
                                </button>
                                <button
                                    type="submit"
                                    className="btn btn--primary"
                                    disabled={isSubmitting || !selectedRole || !reason.trim()}
                                >
                                    {isSubmitting ? 'Submitting...' : 'Submit Request'}
                                </button>
                            </div>
                        </form>
                    )}
                </div>
            </div>
        </div>
    );
};

export default AccessRequestModal;
