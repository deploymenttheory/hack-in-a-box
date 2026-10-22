import { defineConfig, devices } from '@playwright/test'
export default defineConfig({
  testDir: './e2e',
  testIgnore: 'public-portal.spec.ts',
  use: { baseURL: 'http://127.0.0.1:4173', trace: 'retain-on-failure' },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'] } },
    { name: 'mobile', use: { ...devices['iPhone 13'], defaultBrowserType: 'chromium' } },
  ],
  webServer: { command: 'npm run preview:admin -- --port 4173', port: 4173, reuseExistingServer: !process.env.CI },
})
