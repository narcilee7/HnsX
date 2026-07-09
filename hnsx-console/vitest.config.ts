import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import path from 'node:path'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.{test,spec}.{ts,tsx}', '../observability/src/**/*.{test,spec}.{ts,tsx}'],
    exclude: ['node_modules', 'dist', '.vite', 'coverage'],
    css: false,
    coverage: {
      provider: 'v8',
      reporter: ['text', 'html', 'json-summary'],
      reportsDirectory: './coverage',
      include: [
        'src/**/*.{ts,tsx}',
        '../observability/src/**/*.{ts,tsx}',
      ],
      exclude: [
        'src/**/*.test.{ts,tsx}',
        'src/**/*.spec.{ts,tsx}',
        'src/test/**',
        '../observability/src/**/*.test.{ts,tsx}',
        '../observability/src/**/*.spec.{ts,tsx}',
        '../observability/src/test/**',
        'src/types/**',
        '../observability/src/types/**',
        'src/main.tsx',
      ],
      thresholds: {
        // 起步保守，让覆盖率基线随测试自然增长
        lines: 0,
        statements: 0,
        functions: 0,
        branches: 0,
      },
    },
  },
})