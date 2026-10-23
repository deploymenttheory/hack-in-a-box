import { defineConfig } from '@playwright/test'
import base from './playwright.primitives.config'
export default defineConfig({ ...base,
 testIgnore: [],
 testMatch: ['design-system.spec.ts', 'fleet-primitives.spec.ts', 'teams-workflow.spec.ts', 'registration.spec.ts', 'email-account.spec.ts', 'connection-recovery.spec.ts'],
 outputDir: 'test-results-primitives-public',
 use: { ...base.use, baseURL: 'http://127.0.0.1:4392' },
 webServer: { command: 'npm run preview -- --port 4392 --strictPort', port: 4392, reuseExistingServer: false },
})
