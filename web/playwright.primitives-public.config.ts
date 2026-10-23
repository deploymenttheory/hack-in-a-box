import { defineConfig } from '@playwright/test'
import base from './playwright.primitives.config'
export default defineConfig({ ...base,
 testIgnore: [], grepInvert: /@admin/,
 testMatch: ['design-system.spec.ts', 'fleet-primitives.spec.ts', 'fleet-primitives-evidence.spec.ts', 'registration.spec.ts', 'room-display.spec.ts', 'email-account.spec.ts', 'connection-recovery.spec.ts'],
 outputDir: 'test-results-primitives-public',
 use: { ...base.use, baseURL: process.env.FLEET_EVIDENCE_URL ?? 'http://127.0.0.1:4392' },
 webServer: process.env.FLEET_EVIDENCE_URL ? undefined : { command: 'npm run preview -- --port 4392 --strictPort', port: 4392, reuseExistingServer: false },
})
