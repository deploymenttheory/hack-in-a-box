import { defineConfig, devices } from '@playwright/test'
import { resolve } from 'node:path'
const externalOrigin = process.env.STYLEGUIDE_TEST_ORIGIN
export default defineConfig({
  testDir: './styleguide-tests', outputDir: 'styleguide-test-results',
  use: { baseURL: externalOrigin ?? 'http://127.0.0.1:4186', trace: 'retain-on-failure' },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'] } },
    { name: 'mobile', use: { ...devices['iPhone 13'], defaultBrowserType: 'chromium' } },
  ],
  webServer: externalOrigin ? undefined : {
    command: 'go run ../cmd/styleguide', url: 'http://127.0.0.1:4186/readyz', reuseExistingServer: false,
    env: { HACKBOX_HTTP_ADDR: '127.0.0.1:4186', HACKBOX_WEB_DIR: resolve('dist-styleguide') },
  },
})
