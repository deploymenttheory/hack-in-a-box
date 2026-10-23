import { defineConfig } from '@playwright/test'
import base from './playwright.config'
const participant = process.env.FLEET_PORTAL === 'participant'
export default defineConfig({ ...base, testIgnore: [], workers: 2, grepInvert: participant ? /@admin/ : /@participant/,
  testMatch: ['portal-shell.spec.ts', 'portal-shell-evidence.spec.ts', ...(participant ? ['public-portal.spec.ts'] : ['events.spec.ts']), 'email-account.spec.ts', 'github.spec.ts', 'connection-recovery.spec.ts', 'design-system.spec.ts'],
  outputDir: participant ? 'test-results-shell-participant' : 'test-results-shell-admin',
  use: { ...base.use, baseURL: process.env.FLEET_EVIDENCE_URL ?? `http://127.0.0.1:${participant ? 4396 : 4395}` },
  webServer: process.env.FLEET_EVIDENCE_URL ? undefined : { command: `npm run ${participant ? 'preview' : 'preview:admin'} -- --port ${participant ? 4396 : 4395} --strictPort`, port: participant ? 4396 : 4395, reuseExistingServer: false },
})
