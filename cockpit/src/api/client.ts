/**
 * Nexus REST API Client
 * 
 * Typed API client for interacting with the Nexus API Gateway.
 * Provides functions for session management, WebTransport connections,
 * and push notification subscriptions.
 */

import type { PushSubscriptionData } from '../push';

// ============================================================================
// Types
// ============================================================================

// Session states matching backend
export const SESSION_STATES = {
  PROVISIONING: 'PROVISIONING',
  RUNNING: 'RUNNING',
  DETACHED: 'DETACHED',
  RE_ATTACHING: 'RE_ATTACHING',
  SUSPENDED: 'SUSPENDED',
  TERMINATED: 'TERMINATED',
} as const;

export type SessionState = typeof SESSION_STATES[keyof typeof SESSION_STATES];

// Session model from API
export interface Session {
  id: string;
  satellite_id: string;
  user_id: string;
  name: string;
  agent_binary: string;
  agent_args: string[];
  state: SessionState;
  os_pid?: number;
  pts_name?: string;
  cols: number;
  rows: number;
  recording_enabled: boolean;
  last_activity_at: string;
  started_at?: string;
  detached_at?: string;
  suspended_at?: string;
  terminated_at?: string;
  created_at: string;
}

// Paginated sessions response
export interface SessionsPaginatedResponse {
  items: Session[];
  count: number;
  total: number;
  next_cursor?: string;
}

// Session with computed properties for UI
export interface SessionWithMeta extends Session {
  dmsExpiresAt?: number;
  agentType: string;
  satellite: string;
}

// API response types
export interface SessionsResponse {
  sessions: Session[];
}

export interface SessionDetailResponse {
  session: Session;
  events: SessionEvent[];
}

export interface SessionEvent {
  id: number;
  session_id: string;
  event_type: string;
  payload?: Record<string, unknown>;
  created_at: string;
}

// Satellite model from API
export interface Satellite {
  id: string;
  name: string;
  owner_id: string;
  status: string;
  os?: string;
  arch?: string;
  version?: string;
  available_agents: string[];
  created_at: string;
  updated_at: string;
}

export interface SessionActionResponse {
  status: string;
  state?: SessionState;
  session?: string;
  error?: string;
}

// Push notification subscription request
export interface PushSubscriptionRequest {
  endpoint: string;
  keys: {
    p256dh: string;
    auth: string;
  };
  expirationTime: number | null;
  vapidPublicKey: string;
}

export interface PushSubscriptionResponse {
  success: boolean;
  subscriptionId?: string;
}

// WebTransport connection config
export interface WebTransportConnectionConfig {
  sessionId: string;
  jwt: string;
  serverUrl?: string;
}

// ============================================================================
// API Client
// ============================================================================

const API_BASE_URL = '/api/v1';

// Helper function to get auth token from OIDC session
function getAuthToken(): string | null {
  // Read from sessionStorage where AuthProvider stores the OIDC access token
  return sessionStorage.getItem('oidc_access_token') || localStorage.getItem('auth_token');
}

// Helper for making authenticated requests
export async function apiRequest<T>(
  endpoint: string,
  options: RequestInit = {}
): Promise<T> {
  const token = getAuthToken();
  const headers: HeadersInit = {
    'Content-Type': 'application/json',
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...options.headers,
  };

  const response = await globalThis.fetch(`${API_BASE_URL}${endpoint}`, {
    ...options,
    headers,
  });

  // Handle 401 — token expired or invalid, redirect to login
  if (response.status === 401) {
    sessionStorage.removeItem('oidc_access_token');
    sessionStorage.removeItem('oidc_expires_at');
    sessionStorage.removeItem('oidc_user_info');
    // Only redirect if we're not already on the login page
    if (!window.location.pathname.startsWith('/login') && !window.location.pathname.startsWith('/auth/')) {
      window.location.href = '/login';
    }
    throw new Error('Authentication required');
  }

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: 'Request failed' }));
    throw new Error(error.error || `HTTP ${response.status}`);
  }

  return response.json();
}

