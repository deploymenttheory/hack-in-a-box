import { defineConfig } from '@playwright/test'
import base from './playwright.config'
const participant = process.env.RESOURCE_PORTAL === 'participant'
const port = participant ? 4329 : 4328
// This is the complete owned resource verification inventory. Run without file
// arguments so nested embargo/authoring and shared consumers cannot be omitted.
const shared = ['resources.spec.ts', 'embargo.spec.ts', 'event-guidance.spec.ts', 'objective-disclosure.spec.ts', 'design-system.spec.ts', 'fleet-primitives.spec.ts', 'fleet-primitives-evidence.spec.ts']
const admin = ['content.spec.ts', 'objective-authoring.spec.ts', 'preparation.spec.ts', 'deliverables.spec.ts']
const attendee = ['team-access.spec.ts', 'participant-deliverables.spec.ts', 'participant-guidance.spec.ts', 'resource-private-evidence.spec.ts']
export default defineConfig({ ...base, testMatch: [...shared, ...(participant ? attendee : admin)], outputDir: `test-results-resources-${participant ? 'participant' : 'admin'}`, testIgnore: participant ? [] : base.testIgnore, grepInvert: participant ? /@admin/ : undefined, workers: 2, use: { ...base.use, baseURL: `http://127.0.0.1:${port}` }, webServer: { command: `${participant ? 'npm run preview' : 'npm run preview:admin'} -- --port ${port}`, port, reuseExistingServer: false } })
