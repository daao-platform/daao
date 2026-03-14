import React, { useState, useEffect, useCallback } from 'react';
import { getSatellites, getSatelliteTelemetry, getSatelliteTelemetryHistory, Satellite, TelemetryData, TelemetryPoint } from '../api/client';
import Sparkline from './Sparkline';

interface GaugeProps {
    label: string;
    value: number;
    unit?: string;
    color: string;
    sparklineData?: number[];
}

/**
 * Circular arc gauge — Horizontal: label+value left, arc right.
 */
const Gauge: React.FC<GaugeProps> = ({ label, value, unit = '%', color }) => {
    const radius = 16;
    const strokeWidth = 3;
    const circumference = 2 * Math.PI * radius;
    const progress = Math.min(Math.max(value, 0), 100) / 100;
    const dashOffset = circumference * (1 - progress);
    const size = (radius + strokeWidth) * 2;

    return (
        <div className="gauge">
            <div className="gauge__info">
                <span className="gauge__label">{label}</span>
                <span className="gauge__number">{value.toFixed(0)}%</span>
            </div>
            <div className="gauge__ring">
                <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
                    <circle
                        cx={size / 2} cy={size / 2} r={radius}
                        fill="none"
                        stroke="var(--bg-elevated)"
                        strokeWidth={strokeWidth}
                    />
                    <circle
                        cx={size / 2} cy={size / 2} r={radius}
                        fill="none"
                        stroke={color}
                        strokeWidth={strokeWidth}
                        strokeDasharray={circumference}
                        strokeDashoffset={dashOffset}
                        strokeLinecap="round"
                        transform={`rotate(-90 ${size / 2} ${size / 2})`}
                        className="gauge__arc"
                    />
                </svg>
            </div>
        </div>
    );
};

/**
 * Color for a metric based on value thresholds.
 */
function getGaugeColor(value: number): string {
    if (value < 70) return 'var(--accent)';
    if (value < 90) return 'var(--warning)';
    return 'var(--danger)';
}

/**
 * System Health section for the Dashboard.
 * Fetches telemetry from the first active satellite and displays CPU/Memory/Disk gauges.
 */
const SystemHealth: React.FC = () => {
    const [telemetry, setTelemetry] = useState<TelemetryData | null>(null);
    const [history, setHistory] = useState<TelemetryPoint[]>([]);
    const [activeSatellite, setActiveSatellite] = useState<Satellite | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    const fetchData = useCallback(async () => {
        try {
            const satellites = await getSatellites();
            const active = satellites.find(s => s.status?.toLowerCase() === 'active');
            if (!active) {
                setActiveSatellite(null);
                setTelemetry(null);
                setLoading(false);
                return;
            }
            setActiveSatellite(active);

            const [telem, hist] = await Promise.all([
                getSatelliteTelemetry(active.id).catch(() => null),
                getSatelliteTelemetryHistory(active.id).catch(() => []),
            ]);

            if (telem) setTelemetry(telem);
            if (hist) setHistory(hist);
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setLoading(false);
        }
    }, []);

    // Initial fetch + 10s refresh
    useEffect(() => {
        fetchData();
        const interval = setInterval(fetchData, 10000);
        return () => clearInterval(interval);
    }, [fetchData]);

    // Loading skeleton
    if (loading) {
        return (
            <div className="system-health">
                <h2 className="system-health__title">
                    <span className="material-symbols-outlined">analytics</span>
                    System Health
                </h2>
                <div className="system-health__grid">
                    {[1, 2, 3].map(i => (
                        <div key={i} className="gauge gauge--skeleton">
                            <div className="skeleton skeleton--text" style={{ width: 60, height: 14 }} />
                            <div className="skeleton skeleton--circle" style={{ width: 92, height: 92 }} />
                        </div>
                    ))}
                </div>
            </div>
        );
    }

    // No active satellite
    if (!activeSatellite || !telemetry) {
        return (
            <div className="system-health">
                <h2 className="system-health__title">
                    <span className="material-symbols-outlined">analytics</span>
                    System Health
                </h2>
                <div className="system-health__empty">
                    <span className="text-muted">No active satellite — health metrics will appear when a satellite connects.</span>
                </div>
            </div>
        );
    }

    if (error) return null;

    const cpuHistory = history.map(p => p.cpu_percent);
    const memHistory = history.map(p => p.memory_percent);
    const diskHistory = history.map(p => p.disk_percent);

    return (
        <div className="system-health">
            <h2 className="system-health__title">
                <span className="material-symbols-outlined">analytics</span>
                System Health
                {activeSatellite && <span className="system-health__satellite">{activeSatellite.name}</span>}
            </h2>
            <div className="system-health__grid">
                <Gauge
                    label="CPU Usage"
                    value={telemetry.cpu_percent}
                    color={getGaugeColor(telemetry.cpu_percent)}
                />
                <Gauge
                    label="Memory Usage"
                    value={telemetry.memory_percent}
                    color={getGaugeColor(telemetry.memory_percent)}
                />
                <Gauge
                    label="Disk Usage"
                    value={telemetry.disk_percent}
                    color={getGaugeColor(telemetry.disk_percent)}
                />
            </div>
        </div>
    );
};

export default SystemHealth;
