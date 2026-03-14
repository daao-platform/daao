/**
 * ScheduleConfig — Schedule configuration panel for scheduled agent runs
 *
 * Features:
 * - Cron expression input with human-readable helper text
 * - Satellite selector dropdown (fetches from API)
 * - On-failure strategy dropdown (notify/retry/escalate)
 * - Max retries input (1-10)
 * - Save/Delete buttons for schedule management
 * - Next run time display (client-side calculation)
 * - Enterprise-gated: locked message for community license
 */

import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { apiRequest, getSatellites, type Satellite } from '../api/client';
import { useLicense } from '../hooks/useLicense';
import EnterpriseBadge from './EnterpriseBadge';

// ============================================================================
// Types
// ============================================================================

/** On-failure strategy options */
export type OnFailureStrategy = 'notify' | 'retry' | 'escalate';

/** Schedule configuration stored in agent definition */
export interface AgentSchedule {
  cron_expr: string;
  satellite_id: string;
  max_retries: number;
  on_failure: OnFailureStrategy;
}

/** Props for ScheduleConfig component */
interface ScheduleConfigProps {
  /** Agent ID to configure schedule for */
  agentId: string;
  /** Initial schedule data (if agent already has a schedule) */
  initialSchedule?: AgentSchedule | null;
  /** Callback when schedule is saved successfully */
  onSave?: (schedule: AgentSchedule) => void;
  /** Callback when schedule is deleted successfully */
  onDelete?: () => void;
}

// ============================================================================
// Cron Helper Functions
// ============================================================================

/**
 * Parse common cron expressions to human-readable format
 */
function cronToHumanReadable(cronExpr: string): string {
  if (!cronExpr) return '';

  const parts = cronExpr.trim().split(/\s+/);
  if (parts.length < 5) return 'Invalid cron expression';

  const [minute, hour, dayOfMonth, month, dayOfWeek] = parts;

  // Common patterns
  if (minute === '0' && hour === '*' && dayOfMonth === '*' && month === '*' && dayOfWeek === '*') {
    return 'Every hour';
  }
  if (minute === '0' && hour === '*/6' && dayOfMonth === '*' && month === '*' && dayOfWeek === '*') {
    return 'Every 6 hours';
  }
  if (minute === '0' && hour === '*/12' && dayOfMonth === '*' && month === '*' && dayOfWeek === '*') {
    return 'Every 12 hours';
  }
  if (minute === '0' && hour === '0' && dayOfMonth === '*' && month === '*' && dayOfWeek === '*') {
    return 'Daily at midnight';
  }
  if (minute === '0' && hour === '2' && dayOfMonth === '*' && month === '*' && dayOfWeek === '*') {
    return 'Daily at 2am';
  }
  if (minute === '0' && hour === '6' && dayOfMonth === '*' && month === '*' && dayOfWeek === '*') {
    return 'Daily at 6am';
  }
  if (minute === '0' && hour === '9' && dayOfMonth === '*' && month === '*' && dayOfWeek === '*') {
    return 'Daily at 9am';
  }
  if (minute === '0' && hour === '12' && dayOfMonth === '*' && month === '*' && dayOfWeek === '*') {
    return 'Daily at noon';
  }
  if (minute === '0' && hour === '18' && dayOfMonth === '*' && month === '*' && dayOfWeek === '*') {
    return 'Daily at 6pm';
  }

  // Weekly patterns
  if (dayOfWeek !== '*' && dayOfMonth === '*') {
    const dayNames = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
    const days = dayOfWeek.split(',').map(d => {
      const idx = parseInt(d, 10);
      return idx >= 0 && idx <= 6 ? dayNames[idx] : d;
    }).join(', ');
    if (minute !== '*' && hour !== '*') {
      return `Weekly on ${days} at ${hour.padStart(2, '0')}:${minute.padStart(2, '0')}`;
    }
    return `Weekly on ${days}`;
  }

  // Monthly patterns
  if (dayOfMonth !== '*' && dayOfMonth !== '?') {
    if (minute !== '*' && hour !== '*') {
      return `Monthly on day ${dayOfMonth} at ${hour.padStart(2, '0')}:${minute.padStart(2, '0')}`;
    }
    return `Monthly on day ${dayOfMonth}`;
  }

  // Every X minutes
  if (minute.startsWith('*/')) {
    const interval = minute.slice(2);
    return `Every ${interval} minutes`;
  }

  // Every X hours
  if (hour.startsWith('*/')) {
    const interval = hour.slice(2);
    return `Every ${interval} hours`;
  }

  // Specific time
  if (minute !== '*' && hour !== '*') {
    return `Daily at ${hour.padStart(2, '0')}:${minute.padStart(2, '0')}`;
  }

  return `Cron: ${cronExpr}`;
}

