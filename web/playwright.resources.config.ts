import { defineConfig } from '@playwright/test'
import base from './playwright.config'
const participant = process.env.RESOURCE_PORTAL === 'participant'
const port = participant ? 4329 : 4328
export default defineConfig({ ...base, outputDir: `test-results-resources-${participant ? 'participant' : 'admin'}`, testIgnore: participant ? [] : base.testIgnore, grepInvert: participant ? /@admin/ : undefined, workers: 2, use: { ...base.use, baseURL: `http://127.0.0.1:${port}` }, webServer: { command: `${participant ? 'npm run preview' : 'npm run preview:admin'} -- --port ${port}`, port, reuseExistingServer: false } })