// ============================================================================
// Session API
// ============================================================================

/**
 * Get all sessions for the authenticated user with optional pagination
 */
export interface GetSessionsParams {
  cursor?: string;
  limit?: number;
}

export async function getSessions(params?: GetSessionsParams): Promise<SessionsPaginatedResponse> {
  let endpoint = '/sessions';
  const queryParams = new URLSearchParams();

  if (params) {
    if (params.cursor) {
      queryParams.append('cursor', params.cursor);
    }
    if (params.limit) {
      queryParams.append('limit', params.limit.toString());
    }
  }

  const queryString = queryParams.toString();
  if (queryString) {
    endpoint += `?${queryString}`;
  }

  return apiRequest<SessionsPaginatedResponse>(endpoint);
}

/**
 * Create a new session
 */
export interface CreateSessionRequest {
  name: string;
  satellite_id: string;
  agent_binary: string;
  agent_args?: string[];
  working_dir?: string;
  cols?: number;
  rows?: number;
}

export async function createSession(request: CreateSessionRequest): Promise<Session> {
  return apiRequest<Session>('/sessions', {
    method: 'POST',
    body: JSON.stringify(request),
  });
}

// ============================================================================
// Satellites API
// ============================================================================

/**
 * Get all satellites for the authenticated user
 */
export async function getSatellites(): Promise<Satellite[]> {
  return apiRequest<Satellite[]>('/satellites');
}

/**
 * Create a new satellite
 */
export interface CreateSatelliteRequest {
  name: string;
}

export async function createSatellite(request: CreateSatelliteRequest): Promise<Satellite> {
  return apiRequest<Satellite>('/satellites', {
    method: 'POST',
    body: JSON.stringify(request),
  });
}

/**
 * Delete a satellite
 */
export async function deleteSatellite(satelliteId: string): Promise<{ status: string }> {
  return apiRequest<{ status: string }>(`/satellites/${satelliteId}`, {
    method: 'DELETE',
  });
}

/**
 * Rename a satellite
 */
export async function renameSatellite(satelliteId: string, name: string): Promise<{ id: string; name: string }> {
  return apiRequest<{ id: string; name: string }>(`/satellites/${satelliteId}/name`, {
    method: 'PATCH',
    body: JSON.stringify({ name }),
  });
}

// ============================================================================
// Satellite Telemetry API
// ============================================================================

export interface TelemetryGPU {
  index: number;
  name: string;
  utilization_percent: number;
  memory_used_bytes: number;
  memory_total_bytes: number;
  temperature_celsius: number;
}

export interface TelemetryData {
  satellite_id: string;
  cpu_percent: number;
  memory_percent: number;
  memory_used_bytes: number;
  memory_total_bytes: number;
  disk_percent: number;
  disk_used_bytes: number;
  disk_total_bytes: number;
  gpus: TelemetryGPU[];
  active_sessions: number;
  collected_at: string;
}

export interface TelemetryPoint {
  cpu_percent: number;
  memory_percent: number;
  disk_percent: number;
  gpus: TelemetryGPU[];
  active_sessions: number;
  timestamp: string;
}

/**
 * Get latest telemetry for a satellite
 */
export async function getSatelliteTelemetry(satelliteId: string): Promise<TelemetryData> {
  return apiRequest<TelemetryData>(`/satellites/${satelliteId}/telemetry`);
}

/**
 * Get telemetry history for sparklines (last 60 data points)
 */
export async function getSatelliteTelemetryHistory(satelliteId: string): Promise<TelemetryPoint[]> {
  return apiRequest<TelemetryPoint[]>(`/satellites/${satelliteId}/telemetry/history`);
}

/**
 * Get a specific session by ID
 */
export async function getSession(sessionId: string): Promise<Session> {
  return apiRequest<Session>(`/sessions/${sessionId}`);
}