/**
 * Calculate the next run time from a cron expression (simplified approximation)
 * Note: This is a client-side approximation, not a full cron parser
 */
function calculateNextRun(cronExpr: string): Date | null {
  if (!cronExpr) return null;

  const parts = cronExpr.trim().split(/\s+/);
  if (parts.length < 5) return null;

  const [minute, hour, dayOfMonth, month, dayOfWeek] = parts;
  const now = new Date();
  const next = new Date(now);

  // Reset to start of current hour for efficiency
  next.setMinutes(0, 0, 0);

  // Simple case: daily at specific time
  if (minute !== '*' && hour !== '*' && dayOfMonth === '*' && month === '*') {
    const targetHour = parseInt(hour, 10);
    const targetMin = parseInt(minute, 10);

    if (!isNaN(targetHour) && !isNaN(targetMin)) {
      next.setHours(targetHour, targetMin, 0, 0);
      if (next <= now) {
        next.setDate(next.getDate() + 1);
      }
      return next;
    }
  }

  // Every X hours
  if (minute !== '*' && hour?.startsWith('*/')) {
    const interval = parseInt(hour.slice(2), 10);
    if (!isNaN(interval)) {
      const targetMin = parseInt(minute, 10);
      next.setHours(Math.ceil(now.getHours() / interval) * interval, targetMin, 0, 0);
      if (next <= now) {
        next.setHours(next.getHours() + interval);
      }
      return next;
    }
  }

  // Every X minutes
  if (minute?.startsWith('*/')) {
    const interval = parseInt(minute.slice(2), 10);
    if (!isNaN(interval)) {
      const minutesUntilNext = Math.ceil(now.getMinutes() / interval) * interval;
      next.setMinutes(minutesUntilNext, 0, 0);
      if (next <= now) {
        next.setMinutes(next.getMinutes() + interval);
      }
      return next;
    }
  }

  // Fallback: add 1 hour
  next.setHours(next.getHours() + 1, 0, 0, 0);
  return next;
}

// ============================================================================
// API Functions
// ============================================================================

/**
 * Save agent schedule configuration
 */
async function saveAgentSchedule(agentId: string, schedule: AgentSchedule): Promise<AgentSchedule> {
  return apiRequest<AgentSchedule>(`/agents/${agentId}/schedule`, {
    method: 'PUT',
    body: JSON.stringify(schedule),
  });
}

/**
 * Delete agent schedule configuration
 */
async function deleteAgentSchedule(agentId: string): Promise<{ status: string }> {
  return apiRequest<{ status: string }>(`/agents/${agentId}/schedule`, {
    method: 'DELETE',
  });
}

// ============================================================================
// Component
// ============================================================================

