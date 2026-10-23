import { defineConfig } from '@playwright/test'
import base from './playwright.config'
export default defineConfig({ ...base,
 testMatch: ['design-system.spec.ts', 'fleet-primitives.spec.ts', 'fleet-primitives-evidence.spec.ts', 'teams-workflow.spec.ts', 'events.spec.ts', 'email-account.spec.ts', 'connection-recovery.spec.ts', 'registration-forms.spec.ts'],
 outputDir: 'test-results-primitives-admin',
 use: { ...base.use, baseURL: process.env.FLEET_EVIDENCE_URL ?? 'http://127.0.0.1:4391' },
 webServer: process.env.FLEET_EVIDENCE_URL ? undefined : { command: 'npm run preview:admin -- --port 4391 --strictPort', port: 4391, reuseExistingServer: false },
})