/**
 * Attach to a session (resume a detached session)
 */
export async function attachSession(sessionId: string): Promise<Session> {
  return apiRequest<Session>(`/sessions/${sessionId}/attach`, {
    method: 'POST',
  });
}

/**
 * Detach from a session (keep it running in background)
 */
export async function detachSession(sessionId: string): Promise<SessionActionResponse> {
  return apiRequest<SessionActionResponse>(`/sessions/${sessionId}/detach`, {
    method: 'POST',
  });
}

/**
 * Suspend a running session (trigger DMS)
 */
export async function suspendSession(sessionId: string): Promise<SessionActionResponse> {
  return apiRequest<SessionActionResponse>(`/sessions/${sessionId}/suspend`, {
    method: 'POST',
  });
}

/**
 * Resume a suspended session
 */
export async function resumeSession(sessionId: string): Promise<SessionActionResponse> {
  return apiRequest<SessionActionResponse>(`/sessions/${sessionId}/resume`, {
    method: 'POST',
  });
}

/**
 * Kill/terminate a session
 */
export async function killSession(sessionId: string): Promise<SessionActionResponse> {
  return apiRequest<SessionActionResponse>(`/sessions/${sessionId}/kill`, {
    method: 'POST',
  });
}

/**
 * Delete a terminated session
 */
export async function deleteSession(sessionId: string): Promise<{ status: string }> {
  return apiRequest<{ status: string }>(`/sessions/${sessionId}`, {
    method: 'DELETE',
  });
}

/**
 * Rename a session
 */
export async function renameSession(sessionId: string, name: string): Promise<void> {
  await apiRequest<{ id: string; name: string }>(`/sessions/${sessionId}/name`, {
    method: 'PATCH',
    body: JSON.stringify({ name }),
  });
}

// ============================================================================
// WebTransport API
// ============================================================================

/**
 * Get WebTransport connection URL for a session
 * Returns the WebTransport endpoint for the given session
 */
export function getWebTransportUrl(sessionId: string, serverUrl?: string): string {
  const baseUrl = serverUrl || (window.location.protocol === 'https:'
    ? 'wss://' + window.location.host
    : 'ws://' + window.location.host);
  return `${baseUrl}/api/v1/sessions/${sessionId}/transport`;
}

// ============================================================================
// Push Notification API
// ============================================================================

/**
 * Subscribe to push notifications
 * Sends the push subscription and VAPID key to the backend
 */
export async function subscribeToPushNotifications(
  subscription: PushSubscriptionData,
  vapidPublicKey: string
): Promise<PushSubscriptionResponse> {
  const request: PushSubscriptionRequest = {
    endpoint: subscription.endpoint,
    keys: subscription.keys,
    expirationTime: subscription.expirationTime,
    vapidPublicKey,
  };

  return apiRequest<PushSubscriptionResponse>('/push/subscribe', {
    method: 'POST',
    body: JSON.stringify(request),
  });
}

/**
 * Unsubscribe from push notifications
 */
export async function unsubscribeFromPushNotifications(
  endpoint: string
): Promise<{ success: boolean }> {
  return apiRequest<{ success: boolean }>('/push/unsubscribe', {
    method: 'POST',
    body: JSON.stringify({ endpoint }),
  });
}

// ============================================================================
// Config Cache
// ============================================================================

interface DaaoConfig {
  dms_ttl_minutes: number;
  heartbeat_interval_seconds: number;
}

let _cachedConfig: DaaoConfig | null = null;

/**
 * Get server configuration (cached after first fetch)
 */
export async function getConfig(): Promise<DaaoConfig> {
  if (_cachedConfig) return _cachedConfig;
  try {
    _cachedConfig = await apiRequest<DaaoConfig>('/config');
    return _cachedConfig;
  } catch {
    // Fallback to defaults if config endpoint is unavailable
    return { dms_ttl_minutes: 60, heartbeat_interval_seconds: 30 };
  }
}

