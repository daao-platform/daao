import React, { useState, useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { Session, getSessions, attachSession, detachSession, suspendSession, killSession, SessionsPaginatedResponse } from '../api/client';
import { useApi, useWebSocket } from '../hooks';
import { TerminalIcon, PlusIcon } from '../components/Icons';
import TerminalPreview from '../components/TerminalPreview';
import SystemHealth from '../components/SystemHealth';

/**
 * State badge component
 */
const StateBadge: React.FC<{ state: string }> = ({ state }) => {
  const s = state?.toUpperCase() || 'UNKNOWN';
  let className = 'badge ';
  switch (s) {
    case 'RUNNING': className += 'badge--running'; break;
    case 'DETACHED': className += 'badge--detached'; break;
    case 'SUSPENDED': className += 'badge--suspended'; break;
    case 'PROVISIONING': className += 'badge--provisioning'; break;
    case 'TERMINATED': className += 'badge--terminated'; break;
    default: className += 'badge--terminated';
  }
  return (
    <span className={className}>
      <span className="badge__dot" />
      {s}
    </span>
  );
};

/**
 * Stat Card component — Stitch mockup style with icon + optional bars
 */
const StatCard: React.FC<{
  label: string;
  value: number | string;
  trend?: string;
  icon?: string;
  iconClass?: string;
}> = ({ label, value, trend, icon, iconClass }) => (
  <div className="stat-card animate-fadeIn">
    <div className="stat-card__header">
      <span className="stat-card__label">{label}</span>
      {icon && <span className={`material-symbols-outlined stat-card__icon ${iconClass || ''}`}>{icon}</span>}
    </div>
    <div className="stat-card__bottom">
      <span className="stat-card__value">{value}</span>
      {trend && <span className="stat-card__trend">{trend}</span>}
    </div>
  </div>
);

/**
 * Loading Skeleton for Stat Cards
 */
const StatCardSkeleton: React.FC = () => (
  <div className="stat-card stat-card--skeleton">
    <div className="skeleton skeleton--text skeleton--label" style={{ width: '60px', height: '14px' }} />
    <div className="skeleton skeleton--text skeleton--value" style={{ width: '40px', height: '28px', marginTop: '8px' }} />
  </div>
);

/**
 * Loading Skeleton for Session Cards
 */
const SessionCardSkeleton: React.FC = () => (
  <div className="session-card session-card--skeleton">
    <div className="session-card__header">
      <div>
        <div className="skeleton skeleton--text" style={{ width: '120px', height: '16px' }} />
      </div>
      <div className="skeleton skeleton--badge" style={{ width: '70px', height: '22px' }} />
    </div>
    <div className="session-card__meta">
      <div className="skeleton skeleton--text" style={{ width: '50px', height: '12px' }} />
      <span className="session-card__meta-dot" />
      <div className="skeleton skeleton--text" style={{ width: '80px', height: '12px' }} />
    </div>
    <div className="session-card__actions">
      <div className="skeleton skeleton--button" style={{ width: '70px', height: '32px' }} />
    </div>
  </div>
);

/**
 * Session Card component
 */
const SessionCard: React.FC<{
  session: Session;
  onAction: (sessionId: string, action: string) => void;
}> = ({ session, onAction }) => {
  const navigate = useNavigate();
  const state = session.state?.toUpperCase() || 'UNKNOWN';
  const stateColor = state === 'RUNNING' ? 'var(--accent)' : state === 'DETACHED' ? 'var(--warning)' : state === 'SUSPENDED' ? 'var(--info)' : 'var(--text-muted)';

  const handleAttach = () => {
    if (state === 'RUNNING' || state === 'DETACHED') {
      navigate(`/session/${session.id}`);
    }
  };

  const timeAgo = (dateStr: string): string => {
    if (!dateStr) return '';
    const diff = Date.now() - new Date(dateStr).getTime();
    const mins = Math.floor(diff / 60000);
    if (mins < 1) return 'just now';
    if (mins < 60) return `${mins}m ago`;
    const hrs = Math.floor(mins / 60);
    if (hrs < 24) return `${hrs}h ago`;
    return `${Math.floor(hrs / 24)}d ago`;
  };

  return (
    <div className={`session-card animate-fadeIn session-card--${state.toLowerCase()}`}>
      <div className="session-card__grid">
        {/* Column 1: Name + Status */}
        <div className="session-card__info">
          <div className="session-card__name-wrapper" style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <span className="session-card__name">{session.name || session.id.slice(0, 12)}</span>
          </div>
          <div className="session-card__status-line" style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '13px', marginTop: '4px', color: stateColor, fontWeight: 500 }}>
            <div className={`session-card__status-dot session-card__status-dot--${state.toLowerCase()}`} style={{ width: '8px', height: '8px', borderRadius: '50%', backgroundColor: 'currentColor' }} />
            {state}
          </div>
        </div>

        {/* Column 2: Details with icons */}
        <div className="session-card__details" style={{ display: 'flex', flexDirection: 'column', gap: '6px', color: 'var(--text-muted)', fontSize: '13px' }}>
          <div className="session-card__detail-row" style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <span className="material-symbols-outlined" style={{ fontSize: '16px' }}>terminal</span>
            <span>{session.agent_binary || 'bash'}{session.cols && session.rows ? ` (${session.cols}×${session.rows})` : ''}</span>
          </div>
          <div className="session-card__detail-row" style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <span className="material-symbols-outlined" style={{ fontSize: '16px' }}>satellite_alt</span>
            <span>{session.satellite_id?.slice(0, 12) || 'local'}</span>
          </div>
          {session.last_activity_at && (
            <div className="session-card__detail-row" style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <span className="material-symbols-outlined" style={{ fontSize: '16px' }}>timer</span>
              <span>{timeAgo(session.last_activity_at)}</span>
            </div>
          )}
        </div>

        {/* Column 3: Terminal preview */}
        <div className="session-card__terminal-preview" style={{ minWidth: 0, overflow: 'hidden' }}>
          <TerminalPreview sessionId={session.id} />
        </div>

        {/* Column 4: Actions */}
        <div className="session-card__actions" style={{ display: 'flex', flexDirection: 'column', gap: '8px', alignItems: 'flex-end', justifyContent: 'center' }}>
          {(state === 'RUNNING' || state === 'DETACHED') && (
            <button className="btn btn--primary btn--sm" onClick={handleAttach}>
              Attach
            </button>
          )}
          {state === 'RUNNING' && (
            <button className="btn btn--outline btn--sm" onClick={() => onAction(session.id, 'detach')}>
              Detach
            </button>
          )}
          {state === 'SUSPENDED' && (
            <button className="btn btn--outline btn--sm" onClick={() => onAction(session.id, 'resume')}>
              Resume
            </button>
          )}
          {state !== 'TERMINATED' && (
            <button className="btn btn--danger btn--sm" onClick={() => onAction(session.id, 'kill')}>
              Kill
            </button>
          )}
        </div>
      </div>
    </div>
  );
};

