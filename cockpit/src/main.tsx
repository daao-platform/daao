import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import AppLayout from './components/AppLayout';
import Dashboard from './pages/Dashboard';
import Sessions from './pages/Sessions';
import MultiSessionDashboard from './pages/MultiSessionDashboard';
import Satellites from './pages/Satellites';
import Forge from './pages/Forge';
import { AgentBuilderPage } from './components/AgentBuilder';
import AgentRunPage from './pages/AgentRunPage';
import AgentRunHistory from './pages/AgentRunHistory';
import Settings from './pages/Settings';
import UserManagement from './pages/UserManagement';
import TerminalView from './pages/TerminalView';
import RecordingPlayer from './pages/RecordingPlayer';
import Recordings from './pages/Recordings';
import Notifications from './pages/Notifications';
import Proposals from './pages/Proposals';
import AuditLog from './pages/AuditLog';
import Pipelines from './pages/Pipelines';
import PipelineBuilder from './pages/PipelineBuilder';
import LoginPage from './pages/LoginPage';
import AuthCallback from './pages/AuthCallback';
import { AuthProvider, useAuth } from './auth/AuthProvider';
import { ErrorBoundary } from './components/ErrorBoundary';
import { ToastProvider } from './components/Toast';
import { LicenseProvider } from './hooks/useLicense';
import './index.css';

/**
 * ProtectedRoute — renders children only if authenticated,
 * otherwise redirects to login page.
 */
const ProtectedRoute: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { isAuthenticated, isLoading } = useAuth();

  if (isLoading) {
    return (
      <div style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: 'var(--bg)',
      }}>
        <div className="spinner" />
      </div>
    );
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }

  return <>{children}</>;
};

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ErrorBoundary>
      <BrowserRouter>
        <AuthProvider>
          <ToastProvider>
            <LicenseProvider>
              <Routes>
                {/* Public routes */}
                <Route path="login" element={<LoginPage />} />
                <Route path="auth/callback" element={<AuthCallback />} />
                {/* Protected routes */}
                <Route element={<ProtectedRoute><AppLayout /></ProtectedRoute>}>
                  <Route index element={<Dashboard />} />
                  <Route path="sessions" element={<Sessions />} />
                  <Route path="sessions/multi" element={<MultiSessionDashboard />} />
                  <Route path="satellites" element={<Satellites />} />
                  <Route path="agents" element={<Navigate to="/forge" />} />
                  <Route path="forge" element={<Forge />} />
                  <Route path="forge/builder" element={<AgentBuilderPage />} />
                  <Route path="forge/builder/:agentId" element={<AgentBuilderPage />} />
                  <Route path="forge/runs" element={<AgentRunHistory />} />
                  <Route path="forge/run/:runId" element={<AgentRunPage />} />
                  <Route path="settings" element={<Settings />} />
                  <Route path="settings/users" element={<UserManagement />} />
                  <Route path="session/:sessionId" element={<TerminalView />} />
                  <Route path="recordings" element={<Recordings />} />
                  <Route path="notifications" element={<Notifications />} />
                  <Route path="proposals" element={<Proposals />} />
                  <Route path="audit-log" element={<AuditLog />} />
                  <Route path="pipelines" element={<Pipelines />} />
                  <Route path="pipelines/new" element={<PipelineBuilder />} />
                  <Route path="pipelines/:id/edit" element={<PipelineBuilder />} />
                  <Route path="recording/:recordingId" element={<RecordingPlayer />} />
                </Route>
              </Routes>
            </LicenseProvider>
          </ToastProvider>
        </AuthProvider>
      </BrowserRouter>
    </ErrorBoundary>
  </React.StrictMode>
);