// ============================================================================
// Recording API
// ============================================================================

export interface Recording {
  id: string;
  session_id: string;
  filename: string;
  size_bytes: number;
  duration_ms: number;
  started_at: string;
  stopped_at?: string;
  created_at: string;
}

export async function getRecordingConfig(): Promise<{ recording_enabled: boolean }> {
  return apiRequest<{ recording_enabled: boolean }>('/config/recording');
}

export async function setRecordingConfig(enabled: boolean): Promise<{ recording_enabled: boolean }> {
  return apiRequest<{ recording_enabled: boolean }>('/config/recording', {
    method: 'PUT',
    body: JSON.stringify({ recording_enabled: enabled }),
  });
}

export async function startRecording(sessionId: string, cols?: number, rows?: number): Promise<{ recording_id: string; status: string }> {
  return apiRequest<{ recording_id: string; status: string }>(`/sessions/${sessionId}/recording/start`, {
    method: 'POST',
    body: JSON.stringify({ cols, rows }),
  });
}

export async function stopRecording(sessionId: string): Promise<{ duration_ms: number; size_bytes: number; status: string }> {
  return apiRequest<{ duration_ms: number; size_bytes: number; status: string }>(`/sessions/${sessionId}/recording/stop`, {
    method: 'POST',
  });
}

export async function listRecordings(sessionId: string): Promise<Recording[]> {
  return apiRequest<Recording[]>(`/sessions/${sessionId}/recordings`);
}

export interface RecordingWithSession extends Recording {
  session_name: string;
}

export async function listAllRecordings(): Promise<RecordingWithSession[]> {
  return apiRequest<RecordingWithSession[]>('/recordings');
}

export async function getRecording(recordingId: string): Promise<Recording> {
  return apiRequest<Recording>(`/recordings/${recordingId}`);
}

export async function toggleSessionRecording(sessionId: string, enabled: boolean): Promise<{ recording_enabled: boolean }> {
  return apiRequest<{ recording_enabled: boolean }>(`/sessions/${sessionId}/recording`, {
    method: 'PATCH',
    body: JSON.stringify({ recording_enabled: enabled }),
  });
}

export function getRecordingStreamUrl(recordingId: string): string {
  return `${API_BASE_URL}/recordings/${recordingId}/stream`;
}

// ============================================================================
// Utility Functions
// ============================================================================

/**
 * Transform API session to UI session with computed properties
 * Uses cached config for DMS TTL (falls back to 60min if not yet fetched)
 */
export function transformSession(session: Session): SessionWithMeta {
  const dmsTTLMs = (_cachedConfig?.dms_ttl_minutes ?? 60) * 60 * 1000;
  return {
    ...session,
    agentType: session.agent_binary || 'unknown',
    satellite: session.satellite_id || 'unknown',
    // Compute DMS expiry using server-configured TTL
    dmsExpiresAt: session.state === SESSION_STATES.RUNNING && session.last_activity_at
      ? new Date(session.last_activity_at).getTime() + dmsTTLMs
      : undefined,
  };
}

/**
 * Transform API sessions to UI sessions
 */
export function transformSessions(sessions: Session[]): SessionWithMeta[] {
  return sessions.map(transformSession);
}

// ============================================================================
// Notifications API
// ============================================================================

export type NotificationType = 'SESSION_TERMINATED' | 'SESSION_SUSPENDED' | 'SESSION_ERROR' | 'SATELLITE_OFFLINE';
export type NotificationPriority = 'INFO' | 'WARNING' | 'CRITICAL';

export interface NotificationItem {
  id: string;
  user_id: string;
  type: NotificationType;
  priority: NotificationPriority;
  title: string;
  body: string;
  session_id?: string;
  satellite_id?: string;
  payload?: Record<string, unknown>;
  read: boolean;
  created_at: string;
}

export interface NotificationsListResponse {
  notifications: NotificationItem[];
  count: number;
  next_cursor: string;
}

