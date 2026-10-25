import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  testMatch: ['team-deliverable-lifetimes.spec.ts', 'deliverables.spec.ts', 'participant-deliverables.spec.ts'],
  outputDir: 'test-results-cutover', workers: 1, retries: 0,
  use: { trace: 'retain-on-failure' },
  projects: ['administrator', 'regular'].flatMap(portal => ['desktop', 'mobile'].map(viewport => ({
    name: `${portal}-${viewport}`, metadata: { portal },
    use: { ...(viewport === 'mobile' ? devices['iPhone 13'] : devices['Desktop Chrome']), defaultBrowserType: 'chromium' as const, baseURL: portal === 'administrator' ? 'http://127.0.0.1:4185' : 'http://127.0.0.1:4285' },
  }))),
  webServer: [
    { command: 'npm run preview:admin -- --port 4185 --strictPort', port: 4185, reuseExistingServer: false },
    { command: 'npm run preview -- --port 4285 --strictPort', port: 4285, reuseExistingServer: false },
  ],
})