export function ScheduleConfig({ agentId, initialSchedule, onSave, onDelete }: ScheduleConfigProps) {
  const { isCommunity, isEnterprise, loading: licenseLoading } = useLicense();

  // Form state
  const [cronExpr, setCronExpr] = useState(initialSchedule?.cron_expr || '0 */6 * * *');
  const [satelliteId, setSatelliteId] = useState(initialSchedule?.satellite_id || '');
  const [maxRetries, setMaxRetries] = useState(initialSchedule?.max_retries || 3);
  const [onFailure, setOnFailure] = useState<OnFailureStrategy>(initialSchedule?.on_failure || 'notify');

  // UI state
  const [satellites, setSatellites] = useState<Satellite[]>([]);
  const [loadingSatellites, setLoadingSatellites] = useState(false);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  // Load satellites on mount
  useEffect(() => {
    const loadSatellites = async () => {
      setLoadingSatellites(true);
      try {
        const satList = await getSatellites();
        setSatellites(satList);
        // Set default satellite if not already set
        if (!satelliteId && satList.length > 0) {
          setSatelliteId(satList[0].id);
        }
      } catch (err) {
        console.error('Failed to load satellites:', err);
        setError('Failed to load satellites');
      } finally {
        setLoadingSatellites(false);
      }
    };
    loadSatellites();
  }, []);

  // Initialize form when initialSchedule changes
  useEffect(() => {
    if (initialSchedule) {
      setCronExpr(initialSchedule.cron_expr || '0 */6 * * *');
      setSatelliteId(initialSchedule.satellite_id || '');
      setMaxRetries(initialSchedule.max_retries || 3);
      setOnFailure(initialSchedule.on_failure || 'notify');
    }
  }, [initialSchedule]);

  // Compute next run time
  const nextRun = useMemo(() => {
    if (!cronExpr || isCommunity) return null;
    return calculateNextRun(cronExpr);
  }, [cronExpr, isCommunity]);

  // Human-readable cron description
  const cronDescription = useMemo(() => {
    return cronToHumanReadable(cronExpr);
  }, [cronExpr]);

  // Validate form
  const isValid = useCallback(() => {
    if (!cronExpr.trim()) return false;
    if (!satelliteId) return false;
    if (maxRetries < 1 || maxRetries > 10) return false;
    return true;
  }, [cronExpr, satelliteId, maxRetries]);

  // Handle save
  const handleSave = async () => {
    if (!isValid()) {
      setError('Please fill in all required fields correctly');
      return;
    }

    setSaving(true);
    setError(null);
    setSuccess(null);

    try {
      const schedule: AgentSchedule = {
        cron_expr: cronExpr,
        satellite_id: satelliteId,
        max_retries: maxRetries,
        on_failure: onFailure,
      };

      await saveAgentSchedule(agentId, schedule);
      setSuccess('Schedule saved successfully');
      onSave?.(schedule);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to save schedule';
      setError(message);
    } finally {
      setSaving(false);
    }
  };

  // Handle delete
  const handleDelete = async () => {
    setDeleting(true);
    setError(null);
    setSuccess(null);

    try {
      await deleteAgentSchedule(agentId);
      setSuccess('Schedule deleted successfully');
      onDelete?.();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to delete schedule';
      setError(message);
    } finally {
      setDeleting(false);
    }
  };

  // Format next run for display
  const formatNextRun = (date: Date): string => {
    return date.toLocaleString(undefined, {
      weekday: 'short',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  };

  // Community license: show upgrade message
  if (isCommunity && !licenseLoading) {
    return (
      <div className="schedule-config">
        <div className="schedule-config__header">
          <h3 className="schedule-config__title">Schedule</h3>
          <EnterpriseBadge />
        </div>
        <div className="schedule-config__locked">
          <div className="schedule-config__locked-icon">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
              <path d="M7 11V7a5 5 0 0 1 10 0v4" />
            </svg>
          </div>
          <div className="schedule-config__locked-content">
            <h4>Coming Soon</h4>
            <p>Scheduled agent runs are planned for a future DAAO Enterprise release.</p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="schedule-config">
      <div className="schedule-config__header">
        <h3 className="schedule-config__title">Schedule</h3>
        <EnterpriseBadge size="small" />
      </div>

      {/* Error/Success Messages */}
      {error && (
        <div className="schedule-config__message schedule-config__message--error">
          {error}
        </div>
      )}
      {success && (
        <div className="schedule-config__message schedule-config__message--success">
          {success}
        </div>
      )}

      {/* Cron Expression */}
      <div className="schedule-config__field">
        <label className="schedule-config__label" htmlFor="cron-expr">
          Cron Expression
          <span className="schedule-config__required">*</span>
        </label>
        <input
          id="cron-expr"
          type="text"
          className="schedule-config__input"
          value={cronExpr}
          onChange={(e) => setCronExpr(e.target.value)}
          placeholder="0 */6 * * *"
        />
        <span className="schedule-config__helper">
          {cronDescription || 'e.g., 0 */6 * * * = every 6 hours'}
        </span>
      </div>

      {/* Satellite Selector */}
      <div className="schedule-config__field">
        <label className="schedule-config__label" htmlFor="satellite">
          Satellite
          <span className="schedule-config__required">*</span>
        </label>
        <select
          id="satellite"
          className="schedule-config__select"
          value={satelliteId}
          onChange={(e) => setSatelliteId(e.target.value)}
          disabled={loadingSatellites}
        >
          <option value="">Select a satellite</option>
          {satellites.map((sat) => (
            <option key={sat.id} value={sat.id}>
              {sat.name}
            </option>
          ))}
        </select>
        {loadingSatellites && (
          <span className="schedule-config__helper">Loading satellites...</span>
        )}
      </div>

      {/* On-Failure Strategy */}
      <div className="schedule-config__field">
        <label className="schedule-config__label" htmlFor="on-failure">
          On Failure
        </label>
        <select
          id="on-failure"
          className="schedule-config__select"
          value={onFailure}
          onChange={(e) => setOnFailure(e.target.value as OnFailureStrategy)}
        >
          <option value="notify">Notify</option>
          <option value="retry">Retry</option>
          <option value="escalate">Escalate</option>
        </select>
        <span className="schedule-config__helper">
          {onFailure === 'notify' && 'Send notification on failure'}
          {onFailure === 'retry' && 'Retry with configured max retries'}
          {onFailure === 'escalate' && 'Escalate to admin on failure'}
        </span>
      </div>

      {/* Max Retries */}
      <div className="schedule-config__field">
        <label className="schedule-config__label" htmlFor="max-retries">
          Max Retries
        </label>
        <input
          id="max-retries"
          type="number"
          className="schedule-config__input schedule-config__input--number"
          value={maxRetries}
          onChange={(e) => {
            const val = parseInt(e.target.value, 10);
            if (!isNaN(val)) {
              setMaxRetries(Math.min(10, Math.max(1, val)));
            }
          }}
          min={1}
          max={10}
        />
        <span className="schedule-config__helper">
          Number of retry attempts (1-10)
        </span>
      </div>

      {/* Next Run Display */}
      {nextRun && (
        <div className="schedule-config__next-run">
          <span className="schedule-config__next-run-label">Next run:</span>
          <span className="schedule-config__next-run-time">
            {formatNextRun(nextRun)}
          </span>
        </div>
      )}

      {/* Action Buttons */}
      <div className="schedule-config__actions">
        <button
          type="button"
          className="schedule-config__button schedule-config__button--primary"
          onClick={handleSave}
          disabled={saving || !isValid()}
        >
          {saving ? 'Saving...' : 'Save Schedule'}
        </button>
        {initialSchedule && (
          <button
            type="button"
            className="schedule-config__button schedule-config__button--danger"
            onClick={handleDelete}
            disabled={deleting}
          >
            {deleting ? 'Deleting...' : 'Delete Schedule'}
          </button>
        )}
      </div>
    </div>
  );
};

export default ScheduleConfig;
