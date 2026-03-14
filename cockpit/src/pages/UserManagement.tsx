import React, { useState } from 'react';
import { useApi } from '../hooks';
import { useAuth } from '../auth/AuthProvider';
import { UserIcon, PlusIcon, XIcon } from '../components/Icons';

// User type from API
interface User {
    id: string;
    email: string;
    name: string;
    role: 'owner' | 'admin' | 'viewer';
    avatar_url?: string;
    last_login_at?: string;
    created_at: string;
}

// Role type
type UserRole = 'owner' | 'admin' | 'viewer';

// Invite user request
interface InviteUserRequest {
    email: string;
    role: UserRole;
}



const timeAgo = (dateStr: string): string => {
    if (!dateStr) return 'Never';
    const diff = Date.now() - new Date(dateStr).getTime();
    const mins = Math.floor(diff / 60000);
    if (mins < 1) return 'Just now';
    if (mins < 60) return `${mins}m ago`;
    const hrs = Math.floor(mins / 60);
    if (hrs < 24) return `${hrs}h ago`;
    return `${Math.floor(hrs / 24)}d ago`;
};

const getRoleBadgeClass = (role: string): string => {
    switch (role) {
        case 'owner':
            return 'badge badge--accent';
        case 'admin':
            return 'badge badge--info';
        case 'viewer':
            return 'badge';
        default:
            return 'badge';
    }
};

