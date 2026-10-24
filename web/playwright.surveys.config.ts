import { defineConfig } from '@playwright/test'
import base from './playwright.config'
const participant = process.env.SURVEY_PORTAL === 'participant'
const port = participant ? 4333 : 4332
// Run without filenames for the full survey and shared-consumer inventory.
export default defineConfig({ ...base, testMatch: ['survey-directory.spec.ts', 'survey-privacy.spec.ts', 'survey-reader.spec.ts', 'survey-workspace.spec.ts', 'design-system.spec.ts', 'fleet-primitives.spec.ts', 'fleet-primitives-evidence.spec.ts'], testIgnore: [], grepInvert: participant ? /@admin/ : undefined, outputDir: `test-results-surveys-${participant ? 'participant' : 'admin'}`, workers: 2, use: { ...base.use, baseURL: `http://127.0.0.1:${port}` }, webServer: { command: `${participant ? 'npm run preview' : 'npm run preview:admin'} -- --port ${port}`, port, reuseExistingServer: false } })
