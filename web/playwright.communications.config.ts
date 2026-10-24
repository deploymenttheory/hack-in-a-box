import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  testMatch: ['inbox.spec.ts', 'campaigns.spec.ts', 'teams-workflow.spec.ts', 'delivery-history.spec.ts', 'automation-recovery.spec.ts', 'communications-private.spec.ts', 'communications-surface.spec.ts'],
  outputDir: 'test-results-communications',
  use: { trace: 'retain-on-failure' },
  projects: [
    { name: 'admin-desktop', use: { ...devices['Desktop Chrome'], baseURL: 'http://127.0.0.1:4490' } },
    { name: 'admin-mobile', use: { ...devices['iPhone 13'], defaultBrowserType: 'chromium', baseURL: 'http://127.0.0.1:4490' } },
    { name: 'regular-desktop', testIgnore: ['campaigns.spec.ts', 'teams-workflow.spec.ts'], use: { ...devices['Desktop Chrome'], baseURL: 'http://127.0.0.1:4491' } },
    { name: 'regular-mobile', testIgnore: ['campaigns.spec.ts', 'teams-workflow.spec.ts'], use: { ...devices['iPhone 13'], defaultBrowserType: 'chromium', baseURL: 'http://127.0.0.1:4491' } },
  ],
  webServer: [
    { command: 'npm run preview:admin -- --port 4490 --strictPort', port: 4490, reuseExistingServer: false },
    { command: 'npm run preview -- --port 4491 --strictPort', port: 4491, reuseExistingServer: false },
  ],
})
