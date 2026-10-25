import { defineConfig } from '@playwright/test'
import { resolve } from 'node:path'
export default defineConfig({
  testDir: './accessibility', testMatch: 'guide.spec.ts',
  outputDir: 'test-results-accessibility-guide', workers: 1, retries: 0,
  timeout: 60_000, expect: { timeout: 10_000 },
  reporter: [['./accessibility/provenance.ts'], ['list'], ['json', { outputFile: 'test-results-accessibility-guide/results.json' }]],
  use: { baseURL: 'http://127.0.0.1:4196', viewport: { width: 1280, height: 900 }, trace: 'retain-on-failure', screenshot: 'only-on-failure' },
  projects: [{ name: 'chromium', use: { browserName: 'chromium' } }],
  webServer: {
    command: 'go run ../cmd/styleguide', url: 'http://127.0.0.1:4196/readyz', reuseExistingServer: false,
    env: { HACKBOX_HTTP_ADDR: '127.0.0.1:4196', HACKBOX_WEB_DIR: resolve('dist-styleguide') },
  },
})
