import { defineConfig } from '@playwright/test'
import base from './playwright.config'
export default defineConfig({ ...base, outputDir: 'test-results-public', testIgnore: [], testMatch: ['public-portal.spec.ts', 'participant-teams.spec.ts'],
  use: { ...base.use, baseURL: 'http://127.0.0.1:4274' },
  webServer: { command: 'npm run preview -- --port 4274', port: 4274, reuseExistingServer: false },
})