/**
 * Dashboard page — Session overview with stats and real-time updates
 */
const Dashboard: React.FC = () => {
  const navigate = useNavigate();
  // Build WebSocket URL for session streaming
  const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = `${wsProtocol}//${window.location.host}/api/v1/sessions/stream`;

  // Use useApi for initial session fetch
  const { data: apiData, loading: apiLoading, error: apiError, refetch } = useApi<SessionsPaginatedResponse>(() => getSessions());
  const apiSessions = apiData?.items ?? [];

  // Use useWebSocket for real-time updates
  const { lastMessage: wsMessage, connected: wsConnected } = useWebSocket(wsUrl);

  // Combine API data and WebSocket updates
  const [sessions, setSessions] = useState<Session[]>([]);
  const [wsLoading, setWsLoading] = useState(true);

  // Initialize sessions from API
  useEffect(() => {
    const items = apiData?.items;
    if (items !== undefined) {
      setSessions(Array.isArray(items) ? items : []);
      setWsLoading(false);
    }
  }, [apiData]);

  // Update sessions from WebSocket messages
  useEffect(() => {
    if (wsMessage && typeof wsMessage === 'object') {
      const msg = wsMessage as { type?: string; sessions?: Session[] };
      if (msg.type === 'session_update' && Array.isArray(msg.sessions)) {
        setSessions(msg.sessions);
      }
    }
  }, [wsMessage]);

  // Compute stats from real session data
  const activeSessions = sessions.filter(s => s.state?.toUpperCase() === 'RUNNING').length;
  const detachedSessions = sessions.filter(s => s.state?.toUpperCase() === 'DETACHED').length;
  const suspendedSessions = sessions.filter(s => s.state?.toUpperCase() === 'SUSPENDED').length;

  // Get running sessions for Quick Access (first 3)
  const runningSessions = sessions
    .filter(s => s.state?.toUpperCase() === 'RUNNING' || s.state?.toUpperCase() === 'DETACHED')
    .slice(0, 3);

  // Loading state
  const loading = apiLoading || wsLoading;

  // Handle session actions
  const handleAction = async (sessionId: string, action: string) => {
    try {
      switch (action) {
        case 'detach': await detachSession(sessionId); break;
        case 'suspend': await suspendSession(sessionId); break;
        case 'resume': await attachSession(sessionId); break;
        case 'kill': await killSession(sessionId); break;
      }
      // Refresh sessions after action
      refetch();
    } catch (err) {
      console.error(`Failed to ${action} session:`, err);
    }
  };

  // Greeting based on time of day
  const getGreeting = () => {
    const hour = new Date().getHours();
    if (hour < 12) return 'Good morning';
    if (hour < 17) return 'Good afternoon';
    return 'Good evening';
  };

  return (
    <div>
      {/* Page Header */}
      <div className="page-header">
        <div className="page-header-greeting">{getGreeting()}</div>
        <h1 className="page-header-title">Dashboard</h1>
        <div className="page-header-subtitle">
          <span className={`status-dot ${wsConnected ? 'status-dot--online' : 'status-dot--offline'}`} />
          {wsConnected ? 'Connected' : 'Connecting...'}
        </div>
      </div>

      {/* Stat Cards - Show skeleton while loading */}
      <div className="stat-grid">
        {loading ? (
          <>
            <StatCardSkeleton />
            <StatCardSkeleton />
            <StatCardSkeleton />
            <StatCardSkeleton />
          </>
        ) : (
          <>
            <StatCard label="Active Sessions" value={activeSessions} icon="play_circle" />
            <StatCard label="Detached" value={detachedSessions} icon="link_off" iconClass="stat-card__icon--muted" trend={detachedSessions === 0 ? 'No change' : undefined} />
            <StatCard label="Suspended" value={suspendedSessions} icon="pause_circle" iconClass="stat-card__icon--warning" />
            <StatCard label="Total Instances" value={sessions.length} icon="dynamic_feed" />
          </>
        )}
      </div>

      {/* System Health Gauges */}
      {!loading && <SystemHealth />}

      {/* Quick Access Section - Show first 3 running sessions */}
      {!loading && runningSessions.length > 0 && (
        <>
          <div className="section-header">
            <h2 className="section-title">Quick Access</h2>
          </div>
          <div className="quick-access-grid">
            {runningSessions.map(session => (
              <div
                key={session.id}
                className="quick-access-card"
                onClick={() => {
                  const state = session.state?.toUpperCase() || '';
                  if (state === 'RUNNING' || state === 'DETACHED') {
                    window.location.href = `/session/${session.id}`;
                  }
                }}
              >
                <div className="quick-access-card__icon">
                  <TerminalIcon size={20} />
                </div>
                <div className="quick-access-card__info">
                  <div className="quick-access-card__name">{session.name || session.id.slice(0, 12)}</div>
                  <div className="quick-access-card__meta">{session.agent_binary || 'agent'}</div>
                </div>
                <StateBadge state={session.state} />
              </div>
            ))}
          </div>
        </>
      )}

      {/* Error State */}
      {apiError && (
        <div className="error-state">
          <div className="error-state__title">Failed to load sessions</div>
          <div className="error-state__desc">{apiError.message}</div>
          <button className="btn btn--primary" onClick={() => refetch()}>
            Retry
          </button>
        </div>
      )}

      {/* Sessions */}
      <div className="section-header">
        <h2 className="section-title">Active Sessions</h2>
      </div>

      {loading ? (
        <div className="session-list">
          <SessionCardSkeleton />
          <SessionCardSkeleton />
          <SessionCardSkeleton />
        </div>
      ) : sessions.length === 0 ? (
        <div className="empty-state">
          <div className="empty-state__icon">
            <TerminalIcon size={28} />
          </div>
          <div className="empty-state__title">No active sessions</div>
          <div className="empty-state__desc">
            Sessions will appear here when AI agents are running on your satellites.
          </div>
          <button className="btn btn--primary btn--sm" style={{ marginTop: 16 }} onClick={() => navigate('/sessions')}>
            <PlusIcon size={16} />
            New Session
          </button>
        </div>
      ) : (
        <div className="session-list">
          {sessions.map(session => (
            <SessionCard
              key={session.id}
              session={session}
              onAction={handleAction}
            />
          ))}
        </div>
      )}
    </div>
  );
};

export default Dashboard;
