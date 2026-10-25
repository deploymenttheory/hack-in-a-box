import { defineConfig } from '@playwright/test'

// Go owns PostgreSQL, native HTTP servers, sessions and private control IPC.
// There is deliberately no Vite/mock API webServer in this configuration.
export default defineConfig({
  testDir: './live-rehearsal',
  outputDir: 'test-results-live',
  workers: 1,
  fullyParallel: false,
  retries: 0,
  timeout: 60_000,
  expect: { timeout: 15_000 },
  use: { trace: 'retain-on-failure', screenshot: 'only-on-failure' },
  projects: [
    { name: 'chromium', use: { browserName: 'chromium' } },
    { name: 'firefox-display', testMatch: 'display.spec.ts', use: { browserName: 'firefox' } },
  ],
})
