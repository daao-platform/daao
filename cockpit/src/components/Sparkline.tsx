import React from 'react';

interface SparklineProps {
    data: number[];
    width?: number;
    height?: number;
    color?: string;
    filled?: boolean;
    strokeWidth?: number;
    className?: string;
}

/**
 * Lightweight SVG sparkline chart.
 * Renders a polyline from normalized data points with optional gradient fill.
 */
const Sparkline: React.FC<SparklineProps> = ({
    data,
    width = 120,
    height = 32,
    color = 'var(--accent)',
    filled = true,
    strokeWidth = 1.5,
    className = '',
}) => {
    if (!data || data.length < 2) {
        return (
            <svg width={width} height={height} className={`sparkline ${className}`}>
                <line
                    x1={0} y1={height / 2} x2={width} y2={height / 2}
                    stroke="var(--text-muted)" strokeWidth={1} strokeDasharray="4 4"
                    opacity={0.3}
                />
            </svg>
        );
    }

    const padding = 2;
    const chartWidth = width - padding * 2;
    const chartHeight = height - padding * 2;
    const max = Math.max(...data, 1);
    const min = Math.min(...data, 0);
    const range = max - min || 1;

    const points = data.map((value, i) => {
        const x = padding + (i / (data.length - 1)) * chartWidth;
        const y = padding + chartHeight - ((value - min) / range) * chartHeight;
        return `${x},${y}`;
    });

    const polylinePoints = points.join(' ');
    const gradientId = `sparkline-grad-${Math.random().toString(36).slice(2, 8)}`;

    // Build closed path for fill area
    const firstX = padding;
    const lastX = padding + chartWidth;
    const fillPath = `M ${points[0]} ${points.slice(1).map(p => `L ${p}`).join(' ')} L ${lastX},${height} L ${firstX},${height} Z`;

    return (
        <svg width={width} height={height} className={`sparkline ${className}`}>
            {filled && (
                <>
                    <defs>
                        <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
                            <stop offset="0%" stopColor={color} stopOpacity={0.25} />
                            <stop offset="100%" stopColor={color} stopOpacity={0.02} />
                        </linearGradient>
                    </defs>
                    <path d={fillPath} fill={`url(#${gradientId})`} />
                </>
            )}
            <polyline
                points={polylinePoints}
                fill="none"
                stroke={color}
                strokeWidth={strokeWidth}
                strokeLinecap="round"
                strokeLinejoin="round"
            />
        </svg>
    );
};

export default Sparkline;