export interface NotificationPreferences {
  user_id: string;
  min_priority: NotificationPriority;
  browser_enabled: boolean;
  session_terminated: boolean;
  session_error: boolean;
  satellite_offline: boolean;
  session_suspended: boolean;
}

export async function getNotifications(limit = 50, cursor?: string): Promise<NotificationsListResponse> {
  const params = new URLSearchParams({ limit: limit.toString() });
  if (cursor) params.append('cursor', cursor);
  return apiRequest<NotificationsListResponse>(`/notifications?${params}`);
}

export async function getUnreadCount(): Promise<{ count: number }> {
  return apiRequest<{ count: number }>('/notifications/unread-count');
}

export async function markNotificationRead(id: string): Promise<void> {
  await apiRequest(`/notifications/${id}/read`, { method: 'PATCH' });
}

export async function markAllNotificationsRead(): Promise<void> {
  await apiRequest('/notifications/read-all', { method: 'POST' });
}

export async function getNotificationPreferences(): Promise<NotificationPreferences> {
  return apiRequest<NotificationPreferences>('/notifications/preferences');
}

export async function updateNotificationPreferences(prefs: Partial<NotificationPreferences>): Promise<NotificationPreferences> {
  return apiRequest<NotificationPreferences>('/notifications/preferences', {
    method: 'PUT',
    body: JSON.stringify(prefs),
  });
}

// ============================================================================
// License API
// ============================================================================

export interface EnterpriseFeature {
  ID: string;
  Name: string;
  Description: string;
}

export interface LicenseInfo {
  tier: string;
  max_users: number;
  max_satellites: number;
  max_recordings: number;
  telemetry_retention_hours: number;
  customer?: string;
  expires_at?: number;
  enterprise_features: EnterpriseFeature[];
}

/**
 * Get license information (tier, limits, enterprise features)
 */
export async function getLicenseInfo(): Promise<LicenseInfo> {
  return apiRequest<LicenseInfo>('/license');
}

// ============================================================================
// HITL Proposals API (Enterprise)
// ============================================================================

export type RiskLevel = 'low' | 'medium' | 'high' | 'critical';
export type ProposalStatus = 'pending' | 'approved' | 'denied' | 'expired' | 'cancelled';

export interface Proposal {
  id: string;
  session_id: string;
  satellite_id: string;
  proposal_id: string;
  command: string;
  justification: string;
  risk_level: RiskLevel;
  status: ProposalStatus;
  decided_by?: string;
  decided_at?: string;
  decision_reason?: string;
  created_at: string;
  expires_at: string;
}

export interface ProposalsListResponse {
  proposals: Proposal[];
  pending_count: number;
}

export interface ProposalCountResponse {
  pending_count: number;
  enabled: boolean;
}

export async function getProposals(status?: 'pending'): Promise<ProposalsListResponse> {
  const params = status ? `?status=${status}` : '';
  return apiRequest<ProposalsListResponse>(`/proposals${params}`);
}

export async function getProposal(id: string): Promise<Proposal> {
  return apiRequest<Proposal>(`/proposals/${id}`);
}

export async function approveProposal(id: string, reason?: string): Promise<Proposal> {
  return apiRequest<Proposal>(`/proposals/${id}/approve`, {
    method: 'POST',
    body: JSON.stringify({ reason: reason || '' }),
  });
}

export async function denyProposal(id: string, reason?: string): Promise<Proposal> {
  return apiRequest<Proposal>(`/proposals/${id}/deny`, {
    method: 'POST',
    body: JSON.stringify({ reason: reason || '' }),
  });
}

export async function getProposalCount(): Promise<ProposalCountResponse> {
  return apiRequest<ProposalCountResponse>('/proposals/count');
}

// ============================================================================
// Session Preview API
// ============================================================================

export interface SessionPreviewResponse {
  text: string;
  has_content: boolean;
}

export async function getSessionPreview(sessionId: string): Promise<SessionPreviewResponse> {
  return apiRequest<SessionPreviewResponse>(`/sessions/${sessionId}/preview`);
}

