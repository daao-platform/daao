import React from 'react';

export type LoadingSpinnerSize = 'sm' | 'md' | 'lg';

export interface LoadingSpinnerProps {
  size?: LoadingSpinnerSize;
  className?: string;
}

const sizeMap = {
  sm: 16,
  md: 24,
  lg: 40,
};

const LoadingSpinner: React.FC<LoadingSpinnerProps> = ({
  size = 'md',
  className = '',
}) => {
  const dimension = sizeMap[size];
  const strokeWidth = size === 'sm' ? 3 : size === 'lg' ? 3 : 2.5;

  return React.createElement('div', {
    className: `loading-spinner ${className}`.trim(),
    'data-size': size,
  },
    React.createElement('svg', {
      xmlns: 'http://www.w3.org/2000/svg',
      viewBox: `0 0 ${dimension} ${dimension}`,
      className: 'spinner-icon',
      style: {
        width: dimension,
        height: dimension,
      },
    },
      React.createElement('circle', {
        cx: dimension / 2,
        cy: dimension / 2,
        r: (dimension / 2) - strokeWidth,
        fill: 'none',
        stroke: 'var(--accent, #2dd4bf)',
        strokeWidth: strokeWidth,
        strokeDasharray: (dimension / 2) * Math.PI * 0.75,
        strokeLinecap: 'round',
      })
    )
  );
};

export default LoadingSpinner;
