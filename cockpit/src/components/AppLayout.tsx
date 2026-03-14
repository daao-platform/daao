import React, { useState, useRef, useEffect } from 'react';
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import {
    DashboardIcon,
    SessionsIcon,
    GridViewIcon,
    RecordingsIcon,
    SatellitesIcon,
    AgentsIcon,
    SettingsIcon,
    UserIcon,
    AuditIcon,
    PipelineIcon,
    HistoryIcon,
} from './Icons';
import NotificationBell from './NotificationBell';
import { useAuth } from '../auth/AuthProvider';

/**
 * Navigation items — primary workflow tools
 */
const primaryNav = [
    { path: '/', label: 'Dashboard', icon: DashboardIcon },
    { path: '/sessions', label: 'Sessions', icon: SessionsIcon },
    { path: '/sessions/multi', label: 'Multi-View', icon: GridViewIcon },
    { path: '/satellites', label: 'Satellites', icon: SatellitesIcon },
    { path: '/forge', label: 'Forge', icon: AgentsIcon },
    { path: '/pipelines', label: 'Pipelines', icon: PipelineIcon },
    { path: '/forge/runs', label: 'Runs', icon: HistoryIcon },
    { path: '/recordings', label: 'Recordings', icon: RecordingsIcon },
];

/** Logout icon */
const LogoutIcon: React.FC<{ size?: number }> = ({ size = 16 }) => (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none"
        stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
        <polyline points="16 17 21 12 16 7" />
        <line x1="21" y1="12" x2="9" y2="12" />
    </svg>
);

/**
 * AppLayout — Shell component with sidebar (desktop) and bottom tabs (mobile)
 */
const AppLayout: React.FC = () => {
    const location = useLocation();
    const navigate = useNavigate();
    const { hasPermission, logout, user } = useAuth();
    const [showUserMenu, setShowUserMenu] = useState(false);
    const menuRef = useRef<HTMLDivElement>(null);

    // Close menu on click outside
    useEffect(() => {
        const handleClickOutside = (e: MouseEvent) => {
            if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
                setShowUserMenu(false);
            }
        };
        if (showUserMenu) {
            document.addEventListener('mousedown', handleClickOutside);
            return () => document.removeEventListener('mousedown', handleClickOutside);
        }
    }, [showUserMenu]);

    // Close menu on route change
    useEffect(() => { setShowUserMenu(false); }, [location.pathname]);

    const isTerminalView = location.pathname.startsWith('/session/');
    const isFullBleed = isTerminalView || location.pathname === '/sessions/multi';

    const handleLogout = () => {
        setShowUserMenu(false);
        logout();
        navigate('/login', { replace: true });
    };

    const userInitial = user?.name?.[0]?.toUpperCase() || user?.email?.[0]?.toUpperCase() || '?';

    return (
        <div className="app-layout">
            {/* Desktop Sidebar */}
            <nav className="sidebar" role="navigation" aria-label="Main navigation">
                <div className="sidebar-logo" aria-label="DAAO">
                    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                        <ellipse cx="12" cy="12" rx="10" ry="4.5" stroke="currentColor" strokeWidth="1.5" strokeOpacity="0.5" transform="rotate(-30 12 12)" />
                        <ellipse cx="12" cy="12" rx="10" ry="4.5" stroke="currentColor" strokeWidth="1.5" strokeOpacity="0.5" transform="rotate(30 12 12)" />
                        <circle cx="12" cy="12" r="3" fill="currentColor" />
                        <circle cx="4" cy="6.5" r="1.8" fill="currentColor" fillOpacity="0.7" />
                        <circle cx="20" cy="6.5" r="1.8" fill="currentColor" fillOpacity="0.7" />
                        <circle cx="12" cy="20.5" r="1.8" fill="currentColor" fillOpacity="0.7" />
                    </svg>
                </div>

                <div className="sidebar-nav">
                    {primaryNav.map((item) => (
                        <NavLink
                            key={item.path}
                            to={item.path}
                            end={item.path === '/'}
                            className={({ isActive }) =>
                                `sidebar-link${isActive ? ' active' : ''}`
                            }
                            title={item.label}
                        >
                            <item.icon size={20} />
                            <span className="sr-only">{item.label}</span>
                        </NavLink>
                    ))}

                    <div className="sidebar-divider" />

                    <NavLink
                        to="/settings"
                        className={({ isActive }) =>
                            `sidebar-link${isActive && location.pathname === '/settings' ? ' active' : ''}`
                        }
                        title="Settings"
                    >
                        <SettingsIcon size={20} />
                        <span className="sr-only">Settings</span>
                    </NavLink>

                    {hasPermission('owner') && (
                        <NavLink
                            to="/settings/users"
                            className={({ isActive }) =>
                                `sidebar-link${isActive ? ' active' : ''}`
                            }
                            title="Users"
                        >
                            <UserIcon size={20} />
                            <span className="sr-only">Users</span>
                        </NavLink>
                    )}
                    {hasPermission('admin') && (
                        <NavLink
                            to="/audit-log"
                            className={({ isActive }) =>
                                `sidebar-link${isActive ? ' active' : ''}`
                            }
                            title="Audit Log"
                        >
                            <AuditIcon size={20} />
                            <span className="sr-only">Audit Log</span>
                        </NavLink>
                    )}
                </div>

                <div className="sidebar-bottom">
                    <NotificationBell />
                    <div className="sidebar-user-menu" ref={menuRef}>
                        <button
                            className={`sidebar-avatar${showUserMenu ? ' active' : ''}`}
                            onClick={() => setShowUserMenu(!showUserMenu)}
                            aria-label="User menu"
                            title={user?.email || 'User'}
                        >
                            {userInitial}
                        </button>

                        {showUserMenu && (
                            <div className="user-popover">
                                <div className="user-popover__header">
                                    <div className="user-popover__avatar">{userInitial}</div>
                                    <div className="user-popover__info">
                                        <div className="user-popover__name">{user?.name || 'User'}</div>
                                        <div className="user-popover__email">{user?.email || ''}</div>
                                    </div>
                                </div>
                                <div className="user-popover__role">
                                    <span className={`badge badge--${user?.role || 'viewer'}`}>
                                        {user?.role || 'viewer'}
                                    </span>
                                </div>
                                <div className="user-popover__divider" />
                                <button className="user-popover__action user-popover__logout" onClick={handleLogout}>
                                    <LogoutIcon size={16} />
                                    Sign Out
                                </button>
                            </div>
                        )}
                    </div>
                </div>
            </nav>

            {/* Main Content Area */}
            <main className="app-main" style={isTerminalView ? { paddingBottom: 0 } : undefined}>
                <div className={isFullBleed ? '' : 'app-content'}>
                    <Outlet />
                </div>
            </main>

            {/* Mobile Bottom Tabs */}
            {!isTerminalView && (
                <nav className="bottom-tabs" role="navigation" aria-label="Tab navigation">
                    {primaryNav.map((item) => (
                        <NavLink
                            key={item.path}
                            to={item.path}
                            end={item.path === '/'}
                            className={({ isActive }) =>
                                `tab-link${isActive ? ' active' : ''}`
                            }
                        >
                            <item.icon size={22} />
                            <span>{item.label}</span>
                        </NavLink>
                    ))}
                </nav>
            )}
        </div>
    );
};

export default AppLayout;
