import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'node',
    globals: true,
    include: ['.pi/workspace/tests/**/*.test.ts', '.pi/workspace/agent-registry/tests/**/*.test.ts'],
  },
});
