import { defineConfig, devices } from '@playwright/test'
export default defineConfig({
 testDir: './e2e', testMatch: 'fleet-patterns.spec.ts', outputDir: 'test-results-patterns',
 use: { baseURL: 'http://127.0.0.1:4393', trace: 'retain-on-failure' },
 projects: [
  { name: 'admin-desktop', use: { ...devices['Desktop Chrome'] } },
  { name: 'admin-mobile', use: { ...devices['iPhone 13'], defaultBrowserType: 'chromium' } },
  { name: 'participant-desktop', use: { ...devices['Desktop Chrome'] } },
  { name: 'participant-mobile', use: { ...devices['iPhone 13'], defaultBrowserType: 'chromium' } },
 ],
 webServer: { command: 'npm run preview:patterns', port: 4393, reuseExistingServer: false },
})