// ============================================================================
// Audit Log API
// ============================================================================

export interface AuditLogEntry {
  id: string;
  actor_id: string | null;
  actor_email: string;
  action: string;
  resource_type: string;
  resource_id: string | null;
  details: Record<string, unknown> | null;
  ip_address: string | null;
  created_at: string;
}

export interface AuditLogResponse {
  entries: AuditLogEntry[];
  total: number;
  limit: number;
  offset: number;
}

export interface GetAuditLogParams {
  action?: string;
  resource_type?: string;
  since?: string;
  until?: string;
  limit?: number;
  offset?: number;
}

export async function getAuditLog(params?: GetAuditLogParams): Promise<AuditLogResponse> {
  const queryParams = new URLSearchParams();
  
  if (params?.action) queryParams.append('action', params.action);
  if (params?.resource_type) queryParams.append('resource_type', params.resource_type);
  if (params?.since) queryParams.append('since', params.since);
  if (params?.until) queryParams.append('until', params.until);
  if (params?.limit) queryParams.append('limit', params.limit.toString());
  if (params?.offset) queryParams.append('offset', params.offset.toString());
  
  const queryString = queryParams.toString();
  const endpoint = `/audit-log${queryString ? `?${queryString}` : ''}`;
  
  return apiRequest<AuditLogResponse>(endpoint);
}

// Export audit log to CSV
export async function exportAuditLogCsv(params?: GetAuditLogParams): Promise<void> {
  const queryParams = new URLSearchParams();
  queryParams.append('format', 'csv');
  
  if (params?.action) queryParams.append('action', params.action);
  if (params?.resource_type) queryParams.append('resource_type', params.resource_type);
  if (params?.since) queryParams.append('since', params.since);
  if (params?.until) queryParams.append('until', params.until);
  // For export, we don't use pagination limits
  if (params?.limit) queryParams.append('limit', '10000'); // Max export
  if (params?.offset) queryParams.append('offset', '0');
  
  const token = getAuthToken();
  const response = await globalThis.fetch(`${API_BASE_URL}/audit-log/export?${queryParams}`, {
    headers: {
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
  });
  
  if (!response.ok) {
    if (response.status === 403) {
      throw new Error('Permission denied');
    }
    throw new Error(`Export failed: ${response.status}`);
  }
  
  // Create download link
  const blob = await response.blob();
  const url = window.URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `audit-log-${new Date().toISOString().split('T')[0]}.csv`;
  document.body.appendChild(a);
  a.click();
  window.URL.revokeObjectURL(url);
  document.body.removeChild(a);
}

// ============================================================================
// Exports
// ============================================================================

export default {
  getSessions,
  getSession,
  createSession,
  attachSession,
  detachSession,
  suspendSession,
  resumeSession,
  killSession,
  deleteSession,
  renameSession,
  getSatellites,
  createSatellite,
  deleteSatellite,
  renameSatellite,
  getSatelliteTelemetry,
  getSatelliteTelemetryHistory,
  getWebTransportUrl,
  subscribeToPushNotifications,
  unsubscribeFromPushNotifications,
  transformSession,
  transformSessions,
  SESSION_STATES,
  // Recording API
  getRecordingConfig,
  setRecordingConfig,
  startRecording,
  stopRecording,
  listRecordings,
  getRecording,
  toggleSessionRecording,
  getRecordingStreamUrl,
  // Notifications API
  getNotifications,
  getUnreadCount,
  markNotificationRead,
  markAllNotificationsRead,
  getNotificationPreferences,
  updateNotificationPreferences,
  // License API
  getLicenseInfo,
  // HITL Proposals API
  getProposals,
  getProposal,
  approveProposal,
  denyProposal,
  getProposalCount,
  // Session Preview API
  getSessionPreview,
  // Audit Log API
  getAuditLog,
  exportAuditLogCsv,
};