const UserManagement: React.FC = () => {
    const { user: currentUser, accessToken, hasPermission } = useAuth();
    const isOwner = hasPermission('owner');

    const [showInviteModal, setShowInviteModal] = useState(false);
    const [inviteEmail, setInviteEmail] = useState('');
    const [inviteRole, setInviteRole] = useState<UserRole>('viewer');
    const [inviteLoading, setInviteLoading] = useState(false);
    const [inviteError, setInviteError] = useState<string | null>(null);
    const [deletingId, setDeletingId] = useState<string | null>(null);
    const [changingRoleId, setChangingRoleId] = useState<string | null>(null);
    const [createdCredentials, setCreatedCredentials] = useState<{ email: string; password: string } | null>(null);
    const [copied, setCopied] = useState(false);

    // Use useApi hook to fetch users
    const { data: users, loading, error, refetch } = useApi<User[]>(() =>
        fetch('/api/v1/users', {
            headers: { Authorization: `Bearer ${accessToken}` }
        }).then(r => {
            if (!r.ok) throw new Error('Failed to fetch users');
            return r.json();
        })
    );

    // Handle invite/create user
    const handleInvite = async (e: React.FormEvent) => {
        e.preventDefault();
        if (!inviteEmail.trim()) return;

        setInviteLoading(true);
        setInviteError(null);

        try {
            const response = await fetch('/api/v1/users/invite', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    Authorization: `Bearer ${accessToken}`,
                },
                body: JSON.stringify({ email: inviteEmail.trim(), role: inviteRole }),
            });

            if (!response.ok) {
                const err = await response.json().catch(() => ({ error: 'Failed to create user' }));
                throw new Error(err.error || 'Failed to create user');
            }

            const data = await response.json();

            // Show credentials dialog
            setCreatedCredentials({
                email: inviteEmail.trim(),
                password: data.temporary_password,
            });
            setShowInviteModal(false);
            setInviteEmail('');
            setInviteRole('viewer');
            refetch();
        } catch (err) {
            setInviteError(err instanceof Error ? err.message : 'Failed to create user');
        } finally {
            setInviteLoading(false);
        }
    };

    const handleCopyCredentials = () => {
        if (!createdCredentials) return;
        const text = `Email: ${createdCredentials.email}\nTemporary Password: ${createdCredentials.password}`;
        navigator.clipboard.writeText(text).then(() => {
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
        });
    };

    // Handle role change
    const handleRoleChange = async (userId: string, newRole: UserRole) => {
        setChangingRoleId(userId);
        try {
            await fetch(`/api/v1/users/${userId}/role`, {
                method: 'PATCH',
                headers: {
                    'Content-Type': 'application/json',
                    Authorization: `Bearer ${accessToken}`,
                },
                body: JSON.stringify({ role: newRole }),
            });
            refetch();
        } catch (err) {
            console.error('Failed to change role:', err);
        } finally {
            setChangingRoleId(null);
        }
    };

    // Handle delete user
    const handleDelete = async (userId: string) => {
        if (!confirm('Are you sure you want to delete this user? This action cannot be undone.')) {
            return;
        }

        setDeletingId(userId);
        try {
            await fetch(`/api/v1/users/${userId}`, {
                method: 'DELETE',
                headers: {
                    Authorization: `Bearer ${accessToken}`,
                },
            });
            refetch();
        } catch (err) {
            console.error('Failed to delete user:', err);
        } finally {
            setDeletingId(null);
        }
    };

    // Check if user is current user (for delete self-protection)
    const isCurrentUser = (userId: string): boolean => {
        return currentUser?.sub === userId;
    };

    // Close modal on overlay click
    const handleOverlayClick = (e: React.MouseEvent) => {
        if (e.target === e.currentTarget) {
            setShowInviteModal(false);
        }
    };

    // Permission check
    if (!isOwner) {
        return (
            <div className="empty-state">
                <div className="empty-state__icon">
                    <UserIcon size={28} />
                </div>
                <div className="empty-state__title">Insufficient permissions</div>
                <div className="empty-state__desc">
                    You need owner permissions to access this page.
                </div>
            </div>
        );
    }

    return (
        <div>
            <div className="page-header">
                <h1 className="page-header-title">User Management</h1>
                <div className="page-header-subtitle">Manage team members and their access</div>
            </div>

            {/* Loading State */}
            {loading && (
                <div className="empty-state">
                    <div className="empty-state__desc">Loading users...</div>
                </div>
            )}

            {/* Error State */}
            {error && !loading && (
                <div className="empty-state">
                    <div className="empty-state__icon">
                        <UserIcon size={28} />
                    </div>
                    <div className="empty-state__title">Failed to load users</div>
                    <div className="empty-state__desc">
                        {error instanceof Error ? error.message : 'An unexpected error occurred'}
                    </div>
                    <button className="btn btn--primary btn--sm" onClick={refetch} style={{ marginTop: 16 }}>
                        Retry
                    </button>
                </div>
            )}

            {/* Users Table */}
            {!loading && !error && users && (
                <>
                    <div className="section-header">
                        <h2 className="section-title">
                            {users.length} User{users.length !== 1 ? 's' : ''}
                        </h2>
                        <button
                            className="btn btn--primary btn--sm"
                            onClick={() => setShowInviteModal(true)}
                        >
                            <PlusIcon size={16} />
                            Invite User
                        </button>
                    </div>

                    {/* Users Table */}
                    <div style={{
                        background: 'var(--bg-elevated)',
                        borderRadius: 'var(--radius-md)',
                        border: '1px solid var(--border)',
                        overflow: 'hidden',
                    }}>
                        <table style={{
                            width: '100%',
                            borderCollapse: 'collapse',
                            fontSize: 14,
                        }}>
                            <thead>
                                <tr style={{
                                    borderBottom: '1px solid var(--border)',
                                    background: 'rgba(0,0,0,0.2)',
                                }}>
                                    <th style={{
                                        padding: '12px 16px',
                                        textAlign: 'left',
                                        fontWeight: 600,
                                        color: 'var(--text-muted)',
                                        fontSize: 12,
                                        textTransform: 'uppercase',
                                        letterSpacing: '0.05em',
                                    }}>User</th>
                                    <th style={{
                                        padding: '12px 16px',
                                        textAlign: 'left',
                                        fontWeight: 600,
                                        color: 'var(--text-muted)',
                                        fontSize: 12,
                                        textTransform: 'uppercase',
                                        letterSpacing: '0.05em',
                                    }}>Role</th>
                                    <th style={{
                                        padding: '12px 16px',
                                        textAlign: 'left',
                                        fontWeight: 600,
                                        color: 'var(--text-muted)',
                                        fontSize: 12,
                                        textTransform: 'uppercase',
                                        letterSpacing: '0.05em',
                                    }}>Last Login</th>
                                    <th style={{
                                        padding: '12px 16px',
                                        textAlign: 'right',
                                        fontWeight: 600,
                                        color: 'var(--text-muted)',
                                        fontSize: 12,
                                        textTransform: 'uppercase',
                                        letterSpacing: '0.05em',
                                    }}>Actions</th>
                                </tr>
                            </thead>
                            <tbody>
                                {users.map((user) => {
                                    const isSelf = isCurrentUser(user.id);
                                    const isChangingRole = changingRoleId === user.id;
                                    const isDeleting = deletingId === user.id;

                                    return (
                                        <tr
                                            key={user.id}
                                            style={{
                                                borderBottom: '1px solid var(--border)',
                                            }}
                                        >
                                            <td style={{ padding: '16px' }}>
                                                <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                                                    {user.avatar_url ? (
                                                        <img
                                                            src={user.avatar_url}
                                                            alt={user.name}
                                                            style={{
                                                                width: 36,
                                                                height: 36,
                                                                borderRadius: '50%',
                                                                objectFit: 'cover',
                                                            }}
                                                        />
                                                    ) : (
                                                        <div style={{
                                                            width: 36,
                                                            height: 36,
                                                            borderRadius: '50%',
                                                            background: 'var(--bg-primary)',
                                                            border: '1px solid var(--border)',
                                                            display: 'flex',
                                                            alignItems: 'center',
                                                            justifyContent: 'center',
                                                        }}>
                                                            <UserIcon size={18} />
                                                        </div>
                                                    )}
                                                    <div>
                                                        <div style={{ fontWeight: 600, color: 'var(--text)' }}>
                                                            {user.name}
                                                            {isSelf && (
                                                                <span style={{
                                                                    marginLeft: 8,
                                                                    fontSize: 11,
                                                                    color: 'var(--text-muted)',
                                                                    fontWeight: 400,
                                                                }}>
                                                                    (you)
                                                                </span>
                                                            )}
                                                        </div>
                                                        <div style={{ fontSize: 13, color: 'var(--text-muted)' }}>
                                                            {user.email}
                                                        </div>
                                                    </div>
                                                </div>
                                            </td>
                                            <td style={{ padding: '16px' }}>
                                                <span className={getRoleBadgeClass(user.role)}>
                                                    {user.role.charAt(0).toUpperCase() + user.role.slice(1)}
                                                </span>
                                            </td>
                                            <td style={{ padding: '16px', color: 'var(--text-secondary)' }}>
                                                {timeAgo(user.last_login_at || user.created_at)}
                                            </td>
                                            <td style={{ padding: '16px', textAlign: 'right' }}>
                                                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 8 }}>
                                                    {/* Role dropdown - only owners can change roles, disabled for self if owner */}
                                                    <select
                                                        value={user.role}
                                                        onChange={(e) => handleRoleChange(user.id, e.target.value as UserRole)}
                                                        disabled={isChangingRole || (user.role === 'owner' && isSelf)}
                                                        style={{
                                                            padding: '6px 10px',
                                                            fontSize: 13,
                                                            borderRadius: 'var(--radius-sm)',
                                                            border: '1px solid var(--border)',
                                                            background: 'var(--bg-primary)',
                                                            color: 'var(--text)',
                                                            cursor: isChangingRole ? 'wait' : 'pointer',
                                                            opacity: isChangingRole ? 0.7 : 1,
                                                        }}
                                                    >
                                                        <option value="viewer">Viewer</option>
                                                        <option value="admin">Admin</option>
                                                        <option value="owner">Owner</option>
                                                    </select>

                                                    {/* Delete button - only owners can delete, disabled for self */}
                                                    <button
                                                        className="btn btn--danger btn--sm"
                                                        onClick={() => handleDelete(user.id)}
                                                        disabled={isDeleting || isSelf}
                                                        title={isSelf ? 'Cannot delete your own account' : 'Delete user'}
                                                        style={{
                                                            opacity: isSelf ? 0.5 : 1,
                                                            cursor: isSelf ? 'not-allowed' : 'pointer',
                                                        }}
                                                    >
                                                        {isDeleting ? 'Deleting...' : 'Delete'}
                                                    </button>
                                                </div>
                                            </td>
                                        </tr>
                                    );
                                })}
                            </tbody>
                        </table>
                    </div>
                </>
            )}

            {/* Create User Modal */}
            {showInviteModal && (
                <div className="modal-overlay" onClick={handleOverlayClick}>
                    <div className="modal" role="dialog" aria-modal="true" aria-labelledby="invite-title" style={{ maxWidth: 440 }}>
                        <div className="modal__header">
                            <h2 id="invite-title" className="modal__title">
                                Create User
                            </h2>
                            <button className="modal__close" onClick={() => setShowInviteModal(false)} type="button" aria-label="Close">
                                <XIcon size={20} />
                            </button>
                        </div>

                        <form onSubmit={handleInvite}>
                            <div className="modal__body">
                                {inviteError && (
                                    <div className="form-error" style={{ marginBottom: 16 }}>
                                        {inviteError}
                                    </div>
                                )}
                                <div className="form-group">
                                    <label htmlFor="invite-email" className="form-label">
                                        Email Address
                                        <span className="form-label--required"> *</span>
                                    </label>
                                    <input
                                        id="invite-email"
                                        type="email"
                                        className="form-input"
                                        placeholder="user@example.com"
                                        value={inviteEmail}
                                        onChange={(e) => setInviteEmail(e.target.value)}
                                        required
                                        autoFocus
                                    />
                                </div>
                                <div className="form-group">
                                    <label htmlFor="invite-role" className="form-label">
                                        Role
                                    </label>
                                    <select
                                        id="invite-role"
                                        className="form-input"
                                        value={inviteRole}
                                        onChange={(e) => setInviteRole(e.target.value as UserRole)}
                                    >
                                        <option value="viewer">Viewer</option>
                                        <option value="admin">Admin</option>
                                    </select>
                                    <p style={{ fontSize: 12, color: 'var(--text-muted)', marginTop: 6 }}>
                                        A temporary password will be generated. Share it with the user securely.
                                    </p>
                                </div>
                            </div>
                            <div className="modal__footer">
                                <button
                                    type="button"
                                    className="btn btn--secondary"
                                    onClick={() => setShowInviteModal(false)}
                                    disabled={inviteLoading}
                                >
                                    Cancel
                                </button>
                                <button
                                    type="submit"
                                    className="btn btn--primary"
                                    disabled={inviteLoading || !inviteEmail.trim()}
                                >
                                    {inviteLoading ? 'Creating...' : 'Create User'}
                                </button>
                            </div>
                        </form>
                    </div>
                </div>
            )}

            {/* Credentials Dialog — shown once after user creation */}
            {createdCredentials && (
                <div className="modal-overlay">
                    <div className="modal" role="dialog" aria-modal="true" style={{ maxWidth: 480 }}>
                        <div className="modal__header">
                            <h2 className="modal__title" style={{ color: 'var(--success)' }}>
                                ✓ User Created
                            </h2>
                        </div>
                        <div className="modal__body">
                            <p style={{ marginBottom: 16, color: 'var(--text-secondary)', fontSize: 13 }}>
                                Share these credentials with the user. The temporary password is shown <strong>only once</strong>.
                            </p>
                            <div style={{
                                background: 'var(--bg-elevated)',
                                border: '1px solid var(--border)',
                                borderRadius: 'var(--radius-md)',
                                padding: 16,
                                fontFamily: 'var(--font-mono)',
                                fontSize: 13,
                                lineHeight: 1.8,
                            }}>
                                <div><span style={{ color: 'var(--text-muted)' }}>Email:</span> {createdCredentials.email}</div>
                                <div><span style={{ color: 'var(--text-muted)' }}>Password:</span> {createdCredentials.password}</div>
                            </div>
                        </div>
                        <div className="modal__footer">
                            <button
                                className="btn btn--secondary"
                                onClick={handleCopyCredentials}
                            >
                                {copied ? '✓ Copied!' : 'Copy Credentials'}
                            </button>
                            <button
                                className="btn btn--primary"
                                onClick={() => { setCreatedCredentials(null); setCopied(false); }}
                            >
                                Done
                            </button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
};

export default UserManagement;
